package browser

import (
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/HediAbed/opsmate/internal/ui/screen"
)

func TestBrowserEscapeWithoutClearableStateGoesBack(t *testing.T) {
	model := newTestBrowserModel("default")
	model.SetSize(120, 40)
	model.state = stateBrowsing
	model.errBanner = ""
	model.selected = nil

	_, command := model.handleBrowsingKey("esc", tea.KeyPressMsg{})
	if command == nil {
		t.Fatal("escape without clearable state returned no command")
	}
	if _, ok := command().(screen.GoBackMsg); !ok {
		t.Errorf("escape message = %T, want screen.GoBackMsg", command())
	}
}

func TestBrowserEscapeClearsSelectionFirst(t *testing.T) {
	model := newTestBrowserModel("default")
	model.SetSize(120, 40)
	model.errBanner = "boom"
	model.selected = map[string]bool{"alpha": true}

	updated, command := model.handleBrowsingKey("esc", tea.KeyPressMsg{})
	if command != nil {
		t.Fatal("escape with a selection returned a command")
	}
	if len(updated.selected) != 0 {
		t.Fatal("escape did not clear the selection")
	}
	if updated.errBanner == "" {
		t.Fatal("escape cleared the error before the selection")
	}
}

func TestBrowserEscapeClearsErrorBannerFirst(t *testing.T) {
	model := newTestBrowserModel("default")
	model.SetSize(120, 40)
	model.errBanner = "boom"

	updated, command := model.handleBrowsingKey("esc", tea.KeyPressMsg{})
	if command != nil {
		t.Fatal("escape with an error returned a command")
	}
	if updated.errBanner != "" {
		t.Fatal("escape did not clear the error")
	}
}
