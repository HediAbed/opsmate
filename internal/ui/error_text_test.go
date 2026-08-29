package ui

import (
	"errors"
	"strings"
	"testing"
)

func TestOperationErrorTextPrefixesActionAndSanitizesControls(t *testing.T) {
	raw := "request failed\x1b[31m\nretry later"
	got := operationErrorText("describe", errors.New(raw))
	if !strings.HasPrefix(got, "describe: ") {
		t.Errorf("expected 'describe:' prefix, got %q", got)
	}
	if strings.Contains(got, "\x1b") || strings.Contains(got, "\n") {
		t.Errorf("terminal controls should be removed, got %q", got)
	}
}

func TestOperationErrorTextHandlesMissingCause(t *testing.T) {
	if got := operationErrorText("inspect", nil); got != "inspect: unknown error" {
		t.Fatalf("operationErrorText(nil) = %q", got)
	}
}

func TestBatchAllNamespacesErrorTextBuildsConsistentMessage(t *testing.T) {
	for _, action := range []string{"restart", "delete", "scale"} {
		got := batchAllNamespacesErrorText(action)
		if !strings.Contains(got, action) {
			t.Errorf("action %q must appear in message; got %q", action, got)
		}
		if !strings.Contains(got, "all-namespaces mode") {
			t.Errorf("message must explain the constraint; got %q", got)
		}
	}
}

func TestShellPodPhaseErrorTextIncludesNameAndStatus(t *testing.T) {
	got := shellPodPhaseErrorText("web-app-7d8f9", "Pending")
	if !strings.Contains(got, "web-app-7d8f9") {
		t.Errorf("pod name missing; got %q", got)
	}
	if !strings.Contains(got, "Pending") {
		t.Errorf("status missing; got %q", got)
	}
}

func TestAnalysisErrorTextPrefixesAndContainsCause(t *testing.T) {
	got := analysisErrorText(errors.New("rate limit exceeded"))
	if !strings.HasPrefix(got, "Analysis error: ") {
		t.Errorf("expected 'Analysis error:' prefix; got %q", got)
	}
	if !strings.Contains(got, "rate limit exceeded") {
		t.Errorf("underlying error must be included; got %q", got)
	}
}
