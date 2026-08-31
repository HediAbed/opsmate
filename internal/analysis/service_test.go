package analysis

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/HediAbed/opsmate/internal/analysis/provider"
)

type stubProvider struct {
	name string
}

func (s *stubProvider) Name() string {
	return s.name
}

func (s *stubProvider) Chat(context.Context, string, string) (string, error) {
	return s.name, nil
}

func TestServiceReportsProviderState(t *testing.T) {
	disabled := NewService(nil)
	if disabled.Available() || disabled.SupportsStreaming() || disabled.ProviderName() != unavailableProviderName {
		t.Fatalf("disabled service state = (%t, %t, %q)", disabled.Available(), disabled.SupportsStreaming(), disabled.ProviderName())
	}

	configured := NewService(&stubProvider{name: "configured"})
	if !configured.Available() || configured.SupportsStreaming() || configured.ProviderName() != "configured" {
		t.Fatalf("configured service state = (%t, %t, %q)", configured.Available(), configured.SupportsStreaming(), configured.ProviderName())
	}
}

func TestServiceSupportsConcurrentReads(_ *testing.T) {
	service := NewService(&stubProvider{name: "configured"})
	const readers = 16
	const iterations = 500
	var reads sync.WaitGroup
	for range readers {
		reads.Go(func() {
			for range iterations {
				_ = service.Available()
				_ = service.ProviderName()
				_ = service.SupportsStreaming()
			}
		})
	}
	reads.Wait()
}

func TestNewServiceFromEnvironment(t *testing.T) {
	t.Setenv("OPSMATE_PROVIDER_URL", "")
	t.Setenv("OPSMATE_PROVIDER_MODEL", "")
	t.Setenv("OPSMATE_PROVIDER_API_KEY", "")
	disabled, err := NewServiceFromEnvironment()
	if err != nil || disabled.Available() {
		t.Fatalf("disabled service = (%+v, %v)", disabled, err)
	}

	t.Setenv("OPSMATE_PROVIDER_URL", "https://provider.invalid/v1/chat/completions")
	t.Setenv("OPSMATE_PROVIDER_MODEL", "model")
	configured, err := NewServiceFromEnvironment()
	if err != nil || !configured.Available() {
		t.Fatalf("configured service = (%+v, %v)", configured, err)
	}

	t.Setenv("OPSMATE_PROVIDER_URL", "http://provider.invalid/v1/chat/completions")
	invalid, err := NewServiceFromEnvironment()
	if !errors.Is(err, provider.ErrProviderURLInsecure) || invalid.Available() {
		t.Fatalf("invalid service = (%+v, %v)", invalid, err)
	}
}
