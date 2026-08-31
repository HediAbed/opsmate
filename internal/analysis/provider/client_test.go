package provider

import (
	"errors"
	"testing"
)

func clearProviderEnvironment(t *testing.T) {
	t.Helper()
	t.Setenv(providerURLEnvironment, "")
	t.Setenv(providerModelEnvironment, "")
	t.Setenv(providerKeyEnvironment, "")
}

func TestNewHTTPClientValidatesConfiguration(t *testing.T) {
	tests := []struct {
		name   string
		config Config
		cause  error
	}{
		{name: "missing URL", config: Config{Model: "model"}, cause: ErrProviderURLRequired},
		{name: "missing model", config: Config{URL: "https://provider.invalid/chat"}, cause: ErrProviderModelRequired},
		{name: "invalid URL", config: Config{URL: "://", Model: "model"}, cause: ErrProviderURLInvalid},
		{name: "relative URL", config: Config{URL: "/chat", Model: "model"}, cause: ErrProviderURLInvalid},
		{name: "embedded user info", config: Config{URL: "https://identity@provider-invalid/chat", Model: "model"}, cause: ErrProviderURLInvalid},
		{name: "fragment", config: Config{URL: "https://provider.invalid/chat#part", Model: "model"}, cause: ErrProviderURLInvalid},
		{name: "remote HTTP", config: Config{URL: "http://provider.invalid/chat", Model: "model"}, cause: ErrProviderURLInsecure},
		{name: "invalid key", config: Config{URL: "https://provider.invalid/chat", Model: "model", APIKey: "line\nbreak"}, cause: ErrProviderAPIKeyInvalid},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			provider, err := NewHTTPClient(test.config)
			if provider != nil || !errors.Is(err, test.cause) {
				t.Fatalf("NewHTTPClient() = (%v, %v), want cause %v", provider, err, test.cause)
			}
		})
	}
}

func TestNewHTTPClientAcceptsSecureAndLoopbackEndpoints(t *testing.T) {
	endpoints := []string{
		"https://provider.invalid/chat",
		"http://localhost:8080/chat",
		"http://127.0.0.1:8080/chat",
		"http://[::1]:8080/chat",
	}
	for _, endpoint := range endpoints {
		t.Run(endpoint, func(t *testing.T) {
			provider, err := NewHTTPClient(Config{URL: "  " + endpoint + "  ", Model: "  model  ", APIKey: "key"})
			if err != nil || provider == nil {
				t.Fatalf("NewHTTPClient() = (%v, %v)", provider, err)
			}
			if provider.url != endpoint || provider.model != "model" || provider.apiKey != "key" {
				t.Fatalf("provider = %+v", provider)
			}
			if provider.Name() != configuredClientName {
				t.Fatalf("Name() = %q", provider.Name())
			}
		})
	}
	if isLoopbackHost("192.0.2.1") || isLoopbackHost("provider.invalid") {
		t.Fatal("non-loopback host accepted")
	}
}

func TestDetectUsesNormalizedEnvironment(t *testing.T) {
	clearProviderEnvironment(t)
	provider, err := Detect()
	if err != nil || provider != nil {
		t.Fatalf("Detect(unconfigured) = (%v, %v)", provider, err)
	}

	t.Setenv(providerURLEnvironment, "https://provider.invalid/chat")
	t.Setenv(providerModelEnvironment, "model")
	t.Setenv(providerKeyEnvironment, "key")
	provider, err = Detect()
	if err != nil || provider == nil || provider.Name() != configuredClientName {
		t.Fatalf("Detect(configured) = (%v, %v)", provider, err)
	}
}
