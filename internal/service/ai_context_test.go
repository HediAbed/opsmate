package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestAIReadOnlyContextCommands_OnlyGetAndConfig(t *testing.T) {
	for _, cmd := range aiReadOnlyContextCommands("default") {
		if len(cmd.Args) == 0 {
			t.Fatalf("%s has no args", cmd.Title)
		}
		switch cmd.Args[0] {
		case "get":
		case "config":
			if len(cmd.Args) < 2 || cmd.Args[1] != "current-context" {
				t.Fatalf("unexpected config command: %v", cmd.Args)
			}
		default:
			t.Fatalf("automatic AI context command must be read-only get/config, got: %v", cmd.Args)
		}
		for _, arg := range cmd.Args {
			switch arg {
			case "delete", "apply", "patch", "edit", "scale", "rollout", "restart", "exec", "create":
				t.Fatalf("automatic AI context command includes mutating token %q: %v", arg, cmd.Args)
			}
		}
	}
}

func TestBuildAIClusterSearchContext_IncludesSnapshotsAndMatches(t *testing.T) {
	var calls []string
	var callsMu sync.Mutex
	runner := aiContextRunnerFunc(func(_ context.Context, _ time.Duration, args ...string) (string, error) {
		return recordAIContextFixtureCall(&callsMu, &calls, args)
	})

	clusterContext, err := buildAIClusterSearchContext(
		context.Background(), runner, "why is checkout-api crashing?", "ops",
	)
	if err != nil {
		t.Fatalf("build context: %v", err)
	}
	assertContainsAll(t, clusterContext, "read-only automatic Kubernetes search", "checkout-api", "CrashLoopBackOff", "Matching lines")
	assertNoMutatingContextCalls(t, calls)
}

func recordAIContextFixtureCall(callsMu *sync.Mutex, calls *[]string, args []string) (string, error) {
	command := strings.Join(args, " ")
	callsMu.Lock()
	*calls = append(*calls, command)
	callsMu.Unlock()
	switch {
	case command == "config current-context":
		return "kind-dev", nil
	case strings.Contains(command, "pods"):
		return "NAMESPACE NAME READY STATUS RESTARTS AGE\nops checkout-api 0/1 CrashLoopBackOff 7 3m\n", nil
	default:
		return "ok\n", nil
	}
}

func assertContainsAll(t *testing.T, value string, expected ...string) {
	t.Helper()
	for _, substring := range expected {
		if !strings.Contains(value, substring) {
			t.Errorf("value should contain %q, got %q", substring, value)
		}
	}
}

func assertNoMutatingContextCalls(t *testing.T, calls []string) {
	t.Helper()
	for _, call := range calls {
		if strings.Contains(call, "delete") || strings.Contains(call, "apply") || strings.Contains(call, "rollout") {
			t.Fatalf("unexpected mutating call: %s", call)
		}
	}
}

func TestBuildAIClusterSearchContext_RecordsReadErrors(t *testing.T) {
	runner := aiContextRunnerFunc(func(_ context.Context, _ time.Duration, args ...string) (string, error) {
		return "", fmt.Errorf("boom: %w", errors.New(strings.Join(args, " ")))
	})

	clusterContext, err := buildAIClusterSearchContext(context.Background(), runner, "what is wrong?", "")
	if err != nil {
		t.Fatalf("build context: %v", err)
	}
	if !strings.Contains(clusterContext, "error: boom") {
		t.Errorf("context should retain read errors for the model, got %q", clusterContext)
	}
}

func TestBuildAIClusterSearchContext_CancelsAllCommands(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	runner := aiContextRunnerFunc(func(ctx context.Context, _ time.Duration, _ ...string) (string, error) {
		<-ctx.Done()
		return "", ctx.Err()
	})
	_, err := buildAIClusterSearchContext(ctx, runner, "query", "default")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", err)
	}
}

func TestAISearchKeywords_DropsCommonWords(t *testing.T) {
	got := aiSearchKeywords("what do you see with checkout-api CrashLoopBackOff in namespace ops?")
	joined := strings.Join(got, ",")
	if strings.Contains(joined, "what") || strings.Contains(joined, "namespace") {
		t.Errorf("keywords should drop common words, got %v", got)
	}
	if !strings.Contains(joined, "checkout-api") || !strings.Contains(joined, "crashloopbackoff") {
		t.Errorf("keywords should keep resource/problem terms, got %v", got)
	}
}

func TestAnalyzeWithClusterSearch_UsesRawQuestionForSearchAndTypedPayload(t *testing.T) {
	const question = "why is checkout-api failing?"
	provider := &capturingContextProvider{response: "diagnosis"}
	runner := aiContextRunnerFunc(func(_ context.Context, _ time.Duration, _ ...string) (string, error) {
		return "checkout-api CrashLoopBackOff", nil
	})

	message := analyzeWithClusterSearch(
		provider,
		runner,
		"trusted instructions",
		question,
		"Earlier answer",
		"operations",
	)
	if message.Err != nil || message.Response != "diagnosis" {
		t.Fatalf("analysis message = %+v", message)
	}
	if strings.Contains(provider.systemPrompt, question) {
		t.Fatalf("question reached trusted instructions: %q", provider.systemPrompt)
	}
	var payload clusterAnalysisRequest
	if err := json.Unmarshal([]byte(provider.userMessage), &payload); err != nil {
		t.Fatalf("decode provider payload: %v", err)
	}
	if payload.Question != question || payload.ConversationMemory != "Earlier answer" {
		t.Fatalf("provider payload = %+v", payload)
	}
	if !strings.Contains(payload.ClusterContext, "Search keywords: checkout-api") {
		t.Fatalf("cluster search did not use the raw question: %q", payload.ClusterContext)
	}
}

func TestRenderAIClusterSearchContextBoundsTotalOutput(t *testing.T) {
	commands := []aiContextCommand{
		{Title: "Pods", Args: []string{"get", "pods"}},
		{Title: "Deployments", Args: []string{"get", "deployments"}},
		{Title: "Services", Args: []string{"get", "services"}},
	}
	longOutput := strings.Repeat("x", maxAIContextSectionRunes*2)
	results := []aiContextCommandResult{{Output: longOutput}, {Output: longOutput}, {Output: longOutput}}

	contextText := renderAIClusterSearchContext("pods", "default", commands, results)

	if count := len([]rune(contextText)); count > maxAIContextTotalRunes+3 {
		t.Fatalf("context length = %d, exceeds bounded output", count)
	}
}

func TestAIAnalyzeWithClusterSearchRequiresProvider(t *testing.T) {
	withCleanProvider(t)

	message := AIAnalyzeWithClusterSearch("system", "show health", "", "default")().(AnalysisMsg)

	if !errors.Is(message.Err, ErrProviderNotConfigured) {
		t.Fatalf("error = %v, want missing-provider classification", message.Err)
	}
}

func TestAnalyzeWithClusterSearchContextPropagatesCollectionFailure(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	runner := aiContextRunnerFunc(func(ctx context.Context, _ time.Duration, _ ...string) (string, error) {
		<-ctx.Done()
		return "", ctx.Err()
	})

	message := analyzeWithClusterSearchContext(
		ctx,
		&capturingContextProvider{response: "unused"},
		runner,
		"system",
		"question",
		"",
		"default",
	)

	if !errors.Is(message.Err, context.Canceled) {
		t.Fatalf("error = %v, want cancellation", message.Err)
	}
}

func TestAnalyzeWithClusterSearchRejectsProviderFailureAndEmptyResponse(t *testing.T) {
	runner := aiContextRunnerFunc(func(context.Context, time.Duration, ...string) (string, error) {
		return "healthy", nil
	})
	sentinel := errors.New("analysis failed")

	failed := analyzeWithClusterSearch(
		&configuredProvider{err: sentinel}, runner, "system", "question", "", "default",
	)
	if !errors.Is(failed.Err, sentinel) {
		t.Fatalf("failure = %v, want provider error", failed.Err)
	}

	empty := analyzeWithClusterSearch(
		&configuredProvider{response: " \n"}, runner, "system", "question", "", "default",
	)
	if !errors.Is(empty.Err, ErrProviderEmptyResponse) {
		t.Fatalf("failure = %v, want empty-response classification", empty.Err)
	}
}

func TestProviderCommandErrorPrefersDeadline(t *testing.T) {
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()
	sentinel := errors.New("provider failed")

	err := providerCommandError(ctx, "backend", "inspect", sentinel)

	if !errors.Is(err, context.DeadlineExceeded) || errors.Is(err, sentinel) {
		t.Fatalf("error = %v, want deadline classification", err)
	}
}

func TestAISearchKeywordsDeduplicatesAndCapsResults(t *testing.T) {
	query := "alpha alpha beta gamma delta epsilon zeta eta theta iota kappa lambda sigma"
	keywords := aiSearchKeywords(query)

	if len(keywords) != maxAISearchKeywords {
		t.Fatalf("keyword count = %d, want %d", len(keywords), maxAISearchKeywords)
	}
	if keywords[0] != "alpha" || keywords[1] != "beta" {
		t.Fatalf("keywords = %v, want stable unique ordering", keywords)
	}
}

func TestMatchingLinesHandlesEmptyInputAndLimit(t *testing.T) {
	if got := matchingLines("alpha", nil, 1); got != "" {
		t.Fatalf("empty keywords result = %q", got)
	}
	if got := matchingLines("alpha", []string{"alpha"}, 0); got != "" {
		t.Fatalf("zero limit result = %q", got)
	}

	got := matchingLines("alpha first\r\nalpha second\nalpha third", []string{"alpha"}, 2)
	if !strings.Contains(got, "alpha first") || !strings.Contains(got, "matches truncated") {
		t.Fatalf("limited matches = %q", got)
	}
}

type aiContextRunnerFunc func(context.Context, time.Duration, ...string) (string, error)

func (run aiContextRunnerFunc) RunText(ctx context.Context, timeout time.Duration, args ...string) (string, error) {
	return run(ctx, timeout, args...)
}

type capturingContextProvider struct {
	response     string
	systemPrompt string
	userMessage  string
}

func (*capturingContextProvider) Name() string {
	return "test provider"
}

func (provider *capturingContextProvider) Chat(
	_ context.Context,
	systemPrompt string,
	userMessage string,
) (string, error) {
	provider.systemPrompt = systemPrompt
	provider.userMessage = userMessage
	return provider.response, nil
}
