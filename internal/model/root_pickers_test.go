package model

import (
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/HediAbed/opsmate/internal/service"
)

func TestRootHandleNSPicker_EnterFirstSelectsAllNamespaces(t *testing.T) {
	m := freshRoot(t)
	m.showNSPicker = true
	m.namespaces = []string{"default", "kube-system"}
	m.nsCursor = 0
	model, _ := m.handleNSPicker("enter")
	r := model.(RootModel)
	if r.showNSPicker {
		t.Error("enter should close picker")
	}
	if r.namespace != "" {
		t.Errorf("cursor 0 (All Namespaces) should set ns=''; got %q", r.namespace)
	}
}

func TestRootHandleNSPicker_EnterSpecificNamespace(t *testing.T) {
	m := freshRoot(t)
	m.showNSPicker = true
	m.namespaces = []string{"default", "kube-system"}
	m.nsCursor = 2
	model, _ := m.handleNSPicker("enter")
	r := model.(RootModel)
	if r.namespace != "kube-system" {
		t.Errorf("cursor 2 should select kube-system; got %q", r.namespace)
	}
}

func TestRootHandleNSPicker_KAndJVimNavigation(t *testing.T) {
	m := freshRoot(t)
	m.showNSPicker = true
	m.namespaces = []string{"default", "ns2", "ns3"}
	m.nsCursor = 1

	model, _ := m.handleNSPicker("k")
	if model.(RootModel).nsCursor != 0 {
		t.Error("k (vim up) should retract cursor")
	}

	m.nsCursor = 0
	model, _ = m.handleNSPicker("j")
	if model.(RootModel).nsCursor != 1 {
		t.Error("j (vim down) should advance cursor")
	}
}

func TestRootHandleNSPickerMouse_ClickSelects(t *testing.T) {
	m := freshRoot(t)
	m.showNSPicker = true
	m.namespaces = []string{"default", "ns2"}
	contentHeight := m.height - 1
	maxVisible := contentHeight - 6
	if maxVisible < 5 {
		maxVisible = 5
	}
	visibleCount := minInt(maxVisible, len(m.namespaces)+1)
	itemStart := (contentHeight-(visibleCount+6))/2 + 3

	model, _ := m.handleNSPickerMouse(tea.MouseClickMsg{
		X: 5, Y: itemStart, Button: tea.MouseLeft,
	})
	r := model.(RootModel)
	if r.showNSPicker {
		t.Error("click on row should close picker")
	}
}

func TestRootHandleNSPickerMouse_WheelDownAdvancesCursor(t *testing.T) {
	m := freshRoot(t)
	m.showNSPicker = true
	m.namespaces = []string{"a", "b", "c"}
	m.nsCursor = 0
	model, _ := m.handleNSPickerMouse(tea.MouseWheelMsg{Button: tea.MouseWheelDown})
	r := model.(RootModel)
	if r.nsCursor != 1 {
		t.Errorf("wheel down should advance; got %d", r.nsCursor)
	}
}

func TestRootHandleNSPickerMouse_WheelUpRetractsCursor(t *testing.T) {
	m := freshRoot(t)
	m.showNSPicker = true
	m.namespaces = []string{"a", "b", "c"}
	m.nsCursor = 2
	model, _ := m.handleNSPickerMouse(tea.MouseWheelMsg{Button: tea.MouseWheelUp})
	r := model.(RootModel)
	if r.nsCursor != 1 {
		t.Errorf("wheel up should retract; got %d", r.nsCursor)
	}
}

func TestRootHandleNSPickerMouse_RightClickIgnored(t *testing.T) {
	m := freshRoot(t)
	m.showNSPicker = true
	m.namespaces = []string{"a"}
	model, _ := m.handleNSPickerMouse(tea.MouseClickMsg{X: 5, Y: 10, Button: tea.MouseRight})
	r := model.(RootModel)
	if !r.showNSPicker {
		t.Error("right click should not affect the picker")
	}
}

func TestRootHandleCtxPickerMouse_WheelDownAdvancesCursor(t *testing.T) {
	m := freshRoot(t)
	m.showCtxPicker = true
	m.contexts = []service.KubeContext{{Name: "a"}, {Name: "b"}, {Name: "c"}}
	m.ctxCursor = 0
	model, _ := m.handleCtxPickerMouse(tea.MouseWheelMsg{Button: tea.MouseWheelDown})
	r := model.(RootModel)
	if r.ctxCursor != 1 {
		t.Errorf("ctx wheel down should advance; got %d", r.ctxCursor)
	}
}

func TestRootHandleCtxPicker_EnterSwitchesContext(t *testing.T) {
	m := freshRoot(t)
	m.showCtxPicker = true
	m.contexts = []service.KubeContext{{Name: "a"}, {Name: "b"}}
	m.ctxCursor = 1
	model, cmd := m.handleCtxPicker("enter")
	r := model.(RootModel)
	if r.showCtxPicker {
		t.Error("enter should close ctx picker")
	}
	if cmd == nil {
		t.Error("enter should return ctx switch cmd")
	}
}

func TestRootRenderContent_EachScreenRenders(t *testing.T) {
	for _, s := range []screenID{ScreenDashboard, ScreenBrowser, ScreenLogs, ScreenAI, ScreenHelm, ScreenCRDs} {
		m := freshRoot(t)
		m.screen = s
		out := m.renderContent()
		if out == "" {
			t.Errorf("renderContent for screen %d should produce non-empty output", s)
		}
	}
}

func TestRootRenderContent_AIVisibleSplitsLayout(t *testing.T) {
	m := freshRoot(t)
	m.aiPanel.SetVisible(true)
	out := m.renderContent()
	if out == "" {
		t.Error("renderContent with AI panel visible should still produce output")
	}
}

func TestRootRenderContent_CmdPaletteOverlay(t *testing.T) {
	m := freshRoot(t)
	m.showCmdPalette = true
	out := m.renderContent()
	if out == "" {
		t.Error("renderContent with cmd palette should produce output")
	}
}

func TestRootHandleCtxPicker_NavAndEsc(t *testing.T) {
	m := freshRoot(t)
	m.showCtxPicker = true
	m.contexts = []service.KubeContext{{Name: "a"}, {Name: "b"}}
	m.ctxCursor = 0

	model, _ := m.handleCtxPicker("down")
	if model.(RootModel).ctxCursor != 1 {
		t.Error("down should advance ctx cursor")
	}

	model, _ = model.(RootModel).handleCtxPicker("up")
	if model.(RootModel).ctxCursor != 0 {
		t.Error("up should retract ctx cursor")
	}

	model, _ = model.(RootModel).handleCtxPicker("esc")
	if model.(RootModel).showCtxPicker {
		t.Error("esc should close ctx picker")
	}
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
