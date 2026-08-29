package ui

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestCopyToClipboardDispatchesOSC52WithExactPayload(t *testing.T) {
	const payload = "secret-token-xyzzy"
	status, command := copyToClipboard(payload, "1 secret")
	if !strings.Contains(status, "1 secret") {
		t.Errorf("status must include the descriptor; got %q", status)
	}
	if strings.Contains(status, "Copied") {
		t.Errorf("status must not claim 'Copied' because OSC 52 acceptance cannot be verified; got %q", status)
	}
	if !strings.Contains(status, "OSC 52") {
		t.Errorf("status must indicate the OSC 52 mechanism; got %q", status)
	}

	if command == nil {
		t.Fatal("command must be non-nil")
	}
	message := command()
	batch, ok := message.(tea.BatchMsg)
	if !ok {
		t.Fatalf("command must produce tea.BatchMsg; got %T", message)
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
