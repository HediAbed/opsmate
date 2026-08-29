package analysis

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestReadStreamEvent_Chunk(t *testing.T) {
	events := make(chan StreamEvent, 1)
	events <- newStreamChunk("hi")
	chunk := readStreamEvent(events)
	if chunk.Chunk != "hi" || chunk.Err != nil || chunk.Done {
		t.Errorf("unexpected chunk message: %+v", chunk)
	}
}

func TestReadStreamEvent_Err(t *testing.T) {
	events := make(chan StreamEvent, 1)
	sentinel := errors.New("boom")
	events <- newStreamFailure(sentinel)
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
	failureEvent := newStreamFailure(sentinel)
	if failed, err := failureEvent.Failure(); !failed || !errors.Is(err, sentinel) {
		t.Fatalf("Failure() = (%t, %v), want failure event", failed, err)
	}
	if chunk, ok := failureEvent.ChunkValue(); ok || chunk != "" {
		t.Fatalf("ChunkValue() = (%q, %t), want no chunk", chunk, ok)
	}

	chunkEvent := newStreamChunk("answer")
	if failed, err := chunkEvent.Failure(); failed || err != nil {
		t.Fatalf("Failure() = (%t, %v), want no failure", failed, err)
	}
}

func TestHTTPProviderRejectsMissingContext(t *testing.T) {
	provider := &HTTPProvider{}
	var missingContext context.Context
	if _, err := provider.Chat(missingContext, "system", "user"); !errors.Is(err, ErrProviderContextRequired) {
		t.Fatalf("Chat(nil) error = %v, want context-required error", err)
	}
	if err := provider.ChatStream(missingContext, "system", "user", make(chan StreamEvent, 1)); !errors.Is(err, ErrProviderContextRequired) {
		t.Fatalf("ChatStream(nil) error = %v, want context-required error", err)
	}
}

func TestAnalysisAnalyzeStream_ProviderError_SurfacesThroughEvents(t *testing.T) {
	prev := getActiveProvider()
	t.Cleanup(func() { setActiveProvider(prev) })

	sentinel := errors.New("provider crashed")
	setActiveProvider(&fakeStreamingProvider{err: sentinel})

	startCmd, events, _ := AnalyzeStream("sys", "user")
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
	events <- newStreamChunk("queued")
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
	previous := getActiveProvider()
	t.Cleanup(func() { setActiveProvider(previous) })
	setActiveProvider(&fakeStreamingProvider{chunks: []string{"answer"}})

	start, events, _ := AnalyzeStream("system", "user")
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
	events <- newStreamChunk("queued")
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
	prev := getActiveProvider()
	t.Cleanup(func() { setActiveProvider(prev) })
	setActiveProvider(nil)

	startCmd, events, _ := AnalyzeStream("sys", "user")
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
		events <- newStreamChunk(c)
	}
	if f.returning != nil {
		close(f.returning)
	}
	return f.err
}
