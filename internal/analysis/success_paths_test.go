package analysis

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/HediAbed/opsmate/internal/analysis/command"
	"github.com/HediAbed/opsmate/internal/analysis/provider"
)

func serviceWithTestHTTPProvider(t *testing.T, body string) Service {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)

	client, err := provider.NewHTTPClient(provider.Config{URL: srv.URL, Model: "model", APIKey: "key"})
	if err != nil {
		t.Fatalf("NewHTTPClient() error = %v", err)
	}
	return NewService(client)
}

func TestAnalysisAnalyze_SuccessPathReturnsResponse(t *testing.T) {
	service := serviceWithTestHTTPProvider(t, `{"choices":[{"message":{"content":"analysis result"}}]}`)
	msg := service.Analyze("sys", "user")().(AnalysisMsg)
	if msg.Err != nil {
		t.Fatalf("err: %v", msg.Err)
	}
	if !strings.Contains(msg.Response, "analysis result") {
		t.Errorf("response = %q", msg.Response)
	}
}

func TestAnalysisAnalyze_EmptyResponseIsError(t *testing.T) {
	service := serviceWithTestHTTPProvider(t, `{"choices":[{"message":{"content":""}}]}`)
	msg := service.Analyze("sys", "user")().(AnalysisMsg)
	if msg.Err == nil {
		t.Error("empty response should error")
	}
}

func TestAnalysisGenerateCommand_SuccessReturnsCommandAndExplanation(t *testing.T) {
	service := serviceWithTestHTTPProvider(t, `{"choices":[{"message":{"content":"kubectl get pods\nlists all pods"}}]}`)
	msg := service.GenerateCommand("list pods", "default")().(GeneratedCommandMsg)
	if msg.Err != nil {
		t.Fatalf("err: %v", msg.Err)
	}
	if !strings.Contains(msg.Command, "kubectl get pods") {
		t.Errorf("command = %q", msg.Command)
	}
	if !strings.Contains(msg.Command, "--namespace=default") {
		t.Errorf("generated command is not confined to the active namespace: %q", msg.Command)
	}
}

func TestAnalysisGenerateCommand_RejectsCommandsThatCannotExecute(t *testing.T) {
	tests := []struct {
		name     string
		response string
		wantErr  error
	}{
		{name: "mutating", response: "kubectl delete pod checkout", wantErr: command.ErrForbiddenCommand},
		{name: "malformed", response: "not a kubectl command", wantErr: command.ErrForbiddenCommand},
		{name: "alternate target", response: "kubectl get pods --server=https://other.example", wantErr: command.ErrForbiddenCommand},
		{name: "sensitive", response: "kubectl get secrets -o yaml", wantErr: command.ErrSensitiveDataCommand},
		{name: "different namespace", response: "kubectl get pods -n other", wantErr: command.ErrCommandScope},
		{name: "all namespaces", response: "kubectl get pods -A", wantErr: command.ErrCommandScope},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			service := serviceWithTestHTTPProvider(t, `{"choices":[{"message":{"content":"`+test.response+`"}}]}`)
			msg := service.GenerateCommand("inspect the cluster", "default")().(GeneratedCommandMsg)
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
	service := serviceWithTestHTTPProvider(t, `{"choices":[{"message":{"content":"this means the container OOMed"}}]}`)
	msg := service.ExplainLogLine("oom-killer", "context", "pod-x")().(LogExplanationMsg)
	if msg.Err != nil {
		t.Fatalf("err: %v", msg.Err)
	}
	if !strings.Contains(msg.Explanation, "OOMed") {
		t.Errorf("explanation = %q", msg.Explanation)
	}
}

func TestAnalysisClusterHealth_SuccessPath(t *testing.T) {
	service := serviceWithTestHTTPProvider(t, `{"choices":[{"message":{"content":"cluster looks healthy"}}]}`)
	msg := service.ClusterHealth("dashboard ctx")().(DashboardHealthMsg)
	if msg.Err != nil {
		t.Fatalf("err: %v", msg.Err)
	}
	if !strings.Contains(msg.Summary, "healthy") {
		t.Errorf("summary = %q", msg.Summary)
	}
}

func TestAnalysisDescribeSummary_SuccessPath(t *testing.T) {
	service := serviceWithTestHTTPProvider(t, `{"choices":[{"message":{"content":"pod is running and healthy"}}]}`)
	msg := service.DescribeSummary("pod", "alpha", "describe text")().(DescribeSummaryMsg)
	if msg.Err != nil {
		t.Fatalf("err: %v", msg.Err)
	}
	if !strings.Contains(msg.Summary, "running") {
		t.Errorf("summary = %q", msg.Summary)
	}
}

func TestAnalysisDescribeSummary_TruncatesLongInput(t *testing.T) {
	long := strings.Repeat("x", 5000)
	service := serviceWithTestHTTPProvider(t, `{"choices":[{"message":{"content":"ok"}}]}`)
	msg := service.DescribeSummary("pod", "alpha", long)().(DescribeSummaryMsg)
	if msg.Err != nil {
		t.Errorf("err: %v", msg.Err)
	}
}
