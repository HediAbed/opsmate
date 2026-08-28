package service

import (
	"context"
	"errors"
	"testing"
	"time"
)

type configuredProvider struct {
	response string
	err      error
	wait     bool
}

func (*configuredProvider) Name() string { return "configured" }

func (provider *configuredProvider) Chat(ctx context.Context, _, _ string) (string, error) {
	if provider.wait {
		<-ctx.Done()
		return "", ctx.Err()
	}
	return provider.response, provider.err
}

func installConfiguredProvider(t *testing.T, provider AIProvider) {
	t.Helper()
	previous := getActiveProvider()
	setActiveProvider(provider)
	t.Cleanup(func() { setActiveProvider(previous) })
}

func TestChatWithTimeoutClassifiesFailures(t *testing.T) {
	sentinel := errors.New("request failed")
	_, err := chatWithTimeout(&configuredProvider{err: sentinel}, time.Second, "inspect", "system", "user")
	if !errors.Is(err, sentinel) {
		t.Fatalf("error = %v, want request failure", err)
	}

	_, err = chatWithTimeout(&configuredProvider{wait: true}, time.Millisecond, "inspect", "system", "user")
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("error = %v, want deadline exceeded", err)
	}

	_, err = chatWithTimeout(&configuredProvider{response: " \n "}, time.Second, "inspect", "system", "user")
	if !errors.Is(err, ErrProviderEmptyResponse) {
		t.Fatalf("error = %v, want empty-response classification", err)
	}
}

func TestProviderCommandsPropagateProviderFailure(t *testing.T) {
	sentinel := errors.New("provider unavailable")
	installConfiguredProvider(t, &configuredProvider{err: sentinel})

	results := []error{
		AIAnalyze("system", "question")().(AnalysisMsg).Err,
		AIGenerateCommand("show pods", "default")().(GeneratedCommandMsg).Err,
		AIExplainLogLine("line", "context", "pod")().(LogExplainMsg).Err,
		AIClusterHealth("context")().(DashHealthMsg).Err,
		AIDescribeSummary("pod", "web", "details")().(DescribeSummaryMsg).Err,
	}
	for index, err := range results {
		if !errors.Is(err, sentinel) {
			t.Errorf("result %d error = %v, want provider failure", index, err)
		}
	}
}

func TestProviderCommandsRejectEmptyProviderResponses(t *testing.T) {
	installConfiguredProvider(t, &configuredProvider{response: ""})

	results := []error{
		AIAnalyze("system", "question")().(AnalysisMsg).Err,
		AIGenerateCommand("show pods", "default")().(GeneratedCommandMsg).Err,
		AIExplainLogLine("line", "context", "pod")().(LogExplainMsg).Err,
		AIClusterHealth("context")().(DashHealthMsg).Err,
		AIDescribeSummary("pod", "web", "details")().(DescribeSummaryMsg).Err,
	}
	for index, err := range results {
		if !errors.Is(err, ErrProviderEmptyResponse) {
			t.Errorf("result %d error = %v, want empty-response classification", index, err)
		}
	}
}

func TestAIAnalyzeStreamFallsBackForNonStreamingProvider(t *testing.T) {
	installConfiguredProvider(t, &configuredProvider{response: "answer"})

	start, events, cancel := AIAnalyzeStream("system", "question")
	defer cancel()
	if events != nil {
		t.Fatal("fallback provider unexpectedly returned a stream")
	}
	message := start().(AnalysisMsg)
	if message.Err != nil || message.Response != "answer" {
		t.Fatalf("analysis = %#v, want successful fallback", message)
	}
}
