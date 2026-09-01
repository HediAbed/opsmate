package browser

import (
	"regexp"
	"strings"
	"testing"

	"github.com/HediAbed/opsmate/internal/cluster"
)

var ansiTestStripper = regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]`)

func stripAnsiForTest(s string) string { return ansiTestStripper.ReplaceAllString(s, "") }

func TestTableFirstRowY_TracksRenderedTitleBarHeight(t *testing.T) {
	m := newBrowserWithMarkerPod(t, "default")
	wantY := findMarkerRowY(t, m.View())
	if got := m.tableFirstRowY(); got != wantY {
		t.Errorf("tableFirstRowY = %d, want %d (rendered Y of the first row)", got, wantY)
	}
}

func TestTableFirstRowY_AccountsForErrorBanner(t *testing.T) {
	m := newBrowserWithMarkerPod(t, "default")
	m.errBanner = "kubectl: connection refused"

	wantY := findMarkerRowY(t, m.View())
	if got := m.tableFirstRowY(); got != wantY {
		t.Errorf("with err banner: tableFirstRowY = %d, want %d", got, wantY)
	}
}

func TestTableFirstRowY_AccountsForFilterBar(t *testing.T) {
	m := newBrowserWithMarkerPod(t, "default")
	m.state = stateFilter
	m.filterActive = true
	m.filterText = "ma"

	wantY := findMarkerRowY(t, m.View())
	if got := m.tableFirstRowY(); got != wantY {
		t.Errorf("with filter bar: tableFirstRowY = %d, want %d", got, wantY)
	}
}

func newBrowserWithMarkerPod(t *testing.T, ns string) BrowserModel {
	t.Helper()
	m := newTestBrowserModel(ns)
	m.SetSize(200, 40)
	m.pods = []cluster.Pod{{Name: "marker", Status: "Running", Ready: "1/1", Age: "1m", Node: "n1"}}
	m.loading = false
	m.rebuildTable()
	return m
}

func findMarkerRowY(t *testing.T, rendered string) int {
	t.Helper()
	plain := stripAnsiForTest(rendered)
	for i, line := range strings.Split(plain, "\n") {
		if strings.Contains(line, "marker") {
			return i
		}
	}
	t.Fatal("could not locate marker row in rendered view")
	return -1
}

type browserClickCase struct {
	kind     string
	rowNames []string
	setup    func(*BrowserModel)
}

func TestBrowserClick_RowSelection(t *testing.T) {
	for _, test := range browserClickCases() {
		t.Run(test.kind, func(t *testing.T) {
			assertBrowserRowClicks(t, test)
		})
	}
}

func browserClickCases() []browserClickCase {
	return []browserClickCase{
		{
			kind:     "pods",
			rowNames: []string{"pod-A", "pod-B", "pod-C"},
			setup: func(m *BrowserModel) {
				m.pods = []cluster.Pod{
					{Name: "pod-A", Status: "Running", Ready: "1/1", Age: "1m", Node: "n1"},
					{Name: "pod-B", Status: "Running", Ready: "1/1", Age: "2m", Node: "n2"},
					{Name: "pod-C", Status: "Running", Ready: "1/1", Age: "3m", Node: "n3"},
				}
			},
		},
		{
			kind:     "deployments",
			rowNames: []string{"dep-A", "dep-B"},
			setup: func(m *BrowserModel) {
				m.deployments = []cluster.Deployment{
					{Name: "dep-A", Ready: "1/1", UpToDate: 1, Available: 1, Age: "1m"},
					{Name: "dep-B", Ready: "1/1", UpToDate: 1, Available: 1, Age: "2m"},
				}
			},
		},
	}
}

func assertBrowserRowClicks(t *testing.T, test browserClickCase) {
	t.Helper()
	model := newTestBrowserModel("default")
	model.SetSize(200, 40)
	model.resourceType = test.kind
	test.setup(&model)
	model.loading = false
	model.rebuildTable()

	rowPositions := renderedRowPositions(model.View(), test.rowNames)
	if len(rowPositions) != len(test.rowNames) {
		t.Fatalf("found %d of %d rendered rows: %v", len(rowPositions), len(test.rowNames), rowPositions)
	}
	for _, rowName := range test.rowNames {
		assertBrowserRowClick(t, model, rowName, rowPositions[rowName])
	}
}

func renderedRowPositions(view string, rowNames []string) map[string]int {
	positions := make(map[string]int, len(rowNames))
	for rowIndex, line := range strings.Split(stripAnsiForTest(view), "\n") {
		trimmed := strings.TrimSpace(strings.Trim(line, "│ "))
		for _, rowName := range rowNames {
			if strings.HasPrefix(trimmed, rowName) {
				positions[rowName] = rowIndex
			}
		}
	}
	return positions
}

func assertBrowserRowClick(t *testing.T, model BrowserModel, rowName string, rowY int) {
	t.Helper()
	const tableColumnX = 50
	updated, _ := model.handleBrowseClick(tableColumnX, rowY)
	selectedRow := updated.resourceTable.SelectedRow()
	if len(selectedRow) == 0 {
		t.Fatalf("click on %q at Y=%d returned no row", rowName, rowY)
	}
	if !strings.HasPrefix(selectedRow[0], rowName) {
		t.Errorf("click on %q at Y=%d selected %q", rowName, rowY, selectedRow[0])
	}
}
