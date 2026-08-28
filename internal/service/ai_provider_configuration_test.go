package service

import (
	"errors"
	"testing"
)

func withCleanProvider(t *testing.T) {
	t.Helper()
	prev := getActiveProvider()
	setActiveProvider(nil)
	t.Cleanup(func() { setActiveProvider(prev) })
}

func TestProviderName_NoneWhenNotConfigured(t *testing.T) {
	withCleanProvider(t)
	if got := ProviderName(); got != "None" {
		t.Errorf("ProviderName with no provider = %q, want None", got)
	}
}

func TestHasAIProvider_FalseWhenUnconfigured(t *testing.T) {
	withCleanProvider(t)
	if HasAIProvider() {
		t.Error("HasAIProvider should be false when none configured")
	}
}

func TestInitAIProvider_AllowsNoConfiguredProvider(t *testing.T) {
	withCleanProvider(t)
	t.Setenv("GEMINI_API_KEY", "")
	t.Setenv("OLLAMA_MODEL", "")
	t.Setenv("OLLAMA_API_URL", "")
	t.Setenv("OLLAMA_ENABLED", "")
	t.Setenv("MOONSHOT_API_KEY", "")
	t.Setenv("CLAUDE_CLI", "")
	if err := InitAIProvider(); err != nil {
		t.Fatalf("initialize unconfigured provider: %v", err)
	}
	if HasAIProvider() {
		t.Fatal("provider must remain unset")
	}
}

func TestDetectProvider_GeminiKeyTakesPrecedence(t *testing.T) {
	t.Setenv("GEMINI_API_KEY", "test-key")
	t.Setenv("OLLAMA_MODEL", "gemma4:e4b")
	t.Setenv("MOONSHOT_API_KEY", "should-be-ignored")
	p, err := DetectProvider()
	if err != nil {
		t.Fatalf("detect provider: %v", err)
	}
	if p == nil || p.Name() != "Gemini" {
		t.Errorf("Gemini should win when both keys set; got %v", p)
	}
}

func TestDetectProvider_OllamaUsedWhenGeminiAbsent(t *testing.T) {
	t.Setenv("GEMINI_API_KEY", "")
	t.Setenv("OLLAMA_MODEL", "gemma4:e4b")
	t.Setenv("MOONSHOT_API_KEY", "should-be-ignored")
	t.Setenv("CLAUDE_CLI", "")
	p, err := DetectProvider()
	if err != nil {
		t.Fatalf("detect provider: %v", err)
	}
	if p == nil || p.Name() != "Ollama" {
		t.Errorf("Ollama should be selected; got %v", p)
	}
}

func TestDetectProvider_KimiUsedWhenGeminiAbsent(t *testing.T) {
	t.Setenv("GEMINI_API_KEY", "")
	t.Setenv("OLLAMA_MODEL", "")
	t.Setenv("OLLAMA_API_URL", "")
	t.Setenv("OLLAMA_ENABLED", "")
	t.Setenv("MOONSHOT_API_KEY", "test-key")
	t.Setenv("CLAUDE_CLI", "")
	p, err := DetectProvider()
	if err != nil {
		t.Fatalf("detect provider: %v", err)
	}
	if p == nil || p.Name() != "Kimi" {
		t.Errorf("Kimi should be selected; got %v", p)
	}
}

func TestDetectProvider_NoneWhenAllAbsent(t *testing.T) {
	t.Setenv("GEMINI_API_KEY", "")
	t.Setenv("OLLAMA_MODEL", "")
	t.Setenv("OLLAMA_API_URL", "")
	t.Setenv("OLLAMA_ENABLED", "")
	t.Setenv("MOONSHOT_API_KEY", "")
	t.Setenv("CLAUDE_CLI", "")
	p, err := DetectProvider()
	if err != nil {
		t.Fatalf("detect provider: %v", err)
	}
	if p != nil {
		t.Errorf("expected nil provider; got %v", p)
	}
}

func TestDetectProvider_ReportsMissingExplicitCLI(t *testing.T) {
	t.Setenv("GEMINI_API_KEY", "")
	t.Setenv("OLLAMA_MODEL", "")
	t.Setenv("OLLAMA_API_URL", "")
	t.Setenv("OLLAMA_ENABLED", "")
	t.Setenv("MOONSHOT_API_KEY", "")
	t.Setenv("CLAUDE_CLI", "1")
	t.Setenv("PATH", t.TempDir())
	provider, err := DetectProvider()
	if provider != nil || err == nil {
		t.Fatalf("result = (%v, %v), want typed configuration error", provider, err)
	}
	var providerErr *ProviderError
	if !errors.As(err, &providerErr) || providerErr.Operation != "configure" {
		t.Fatalf("error = %#v, want configure ProviderError", err)
	}
}

func TestInitAIProviderClearsProviderAfterDetectionFailure(t *testing.T) {
	previous := getActiveProvider()
	setActiveProvider(&configuredProvider{response: "configured"})
	t.Cleanup(func() { setActiveProvider(previous) })
	t.Setenv("GEMINI_API_KEY", "")
	t.Setenv("OLLAMA_MODEL", "")
	t.Setenv("OLLAMA_API_URL", "")
	t.Setenv("OLLAMA_ENABLED", "")
	t.Setenv("MOONSHOT_API_KEY", "")
	t.Setenv("CLAUDE_CLI", "1")
	t.Setenv("PATH", t.TempDir())

	if err := InitAIProvider(); err == nil {
		t.Fatal("missing configured executable did not fail")
	}
	if HasAIProvider() {
		t.Fatal("provider remained active after initialization failure")
	}
}

func TestNewGeminiProvider_HasNameAndKey(t *testing.T) {
	t.Setenv("GEMINI_MODEL", "")
	p := NewGeminiProvider("k")
	if p.Name() != "Gemini" {
		t.Error("Gemini name")
	}
	if p.apiKey != "k" {
		t.Error("apiKey not stored")
	}
	if p.model != defaultGeminiModel {
		t.Fatalf("default model = %q, want %q", p.model, defaultGeminiModel)
	}
}

func TestNewGeminiProvider_UsesConfiguredModel(t *testing.T) {
	t.Setenv("GEMINI_MODEL", "custom-model")
	provider := NewGeminiProvider("key")
	if provider.model != "custom-model" {
		t.Fatalf("model = %q, want custom-model", provider.model)
	}
}

func TestNewKimiProvider_HasName(t *testing.T) {
	t.Setenv("KIMI_MODEL", "")
	p := NewKimiProvider("k")
	if p.Name() != "Kimi" {
		t.Error("Kimi name")
	}
	if p.model != "kimi-k2.6" {
		t.Fatalf("default Kimi model = %q, want kimi-k2.6", p.model)
	}
}

func TestNewOllamaProvider_DefaultsToGemma(t *testing.T) {
	t.Setenv("OLLAMA_MODEL", "")
	t.Setenv("OLLAMA_API_URL", "")
	p := NewOllamaProvider()
	if p.Name() != "Ollama" {
		t.Error("Ollama name")
	}
	if p.model != "gemma4:e4b" {
		t.Errorf("default Ollama model = %q, want gemma4:e4b", p.model)
	}
	if p.apiURL != "http://127.0.0.1:11434/v1/chat/completions" {
		t.Errorf("default Ollama API URL = %q", p.apiURL)
	}
}

func TestNewClaudeCLIProvider_HasName(t *testing.T) {
	p := NewClaudeCLIProvider()
	if p.Name() != "Claude CLI" {
		t.Errorf("Claude CLI name; got %q", p.Name())
	}
}
