package analysis

import (
	"errors"
	"testing"
)

func withCleanProvider(t *testing.T) {
	t.Helper()
	previous := getActiveProvider()
	setActiveProvider(nil)
	t.Cleanup(func() { setActiveProvider(previous) })
}

func clearProviderEnvironment(t *testing.T) {
	t.Helper()
	t.Setenv(providerURLEnvironment, "")
	t.Setenv(providerModelEnvironment, "")
	t.Setenv(providerKeyEnvironment, "")
}

func TestNewHTTPProviderValidatesConfiguration(t *testing.T) {
	tests := []struct {
		name   string
		config ProviderConfig
		cause  error
	}{
		{name: "missing URL", config: ProviderConfig{Model: "model"}, cause: ErrProviderURLRequired},
		{name: "missing model", config: ProviderConfig{URL: "https://provider.invalid/chat"}, cause: ErrProviderModelRequired},
		{name: "invalid URL", config: ProviderConfig{URL: "://", Model: "model"}, cause: ErrProviderURLInvalid},
		{name: "relative URL", config: ProviderConfig{URL: "/chat", Model: "model"}, cause: ErrProviderURLInvalid},
		{name: "embedded user info", config: ProviderConfig{URL: "https://identity@provider-invalid/chat", Model: "model"}, cause: ErrProviderURLInvalid},
		{name: "fragment", config: ProviderConfig{URL: "https://provider.invalid/chat#part", Model: "model"}, cause: ErrProviderURLInvalid},
		{name: "remote HTTP", config: ProviderConfig{URL: "http://provider.invalid/chat", Model: "model"}, cause: ErrProviderURLInsecure},
		{name: "invalid key", config: ProviderConfig{URL: "https://provider.invalid/chat", Model: "model", APIKey: "line\nbreak"}, cause: ErrProviderAPIKeyInvalid},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			provider, err := NewHTTPProvider(test.config)
			if provider != nil || !errors.Is(err, test.cause) {
				t.Fatalf("NewHTTPProvider() = (%v, %v), want cause %v", provider, err, test.cause)
			}
		})
	}
}

func TestNewHTTPProviderAcceptsSecureAndLoopbackEndpoints(t *testing.T) {
	endpoints := []string{
		"https://provider.invalid/chat",
		"http://localhost:8080/chat",
		"http://127.0.0.1:8080/chat",
		"http://[::1]:8080/chat",
	}
	for _, endpoint := range endpoints {
		t.Run(endpoint, func(t *testing.T) {
			provider, err := NewHTTPProvider(ProviderConfig{URL: "  " + endpoint + "  ", Model: "  model  ", APIKey: "key"})
			if err != nil || provider == nil {
				t.Fatalf("NewHTTPProvider() = (%v, %v)", provider, err)
			}
			if provider.url != endpoint || provider.model != "model" || provider.apiKey != "key" {
				t.Fatalf("provider = %+v", provider)
			}
			if provider.Name() != configuredProviderName {
				t.Fatalf("Name() = %q", provider.Name())
			}
		})
	}
	if isLoopbackHost("192.0.2.1") || isLoopbackHost("provider.invalid") {
		t.Fatal("non-loopback host accepted")
	}
}

func TestDetectProviderUsesNormalizedEnvironment(t *testing.T) {
	clearProviderEnvironment(t)
	provider, err := DetectProvider()
	if err != nil || provider != nil {
		t.Fatalf("DetectProvider(unconfigured) = (%v, %v)", provider, err)
	}

	t.Setenv(providerURLEnvironment, "https://provider.invalid/chat")
	t.Setenv(providerModelEnvironment, "model")
	t.Setenv(providerKeyEnvironment, "key")
	provider, err = DetectProvider()
	if err != nil || provider == nil || provider.Name() != configuredProviderName {
		t.Fatalf("DetectProvider(configured) = (%v, %v)", provider, err)
	}
}

func TestInitProviderOwnsProviderLifecycle(t *testing.T) {
	withCleanProvider(t)
	clearProviderEnvironment(t)
	if err := InitProvider(); err != nil || HasProvider() {
		t.Fatalf("InitProvider(unconfigured) = %v, active=%t", err, HasProvider())
	}
	if ProviderName() != "None" {
		t.Fatalf("ProviderName() = %q", ProviderName())
	}

	t.Setenv(providerURLEnvironment, "https://provider.invalid/chat")
	t.Setenv(providerModelEnvironment, "model")
	if err := InitProvider(); err != nil || !HasProvider() {
		t.Fatalf("InitProvider(configured) = %v, active=%t", err, HasProvider())
	}
	if ProviderName() != configuredProviderName {
		t.Fatalf("ProviderName() = %q", ProviderName())
	}

	t.Setenv(providerURLEnvironment, "http://provider.invalid/chat")
	if err := InitProvider(); !errors.Is(err, ErrProviderURLInsecure) {
		t.Fatalf("InitProvider(invalid) = %v", err)
	}
	if HasProvider() {
		t.Fatal("provider remained active after failed initialization")
	}
}
