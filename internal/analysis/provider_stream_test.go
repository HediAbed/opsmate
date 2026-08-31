package analysis

import (
	"context"
	"errors"
	"testing"
	"time"

	analysisprovider "github.com/HediAbed/opsmate/internal/analysis/provider"
)

func TestReadStreamEvent_Chunk(t *testing.T) {
	events := make(chan StreamEvent, 1)
	events <- analysisprovider.NewChunk("hi")
	chunk := readStreamEvent(events)
	if chunk.Chunk != "hi" || chunk.Err != nil || chunk.Done {
		t.Errorf("unexpected chunk message: %+v", chunk)
	}
}

func TestReadStreamEvent_Err(t *testing.T) {
	events := make(chan StreamEvent, 1)
	sentinel := errors.New("boom")
	events <- analysisprovider.NewFailure(sentinel)
	chunk := readStreamEvent(events)
	if !errors.Is(chunk.Err, sentinel) {
		t.Errorf("expected sentinel error, got %v", chunk.Err)
	}
}

func TestReadStreamEvent_InvalidEventReturnsError(t *testing.T) {
	events := make(chan StreamEvent, 1)
	events <- StreamEvent{}
	message := readStreamEvent(events)
	if message.Err == nil {
		t.Fatal("invalid event must return an error")
	}
}

func TestReadStreamEvent_Closed(t *testing.T) {
	events := make(chan StreamEvent)
	close(events)
	chunk := readStreamEvent(events)
	if !chunk.Done {
		t.Errorf("closed channel should produce Done=true, got %+v", chunk)
	}
}

func TestStreamEventAccessorsDistinguishEventKinds(t *testing.T) {
	sentinel := errors.New("failed")
	failureEvent := analysisprovider.NewFailure(sentinel)
	if failed, err := failureEvent.Failure(); !failed || !errors.Is(err, sentinel) {
		t.Fatalf("Failure() = (%t, %v), want failure event", failed, err)
	}
	if chunk, ok := failureEvent.ChunkValue(); ok || chunk != "" {
		t.Fatalf("ChunkValue() = (%q, %t), want no chunk", chunk, ok)
	}

	chunkEvent := analysisprovider.NewChunk("answer")
	if failed, err := chunkEvent.Failure(); failed || err != nil {
		t.Fatalf("Failure() = (%t, %v), want no failure", failed, err)
	}
}

func TestHTTPProviderRejectsMissingContext(t *testing.T) {
	client := &analysisprovider.HTTPClient{}
	var missingContext context.Context
	if _, err := client.Chat(missingContext, "system", "user"); !errors.Is(err, analysisprovider.ErrProviderContextRequired) {
		t.Fatalf("Chat(nil) error = %v, want context-required error", err)
	}
	if err := client.ChatStream(missingContext, "system", "user", make(chan StreamEvent, 1)); !errors.Is(err, analysisprovider.ErrProviderContextRequired) {
		t.Fatalf("ChatStream(nil) error = %v, want context-required error", err)
	}
}

func TestAnalysisAnalyzeStream_ProviderError_SurfacesThroughEvents(t *testing.T) {
	sentinel := errors.New("provider crashed")
	service := NewService(&fakeStreamingProvider{err: sentinel})

	startCmd, events, _ := service.AnalyzeStream("sys", "user")
	if startCmd == nil || events == nil {
		t.Fatal("expected streaming return values, got nil")
	}

	msg := startCmd()
	chunk, ok := msg.(StreamChunkMsg)
	if !ok {
		t.Fatalf("expected StreamChunkMsg, got %T", msg)
	}
	if !errors.Is(chunk.Err, sentinel) {
		t.Errorf("expected sentinel error, got %v", chunk.Err)
	}
}

func TestRunProviderStream_PreservesErrorWhenBufferIsFull(t *testing.T) {
	sentinel := errors.New("provider failed")
	returning := make(chan struct{})
	provider := &fakeStreamingProvider{err: sentinel, returning: returning}
	events := make(chan StreamEvent, 1)
	events <- analysisprovider.NewChunk("queued")
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		runProviderStream(ctx, cancel, provider, "system", "user", events)
		close(done)
	}()
	<-returning

	queued := readStreamEvent(events)
	if queued.Chunk != "queued" {
		t.Fatalf("queued chunk = %q, want queued", queued.Chunk)
	}
	failure := readStreamEvent(events)
	if !errors.Is(failure.Err, sentinel) {
		t.Fatalf("failure = %v, want provider error", failure.Err)
	}
	if completed := readStreamEvent(events); !completed.Done {
		t.Fatal("stream must close after delivering the failure")
	}
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("stream orchestration did not exit")
	}
}

func TestAnalysisAnalyzeStream_ClosesNormallyAfterChunks(t *testing.T) {
	service := NewService(&fakeStreamingProvider{chunks: []string{"answer"}})

	start, events, _ := service.AnalyzeStream("system", "user")
	first := start().(StreamChunkMsg)
	if first.Chunk != "answer" {
		t.Fatalf("chunk = %q, want answer", first.Chunk)
	}
	completed := WaitForStreamChunk(events)().(StreamChunkMsg)
	if !completed.Done {
		t.Fatal("successful stream must close")
	}
}

func TestRunProviderStream_CancellationReleasesBlockedFailure(t *testing.T) {
	returning := make(chan struct{})
	provider := &fakeStreamingProvider{err: errors.New("provider failed"), returning: returning}
	events := make(chan StreamEvent, 1)
	events <- analysisprovider.NewChunk("queued")
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		runProviderStream(ctx, func() {}, provider, "system", "user", events)
		close(done)
	}()
	<-returning
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("canceled stream leaked a blocked goroutine")
	}
	if queued := readStreamEvent(events); queued.Chunk != "queued" {
		t.Fatalf("queued chunk = %q, want queued", queued.Chunk)
	}
	if completed := readStreamEvent(events); !completed.Done {
		t.Fatal("canceled stream must close")
	}
}

func TestAnalysisAnalyzeStream_NoProviderReturnsError(t *testing.T) {
	service := NewService(nil)

	startCmd, events, _ := service.AnalyzeStream("sys", "user")
	if events != nil {
		t.Errorf("no-provider case should return nil channel, got %v", events)
	}
	msg := startCmd()
	analysis, ok := msg.(AnalysisMsg)
	if !ok {
		t.Fatalf("expected AnalysisMsg, got %T", msg)
	}
	if analysis.Err == nil {
		t.Error("expected error for no provider")
	}
}

type fakeStreamingProvider struct {
	chunks    []string
	err       error
	returning chan struct{}
}

func (*fakeStreamingProvider) Name() string { return "fake" }

func (f *fakeStreamingProvider) Chat(_ context.Context, _, _ string) (string, error) {
	return "", f.err
}

func (f *fakeStreamingProvider) ChatStream(_ context.Context, _, _ string, events chan<- StreamEvent) error {
	for _, c := range f.chunks {
		events <- analysisprovider.NewChunk(c)
	}
	if f.returning != nil {
		close(f.returning)
	}
	return f.err
}
