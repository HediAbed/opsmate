package analysis

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/HediAbed/opsmate/internal/analysis/provider"
	"github.com/HediAbed/opsmate/internal/failure"
)

type configuredProvider struct {
	response string
	err      error
	wait     bool
}

func (*configuredProvider) Name() string { return "configured" }

func (client *configuredProvider) Chat(ctx context.Context, _, _ string) (string, error) {
	if client.wait {
		<-ctx.Done()
		return "", ctx.Err()
	}
	return client.response, client.err
}

func TestChatWithTimeoutClassifiesFailures(t *testing.T) {
	sentinel := errors.New("request failed")
	_, err := chatWithTimeout(
		&configuredProvider{err: sentinel},
		time.Second,
		failure.OperationAnalyze,
		"system",
		"user",
	)
	if !errors.Is(err, sentinel) {
		t.Fatalf("error = %v, want request failure", err)
	}

	_, err = chatWithTimeout(
		&configuredProvider{wait: true},
		time.Millisecond,
		failure.OperationAnalyze,
		"system",
		"user",
	)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("error = %v, want deadline exceeded", err)
	}

	_, err = chatWithTimeout(
		&configuredProvider{response: " \n "},
		time.Second,
		failure.OperationAnalyze,
		"system",
		"user",
	)
	if !errors.Is(err, provider.ErrProviderEmptyResponse) {
		t.Fatalf("error = %v, want empty-response classification", err)
	}
}

func TestProviderCommandsPropagateProviderFailure(t *testing.T) {
	sentinel := errors.New("provider unavailable")
	service := NewService(&configuredProvider{err: sentinel})

	results := []error{
		service.Analyze("system", "question")().(AnalysisMsg).Err,
		service.GenerateCommand("show pods", "default")().(GeneratedCommandMsg).Err,
		service.ExplainLogLine("line", "context", "pod")().(LogExplanationMsg).Err,
		service.ClusterHealth("context")().(DashboardHealthMsg).Err,
		service.DescribeSummary("pod", "web", "details")().(DescribeSummaryMsg).Err,
	}
	for index, err := range results {
		if !errors.Is(err, sentinel) {
			t.Errorf("result %d error = %v, want provider failure", index, err)
		}
	}
}

func TestProviderCommandsRejectEmptyProviderResponses(t *testing.T) {
	service := NewService(&configuredProvider{response: ""})

	results := []error{
		service.Analyze("system", "question")().(AnalysisMsg).Err,
		service.GenerateCommand("show pods", "default")().(GeneratedCommandMsg).Err,
		service.ExplainLogLine("line", "context", "pod")().(LogExplanationMsg).Err,
		service.ClusterHealth("context")().(DashboardHealthMsg).Err,
		service.DescribeSummary("pod", "web", "details")().(DescribeSummaryMsg).Err,
	}
	for index, err := range results {
		if !errors.Is(err, provider.ErrProviderEmptyResponse) {
			t.Errorf("result %d error = %v, want empty-response classification", index, err)
		}
	}
}

func TestAnalysisAnalyzeStreamFallsBackForNonStreamingProvider(t *testing.T) {
	service := NewService(&configuredProvider{response: "answer"})

	start, events, cancel := service.AnalyzeStream("system", "question")
	defer cancel()
	if events != nil {
		t.Fatal("fallback provider unexpectedly returned a stream")
	}
	message := start().(AnalysisMsg)
	if message.Err != nil || message.Response != "answer" {
		t.Fatalf("analysis = %#v, want successful fallback", message)
	}
}
