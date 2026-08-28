package model

import (
	"fmt"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/HediAbed/opsmate/internal/service"
	"github.com/HediAbed/opsmate/internal/theme"
)

const (
	uiErrShellNamespaceRequired = "shell: namespace required (select an explicit namespace first)"
)

func kubectlActionErr(action string, err error) string {
	return sanitizeTerminalLine(action + ": " + service.SanitizeKubectlStderr(err.Error()))
}

func batchAllNamespacesErr(action string) string {
	return "batch " + action + " is not supported in all-namespaces mode — pick one namespace first"
}

func shellPodPhaseErr(name, status string) string {
	return sanitizeTerminalLine(fmt.Sprintf("shell: pod %q is in phase %q — can only shell into Running pods", name, status))
}

const copyStatusClearAfter = 3 * time.Second

func copyToClipboard(text, descriptor string) (string, tea.Cmd) {
	banner := theme.Success.Render("Sent " + descriptor + " to clipboard (OSC 52)")
	cmd := tea.Batch(tea.SetClipboard(text), clearStatusAfter(copyStatusClearAfter))
	return banner, cmd
}

func aiErr(err error) string {
	return "AI Error: " + sanitizeTerminalLine(err.Error())
}
