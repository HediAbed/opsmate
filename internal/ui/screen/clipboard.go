package screen

import (
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/HediAbed/opsmate/internal/ui/theme"
)

const copyStatusDuration = 3 * time.Second

func CopyToClipboard(text, descriptor string) (string, tea.Cmd) {
	status := theme.Success.Render("Sent " + descriptor + " to clipboard (OSC 52)")
	command := tea.Batch(tea.SetClipboard(text), ClearStatusAfter(copyStatusDuration))
	return status, command
}
