package analysis

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/HediAbed/opsmate/failure"
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
		context.Background(), source.Client(), "provider", failure.OperationChat, source.URL, headers, []byte(`{}`),
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
		context.Background(), server.Client(), "provider", failure.OperationChat, server.URL, nil, []byte(`{}`),
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
		{name: "detail", err: &ProviderError{Provider: "backend", Operation: failure.OperationChat, StatusCode: 503, Detail: "busy", Err: sentinel}, want: "backend chat (HTTP 503): busy", cause: sentinel},
		{name: "cause", err: &ProviderError{Operation: failure.OperationConnect, Err: sentinel}, want: "provider connect: cause", cause: sentinel},
		{name: "unknown", err: &ProviderError{}, want: "provider: unknown error"},
		{name: "nil", err: nil, want: "provider: unknown error"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := test.err.Error(); got != test.want {
				t.Fatalf("error = %q, want %q", got, test.want)
			}
			if test.cause != nil && !errors.Is(test.err, test.cause) {
				t.Fatalf("unwrap result = %v, want %v", test.err.Unwrap(), test.cause)
			}
			if test.err == nil && test.err.Unwrap() != nil {
				t.Fatal("nil provider error unwrapped to a non-nil error")
			}
		})
	}
}

func TestProviderErrorClassifiesFailures(t *testing.T) {
	tests := []struct {
		name         string
		statusCode   int
		cause        error
		wantCode     failure.Code
		wantRecovery failure.Recovery
	}{
		{name: "unknown", wantCode: failure.CodeUnknown, wantRecovery: failure.RecoveryPermanent},
		{name: "canceled", cause: context.Canceled, wantCode: failure.CodeCanceled, wantRecovery: failure.RecoveryPermanent},
		{name: "deadline", cause: context.DeadlineExceeded, wantCode: failure.CodeDeadlineExceeded, wantRecovery: failure.RecoveryRetryable},
		{name: "missing context", cause: ErrProviderContextRequired, wantCode: failure.CodeInvalidArgument, wantRecovery: failure.RecoveryPermanent},
		{name: "missing URL", cause: ErrProviderURLRequired, wantCode: failure.CodeInvalidArgument, wantRecovery: failure.RecoveryPermanent},
		{name: "invalid URL", cause: ErrProviderURLInvalid, wantCode: failure.CodeInvalidArgument, wantRecovery: failure.RecoveryPermanent},
		{name: "insecure URL", cause: ErrProviderURLInsecure, wantCode: failure.CodeInvalidArgument, wantRecovery: failure.RecoveryPermanent},
		{name: "missing model", cause: ErrProviderModelRequired, wantCode: failure.CodeInvalidArgument, wantRecovery: failure.RecoveryPermanent},
		{name: "invalid key", cause: ErrProviderAPIKeyInvalid, wantCode: failure.CodeInvalidArgument, wantRecovery: failure.RecoveryPermanent},
		{name: "not configured", cause: ErrProviderNotConfigured, wantCode: failure.CodeFailedPrecondition, wantRecovery: failure.RecoveryPermanent},
		{name: "redirect", statusCode: http.StatusTemporaryRedirect, cause: ErrProviderRedirect, wantCode: failure.CodePermissionDenied, wantRecovery: failure.RecoveryPermanent},
		{name: "bad request", statusCode: http.StatusBadRequest, cause: ErrProviderHTTPStatus, wantCode: failure.CodeInvalidArgument, wantRecovery: failure.RecoveryPermanent},
		{name: "unprocessable", statusCode: http.StatusUnprocessableEntity, cause: ErrProviderHTTPStatus, wantCode: failure.CodeInvalidArgument, wantRecovery: failure.RecoveryPermanent},
		{name: "unauthorized", statusCode: http.StatusUnauthorized, cause: ErrProviderHTTPStatus, wantCode: failure.CodeUnauthenticated, wantRecovery: failure.RecoveryPermanent},
		{name: "forbidden", statusCode: http.StatusForbidden, cause: ErrProviderHTTPStatus, wantCode: failure.CodePermissionDenied, wantRecovery: failure.RecoveryPermanent},
		{name: "not found", statusCode: http.StatusNotFound, cause: ErrProviderHTTPStatus, wantCode: failure.CodeNotFound, wantRecovery: failure.RecoveryPermanent},
		{name: "request timeout", statusCode: http.StatusRequestTimeout, cause: ErrProviderHTTPStatus, wantCode: failure.CodeDeadlineExceeded, wantRecovery: failure.RecoveryRetryable},
		{name: "conflict", statusCode: http.StatusConflict, cause: ErrProviderHTTPStatus, wantCode: failure.CodeConflict, wantRecovery: failure.RecoveryRetryable},
		{name: "rate limited", statusCode: http.StatusTooManyRequests, cause: ErrProviderHTTPStatus, wantCode: failure.CodeRateLimited, wantRecovery: failure.RecoveryRetryable},
		{name: "server error lower bound", statusCode: http.StatusInternalServerError, cause: ErrProviderHTTPStatus, wantCode: failure.CodeUnavailable, wantRecovery: failure.RecoveryRetryable},
		{name: "server error upper bound", statusCode: 599, cause: ErrProviderHTTPStatus, wantCode: failure.CodeUnavailable, wantRecovery: failure.RecoveryRetryable},
		{name: "unmapped HTTP status", statusCode: http.StatusTeapot, cause: ErrProviderHTTPStatus, wantCode: failure.CodeUnknown, wantRecovery: failure.RecoveryPermanent},
		{name: "out of range HTTP status", statusCode: 600, cause: ErrProviderHTTPStatus, wantCode: failure.CodeUnknown, wantRecovery: failure.RecoveryPermanent},
		{name: "empty response", cause: ErrProviderEmptyResponse, wantCode: failure.CodeUnavailable, wantRecovery: failure.RecoveryRetryable},
		{name: "oversized response", cause: ErrProviderResponseTooLarge, wantCode: failure.CodeInternal, wantRecovery: failure.RecoveryPermanent},
		{name: "malformed response", cause: ErrProviderMalformedResponse, wantCode: failure.CodeInternal, wantRecovery: failure.RecoveryPermanent},
		{name: "invalid stream event", cause: ErrProviderStreamEvent, wantCode: failure.CodeInternal, wantRecovery: failure.RecoveryPermanent},
		{name: "network failure", cause: &net.DNSError{Err: "unreachable", Name: "provider.invalid"}, wantCode: failure.CodeUnavailable, wantRecovery: failure.RecoveryRetryable},
		{name: "plain failure", cause: errors.New("failed"), wantCode: failure.CodeUnknown, wantRecovery: failure.RecoveryPermanent},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			providerErr := &ProviderError{StatusCode: test.statusCode, Err: test.cause}
			if got := providerErr.FailureCode(); got != test.wantCode {
				t.Fatalf("FailureCode() = %q, want %q", got, test.wantCode)
			}
			if got := failure.CodeOf(providerErr); got != test.wantCode {
				t.Fatalf("CodeOf() = %q, want %q", got, test.wantCode)
			}
			if got := failure.RecoveryOf(providerErr); got != test.wantRecovery {
				t.Fatalf("RecoveryOf() = %d, want %d", got, test.wantRecovery)
			}
		})
	}

	var nilProviderError *ProviderError
	if got := nilProviderError.FailureCode(); got != failure.CodeUnknown {
		t.Fatalf("nil FailureCode() = %q, want %q", got, failure.CodeUnknown)
	}
	if got := failure.CodeOf(nilProviderError); got != failure.CodeUnknown {
		t.Fatalf("CodeOf(nil provider error) = %q, want %q", got, failure.CodeUnknown)
	}
}

func TestProviderTimeoutErrorRetainsDeadline(t *testing.T) {
	err := providerTimeoutError("backend", failure.OperationChat, 250*time.Millisecond)
	if !errors.Is(err, context.DeadlineExceeded) || !strings.Contains(err.Error(), "250ms") {
		t.Fatalf("error = %v, want deadline and duration", err)
	}
}

func TestExecuteProviderRequestRejectsInvalidURL(t *testing.T) {
	_, err := executeProviderRequest(context.Background(), nil, "backend", failure.OperationChat, "://", nil, nil)
	if err == nil || !strings.Contains(err.Error(), "create request") {
		t.Fatalf("error = %v, want request construction failure", err)
	}
}

func TestExecuteProviderRequestPropagatesTransportAndBodyFailures(t *testing.T) {
	sentinel := errors.New("transport failed")
	client := &http.Client{Transport: providerRoundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, sentinel
	})}
	_, err := executeProviderRequest(context.Background(), client, "backend", failure.OperationChat, "http://provider.invalid", nil, nil)
	if !errors.Is(err, sentinel) {
		t.Fatalf("transport error = %v, want sentinel", err)
	}

	readFailure := errors.New("read failed")
	client.Transport = providerRoundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Body: &failingProviderBody{readErr: readFailure}}, nil
	})
	_, err = executeProviderRequest(context.Background(), client, "backend", failure.OperationChat, "http://provider.invalid", nil, nil)
	if !errors.Is(err, readFailure) {
		t.Fatalf("read error = %v, want sentinel", err)
	}

	closeFailure := errors.New("close failed")
	client.Transport = providerRoundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Body: &failingProviderBody{closeErr: closeFailure}}, nil
	})
	body, err := executeProviderRequest(context.Background(), client, "backend", failure.OperationChat, "http://provider.invalid", nil, nil)
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

	client.Transport = providerRoundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusTooManyRequests,
			Body:       io.NopCloser(strings.NewReader("rate limited")),
		}, nil
	})
	if err := openTestProviderStream(client, "http://provider.invalid"); !errors.Is(err, ErrProviderHTTPStatus) {
		t.Fatalf("status error = %v, want HTTP-status error", err)
	}
}

func openTestProviderStream(client *http.Client, endpoint string) error {
	response, err := openProviderStream(context.Background(), client, "backend", failure.OperationStream, endpoint, nil, nil)
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
	closeProviderStream(&failingProviderBody{closeErr: closeFailure}, "backend", failure.OperationStream, &err)
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

func TestExecuteChatCompletionRejectsInvalidResponses(t *testing.T) {
	tests := []struct {
		name      string
		body      string
		cause     error
		operation failure.Operation
		code      failure.Code
	}{
		{name: "malformed", body: "not json", cause: ErrProviderMalformedResponse, operation: failure.OperationDecode, code: failure.CodeInternal},
		{name: "missing choices", body: `{"choices":[]}`, cause: ErrProviderEmptyResponse, operation: failure.OperationChat, code: failure.CodeUnavailable},
		{name: "blank content", body: `{"choices":[{"message":{"content":"  "}}]}`, cause: ErrProviderEmptyResponse, operation: failure.OperationChat, code: failure.CodeUnavailable},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
				_, _ = io.WriteString(response, test.body)
			}))
			defer server.Close()
			_, err := executeChatCompletion(
				context.Background(), server.Client(), "backend", server.URL, "token",
				newChatCompletionRequest("model", "system", "user", false),
			)
			if !errors.Is(err, test.cause) {
				t.Fatalf("error = %v, want cause %v", err, test.cause)
			}
			var providerErr *ProviderError
			if !errors.As(err, &providerErr) || providerErr.Operation != test.operation {
				t.Fatalf("error = %#v, want %q ProviderError", err, test.operation)
			}
			if got := failure.CodeOf(err); got != test.code {
				t.Fatalf("failure code = %q, want %q", got, test.code)
			}
		})
	}
}

func TestExecuteChatCompletionReportsEncodingFailure(t *testing.T) {
	sentinel := errors.New("encode failed")
	_, err := executeChatCompletionWithEncoder(
		context.Background(),
		nil,
		"backend",
		"http://provider.invalid",
		"",
		newChatCompletionRequest("model", "system", "user", false),
		func(chatCompletionRequest) ([]byte, error) { return nil, sentinel },
	)
	if !errors.Is(err, sentinel) {
		t.Fatalf("error = %v, want encoding failure", err)
	}
	var providerErr *ProviderError
	if !errors.As(err, &providerErr) || providerErr.Operation != failure.OperationEncode {
		t.Fatalf("error = %#v, want encode ProviderError", err)
	}
}

func TestHTTPProviderChatStreamReturnsChunks(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if got := request.Header.Get("Authorization"); got != "Bearer token" {
			t.Errorf("authorization = %q, want bearer token", got)
		}
		_, _ = io.WriteString(response, "data: {\"choices\":[{\"delta\":{\"content\":\"hello\"}}]}\n\ndata: [DONE]\n\n")
	}))
	defer server.Close()

	events := make(chan StreamEvent, 1)
	provider := &HTTPProvider{
		url:    server.URL,
		model:  "model",
		apiKey: "token",
		client: server.Client(),
	}
	err := provider.ChatStream(context.Background(), "system", "user", events)
	if err != nil {
		t.Fatalf("ChatStream() error = %v", err)
	}
	chunk, ok := (<-events).ChunkValue()
	if !ok || chunk != "hello" {
		t.Fatalf("stream event = (%q, %t), want hello chunk", chunk, ok)
	}
}

func TestExecuteChatCompletionPropagatesRequestFailure(t *testing.T) {
	_, err := executeChatCompletion(
		context.Background(),
		nil,
		"backend",
		"://",
		"",
		newChatCompletionRequest("model", "system", "user", false),
	)
	if err == nil {
		t.Fatal("invalid endpoint did not fail")
	}
}

func TestExecuteChatCompletionStreamReportsEncodingFailure(t *testing.T) {
	sentinel := errors.New("encode failed")
	err := executeChatCompletionStreamWithEncoder(
		context.Background(),
		nil,
		"backend",
		"http://provider.invalid",
		"",
		newChatCompletionRequest("model", "system", "user", true),
		make(chan StreamEvent, 1),
		func(chatCompletionRequest) ([]byte, error) { return nil, sentinel },
	)
	if !errors.Is(err, sentinel) {
		t.Fatalf("error = %v, want encoding failure", err)
	}
	var providerErr *ProviderError
	if !errors.As(err, &providerErr) || providerErr.Operation != failure.OperationEncode {
		t.Fatalf("error = %#v, want encode ProviderError", err)
	}
}

func TestExecuteChatCompletionStreamReportsCloseFailure(t *testing.T) {
	closeFailure := errors.New("close failed")
	client := &http.Client{Transport: providerRoundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Body: &providerResponseBody{
				Reader:   strings.NewReader("data: [DONE]\n\n"),
				closeErr: closeFailure,
			},
		}, nil
	})}
	err := executeChatCompletionStream(
		context.Background(), client, "backend", "http://provider.invalid", "",
		newChatCompletionRequest("model", "system", "user", true), make(chan StreamEvent, 1),
	)
	if !errors.Is(err, closeFailure) {
		t.Fatalf("error = %v, want close failure", err)
	}
}

func TestExecuteChatCompletionStreamPropagatesOpenFailure(t *testing.T) {
	err := executeChatCompletionStream(
		context.Background(), nil, "backend", "://", "",
		newChatCompletionRequest("model", "system", "user", true), make(chan StreamEvent, 1),
	)
	if err == nil {
		t.Fatal("invalid stream endpoint did not fail")
	}
}

type failingProviderBody struct {
	readErr  error
	closeErr error
}

type providerResponseBody struct {
	io.Reader
	closeErr error
}

func (b *providerResponseBody) Close() error {
	return b.closeErr
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
