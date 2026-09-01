package browser

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/HediAbed/opsmate/internal/cluster"
)

func TestRenderShellSplit_TableColumnsFitInHalfWidth(t *testing.T) {
	m := newTestBrowserModel("default")
	m.SetSize(230, 40)
	m.resourceType = "pods"
	m.pods = []cluster.Pod{
		{Name: "alpha", Namespace: "default", Status: "Running", Ready: "1/1", Restarts: 0, Age: "1m", Node: "n1"},
		{Name: "beta", Namespace: "default", Status: "Running", Ready: "1/1", Restarts: 0, Age: "1m", Node: "n2"},
	}
	m.rebuildTable()
	m.state = stateShell
	m.syncShellLayout(30)

	rendered := m.renderShellSplit(30)

	for _, line := range strings.Split(rendered, "\n") {
		if w := lipgloss.Width(line); w > 230 {
			t.Errorf("rendered line exceeds terminal width 230: w=%d line=%q", w, line)
			break
		}
	}
}

func TestRenderHSplitContent_TableColumnsFitInHalfWidth(t *testing.T) {
	m := newTestBrowserModel("default")
	m.SetSize(230, 40)
	m.resourceType = "pods"
	m.pods = []cluster.Pod{
		{Name: "alpha", Namespace: "default", Status: "Running", Ready: "1/1", Restarts: 0, Age: "1m", Node: "n1"},
	}
	m.rebuildTable()
	m.detailContent = "kind: Pod\nname: alpha"
	m.showDetail = true

	rendered := m.renderHSplitContent(30)

	for _, line := range strings.Split(rendered, "\n") {
		if w := lipgloss.Width(line); w > 230 {
			t.Errorf("rendered line exceeds terminal width 230: w=%d line=%q", w, line)
			break
		}
	}
}
