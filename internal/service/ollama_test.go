package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestOllamaProvider_Chat_HappyPath(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "" {
			t.Errorf("Ollama request should not send auth header; got %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"hello from ollama"}}]}`))
	}))
	defer srv.Close()

	p := &OllamaProvider{model: "gemma4:e4b", apiURL: srv.URL, client: srv.Client()}
	got, err := p.Chat(context.Background(), "sys", "user")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if got != "hello from ollama" {
		t.Errorf("got %q, want hello from ollama", got)
	}
}

func TestOllamaProvider_Chat_ErrorStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("ollama died"))
	}))
	defer srv.Close()

	p := &OllamaProvider{model: "gemma4:e4b", apiURL: srv.URL, client: srv.Client()}
	_, err := p.Chat(context.Background(), "sys", "user")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "500") {
		t.Errorf("error should preserve status code; got %v", err)
	}
}

func TestOllamaProvider_ChatStream_SSEDeliversChunks(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		flusher, _ := w.(http.Flusher)
		_, _ = w.Write([]byte(`data: {"choices":[{"delta":{"content":"hello "}}]}` + "\n\n"))
		if flusher != nil {
			flusher.Flush()
		}
		_, _ = w.Write([]byte(`data: {"choices":[{"delta":{"content":"world"}}]}` + "\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer srv.Close()

	p := &OllamaProvider{model: "gemma4:e4b", apiURL: srv.URL, client: srv.Client()}
	events := make(chan StreamEvent, 10)
	if err := p.ChatStream(context.Background(), "sys", "user", events); err != nil {
		t.Fatalf("stream: %v", err)
	}
	close(events)

	var combined strings.Builder
	for ev := range events {
		if failed, failure := ev.Failure(); failed {
			t.Fatalf("stream failure: %v", failure)
		}
		if chunk, ok := ev.ChunkValue(); ok {
			combined.WriteString(chunk)
		}
	}
	if combined.String() != "hello world" {
		t.Errorf("stream should produce hello world; got %q", combined.String())
	}
}
