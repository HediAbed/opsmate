package service

import (
	"context"
	"sync"
	"testing"
)

type stubProvider struct{ name string }

func (s *stubProvider) Name() string { return s.name }

func (s *stubProvider) Chat(_ context.Context, _, _ string) (string, error) {
	return s.name, nil
}

func TestSetActiveProvider_ConcurrentInitAndRead(t *testing.T) {
	prev := getActiveProvider()
	t.Cleanup(func() { setActiveProvider(prev) })

	providers := []AIProvider{
		&stubProvider{name: "A"},
		&stubProvider{name: "B"},
		&stubProvider{name: "C"},
	}

	const writers = 4
	const readers = 16
	const iterations = 500

	var wg sync.WaitGroup

	for i := 0; i < writers; i++ {
		wg.Add(1)
		go func(seed int) {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				setActiveProvider(providers[(seed+j)%len(providers)])
			}
		}(i)
	}

	for i := 0; i < readers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				_ = getActiveProvider()
				_ = ProviderName()
				_ = SupportsStreaming()
			}
		}()
	}

	wg.Wait()
}

func TestInitAIProvider_NilDoesNotPanic(t *testing.T) {
	prev := getActiveProvider()
	t.Cleanup(func() { setActiveProvider(prev) })

	setActiveProvider(&stubProvider{name: "first"})
	setActiveProvider(nil)

	if got := ProviderName(); got != "None" {
		t.Errorf("ProviderName() with nil provider = %q; want None", got)
	}
	if SupportsStreaming() {
		t.Error("SupportsStreaming() should be false when provider is nil")
	}
}
