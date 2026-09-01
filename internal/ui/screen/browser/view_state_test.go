package browser

import (
	"testing"

	"github.com/HediAbed/opsmate/internal/cluster"
)

func TestBrowserViewDoesNotChangeRetainedWidgetGeometry(t *testing.T) {
	m := newTestBrowserModel("payments")
	m.SetSize(120, 30)
	m.pods = []cluster.Pod{{Name: "api", Namespace: "payments", Status: "Running"}}
	m.rebuildTable()
	m.showDetail = true
	m.state = stateDetail
	m.detailContent = "name: api"
	m.rebuildDetailContent()
	m.syncBrowserLayout()

	tableWidth, tableHeight := m.resourceTable.Width(), m.resourceTable.Height()
	detailWidth, detailHeight := m.detailView.Width(), m.detailView.Height()
	_ = m.View()
	if m.resourceTable.Width() != tableWidth || m.resourceTable.Height() != tableHeight {
		t.Fatalf("View changed table geometry from %dx%d to %dx%d", tableWidth, tableHeight, m.resourceTable.Width(), m.resourceTable.Height())
	}
	if m.detailView.Width() != detailWidth || m.detailView.Height() != detailHeight {
		t.Fatalf("View changed detail geometry from %dx%d to %dx%d", detailWidth, detailHeight, m.detailView.Width(), m.detailView.Height())
	}
}
