package service

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

type providerRoundTripFunc func(*http.Request) (*http.Response, error)

func (roundTrip providerRoundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return roundTrip(request)
}

func TestExecuteProviderRequest_RejectsRedirectWithoutForwardingCredentials(t *testing.T) {
	var redirectedRequests atomic.Int32
	destination := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		redirectedRequests.Add(1)
	}))
	defer destination.Close()

	source := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		http.Redirect(response, request, destination.URL, http.StatusTemporaryRedirect)
	}))
	defer source.Close()

	headers := make(http.Header)
	headers.Set("Authorization", "Bearer secret")
	_, err := executeProviderRequest(
		context.Background(), source.Client(), "provider", "chat", source.URL, headers, []byte(`{}`),
	)
	if !errors.Is(err, ErrProviderRedirect) || !errors.Is(err, ErrProviderHTTPStatus) {
		t.Fatalf("error = %v, want redirect and HTTP-status classifications", err)
	}
	if got := redirectedRequests.Load(); got != 0 {
		t.Fatalf("redirect destination received %d requests, want zero", got)
	}
}

func TestExecuteProviderRequest_ClassifiesHTTPStatusAndSanitizesDetail(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		response.WriteHeader(http.StatusTooManyRequests)
		_, _ = io.WriteString(response, "\x1b[31mrate limited\x1b[0m")
	}))
	defer server.Close()

	_, err := executeProviderRequest(
		context.Background(), server.Client(), "provider", "chat", server.URL, nil, []byte(`{}`),
	)
	if !errors.Is(err, ErrProviderHTTPStatus) {
		t.Fatalf("error = %v, want ErrProviderHTTPStatus", err)
	}
	var providerErr *ProviderError
	if !errors.As(err, &providerErr) || providerErr.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("error = %#v, want HTTP 429 ProviderError", err)
	}
	if providerErr.Detail != "rate limited" {
		t.Fatalf("detail = %q, want sanitized text", providerErr.Detail)
	}
}

func TestReadAndCloseProviderBody_RejectsOversizedResponse(t *testing.T) {
	body := io.NopCloser(strings.NewReader(strings.Repeat("x", maxProviderResponseBytes+1)))
	_, err := readAndCloseProviderBody(body)
	if !errors.Is(err, ErrProviderResponseTooLarge) {
		t.Fatalf("error = %v, want ErrProviderResponseTooLarge", err)
	}
}

func TestReadAndCloseProviderBody_PropagatesReadError(t *testing.T) {
	wantErr := errors.New("read failed")
	_, err := readAndCloseProviderBody(&failingProviderBody{readErr: wantErr})
	if !errors.Is(err, wantErr) {
		t.Fatalf("error = %v, want read failure", err)
	}
}

func TestReadAndCloseProviderBody_PropagatesCloseError(t *testing.T) {
	wantErr := errors.New("close failed")
	_, err := readAndCloseProviderBody(&failingProviderBody{closeErr: wantErr})
	if !errors.Is(err, wantErr) {
		t.Fatalf("error = %v, want close failure", err)
	}
}

func TestTruncateText_PreservesUnicodeBoundaries(t *testing.T) {
	if got := truncateText("one🙂two", 4); got != "one🙂..." {
		t.Fatalf("truncateText = %q, want valid rune boundary", got)
	}
	if got := truncateText("text", 0); got != "" {
		t.Fatalf("zero limit = %q, want empty", got)
	}
}

func TestProviderErrorFormatsAvailableContext(t *testing.T) {
	sentinel := errors.New("cause")
	tests := []struct {
		name  string
		err   *ProviderError
		want  string
		cause error
	}{
		{name: "detail", err: &ProviderError{Provider: "backend", Operation: "chat", StatusCode: 503, Detail: "busy", Err: sentinel}, want: "backend chat (HTTP 503): busy", cause: sentinel},
		{name: "cause", err: &ProviderError{Operation: "connect", Err: sentinel}, want: "provider connect: cause", cause: sentinel},
		{name: "unknown", err: &ProviderError{}, want: "provider: unknown error"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := test.err.Error(); got != test.want {
				t.Fatalf("error = %q, want %q", got, test.want)
			}
			if test.cause != nil && !errors.Is(test.err, test.cause) {
				t.Fatalf("unwrap result = %v, want %v", test.err.Unwrap(), test.cause)
			}
		})
	}
}

func TestProviderTimeoutErrorRetainsDeadline(t *testing.T) {
	err := providerTimeoutError("backend", "chat", 250*time.Millisecond)
	if !errors.Is(err, context.DeadlineExceeded) || !strings.Contains(err.Error(), "250ms") {
		t.Fatalf("error = %v, want deadline and duration", err)
	}
}

func TestExecuteProviderRequestRejectsInvalidURL(t *testing.T) {
	_, err := executeProviderRequest(context.Background(), nil, "backend", "chat", "://", nil, nil)
	if err == nil || !strings.Contains(err.Error(), "create request") {
		t.Fatalf("error = %v, want request construction failure", err)
	}
}

func TestExecuteProviderRequestPropagatesTransportAndBodyFailures(t *testing.T) {
	sentinel := errors.New("transport failed")
	client := &http.Client{Transport: providerRoundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, sentinel
	})}
	_, err := executeProviderRequest(context.Background(), client, "backend", "chat", "http://provider.invalid", nil, nil)
	if !errors.Is(err, sentinel) {
		t.Fatalf("transport error = %v, want sentinel", err)
	}

	readFailure := errors.New("read failed")
	client.Transport = providerRoundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Body: &failingProviderBody{readErr: readFailure}}, nil
	})
	_, err = executeProviderRequest(context.Background(), client, "backend", "chat", "http://provider.invalid", nil, nil)
	if !errors.Is(err, readFailure) {
		t.Fatalf("read error = %v, want sentinel", err)
	}

	closeFailure := errors.New("close failed")
	client.Transport = providerRoundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Body: &failingProviderBody{closeErr: closeFailure}}, nil
	})
	body, err := executeProviderRequest(context.Background(), client, "backend", "chat", "http://provider.invalid", nil, nil)
	if len(body) != 0 || !errors.Is(err, closeFailure) {
		t.Fatalf("result = (%q, %v), want empty body and close failure", body, err)
	}
}

func TestOpenProviderStreamFailurePaths(t *testing.T) {
	if err := openTestProviderStream(nil, "://"); err == nil {
		t.Fatal("invalid URL did not fail")
	}

	transportFailure := errors.New("transport failed")
	client := &http.Client{Transport: providerRoundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, transportFailure
	})}
	if err := openTestProviderStream(client, "http://provider.invalid"); !errors.Is(err, transportFailure) {
		t.Fatalf("transport error = %v, want sentinel", err)
	}

	readFailure := errors.New("read failed")
	client.Transport = providerRoundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusBadGateway, Body: &failingProviderBody{readErr: readFailure}}, nil
	})
	if err := openTestProviderStream(client, "http://provider.invalid"); !errors.Is(err, readFailure) {
		t.Fatalf("body error = %v, want sentinel", err)
	}
}

func openTestProviderStream(client *http.Client, endpoint string) error {
	response, err := openProviderStream(context.Background(), client, "backend", "stream", endpoint, nil, nil)
	if response != nil {
		_ = response.Body.Close()
	}
	return err
}

func TestNewProviderRequestCopiesHeaders(t *testing.T) {
	headers := http.Header{"X-Test": []string{"one", "two"}}
	request, err := newProviderRequest(context.Background(), "http://provider.invalid", headers, []byte(`{}`))
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	headers["X-Test"][0] = "changed"
	if got := request.Header.Values("X-Test"); len(got) != 2 || got[0] != "one" {
		t.Fatalf("copied headers = %v", got)
	}
	if request.Header.Get("Content-Type") != "application/json" {
		t.Fatalf("content type = %q", request.Header.Get("Content-Type"))
	}
}

func TestCloseProviderStreamJoinsCloseFailure(t *testing.T) {
	prior := errors.New("stream failed")
	closeFailure := errors.New("close failed")
	err := error(prior)
	closeProviderStream(&failingProviderBody{closeErr: closeFailure}, "backend", "stream", &err)
	if !errors.Is(err, prior) || !errors.Is(err, closeFailure) {
		t.Fatalf("error = %v, want both failures", err)
	}
}

func TestProviderHTTPClientUsesDefaultWithoutMutatingSource(t *testing.T) {
	if providerHTTPClient(nil) == nil {
		t.Fatal("nil client did not use the default client")
	}
	source := &http.Client{}
	clonedClient := providerHTTPClient(source)
	if clonedClient == source || source.CheckRedirect != nil || clonedClient.CheckRedirect == nil {
		t.Fatal("provider client must be a redirect-safe copy")
	}
}

func TestChatOpenAICompatibleRejectsMalformedAndBlankResponses(t *testing.T) {
	responses := []string{"not json", `{"choices":[{"message":{"content":"  "}}]}`}
	for _, responseBody := range responses {
		server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
			_, _ = io.WriteString(response, responseBody)
		}))
		_, err := chatOpenAICompatible(
			context.Background(), server.Client(), "backend", server.URL, "Bearer token",
			newKimiChatRequest("model", "system", "user", false),
		)
		server.Close()
		if err == nil {
			t.Fatalf("response %q did not fail", responseBody)
		}
	}
}

func TestStreamOpenAICompatiblePropagatesOpenFailure(t *testing.T) {
	err := streamOpenAICompatible(
		context.Background(), nil, "backend", "://", "",
		newKimiChatRequest("model", "system", "user", true), make(chan StreamEvent, 1),
	)
	if err == nil {
		t.Fatal("invalid stream endpoint did not fail")
	}
}

type failingProviderBody struct {
	readErr  error
	closeErr error
}

func (b *failingProviderBody) Read([]byte) (int, error) {
	if b.readErr != nil {
		return 0, b.readErr
	}
	return 0, io.EOF
}

func (b *failingProviderBody) Close() error {
	return b.closeErr
}
