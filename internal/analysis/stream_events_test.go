package analysis

import (
	"errors"
	"testing"
)

func TestWaitForStreamChunk_NilChannelReturnsNilCmd(t *testing.T) {
	if WaitForStreamChunk(nil) != nil {
		t.Error("nil events should return a nil cmd")
	}
}

func TestWaitForStreamChunk_MapsChunkErrorAndClosure(t *testing.T) {
	events := make(chan StreamEvent, 3)
	events <- newStreamChunk("hello")
	events <- newStreamFailure(errors.New("boom"))
	close(events)

	chunk := WaitForStreamChunk(events)().(StreamChunkMsg)
	if chunk.Chunk != "hello" {
		t.Errorf("chunk = %q, want hello", chunk.Chunk)
	}

	errorEvent := WaitForStreamChunk(events)().(StreamChunkMsg)
	if errorEvent.Err == nil {
		t.Error("error event must preserve its error")
	}

	closed := WaitForStreamChunk(events)().(StreamChunkMsg)
	if !closed.Done {
		t.Error("closed channel must produce a completed message")
	}
}

func TestSupportsStreaming_FalseWithoutProvider(t *testing.T) {
	withCleanProvider(t)
	if SupportsStreaming() {
		t.Error("streaming must be unavailable without a provider")
	}
}
