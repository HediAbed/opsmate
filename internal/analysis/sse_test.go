package analysis

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestSSEDataValue_ParsesDataFields(t *testing.T) {
	tests := []struct {
		line string
		want string
		ok   bool
	}{
		{line: "data: value", want: "value", ok: true},
		{line: "data:value", want: "value", ok: true},
		{line: "data: value:with:colons", want: "value:with:colons", ok: true},
		{line: ": heartbeat", ok: false},
		{line: "event: message", ok: false},
		{line: "invalid", ok: false},
	}
	for _, test := range tests {
		t.Run(test.line, func(t *testing.T) {
			got, ok := sseDataValue(test.line)
			if got != test.want || ok != test.ok {
				t.Fatalf("sseDataValue(%q) = (%q, %t), want (%q, %t)", test.line, got, ok, test.want, test.ok)
			}
		})
	}
}

func TestSSEStreamErrorFormatsAndUnwraps(t *testing.T) {
	sentinel := errors.New("failed")
	if got := (&SSEStreamError{Stage: "read"}).Error(); got != "SSE stream (read): unknown error" {
		t.Fatalf("unknown error = %q", got)
	}
	err := &SSEStreamError{Stage: "decode", Err: sentinel}
	if got := err.Error(); got != "SSE stream (decode): failed" {
		t.Fatalf("error = %q", got)
	}
	if !errors.Is(err, sentinel) {
		t.Fatal("stream error did not unwrap its cause")
	}
}

func TestStreamSSE_ForwardsEventsAndStopsAtDone(t *testing.T) {
	body := strings.NewReader("data: A\n\ndata: B\n\ndata: [DONE]\n\ndata: ignored\n\n")
	events := make(chan StreamEvent, 3)
	err := streamSSE(context.Background(), body, events, textSSEDecoder)
	if err != nil {
		t.Fatalf("stream: %v", err)
	}
	close(events)
	if got := collectStreamChunks(events); !reflect.DeepEqual(got, []string{"A", "B"}) {
		t.Fatalf("chunks = %v, want [A B]", got)
	}
}

func TestStreamSSE_JoinsMultilineData(t *testing.T) {
	body := strings.NewReader("data: first\ndata: second\n\n")
	events := make(chan StreamEvent, 1)
	if err := streamSSE(context.Background(), body, events, textSSEDecoder); err != nil {
		t.Fatalf("stream: %v", err)
	}
	close(events)
	if got := collectStreamChunks(events); !reflect.DeepEqual(got, []string{"first\nsecond"}) {
		t.Fatalf("chunks = %q, want joined data lines", got)
	}
}

func TestStreamSSE_DispatchesFinalEventAtEOF(t *testing.T) {
	events := make(chan StreamEvent, 1)
	if err := streamSSE(context.Background(), strings.NewReader("data: final"), events, textSSEDecoder); err != nil {
		t.Fatalf("stream: %v", err)
	}
	close(events)
	if got := collectStreamChunks(events); !reflect.DeepEqual(got, []string{"final"}) {
		t.Fatalf("chunks = %v, want final EOF event", got)
	}
}

func TestStreamSSE_StopsWhenContextIsCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := streamSSE(ctx, strings.NewReader("data: blocked\n\n"), make(chan StreamEvent), textSSEDecoder)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", err)
	}
}

func TestStreamSSE_PropagatesDecoderError(t *testing.T) {
	wantErr := errors.New("bad payload")
	decoder := func([]byte) (string, error) { return "", wantErr }
	err := streamSSE(context.Background(), strings.NewReader("data: invalid\n\n"), make(chan StreamEvent, 1), decoder)
	if !errors.Is(err, wantErr) {
		t.Fatalf("error = %v, want decoder error", err)
	}
	var streamErr *SSEStreamError
	if !errors.As(err, &streamErr) || streamErr.Stage != "decode" {
		t.Fatalf("error = %#v, want decode-stage SSEStreamError", err)
	}
}

func TestStreamSSE_RejectsOversizedLine(t *testing.T) {
	body := strings.NewReader("data: " + strings.Repeat("x", maxSSELineBytes) + "\n\n")
	err := streamSSE(context.Background(), body, make(chan StreamEvent, 1), textSSEDecoder)
	if err == nil {
		t.Fatal("oversized line must fail")
	}
	var streamErr *SSEStreamError
	if !errors.As(err, &streamErr) || streamErr.Stage != "read" {
		t.Fatalf("error = %#v, want read-stage SSEStreamError", err)
	}
}

func TestSSEAccumulatorRejectsOversizedMultilineEvent(t *testing.T) {
	accumulator := sseAccumulator{
		dataLines: make([]string, 0, 5),
		events:    make(chan StreamEvent, 1),
		decode:    textSSEDecoder,
	}
	line := "data: " + strings.Repeat("x", maxSSEEventBytes/4+1)
	for range 3 {
		done, err := accumulator.acceptLine(context.Background(), line)
		if done || err != nil {
			t.Fatalf("event rejected before reaching limit: (%t, %v)", done, err)
		}
	}
	_, err := accumulator.acceptLine(context.Background(), line)
	if !errors.Is(err, ErrSSEEventTooLarge) {
		t.Fatalf("error = %v, want ErrSSEEventTooLarge", err)
	}
}

func TestSSEAccumulatorIgnoresMetadataAndEmptyChunks(t *testing.T) {
	accumulator := sseAccumulator{
		dataLines: make([]string, 0, 1),
		events:    make(chan StreamEvent, 1),
		decode:    func([]byte) (string, error) { return "", nil },
	}
	if done, err := accumulator.acceptLine(context.Background(), "event: message\r"); done || err != nil {
		t.Fatalf("metadata result = (%t, %v)", done, err)
	}
	if done, err := accumulator.dispatch(context.Background()); done || err != nil {
		t.Fatalf("empty dispatch result = (%t, %v)", done, err)
	}
	if done, err := accumulator.acceptLine(context.Background(), "data: ignored"); done || err != nil {
		t.Fatalf("data result = (%t, %v)", done, err)
	}
	if done, err := accumulator.acceptLine(context.Background(), ""); done || err != nil {
		t.Fatalf("empty chunk result = (%t, %v)", done, err)
	}
	if len(accumulator.events) != 0 {
		t.Fatal("empty decoded chunk was emitted")
	}
}

func TestStreamSSE_RejectsExcessiveCumulativeOutput(t *testing.T) {
	chunk := strings.Repeat("x", maxSSEStreamOutputBytes/2+1)
	decoder := func([]byte) (string, error) {
		return chunk, nil
	}
	events := make(chan StreamEvent, 2)
	err := streamSSE(
		context.Background(),
		strings.NewReader("data: first\n\ndata: second\n\n"),
		events,
		decoder,
	)
	if !errors.Is(err, ErrSSEStreamTooLarge) {
		t.Fatalf("error = %v, want ErrSSEStreamTooLarge", err)
	}
	var streamErr *SSEStreamError
	if !errors.As(err, &streamErr) || streamErr.Stage != "limit" {
		t.Fatalf("error = %#v, want limit-stage SSEStreamError", err)
	}
	if len(events) != 1 {
		t.Fatalf("forwarded events = %d, want only the chunk within the limit", len(events))
	}
}

func TestStreamSSE_StripsTerminalControlSequences(t *testing.T) {
	events := make(chan StreamEvent, 1)
	body := strings.NewReader("data: \x1b[31mdanger\x1b[0m\n\n")
	if err := streamSSE(context.Background(), body, events, textSSEDecoder); err != nil {
		t.Fatalf("stream: %v", err)
	}
	close(events)
	if got := collectStreamChunks(events); !reflect.DeepEqual(got, []string{"danger"}) {
		t.Fatalf("chunks = %q, want sanitized text", got)
	}
}

func TestDecodeChatCompletionChunk(t *testing.T) {
	valid := []byte(`{"choices":[{"delta":{"content":"hello"}}]}`)
	text, err := decodeChatCompletionChunk("test", valid)
	if err != nil || text != "hello" {
		t.Fatalf("valid payload = (%q, %v), want hello", text, err)
	}
	text, err = decodeChatCompletionChunk("test", []byte(`{"choices":[]}`))
	if err != nil || text != "" {
		t.Fatalf("empty payload = (%q, %v), want empty without error", text, err)
	}
	_, err = decodeChatCompletionChunk("test", []byte(`not json`))
	if err == nil {
		t.Fatal("malformed payload must fail")
	}
	var providerErr *ProviderError
	if !errors.As(err, &providerErr) || providerErr.Provider != "test" {
		t.Fatalf("error = %#v, want test ProviderError", err)
	}
}

func textSSEDecoder(payload []byte) (string, error) {
	return string(payload), nil
}

func collectStreamChunks(events <-chan StreamEvent) []string {
	chunks := make([]string, 0)
	for event := range events {
		chunk, ok := event.ChunkValue()
		if ok {
			chunks = append(chunks, chunk)
		}
	}
	return chunks
}
