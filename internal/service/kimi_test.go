package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestKimiProvider_Chat_HappyPath(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"hello from kimi"}}]}`))
	}))
	defer srv.Close()
	p := &KimiProvider{apiKey: "k", model: "m", apiURL: srv.URL, client: srv.Client()}
	got, err := p.Chat(context.Background(), "sys", "user")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if got != "hello from kimi" {
		t.Errorf("got %q, want hello from kimi", got)
	}
}

func TestKimiProvider_Chat_ErrorStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte("denied"))
	}))
	defer srv.Close()
	p := &KimiProvider{apiKey: "k", model: "m", apiURL: srv.URL, client: srv.Client()}
	_, err := p.Chat(context.Background(), "sys", "user")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "403") {
		t.Errorf("error should preserve status code; got %v", err)
	}
}

func TestKimiProvider_Chat_EmptyChoicesErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"choices":[]}`))
	}))
	defer srv.Close()
	p := &KimiProvider{apiKey: "k", model: "m", apiURL: srv.URL, client: srv.Client()}
	_, err := p.Chat(context.Background(), "sys", "user")
	if err == nil {
		t.Fatal("empty choices should error")
	}
}

func TestKimiProvider_ChatStream_SSEDeliversChunks(t *testing.T) {
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

	p := &KimiProvider{apiKey: "k", model: "m", apiURL: srv.URL, client: srv.Client()}
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
	if !strings.Contains(combined.String(), "hello") {
		t.Errorf("stream should produce hello+world; got %q", combined.String())
	}
}

func TestKimiProvider_ChatStream_ErrorStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("server died"))
	}))
	defer srv.Close()

	p := &KimiProvider{apiKey: "k", model: "m", apiURL: srv.URL, client: srv.Client()}
	events := make(chan StreamEvent, 4)
	err := p.ChatStream(context.Background(), "sys", "user", events)
	if err == nil {
		t.Error("expected error from 500 response")
	}
}
