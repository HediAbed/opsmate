package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const (
	maxProviderResponseBytes    = 4 * 1024 * 1024
	maxProviderErrorDetailRunes = 200
)

var (
	ErrProviderEmptyResponse    = errors.New("provider returned an empty response")
	ErrProviderNotConfigured    = errors.New("provider is not configured")
	ErrProviderResponseTooLarge = errors.New("provider response exceeded safety limit")
	ErrProviderHTTPStatus       = errors.New("provider returned a non-success HTTP status")
	ErrProviderRedirect         = errors.New("provider redirects are disabled")
)

type ProviderError struct {
	Provider   string
	Operation  string
	StatusCode int
	Detail     string
	Err        error
}

func (e *ProviderError) Error() string {
	prefix := "provider"
	if e.Provider != "" {
		prefix = e.Provider
	}
	if e.Operation != "" {
		prefix += " " + e.Operation
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

func (e *ProviderError) Unwrap() error {
	return e.Err
}

func missingProviderError() error {
	return &ProviderError{
		Operation: "configure",
		Detail:    "set GEMINI_API_KEY, OLLAMA_MODEL, MOONSHOT_API_KEY, or enable the supported CLI provider",
		Err:       ErrProviderNotConfigured,
	}
}

func providerTimeoutError(provider, operation string, timeout time.Duration) error {
	return &ProviderError{
		Provider:  provider,
		Operation: operation,
		Detail:    "timed out after " + timeout.String(),
		Err:       context.DeadlineExceeded,
	}
}

func executeProviderRequest(
	ctx context.Context,
	client *http.Client,
	provider string,
	operation string,
	url string,
	headers http.Header,
	payload []byte,
) (_ []byte, returnErr error) {
	request, err := newProviderRequest(ctx, url, headers, payload)
	if err != nil {
		return nil, &ProviderError{Provider: provider, Operation: operation, Err: err}
	}
	response, err := providerHTTPClient(client).Do(request)
	if err != nil {
		return nil, &ProviderError{Provider: provider, Operation: operation, Err: err}
	}
	defer func() {
		if closeErr := response.Body.Close(); closeErr != nil {
			returnErr = errors.Join(returnErr, &ProviderError{
				Provider:  provider,
				Operation: operation,
				Err:       fmt.Errorf("close response: %w", closeErr),
			})
		}
	}()
	body, readErr := readProviderBody(response.Body)
	if readErr != nil {
		return nil, &ProviderError{Provider: provider, Operation: operation, Err: readErr}
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
	operation string,
	url string,
	headers http.Header,
	payload []byte,
) (*http.Response, error) {
	request, err := newProviderRequest(ctx, url, headers, payload)
	if err != nil {
		return nil, &ProviderError{Provider: provider, Operation: operation, Err: err}
	}
	response, err := providerHTTPClient(client).Do(request)
	if err != nil {
		return nil, &ProviderError{Provider: provider, Operation: operation, Err: err}
	}
	if response.StatusCode == http.StatusOK {
		return response, nil
	}
	body, readErr := readAndCloseProviderBody(response.Body)
	if readErr != nil {
		return nil, &ProviderError{Provider: provider, Operation: operation, Err: readErr}
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

func closeProviderStream(body io.Closer, provider, operation string, returnErr *error) {
	closeErr := body.Close()
	if closeErr == nil {
		return
	}
	wrapped := &ProviderError{Provider: provider, Operation: operation, Err: fmt.Errorf("close response: %w", closeErr)}
	*returnErr = errors.Join(*returnErr, wrapped)
}

func providerStatusError(provider, operation string, statusCode int, body []byte) error {
	detail := strings.TrimSpace(stripANSI(string(body)))
	detail = truncateText(detail, maxProviderErrorDetailRunes)
	providerErr := &ProviderError{
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

type openAICompatibleRequest interface {
	ollamaChatRequest | kimiChatRequest
}

type providerJSONRequest interface {
	geminiChatRequest | ollamaChatRequest | kimiChatRequest | clusterAnalysisRequest
}

func marshalProviderRequest[Request providerJSONRequest](request Request) []byte {
	payload, _ := json.Marshal(request)
	return payload
}

func chatOpenAICompatible[Request openAICompatibleRequest](
	ctx context.Context,
	client *http.Client,
	provider string,
	url string,
	authorization string,
	request Request,
) (string, error) {
	payload := marshalProviderRequest(request)
	headers := providerHeaders(authorization)
	body, err := executeProviderRequest(ctx, client, provider, "chat", url, headers, payload)
	if err != nil {
		return "", err
	}
	var response openAIChatResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return "", &ProviderError{Provider: provider, Operation: "parse response", Err: err}
	}
	if len(response.Choices) == 0 {
		return "", &ProviderError{Provider: provider, Operation: "chat", Err: ErrProviderEmptyResponse}
	}
	content := strings.TrimSpace(stripANSI(response.Choices[0].Message.Content))
	if content == "" {
		return "", &ProviderError{Provider: provider, Operation: "chat", Err: ErrProviderEmptyResponse}
	}
	return content, nil
}

func streamOpenAICompatible[Request openAICompatibleRequest](
	ctx context.Context,
	client *http.Client,
	provider string,
	url string,
	authorization string,
	request Request,
	events chan<- StreamEvent,
) (returnErr error) {
	payload := marshalProviderRequest(request)
	response, err := openProviderStream(ctx, client, provider, "stream", url, providerHeaders(authorization), payload)
	if err != nil {
		return err
	}
	defer closeProviderStream(response.Body, provider, "stream", &returnErr)
	return streamSSE(ctx, response.Body, events, extractKimiChunk)
}

func providerHeaders(authorization string) http.Header {
	headers := make(http.Header)
	if authorization != "" {
		headers.Set("Authorization", authorization)
	}
	return headers
}
