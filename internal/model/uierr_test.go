package model

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestKubectlActionErr_PrefixesActionAndSanitizesStderr(t *testing.T) {
	raw := "aws: [ERROR]: Token has expired and refresh failed\n" +
		"E0508 15:20:26.553933 4052334 memcache.go:265] noise"
	got := kubectlActionErr("describe", errors.New(raw))
	if !strings.HasPrefix(got, "describe: ") {
		t.Errorf("expected 'describe:' prefix, got %q", got)
	}
	if strings.Contains(got, "memcache.go:265") {
		t.Errorf("klog noise should be stripped by SanitizeKubectlStderr, got %q", got)
	}
}

func TestBatchAllNamespacesErr_BuildsConsistentMessage(t *testing.T) {
	for _, action := range []string{"restart", "delete", "scale"} {
		got := batchAllNamespacesErr(action)
		if !strings.Contains(got, action) {
			t.Errorf("action %q must appear in message; got %q", action, got)
		}
		if !strings.Contains(got, "all-namespaces mode") {
			t.Errorf("message must explain the constraint; got %q", got)
		}
	}
}

func TestShellPodPhaseErr_IncludesNameAndStatus(t *testing.T) {
	got := shellPodPhaseErr("web-app-7d8f9", "Pending")
	if !strings.Contains(got, "web-app-7d8f9") {
		t.Errorf("pod name missing; got %q", got)
	}
	if !strings.Contains(got, "Pending") {
		t.Errorf("status missing; got %q", got)
	}
}

func TestAIErr_PrefixesAndContainsUnderlying(t *testing.T) {
	got := aiErr(errors.New("rate limit exceeded"))
	if !strings.HasPrefix(got, "AI Error: ") {
		t.Errorf("expected 'AI Error:' prefix; got %q", got)
	}
	if !strings.Contains(got, "rate limit exceeded") {
		t.Errorf("underlying error must be included; got %q", got)
	}
}

func TestCopyToClipboard_DispatchesOSC52WithExactPayload(t *testing.T) {
	const payload = "secret-token-xyzzy"
	status, cmd := copyToClipboard(payload, "1 secret")
	if !strings.Contains(status, "1 secret") {
		t.Errorf("status must include the descriptor; got %q", status)
	}
	if strings.Contains(status, "Copied") {
		t.Errorf("status must NOT claim 'Copied' (we cannot verify OSC 52 acceptance); got %q", status)
	}
	if !strings.Contains(status, "OSC 52") {
		t.Errorf("status must indicate the OSC 52 mechanism for honesty; got %q", status)
	}

	if cmd == nil {
		t.Fatal("cmd must be non-nil")
	}
	message := cmd()
	batch, ok := message.(tea.BatchMsg)
	if !ok {
		t.Fatalf("cmd must produce tea.BatchMsg; got %T", message)
	}
	if !batchContainsPayload(batch, payload) {
		t.Errorf("the batch must include a SetClipboard message carrying %q exactly", payload)
	}
}

func batchContainsPayload(batch tea.BatchMsg, payload string) bool {
	for _, child := range batch {
		if child == nil {
			continue
		}
		if fmt.Sprintf("%v", child()) == payload {
			return true
		}
	}
	return false
}
