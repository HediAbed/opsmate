package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"

	"github.com/HediAbed/opsmate/internal/failure"
	"github.com/HediAbed/opsmate/internal/terminal"
)

const (
	maxProviderResponseBytes = 4 * 1024 * 1024
	maxErrorDetailRunes      = 200
)

var (
	ErrProviderEmptyResponse     = errors.New("provider returned an empty response")
	ErrProviderMalformedResponse = errors.New("provider response is malformed")
	ErrProviderNotConfigured     = errors.New("provider is not configured")
	ErrProviderResponseTooLarge  = errors.New("provider response exceeded safety limit")
	ErrProviderHTTPStatus        = errors.New("provider returned a non-success HTTP status")
	ErrProviderRedirect          = errors.New("provider redirects are disabled")
	ErrProviderContextRequired   = errors.New("provider context is required")
	ErrProviderURLRequired       = errors.New("provider URL is required")
	ErrProviderURLInvalid        = errors.New("provider URL is invalid")
	ErrProviderURLInsecure       = errors.New("provider URL must use HTTPS or loopback HTTP")
	ErrProviderModelRequired     = errors.New("provider model is required")
	ErrProviderAPIKeyInvalid     = errors.New("provider API key contains invalid characters")
	ErrProviderStreamEvent       = errors.New("provider stream event is invalid")
)

type Error struct {
	Provider   string
	Operation  failure.Operation
	StatusCode int
	Detail     string
	Err        error
}

func (e *Error) Error() string {
	if e == nil {
		return "provider: unknown error"
	}
	prefix := "provider"
	if e.Provider != "" {
		prefix = e.Provider
	}
	if e.Operation != "" {
		prefix += " " + string(e.Operation)
	}
	if e.StatusCode != 0 {
		prefix += fmt.Sprintf(" (HTTP %d)", e.StatusCode)
	}
	if e.Detail != "" {
		return prefix + ": " + e.Detail
	}
	if e.Err != nil {
		return prefix + ": " + e.Err.Error()
	}
	return prefix + ": unknown error"
}

func (e *Error) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

func (e *Error) FailureCode() failure.Code {
	if e == nil {
		return failure.CodeUnknown
	}
	return classifyProviderFailure(e.StatusCode, e.Err)
}

func classifyProviderFailure(statusCode int, err error) failure.Code {
	switch {
	case errors.Is(err, context.Canceled):
		return failure.CodeCanceled
	case errors.Is(err, context.DeadlineExceeded):
		return failure.CodeDeadlineExceeded
	case isInvalidProviderInput(err):
		return failure.CodeInvalidArgument
	case errors.Is(err, ErrProviderNotConfigured):
		return failure.CodeFailedPrecondition
	case errors.Is(err, ErrProviderRedirect):
		return failure.CodePermissionDenied
	case statusCode != 0:
		return providerHTTPFailureCode(statusCode)
	case errors.Is(err, ErrProviderEmptyResponse):
		return failure.CodeUnavailable
	case isInternalProviderFailure(err):
		return failure.CodeInternal
	}
	var networkErr net.Error
	if errors.As(err, &networkErr) {
		return failure.CodeUnavailable
	}
	return failure.CodeUnknown
}

func isInternalProviderFailure(err error) bool {
	return errors.Is(err, ErrProviderResponseTooLarge) ||
		errors.Is(err, ErrProviderMalformedResponse) ||
		errors.Is(err, ErrProviderStreamEvent)
}

func isInvalidProviderInput(err error) bool {
	return errors.Is(err, ErrProviderContextRequired) ||
		errors.Is(err, ErrProviderURLRequired) ||
		errors.Is(err, ErrProviderURLInvalid) ||
		errors.Is(err, ErrProviderURLInsecure) ||
		errors.Is(err, ErrProviderModelRequired) ||
		errors.Is(err, ErrProviderAPIKeyInvalid)
}

func providerHTTPFailureCode(statusCode int) failure.Code {
	if code, found := providerClientFailureCode(statusCode); found {
		return code
	}
	if statusCode >= http.StatusInternalServerError && statusCode <= 599 {
		return failure.CodeUnavailable
	}
	return failure.CodeUnknown
}

func providerClientFailureCode(statusCode int) (failure.Code, bool) {
	switch statusCode {
	case http.StatusBadRequest, http.StatusUnprocessableEntity:
		return failure.CodeInvalidArgument, true
	case http.StatusUnauthorized:
		return failure.CodeUnauthenticated, true
	case http.StatusForbidden:
		return failure.CodePermissionDenied, true
	case http.StatusNotFound:
		return failure.CodeNotFound, true
	case http.StatusRequestTimeout:
		return failure.CodeDeadlineExceeded, true
	case http.StatusConflict:
		return failure.CodeConflict, true
	case http.StatusTooManyRequests:
		return failure.CodeRateLimited, true
	default:
		return failure.CodeUnknown, false
	}
}

func executeProviderRequest(
	ctx context.Context,
	client *http.Client,
	provider string,
	operation failure.Operation,
	url string,
	headers http.Header,
	payload []byte,
) (_ []byte, returnErr error) {
	request, err := newProviderRequest(ctx, url, headers, payload)
	if err != nil {
		return nil, &Error{Provider: provider, Operation: operation, Err: err}
	}
	response, err := providerHTTPClient(client).Do(request)
	if err != nil {
		return nil, &Error{Provider: provider, Operation: operation, Err: err}
	}
	defer func() {
		if closeErr := response.Body.Close(); closeErr != nil {
			returnErr = errors.Join(returnErr, &Error{
				Provider:  provider,
				Operation: operation,
				Err:       fmt.Errorf("close response: %w", closeErr),
			})
		}
	}()
	body, readErr := readProviderBody(response.Body)
	if readErr != nil {
		return nil, &Error{Provider: provider, Operation: operation, Err: readErr}
	}
	if response.StatusCode != http.StatusOK {
		return nil, providerStatusError(provider, operation, response.StatusCode, body)
	}
	return body, nil
}

func openProviderStream(
	ctx context.Context,
	client *http.Client,
	provider string,
	operation failure.Operation,
	url string,
	headers http.Header,
	payload []byte,
) (*http.Response, error) {
	request, err := newProviderRequest(ctx, url, headers, payload)
	if err != nil {
		return nil, &Error{Provider: provider, Operation: operation, Err: err}
	}
	response, err := providerHTTPClient(client).Do(request)
	if err != nil {
		return nil, &Error{Provider: provider, Operation: operation, Err: err}
	}
	if response.StatusCode == http.StatusOK {
		return response, nil
	}
	body, readErr := readAndCloseProviderBody(response.Body)
	if readErr != nil {
		return nil, &Error{Provider: provider, Operation: operation, Err: readErr}
	}
	return nil, providerStatusError(provider, operation, response.StatusCode, body)
}

func newProviderRequest(ctx context.Context, url string, headers http.Header, payload []byte) (*http.Request, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	request.Header = make(http.Header)
	for name, values := range headers {
		request.Header[name] = append([]string(nil), values...)
	}
	request.Header.Set("Content-Type", "application/json")
	return request, nil
}

func readAndCloseProviderBody(body io.ReadCloser) (data []byte, returnErr error) {
	defer func() {
		if closeErr := body.Close(); closeErr != nil && returnErr == nil {
			returnErr = fmt.Errorf("close response: %w", closeErr)
		}
	}()
	return readProviderBody(body)
}

func readProviderBody(body io.Reader) ([]byte, error) {
	limited := io.LimitReader(body, maxProviderResponseBytes+1)
	data, err := io.ReadAll(limited)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}
	if len(data) > maxProviderResponseBytes {
		return nil, ErrProviderResponseTooLarge
	}
	return data, nil
}

func closeProviderStream(body io.Closer, provider string, operation failure.Operation, returnErr *error) {
	closeErr := body.Close()
	if closeErr == nil {
		return
	}
	wrapped := &Error{Provider: provider, Operation: operation, Err: fmt.Errorf("close response: %w", closeErr)}
	*returnErr = errors.Join(*returnErr, wrapped)
}

func providerStatusError(provider string, operation failure.Operation, statusCode int, body []byte) error {
	detail := strings.TrimSpace(terminal.SanitizeText(string(body)))
	detail = truncateText(detail, maxErrorDetailRunes)
	providerErr := &Error{
		Provider:   provider,
		Operation:  operation,
		StatusCode: statusCode,
		Detail:     detail,
		Err:        ErrProviderHTTPStatus,
	}
	if statusCode >= http.StatusMultipleChoices && statusCode < http.StatusBadRequest {
		providerErr.Err = errors.Join(ErrProviderHTTPStatus, ErrProviderRedirect)
	}
	return providerErr
}

func providerHTTPClient(client *http.Client) *http.Client {
	if client == nil {
		client = http.DefaultClient
	}
	requestClient := *client
	requestClient.CheckRedirect = func(_ *http.Request, _ []*http.Request) error {
		return http.ErrUseLastResponse
	}
	return &requestClient
}

func truncateText(text string, maxRunes int) string {
	if maxRunes < 1 {
		return ""
	}
	runes := []rune(text)
	if len(runes) <= maxRunes {
		return text
	}
	return string(runes[:maxRunes]) + "..."
}

type chatRequestEncoder func(chatCompletionRequest) ([]byte, error)

func encodeChatCompletionRequest(request chatCompletionRequest) ([]byte, error) {
	return json.Marshal(request)
}

func executeChatCompletion(
	ctx context.Context,
	client *http.Client,
	provider string,
	url string,
	apiKey string,
	request chatCompletionRequest,
) (string, error) {
	return executeChatCompletionWithEncoder(
		ctx,
		client,
		provider,
		url,
		apiKey,
		request,
		encodeChatCompletionRequest,
	)
}

func executeChatCompletionWithEncoder(
	ctx context.Context,
	client *http.Client,
	provider string,
	url string,
	apiKey string,
	request chatCompletionRequest,
	encode chatRequestEncoder,
) (string, error) {
	payload, err := encode(request)
	if err != nil {
		return "", &Error{Provider: provider, Operation: failure.OperationEncode, Err: err}
	}
	headers := providerHeaders(apiKey)
	body, err := executeProviderRequest(ctx, client, provider, failure.OperationChat, url, headers, payload)
	if err != nil {
		return "", err
	}
	var response chatCompletionResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return "", &Error{
			Provider:  provider,
			Operation: failure.OperationDecode,
			Err:       fmt.Errorf("%w: %w", ErrProviderMalformedResponse, err),
		}
	}
	if len(response.Choices) == 0 {
		return "", &Error{Provider: provider, Operation: failure.OperationChat, Err: ErrProviderEmptyResponse}
	}
	content := strings.TrimSpace(terminal.SanitizeText(response.Choices[0].Message.Content))
	if content == "" {
		return "", &Error{Provider: provider, Operation: failure.OperationChat, Err: ErrProviderEmptyResponse}
	}
	return content, nil
}

func executeChatCompletionStream(
	ctx context.Context,
	client *http.Client,
	provider string,
	url string,
	apiKey string,
	request chatCompletionRequest,
	events chan<- StreamEvent,
) (returnErr error) {
	return executeChatCompletionStreamWithEncoder(
		ctx,
		client,
		provider,
		url,
		apiKey,
		request,
		events,
		encodeChatCompletionRequest,
	)
}

func executeChatCompletionStreamWithEncoder(
	ctx context.Context,
	client *http.Client,
	provider string,
	url string,
	apiKey string,
	request chatCompletionRequest,
	events chan<- StreamEvent,
	encode chatRequestEncoder,
) (returnErr error) {
	payload, err := encode(request)
	if err != nil {
		return &Error{Provider: provider, Operation: failure.OperationEncode, Err: err}
	}
	response, err := openProviderStream(
		ctx,
		client,
		provider,
		failure.OperationStream,
		url,
		providerHeaders(apiKey),
		payload,
	)
	if err != nil {
		return err
	}
	defer closeProviderStream(response.Body, provider, failure.OperationStream, &returnErr)
	return streamSSE(ctx, response.Body, events, func(payload []byte) (string, error) {
		return decodeChatCompletionChunk(provider, payload)
	})
}

func providerHeaders(apiKey string) http.Header {
	headers := make(http.Header)
	if apiKey != "" {
		headers.Set("Authorization", "Bearer "+apiKey)
	}
	return headers
}
