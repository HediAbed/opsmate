package analysis

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	analysisprovider "github.com/HediAbed/opsmate/internal/analysis/provider"
	"github.com/HediAbed/opsmate/internal/failure"
)

type clusterContextProvider struct {
	name         string
	response     string
	err          error
	systemPrompt string
	userMessage  string
}

func (p *clusterContextProvider) Name() string {
	return p.name
}

func (p *clusterContextProvider) Chat(_ context.Context, systemPrompt, userMessage string) (string, error) {
	p.systemPrompt = systemPrompt
	p.userMessage = userMessage
	return p.response, p.err
}

func TestAnalyzeClusterContextUsesTypedUntrustedPayload(t *testing.T) {
	client := &clusterContextProvider{name: "test", response: "cluster is healthy"}
	message := analyzeClusterContextWithProvider(
		context.Background(),
		client,
		"system rules",
		"is the cluster healthy?",
		"previous answer",
		"pod status",
	)
	if message.Err != nil || message.Response != "cluster is healthy" {
		t.Fatalf("analyzeClusterContext() = %+v", message)
	}
	if !strings.Contains(client.systemPrompt, "read-only Kubernetes API calls") {
		t.Fatalf("system prompt = %q", client.systemPrompt)
	}
	var payload clusterAnalysisRequest
	if err := json.Unmarshal([]byte(client.userMessage), &payload); err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	want := clusterAnalysisRequest{
		Question:           "is the cluster healthy?",
		ClusterContext:     "pod status",
		ConversationMemory: "previous answer",
	}
	if payload != want {
		t.Fatalf("payload = %+v, want %+v", payload, want)
	}
}

func TestAnalyzeClusterContextRejectsInvalidProviderResults(t *testing.T) {
	sentinel := errors.New("provider failed")
	tests := []struct {
		name     string
		ctx      context.Context
		provider analysisprovider.Client
		cause    error
	}{
		{name: "missing context", provider: &clusterContextProvider{name: "test"}, cause: analysisprovider.ErrProviderContextRequired},
		{name: "provider failure", ctx: context.Background(), provider: &clusterContextProvider{name: "test", err: sentinel}, cause: sentinel},
		{name: "empty response", ctx: context.Background(), provider: &clusterContextProvider{name: "test", response: "  "}, cause: analysisprovider.ErrProviderEmptyResponse},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			message := analyzeClusterContextWithProvider(test.ctx, test.provider, "system", "question", "", "snapshot")
			if !errors.Is(message.Err, test.cause) {
				t.Fatalf("analyzeClusterContext() error = %v, want cause %v", message.Err, test.cause)
			}
		})
	}
}

func TestAnalyzeClusterContextReportsEncodingFailure(t *testing.T) {
	sentinel := errors.New("encode failed")
	client := &clusterContextProvider{name: "test"}
	message := analyzeClusterContextWithEncoder(
		context.Background(),
		client,
		"system",
		"question",
		"memory",
		"snapshot",
		func(clusterAnalysisRequest) ([]byte, error) { return nil, sentinel },
	)
	if !errors.Is(message.Err, sentinel) {
		t.Fatalf("error = %v, want encoding failure", message.Err)
	}
	var providerErr *analysisprovider.Error
	if !errors.As(message.Err, &providerErr) || providerErr.Operation != failure.OperationEncode {
		t.Fatalf("error = %#v, want encode ProviderError", message.Err)
	}
}

func TestAnalyzeClusterContextUsesConfiguredProvider(t *testing.T) {
	disabled := NewService(nil)
	if message := disabled.AnalyzeClusterContext(context.Background(), "system", "question", "", "snapshot"); !errors.Is(message.Err, analysisprovider.ErrProviderNotConfigured) {
		t.Fatalf("AnalyzeClusterContext() error = %v, want missing provider", message.Err)
	}

	configured := NewService(&clusterContextProvider{name: "test", response: "answer"})
	message := configured.AnalyzeClusterContext(context.Background(), "system", "question", "", "snapshot")
	if message.Err != nil || message.Response != "answer" {
		t.Fatalf("AnalyzeClusterContext() = %+v", message)
	}
}

func TestAnalyzeClusterContextBoundsSnapshot(t *testing.T) {
	client := &clusterContextProvider{name: "test", response: "answer"}
	message := analyzeClusterContextWithProvider(
		context.Background(),
		client,
		"system",
		"question",
		"",
		strings.Repeat("界", maximumContextRunes+10),
	)
	if message.Err != nil {
		t.Fatalf("analyzeClusterContext() error = %v", message.Err)
	}
	var payload clusterAnalysisRequest
	if err := json.Unmarshal([]byte(client.userMessage), &payload); err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	if len([]rune(payload.ClusterContext)) != maximumContextRunes {
		t.Fatalf("cluster context length = %d, want %d", len([]rune(payload.ClusterContext)), maximumContextRunes)
	}
}

func TestProviderCommandErrorPrefersDeadline(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	sentinel := errors.New("failed")
	if err := providerCommandError(ctx, "test", failure.OperationAnalyze, sentinel); !errors.Is(err, sentinel) {
		t.Fatalf("providerCommandError(canceled) = %v, want sentinel", err)
	}

	expired, expiredCancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer expiredCancel()
	if err := providerCommandError(expired, "test", failure.OperationAnalyze, sentinel); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("providerCommandError(expired) = %v, want deadline", err)
	}
}

func TestAnalysisContextTextHelpers(t *testing.T) {
	if got := limitContextText("  short  ", 10); got != "short" {
		t.Fatalf("limitAnalysisContextText(short) = %q", got)
	}
	if got := limitContextText("long", 0); got != "" {
		t.Fatalf("limitAnalysisContextText(zero) = %q", got)
	}
	if got := limitContextText("long", 2); got != "lo" {
		t.Fatalf("limitAnalysisContextText(two) = %q", got)
	}
	if got := quoteUntrustedData("line\nvalue"); got != `UNTRUSTED_CLUSTER_DATA="line\nvalue"` {
		t.Fatalf("quoteUntrustedData() = %q", got)
	}
}
