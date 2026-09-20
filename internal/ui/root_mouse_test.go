package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestMouseCaptureTogglesSoTerminalSelectionWorks(t *testing.T) {
	model := freshRoot(t)
	if model.mouseMode() != tea.MouseModeCellMotion {
		t.Fatalf("default mouse mode = %v, want cell motion", model.mouseMode())
	}

	released, command := model.Update(tea.KeyPressMsg{Code: 'M', Text: "M"})
	root := released.(RootModel)
	if command != nil {
		t.Fatalf("toggle returned command %v, want none", command)
	}
	if root.mouseMode() != tea.MouseModeNone {
		t.Fatalf("released mouse mode = %v, want none", root.mouseMode())
	}
	if !strings.Contains(root.notice, "drag to select text") {
		t.Fatalf("notice = %q, want the terminal-selection hint", root.notice)
	}
	if root.View().MouseMode != tea.MouseModeNone {
		t.Fatal("view did not stop capturing the mouse")
	}

	recaptured, _ := root.Update(tea.KeyPressMsg{Code: 'M', Text: "M"})
	restored := recaptured.(RootModel)
	if restored.mouseMode() != tea.MouseModeCellMotion {
		t.Fatalf("recaptured mouse mode = %v, want cell motion", restored.mouseMode())
	}
	if restored.notice != mouseCapturedNotice {
		t.Fatalf("notice = %q, want %q", restored.notice, mouseCapturedNotice)
	}
}
