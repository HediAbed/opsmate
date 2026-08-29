package ui

import (
	"testing"

	"github.com/HediAbed/opsmate/internal/kube"
)

func TestPFKill_FirstXAsksForConfirmation(t *testing.T) {
	m := newTestRootModel(t, "default")
	m.showPFModal = true
	m.pfSessions = []kube.PortForwardSession{testModelPortForwardSession(t, "s1", "web", 8080, 80)}
	m.pfCursor = 0

	out, cmd := m.handlePFModalKey("x")
	got, ok := out.(RootModel)
	if !ok {
		t.Fatalf("handlePFModalKey returned %T, want RootModel", out)
	}
	if cmd != nil {
		t.Error("first x must not invoke StopPortForward; only stage a confirmation")
	}
	if got.pfConfirmKillID != "s1" {
		t.Errorf("pfConfirmKillID=%q, want %q", got.pfConfirmKillID, "s1")
	}
	if got.pfConfirmKillOf == "" {
		t.Error("pfConfirmKillOf should describe the staged session")
	}
}

func TestPFKill_YConfirmsAndStops(t *testing.T) {
	m := newTestRootModel(t, "default")
	m.showPFModal = true
	m.pfSessions = []kube.PortForwardSession{testModelPortForwardSession(t, "s1", "web", 8080, 80)}
	m.pfConfirmKillID = "s1"
	m.pfConfirmKillOf = "web (8080:80)"

	out, cmd := m.handlePFModalKey("y")
	got := out.(RootModel)
	if cmd == nil {
		t.Error("confirming with y must dispatch StopPortForward")
	}
	if got.pfConfirmKillID != "" || got.pfConfirmKillOf != "" {
		t.Error("confirming must clear the staged kill")
	}
}

func TestPFKill_NCancels(t *testing.T) {
	m := newTestRootModel(t, "default")
	m.showPFModal = true
	m.pfConfirmKillID = "s1"
	m.pfConfirmKillOf = "web (8080:80)"

	out, cmd := m.handlePFModalKey("n")
	got := out.(RootModel)
	if cmd != nil {
		t.Error("cancelling with n must not dispatch any command")
	}
	if got.pfConfirmKillID != "" {
		t.Error("cancelling must clear the staged kill")
	}
}

func TestPFKill_EscCancelsConfirmation(t *testing.T) {
	m := newTestRootModel(t, "default")
	m.showPFModal = true
	m.pfConfirmKillID = "s1"
	m.pfConfirmKillOf = "web"

	out, _ := m.handlePFModalKey("esc")
	got := out.(RootModel)
	if got.pfConfirmKillID != "" {
		t.Error("esc during confirm must clear the staged kill (NOT close the modal)")
	}
	if !got.showPFModal {
		t.Error("esc during confirm must NOT close the modal")
	}
}
