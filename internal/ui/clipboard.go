package ui

import (
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/HediAbed/opsmate/internal/theme"
)

const copyStatusDuration = 3 * time.Second

func copyToClipboard(text, descriptor string) (string, tea.Cmd) {
	banner := theme.Success.Render("Sent " + descriptor + " to clipboard (OSC 52)")
	command := tea.Batch(tea.SetClipboard(text), clearStatusAfter(copyStatusDuration))
	return banner, command
}
