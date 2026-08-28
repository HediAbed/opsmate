package model

import (
	"testing"

	"charm.land/bubbles/v2/table"
	tea "charm.land/bubbletea/v2"
)

func TestBrowser_EscapeWithoutClearableStateGoesBack(t *testing.T) {
	m := NewBrowserModel("default")
	m.SetSize(120, 40)
	m.state = stateBrowsing
	m.errBanner = ""
	m.selected = nil

	_, cmd := m.handleBrowsingKey("esc", tea.KeyPressMsg{})
	if cmd == nil {
		t.Fatal("ESC with no clearable state must emit a command")
	}
	if _, ok := cmd().(GoBackMsg); !ok {
		t.Errorf("ESC must emit GoBackMsg, got %T", cmd())
	}
}

func TestBrowser_EscapeClearsSelectionFirst(t *testing.T) {
	m := NewBrowserModel("default")
	m.SetSize(120, 40)
	m.errBanner = "boom"
	m.selected = map[string]bool{"alpha": true}

	m2, cmd := m.handleBrowsingKey("esc", tea.KeyPressMsg{})
	if cmd != nil {
		t.Error("ESC with selected items must NOT emit GoBackMsg yet")
	}
	if len(m2.selected) != 0 {
		t.Error("ESC must clear selection first")
	}
	if m2.errBanner == "" {
		t.Error("ESC must NOT clear the err banner while selection is being cleared")
	}
}

func TestBrowser_EscapeClearsErrBannerFirst(t *testing.T) {
	m := NewBrowserModel("default")
	m.SetSize(120, 40)
	m.errBanner = "boom"

	m2, cmd := m.handleBrowsingKey("esc", tea.KeyPressMsg{})
	if cmd != nil {
		t.Error("ESC with errBanner must not emit GoBackMsg until banner is cleared")
	}
	if m2.errBanner != "" {
		t.Error("ESC must clear the err banner first")
	}
}

func TestDashboard_PodTableTopBoundary(t *testing.T) {
	m := NewDashboardModel("default")
	m.SetSize(120, 40)
	got := m.podTableTopBoundary()
	want := 40 - dashHelpBarRows - (m.podTable.Height() + 2)
	if got != want {
		t.Errorf("podTableTopBoundary=%d want=%d", got, want)
	}
}

func TestDashboard_MouseClickInTableAreaSelectsRow(t *testing.T) {
	m := NewDashboardModel("default")
	m.SetSize(120, 40)
	m.podTable.SetRows(rowsForTest(5))

	topY := m.podTableTopBoundary()
	clickY := topY + dashTableHeaderRows + 2
	m2, _ := m.Update(tea.MouseClickMsg{X: 10, Y: clickY, Button: tea.MouseLeft})
	if got := m2.podTable.Cursor(); got != 2 {
		t.Errorf("click on row 2 should set cursor=2, got %d", got)
	}
}

func TestDashboard_MouseClickAboveTableIsIgnored(t *testing.T) {
	m := NewDashboardModel("default")
	m.SetSize(120, 40)
	m.podTable.SetRows(rowsForTest(5))
	m.podTable.SetCursor(0)

	m2, _ := m.Update(tea.MouseClickMsg{X: 10, Y: 1, Button: tea.MouseLeft})
	if got := m2.podTable.Cursor(); got != 0 {
		t.Errorf("click above table area should leave cursor at 0, got %d", got)
	}
}

func TestDashboard_NonLeftMouseClickIgnored(t *testing.T) {
	m := NewDashboardModel("default")
	m.SetSize(120, 40)
	m.podTable.SetRows(rowsForTest(5))
	m.podTable.SetCursor(0)

	clickY := m.podTableTopBoundary() + dashTableHeaderRows + 2
	for _, btn := range []tea.MouseButton{tea.MouseRight, tea.MouseMiddle} {
		m2, _ := m.Update(tea.MouseClickMsg{X: 10, Y: clickY, Button: btn})
		if got := m2.podTable.Cursor(); got != 0 {
			t.Errorf("button=%v should not move cursor; got %d", btn, got)
		}
	}
}

func rowsForTest(n int) []table.Row {
	rows := make([]table.Row, n)
	for i := range rows {
		rows[i] = table.Row{"pod-" + itoa(i), "Running", "1/1", "0", "1m", "10m", "20Mi"}
	}
	return rows
}
