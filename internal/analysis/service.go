package analysis

import (
	"github.com/HediAbed/opsmate/internal/analysis/provider"
	"github.com/HediAbed/opsmate/internal/failure"
)

const unavailableProviderName = "None"

type Service struct {
	client provider.Client
}

func NewService(client provider.Client) Service {
	return Service{client: client}
}

func NewServiceFromEnvironment() (Service, error) {
	client, err := provider.Detect()
	if err != nil {
		return Service{}, err
	}
	return NewService(client), nil
}

func (s Service) ProviderName() string {
	if s.client == nil {
		return unavailableProviderName
	}
	return s.client.Name()
}

func (s Service) Available() bool {
	return s.client != nil
}

func (s Service) SupportsStreaming() bool {
	_, supported := s.client.(provider.StreamingClient)
	return supported
}

type StreamEvent = provider.StreamEvent

func missingProviderError() error {
	return &provider.Error{
		Operation: failure.OperationConfigure,
		Detail:    "set OPSMATE_PROVIDER_URL and OPSMATE_PROVIDER_MODEL",
		Err:       provider.ErrProviderNotConfigured,
	}
}

func readStreamEvent(events <-chan StreamEvent) StreamChunkMsg {
	event, open := <-events
	if !open {
		return StreamChunkMsg{Done: true}
	}
	if chunk, ok := event.ChunkValue(); ok {
		return StreamChunkMsg{Chunk: chunk}
	}
	if failed, err := event.Failure(); failed {
		return StreamChunkMsg{Err: err}
	}
	return StreamChunkMsg{Err: &provider.Error{
		Operation: failure.OperationDecode,
		Err:       provider.ErrProviderStreamEvent,
	}}
}
