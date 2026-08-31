package provider

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/HediAbed/opsmate/internal/failure"
)

const (
	providerURLEnvironment   = "OPSMATE_PROVIDER_URL"
	providerModelEnvironment = "OPSMATE_PROVIDER_MODEL"
	providerKeyEnvironment   = "OPSMATE_PROVIDER_API_KEY"
	configuredClientName     = "Configured"
	providerHTTPTimeout      = time.Minute
	providerTemperature      = 0.3
	providerMaximumTokens    = 2048
)

type Client interface {
	Name() string
	Chat(context.Context, string, string) (string, error)
}

type StreamingClient interface {
	Client
	ChatStream(context.Context, string, string, chan<- StreamEvent) error
}

type Config struct {
	URL    string
	Model  string
	APIKey string
}

type HTTPClient struct {
	url    string
	model  string
	apiKey string
	client *http.Client
}

func NewHTTPClient(config Config) (*HTTPClient, error) {
	endpoint := strings.TrimSpace(config.URL)
	model := strings.TrimSpace(config.Model)
	if endpoint == "" {
		return nil, &Error{Operation: failure.OperationConfigure, Err: ErrProviderURLRequired}
	}
	if model == "" {
		return nil, &Error{Operation: failure.OperationConfigure, Err: ErrProviderModelRequired}
	}
	if strings.ContainsAny(config.APIKey, "\r\n") {
		return nil, &Error{Operation: failure.OperationConfigure, Err: ErrProviderAPIKeyInvalid}
	}
	if err := validateProviderURL(endpoint); err != nil {
		return nil, &Error{Operation: failure.OperationConfigure, Err: err}
	}
	return &HTTPClient{
		url:    endpoint,
		model:  model,
		apiKey: config.APIKey,
		client: &http.Client{Timeout: providerHTTPTimeout},
	}, nil
}

func validateProviderURL(endpoint string) error {
	if strings.Contains(endpoint, "#") {
		return ErrProviderURLInvalid
	}
	parsed, err := url.ParseRequestURI(endpoint)
	if err != nil || parsed.Host == "" || parsed.User != nil || parsed.Fragment != "" {
		return ErrProviderURLInvalid
	}
	if parsed.Scheme == "https" {
		return nil
	}
	if parsed.Scheme == "http" && isLoopbackHost(parsed.Hostname()) {
		return nil
	}
	return ErrProviderURLInsecure
}

func isLoopbackHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	address := net.ParseIP(host)
	return address != nil && address.IsLoopback()
}

func (*HTTPClient) Name() string {
	return configuredClientName
}

func (p *HTTPClient) Chat(ctx context.Context, systemPrompt, userMessage string) (string, error) {
	if ctx == nil {
		return "", &Error{Provider: p.Name(), Operation: failure.OperationChat, Err: ErrProviderContextRequired}
	}
	request := newChatCompletionRequest(p.model, systemPrompt, userMessage, false)
	return executeChatCompletion(ctx, p.client, p.Name(), p.url, p.apiKey, request)
}

func (p *HTTPClient) ChatStream(
	ctx context.Context,
	systemPrompt string,
	userMessage string,
	events chan<- StreamEvent,
) error {
	if ctx == nil {
		return &Error{Provider: p.Name(), Operation: failure.OperationStream, Err: ErrProviderContextRequired}
	}
	request := newChatCompletionRequest(p.model, systemPrompt, userMessage, true)
	return executeChatCompletionStream(ctx, p.client, p.Name(), p.url, p.apiKey, request, events)
}

func decodeChatCompletionChunk(provider string, payload []byte) (string, error) {
	var response chatCompletionStreamResponse
	if err := json.Unmarshal(payload, &response); err != nil {
		return "", malformedSSEPayload(provider, err)
	}
	if len(response.Choices) == 0 {
		return "", nil
	}
	return response.Choices[0].Delta.Content, nil
}

func Detect() (Client, error) {
	config := Config{
		URL:    os.Getenv(providerURLEnvironment),
		Model:  os.Getenv(providerModelEnvironment),
		APIKey: os.Getenv(providerKeyEnvironment),
	}
	if config.URL == "" && config.Model == "" && config.APIKey == "" {
		return nil, nil
	}
	return NewHTTPClient(config)
}

type streamEventKind uint8

const (
	streamChunk streamEventKind = iota + 1
	streamFailure
)

type StreamEvent struct {
	kind  streamEventKind
	chunk string
	err   error
}

func NewChunk(chunk string) StreamEvent {
	return StreamEvent{kind: streamChunk, chunk: chunk}
}

func NewFailure(err error) StreamEvent {
	return StreamEvent{kind: streamFailure, err: err}
}

func (e StreamEvent) ChunkValue() (string, bool) {
	return e.chunk, e.kind == streamChunk
}

func (e StreamEvent) Failure() (bool, error) {
	return e.kind == streamFailure, e.err
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatCompletionRequest struct {
	Model       string        `json:"model"`
	Messages    []chatMessage `json:"messages"`
	Temperature float64       `json:"temperature"`
	MaxTokens   int           `json:"max_tokens"`
	Stream      bool          `json:"stream,omitempty"`
}

func newChatCompletionRequest(model, systemPrompt, userMessage string, stream bool) chatCompletionRequest {
	return chatCompletionRequest{
		Model: model,
		Messages: []chatMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userMessage},
		},
		Temperature: providerTemperature,
		MaxTokens:   providerMaximumTokens,
		Stream:      stream,
	}
}

type chatCompletionResponse struct {
	Choices []struct {
		Message chatMessage `json:"message"`
	} `json:"choices"`
}

type chatCompletionStreamResponse struct {
	Choices []struct {
		Delta chatMessage `json:"delta"`
	} `json:"choices"`
}
