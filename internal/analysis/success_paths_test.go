package analysis

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func withTestHTTPProvider(t *testing.T, body string) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)

	prev := getActiveProvider()
	provider, err := NewHTTPProvider(ProviderConfig{URL: srv.URL, Model: "model", APIKey: "key"})
	if err != nil {
		t.Fatalf("NewHTTPProvider() error = %v", err)
	}
	provider.client = srv.Client()
	setActiveProvider(provider)
	t.Cleanup(func() { setActiveProvider(prev) })
}

func TestAnalysisAnalyze_SuccessPathReturnsResponse(t *testing.T) {
	withTestHTTPProvider(t, `{"choices":[{"message":{"content":"analysis result"}}]}`)
	msg := Analyze("sys", "user")().(AnalysisMsg)
	if msg.Err != nil {
		t.Fatalf("err: %v", msg.Err)
	}
	if !strings.Contains(msg.Response, "analysis result") {
		t.Errorf("response = %q", msg.Response)
	}
}

func TestAnalysisAnalyze_EmptyResponseIsError(t *testing.T) {
	withTestHTTPProvider(t, `{"choices":[{"message":{"content":""}}]}`)
	msg := Analyze("sys", "user")().(AnalysisMsg)
	if msg.Err == nil {
		t.Error("empty response should error")
	}
}

func TestAnalysisGenerateCommand_SuccessReturnsCommandAndExplanation(t *testing.T) {
	withTestHTTPProvider(t, `{"choices":[{"message":{"content":"kubectl get pods\nlists all pods"}}]}`)
	msg := GenerateCommand("list pods", "default")().(GeneratedCommandMsg)
	if msg.Err != nil {
		t.Fatalf("err: %v", msg.Err)
	}
	if !strings.Contains(msg.Command, "kubectl get pods") {
		t.Errorf("command = %q", msg.Command)
	}
	if !strings.Contains(msg.Command, "--namespace='default'") {
		t.Errorf("generated command is not confined to the active namespace: %q", msg.Command)
	}
}

func TestAnalysisGenerateCommand_RejectsCommandsThatCannotExecute(t *testing.T) {
	tests := []struct {
		name     string
		response string
		wantErr  error
	}{
		{name: "mutating", response: "kubectl delete pod checkout", wantErr: ErrForbiddenCommand},
		{name: "malformed", response: "not a kubectl command", wantErr: ErrForbiddenCommand},
		{name: "alternate target", response: "kubectl get pods --server=https://other.example", wantErr: ErrForbiddenCommand},
		{name: "sensitive", response: "kubectl get secrets -o yaml", wantErr: ErrSensitiveDataCommand},
		{name: "different namespace", response: "kubectl get pods -n other", wantErr: ErrCommandScope},
		{name: "all namespaces", response: "kubectl get pods -A", wantErr: ErrCommandScope},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			withTestHTTPProvider(t, `{"choices":[{"message":{"content":"`+test.response+`"}}]}`)
			msg := GenerateCommand("inspect the cluster", "default")().(GeneratedCommandMsg)
			if !errors.Is(msg.Err, test.wantErr) {
				t.Fatalf("error = %v, want %v", msg.Err, test.wantErr)
			}
			if msg.Command != "" {
				t.Fatalf("rejected command leaked into confirmation: %q", msg.Command)
			}
		})
	}
}

func TestAnalysisExplainLogLine_SuccessPath(t *testing.T) {
	withTestHTTPProvider(t, `{"choices":[{"message":{"content":"this means the container OOMed"}}]}`)
	msg := ExplainLogLine("oom-killer", "context", "pod-x")().(LogExplanationMsg)
	if msg.Err != nil {
		t.Fatalf("err: %v", msg.Err)
	}
	if !strings.Contains(msg.Explanation, "OOMed") {
		t.Errorf("explanation = %q", msg.Explanation)
	}
}

func TestAnalysisClusterHealth_SuccessPath(t *testing.T) {
	withTestHTTPProvider(t, `{"choices":[{"message":{"content":"cluster looks healthy"}}]}`)
	msg := ClusterHealth("dashboard ctx")().(DashboardHealthMsg)
	if msg.Err != nil {
		t.Fatalf("err: %v", msg.Err)
	}
	if !strings.Contains(msg.Summary, "healthy") {
		t.Errorf("summary = %q", msg.Summary)
	}
}

func TestAnalysisDescribeSummary_SuccessPath(t *testing.T) {
	withTestHTTPProvider(t, `{"choices":[{"message":{"content":"pod is running and healthy"}}]}`)
	msg := DescribeSummary("pod", "alpha", "describe text")().(DescribeSummaryMsg)
	if msg.Err != nil {
		t.Fatalf("err: %v", msg.Err)
	}
	if !strings.Contains(msg.Summary, "running") {
		t.Errorf("summary = %q", msg.Summary)
	}
}

func TestAnalysisDescribeSummary_TruncatesLongInput(t *testing.T) {
	long := strings.Repeat("x", 5000)
	withTestHTTPProvider(t, `{"choices":[{"message":{"content":"ok"}}]}`)
	msg := DescribeSummary("pod", "alpha", long)().(DescribeSummaryMsg)
	if msg.Err != nil {
		t.Errorf("err: %v", msg.Err)
	}
}
