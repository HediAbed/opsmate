package browser

import (
	"testing"
)

func TestBrowserHelpBar_TextIsPrecomputedOnce(t *testing.T) {
	if browserHelpBarText == "" {
		t.Fatal("browserHelpBarText must be precomputed at package init")
	}
	first := browserHelpBarText
	second := buildBrowserHelpBarText()
	if first != second {
		t.Error("buildBrowserHelpBarText must be deterministic")
	}
}

func TestBrowserHelpBar_RenderIsDeterministicAtSameWidth(t *testing.T) {
	m := newTestBrowserModel("default")
	m.SetSize(120, 40)
	if first, second := m.renderHelpBar(), m.renderHelpBar(); first != second {
		t.Error("renderHelpBar must be deterministic across calls at the same width")
	}
}
