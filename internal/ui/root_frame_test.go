package ui

import (
	"testing"

	"charm.land/lipgloss/v2"
)

const (
	frameProbeWidth  = 120
	frameProbeHeight = 40
)

func TestRootFillsEveryRowWithoutACommandPalette(t *testing.T) {
	model := freshRoot(t)
	model.width, model.height = frameProbeWidth, frameProbeHeight
	model.resizeChildren()

	if model.showCmdPalette {
		t.Fatal("setup: the command palette should be closed")
	}
	if got := lipgloss.Height(model.renderContent()); got != frameProbeHeight {
		t.Fatalf("root rendered %d rows, want %d: the closed command palette still consumed one", got, frameProbeHeight)
	}
}

func TestRootStillMakesRoomForAnOpenCommandPalette(t *testing.T) {
	model := freshRoot(t)
	model.width, model.height = frameProbeWidth, frameProbeHeight
	model.showCmdPalette = true
	model.resizeChildren()

	if got := lipgloss.Height(model.renderContent()); got > frameProbeHeight {
		t.Fatalf("root rendered %d rows with the palette open, exceeding %d", got, frameProbeHeight)
	}
}
