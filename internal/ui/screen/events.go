package screen

import (
	"time"

	tea "charm.land/bubbletea/v2"
)

type ID uint8

const (
	Dashboard ID = iota
	Browser
	Logs
	Analysis
	Helm
	CRDs
)

type GoBackMsg struct{}

type ClearStatusMsg struct{}

type DrillDownMsg struct {
	Screen       ID
	ResourceType string
	ResourceName string
	ResourceNS   string
}

func ClearStatusAfter(delay time.Duration) tea.Cmd {
	return tea.Tick(delay, func(time.Time) tea.Msg {
		return ClearStatusMsg{}
	})
}
