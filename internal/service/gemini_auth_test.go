package service

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestGeminiProvider_Chat_UsesHeaderNotQueryParam(t *testing.T) {
	var capturedHeader, capturedRawQuery string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedHeader = r.Header.Get(geminiAPIKeyHeader)
		capturedRawQuery = r.URL.RawQuery

		_, _ = io.WriteString(w, `{"candidates":[{"content":{"parts":[{"text":"ok"}]}}]}`)
	}))
	defer srv.Close()

	g := &GeminiProvider{
		apiKey:  "SECRET-KEY-42",
		model:   "test-model",
		baseURL: srv.URL,
		client:  &http.Client{Timeout: 5 * time.Second},
	}

	out, err := g.Chat(context.Background(), "sys", "user")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != "ok" {
		t.Errorf("got %q; want ok", out)
	}
	if capturedHeader != "SECRET-KEY-42" {
		t.Errorf("header %s = %q; want SECRET-KEY-42", geminiAPIKeyHeader, capturedHeader)
	}
	if strings.Contains(capturedRawQuery, "key=") {
		t.Errorf("API key leaked into URL query: %q", capturedRawQuery)
	}
	if strings.Contains(capturedRawQuery, "SECRET-KEY-42") {
		t.Errorf("raw query should not contain the key: %q", capturedRawQuery)
	}
}

func TestGeminiProviderChatRejectsInvalidResponses(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{name: "malformed", body: "not json"},
		{name: "no candidates", body: `{"candidates":[]}`},
		{name: "no parts", body: `{"candidates":[{"content":{"parts":[]}}]}`},
		{name: "blank text", body: `{"candidates":[{"content":{"parts":[{"text":"  "}]}}]}`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
				_, _ = io.WriteString(response, test.body)
			}))
			defer server.Close()
			provider := &GeminiProvider{model: "model", baseURL: server.URL, client: server.Client()}

			if _, err := provider.Chat(context.Background(), "system", "user"); err == nil {
				t.Fatal("invalid response did not fail")
			}
		})
	}
}

func TestGeminiProviderPropagatesRequestFailures(t *testing.T) {
	provider := &GeminiProvider{model: "model", baseURL: "://", client: http.DefaultClient}
	if _, err := provider.Chat(context.Background(), "system", "user"); err == nil {
		t.Fatal("invalid chat endpoint did not fail")
	}
	if err := provider.ChatStream(context.Background(), "system", "user", make(chan StreamEvent, 1)); err == nil {
		t.Fatal("invalid stream endpoint did not fail")
	}
}

func TestGeminiProviderChatStreamReportsCloseFailure(t *testing.T) {
	closeFailure := errors.New("close failed")
	client := &http.Client{Transport: providerRoundTripFunc(func(*http.Request) (*http.Response, error) {
		body := &providerStreamBody{
			Reader:   strings.NewReader("data: [DONE]\n\n"),
			closeErr: closeFailure,
		}
		return &http.Response{StatusCode: http.StatusOK, Body: body}, nil
	})}
	provider := &GeminiProvider{model: "model", baseURL: "http://provider.invalid", client: client}

	err := provider.ChatStream(context.Background(), "system", "user", make(chan StreamEvent, 1))

	if !errors.Is(err, closeFailure) {
		t.Fatalf("error = %v, want close failure", err)
	}
}

type providerStreamBody struct {
	*strings.Reader
	closeErr error
}

func (body *providerStreamBody) Close() error { return body.closeErr }

func TestGeminiProvider_ChatStream_UsesHeaderNotQueryParam(t *testing.T) {
	var capturedHeader, capturedRawQuery string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedHeader = r.Header.Get(geminiAPIKeyHeader)
		capturedRawQuery = r.URL.RawQuery

		flusher, _ := w.(http.Flusher)
		payload := `{"candidates":[{"content":{"parts":[{"text":"hi"}]}}]}`
		_, _ = io.WriteString(w, "data: "+payload+"\n\n")
		if flusher != nil {
			flusher.Flush()
		}
	}))
	defer srv.Close()

	g := &GeminiProvider{
		apiKey:  "SECRET-STREAM-KEY",
		model:   "test-model",
		baseURL: srv.URL,
		client:  &http.Client{Timeout: 5 * time.Second},
	}

	events := make(chan StreamEvent, 8)
	if err := g.ChatStream(context.Background(), "sys", "user", events); err != nil {
		t.Fatalf("ChatStream err: %v", err)
	}

	if capturedHeader != "SECRET-STREAM-KEY" {
		t.Errorf("header %s = %q; want SECRET-STREAM-KEY", geminiAPIKeyHeader, capturedHeader)
	}
	if strings.Contains(capturedRawQuery, "key=") {
		t.Errorf("API key leaked into stream URL query: %q", capturedRawQuery)
	}

	if !strings.Contains(capturedRawQuery, "alt=sse") {
		t.Errorf("expected alt=sse in query for SSE, got %q", capturedRawQuery)
	}
}
