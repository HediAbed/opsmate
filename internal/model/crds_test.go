package model

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/HediAbed/opsmate/internal/service"
)

func TestCRDsModel_CRDsMsg_PopulatesListView(t *testing.T) {
	m := NewCRDsModel("ns")
	m.SetSize(160, 40)

	out, _ := m.Update(service.CRDsMsg{CRDs: []service.CRD{
		{Name: "certificates.cert-manager.io", Group: "cert-manager.io", Plural: "certificates", Kind: "Certificate", Scope: "Namespaced", Versions: []string{"v1"}, Resource: "certificates.cert-manager.io", Age: "1d"},
	}})
	if out.loading {
		t.Error("loading should clear after fetch")
	}
	if len(out.crds) != 1 {
		t.Fatalf("expected 1 crd, got %d", len(out.crds))
	}
	rows := out.crdsTable.Rows()
	if rows[0][0] != "certificates.cert-manager.io" || rows[0][2] != "Namespaced" {
		t.Errorf("row wiring wrong: %+v", rows[0])
	}
	if rows[0][3] != "v1" {
		t.Errorf("versions should render via JoinVersions; got %q", rows[0][3])
	}
}

func TestCRDsModel_EnterDrillsIntoInstanceView(t *testing.T) {
	m := NewCRDsModel("ns")
	m.SetSize(160, 40)
	m.crds = []service.CRD{
		{Name: "certificates.cert-manager.io", Resource: "certificates.cert-manager.io", Kind: "Certificate"},
	}
	m.crdsTable.SetRows(m.crdsRows())
	m.crdsTable.SetCursor(0)
	m.loading = false

	out, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter, Text: "enter"})
	if out.view != crdsViewInstances {
		t.Error("enter should swap to instance view")
	}
	if out.selectedCRD.Resource != "certificates.cert-manager.io" {
		t.Errorf("selected CRD not pinned; got %+v", out.selectedCRD)
	}
	if !out.loading {
		t.Error("drilldown should show spinner during fetch")
	}
	if cmd == nil {
		t.Error("drilldown should return a fetch cmd")
	}
}

func TestCRDsModel_EnterOnEmptyTableIsNoOp(t *testing.T) {
	m := NewCRDsModel("ns")
	m.SetSize(160, 40)
	m.loading = false

	out, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter, Text: "enter"})
	if out.view != crdsViewList {
		t.Error("enter on empty table must not change view")
	}
}

func TestCRDsModel_EscReturnsToListView(t *testing.T) {
	m := NewCRDsModel("ns")
	m.SetSize(160, 40)
	m.view = crdsViewInstances
	m.selectedCRD = service.CRD{Name: "x.example.com", Resource: "x.example.com"}
	m.instances = []service.CRDInstance{{Name: "i"}}

	out, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEsc, Text: "esc"})
	if out.view != crdsViewList {
		t.Error("esc should walk back to list view")
	}
	if out.selectedCRD.Name != "" {
		t.Error("esc should clear selectedCRD so the breadcrumb doesn't lie")
	}
	if out.instances != nil {
		t.Error("esc should clear cached instances so the next drilldown isn't stale")
	}
}

func TestCRDsModel_StaleInstancesMsg_DroppedAfterCRDSwapped(t *testing.T) {
	m := NewCRDsModel("ns")
	m.SetSize(160, 40)
	m.view = crdsViewInstances
	m.selectedCRD = service.CRD{Resource: "current.example.com"}
	m.loading = true

	stale := service.CRDInstancesMsg{
		Resource:  "old.example.com",
		Namespace: "ns",
		Instances: []service.CRDInstance{{Name: "stale"}},
	}
	out, _ := m.Update(stale)
	if !out.loading {
		t.Error("stale msg should not clear loading")
	}
	if len(out.instances) != 0 {
		t.Errorf("stale instances must be dropped; got %+v", out.instances)
	}
}

func TestCRDsModel_StaleInstancesMsg_DroppedAfterNamespaceSwapped(t *testing.T) {
	m := NewCRDsModel("ns-current")
	m.SetSize(160, 40)
	m.view = crdsViewInstances
	m.selectedCRD = service.CRD{Resource: "x.example.com"}
	m.loading = true

	stale := service.CRDInstancesMsg{
		Resource:  "x.example.com",
		Namespace: "ns-old",
		Instances: []service.CRDInstance{{Name: "stale"}},
	}
	out, _ := m.Update(stale)
	if !out.loading {
		t.Error("stale-ns msg should not clear loading")
	}
	if len(out.instances) != 0 {
		t.Error("stale-ns instances must be dropped")
	}
}

func TestCRDsModel_InstancesMsgPopulatesTable(t *testing.T) {
	m := NewCRDsModel("ns")
	m.SetSize(160, 40)
	m.view = crdsViewInstances
	m.selectedCRD = service.CRD{Resource: "x.example.com", Kind: "X"}

	out, _ := m.Update(service.CRDInstancesMsg{
		Resource:  "x.example.com",
		Namespace: "ns",
		Instances: []service.CRDInstance{
			{Name: "a", Namespace: "ns1", Age: "1d"},
			{Name: "b", Namespace: "ns2", Age: "2h"},
		},
	})
	if out.loading {
		t.Error("loading should clear")
	}
	if len(out.instances) != 2 {
		t.Errorf("expected 2 instances; got %d", len(out.instances))
	}
}

func TestCRDsModel_RKeyRefetchesCurrentView(t *testing.T) {
	m := NewCRDsModel("ns")
	m.SetSize(160, 40)
	m.loading = false

	out, cmd := m.Update(tea.KeyPressMsg{Code: 'r', Text: "r"})
	if !out.loading || cmd == nil {
		t.Error("'r' on list view must set loading + return a fetch cmd")
	}

	m.view = crdsViewInstances
	m.selectedCRD = service.CRD{Resource: "x.example.com"}
	m.loading = false
	out, cmd = m.Update(tea.KeyPressMsg{Code: 'r', Text: "r"})
	if !out.loading || cmd == nil {
		t.Error("'r' on instance view must set loading + return a fetch cmd")
	}
}

func TestCRDsModel_SetNamespace_SameValueIsNoOp(t *testing.T) {
	m := NewCRDsModel("ns")
	m.loading = false
	if cmd := m.SetNamespace("ns"); cmd != nil {
		t.Error("same-namespace SetNamespace must not return a cmd")
	}
}

func TestCRDsModel_SetNamespace_DifferentValueReturnsFetchCmd(t *testing.T) {
	m := NewCRDsModel("ns-a")
	m.loading = false
	cmd := m.SetNamespace("ns-b")
	if cmd == nil {
		t.Error("namespace switch must return a fetch cmd")
	}
	if !m.loading {
		t.Error("namespace switch must set loading=true")
	}
}

func TestCRDsModel_View_EmptyListShowsFriendlyMessage(t *testing.T) {
	m := NewCRDsModel("ns")
	m.SetSize(160, 40)
	m.loading = false

	view := stripAnsiForTest(m.View())
	if !strings.Contains(view, "No CRDs installed") {
		t.Errorf("empty list should show friendly placeholder; view:\n%s", view)
	}
}

func TestCRDsModel_View_EmptyInstancesShowsScopedMessage(t *testing.T) {
	m := NewCRDsModel("ns")
	m.SetSize(160, 40)
	m.view = crdsViewInstances
	m.selectedCRD = service.CRD{Kind: "Certificate"}
	m.loading = false

	view := stripAnsiForTest(m.View())
	if !strings.Contains(view, "No Certificate instances") {
		t.Errorf("empty-instances message should mention the Kind; view:\n%s", view)
	}
}

func TestCRDsModel_HasInputFocus_AlwaysFalseToday(t *testing.T) {
	m := NewCRDsModel("ns")
	if m.HasInputFocus() {
		t.Error("CRDsModel has no inputs today; HasInputFocus must be false")
	}
}

func TestCRDsModel_Init_ReturnsNonNilFetchCmd(t *testing.T) {
	m := NewCRDsModel("ns")
	if cmd := m.Init(); cmd == nil {
		t.Error("Init must return a non-nil cmd so the framework triggers the first fetch")
	}
}

func TestCRDsModel_Activate_SetsLoadingAndReturnsFetchCmd(t *testing.T) {
	m := NewCRDsModel("ns")
	m.SetSize(160, 40)
	m.loading = false

	cmd := m.Activate()
	if !m.loading {
		t.Error("Activate must set loading=true to show the spinner during refetch")
	}
	if cmd == nil {
		t.Error("Activate must return a non-nil refresh cmd")
	}
}

func TestCRDsModel_Activate_FetchesInstancesWhenInInstanceView(t *testing.T) {
	m := NewCRDsModel("ns")
	m.SetSize(160, 40)
	m.view = crdsViewInstances
	m.selectedCRD = service.CRD{Resource: "x.example.com"}

	if cmd := m.Activate(); cmd == nil {
		t.Error("Activate in instance view must still return a fetch cmd")
	}
}

func TestCRDsModel_Deactivate_IsIdempotent(_ *testing.T) {
	m := NewCRDsModel("ns")
	m.Deactivate()
	m.Deactivate()
}

func TestCRDsModel_SelectedCRDName_FromCursorOrDrilldown(t *testing.T) {
	m := NewCRDsModel("ns")
	m.SetSize(160, 40)

	if got := m.SelectedCRDName(); got != "" {
		t.Errorf("empty model SelectedCRDName = %q, want empty", got)
	}

	m.crds = []service.CRD{{Name: "a.example.com"}, {Name: "b.example.com"}}
	m.crdsTable.SetRows(m.crdsRows())
	m.crdsTable.SetCursor(1)
	if got := m.SelectedCRDName(); got != "b.example.com" {
		t.Errorf("list view SelectedCRDName = %q, want b.example.com", got)
	}

	m.view = crdsViewInstances
	m.selectedCRD = service.CRD{Name: "drilled.example.com"}
	if got := m.SelectedCRDName(); got != "drilled.example.com" {
		t.Errorf("instance view SelectedCRDName = %q, want drilled.example.com", got)
	}
}

func TestCRDsModel_CRDsMsg_ErrorClearsRowsAndSetsBanner(t *testing.T) {
	m := NewCRDsModel("ns")
	m.SetSize(160, 40)
	m.crds = []service.CRD{{Name: "stale.example.com"}}
	m.crdsTable.SetRows(m.crdsRows())
	m.loading = true

	out, _ := m.Update(service.CRDsMsg{Err: errStub("boom")})
	if out.err == nil {
		t.Error("error must populate m.err for the banner")
	}
	if len(out.crds) != 0 || len(out.crdsTable.Rows()) != 0 {
		t.Error("error must clear rows so users don't read stale data under an error banner")
	}
	if out.loading {
		t.Error("error must clear loading")
	}
}

func TestCRDsModel_CRDInstancesMsg_ErrorClearsInstancesAndSetsBanner(t *testing.T) {
	m := NewCRDsModel("ns")
	m.SetSize(160, 40)
	m.view = crdsViewInstances
	m.selectedCRD = service.CRD{Resource: "x.example.com"}
	m.instances = []service.CRDInstance{{Name: "stale"}}
	m.instanceTable.SetRows(m.instanceRows())
	m.loading = true

	out, _ := m.Update(service.CRDInstancesMsg{Resource: "x.example.com", Namespace: "ns", Err: errStub("boom")})
	if out.err == nil {
		t.Error("instance error must populate m.err")
	}
	if len(out.instances) != 0 {
		t.Error("instance error must clear cached instances")
	}
}

func TestCRDsModel_EnterInInstanceViewIsNoOp(t *testing.T) {
	m := NewCRDsModel("ns")
	m.SetSize(160, 40)
	m.view = crdsViewInstances
	m.selectedCRD = service.CRD{Resource: "x.example.com"}
	m.instances = []service.CRDInstance{{Name: "i"}}

	out, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter, Text: "enter"})
	if out.view != crdsViewInstances {
		t.Error("enter while in instance view must not navigate (reserved for future row details)")
	}
}

func TestCRDsModel_SpinnerTick_AdvancesAndReturnsCmd(t *testing.T) {
	m := NewCRDsModel("ns")
	_, cmd := m.Update(m.spinner.Tick())
	if cmd == nil {
		t.Error("spinner.TickMsg must produce the next tick cmd")
	}
}

func TestCRDsHandleKey_DefaultForwardsToInstanceTable(_ *testing.T) {
	m := NewCRDsModel("ns")
	m.SetSize(160, 40)
	m.view = crdsViewInstances
	m.selectedCRD = service.CRD{Resource: "x.example.com"}
	m.instances = []service.CRDInstance{{Name: "a"}, {Name: "b"}}
	m.instanceTable.SetRows(m.instanceRows())
	out, _ := m.handleKey(tea.KeyPressMsg{Code: tea.KeyDown, Text: "down"})
	_ = out
}

func TestCRDsHandleKey_DefaultForwardsToCRDTable(_ *testing.T) {
	m := NewCRDsModel("ns")
	m.SetSize(160, 40)
	m.crds = []service.CRD{{Name: "a.example.com"}, {Name: "b.example.com"}}
	m.crdsTable.SetRows(m.crdsRows())
	out, _ := m.handleKey(tea.KeyPressMsg{Code: tea.KeyDown, Text: "down"})
	_ = out
}

func TestCRDsModel_AIOverlayBounds_AccountsForChromeAndClampsToMin(t *testing.T) {
	m := NewCRDsModel("ns")
	m.SetSize(160, 40)
	topOff, panelH, bottomOff := m.AIOverlayBounds(40)
	if topOff <= 0 || bottomOff <= 0 {
		t.Errorf("chrome offsets must be positive; topOff=%d bottomOff=%d", topOff, bottomOff)
	}
	if panelH != 40-topOff-bottomOff {
		t.Errorf("panelHeight = %d; want %d", panelH, 40-topOff-bottomOff)
	}
	_, smallPanel, _ := m.AIOverlayBounds(2)
	if smallPanel < aiOverlayMinPanelHeight {
		t.Errorf("panelHeight should clamp up to aiOverlayMinPanelHeight; got %d", smallPanel)
	}
}

func TestCRDsModel_MouseClick_SelectsRowInListView(t *testing.T) {
	m := NewCRDsModel("ns")
	m.SetSize(120, 30)
	m.crds = []service.CRD{
		{Name: "a.io", Group: "io"},
		{Name: "b.io", Group: "io"},
		{Name: "c.io", Group: "io"},
	}
	m.crdsTable.SetRows(m.crdsRows())
	m.loading = false

	clickY := m.tableFirstRowY() + 2
	out, _ := m.Update(tea.MouseClickMsg{X: 10, Y: clickY, Button: tea.MouseLeft})
	if got := out.crdsTable.Cursor(); got != 2 {
		t.Errorf("list-view click on third row should set cursor=2; got %d", got)
	}
}

func TestCRDsModel_MouseClick_RoutesToInstanceTableInInstanceView(t *testing.T) {
	m := NewCRDsModel("ns")
	m.SetSize(120, 30)
	m.view = crdsViewInstances
	m.selectedCRD = service.CRD{Name: "x.io", Resource: "x"}
	m.instances = []service.CRDInstance{
		{Name: "i1", Namespace: "ns"},
		{Name: "i2", Namespace: "ns"},
	}
	m.instanceTable.SetRows(m.instanceRows())
	m.loading = false

	clickY := m.tableFirstRowY() + 1
	out, _ := m.Update(tea.MouseClickMsg{X: 10, Y: clickY, Button: tea.MouseLeft})
	if got := out.instanceTable.Cursor(); got != 1 {
		t.Errorf("instance-view click should advance instanceTable cursor to 1; got %d", got)
	}
	if got := out.crdsTable.Cursor(); got != 0 {
		t.Errorf("instance-view click must not touch crdsTable; got cursor=%d", got)
	}
}

func TestCRDsModel_MouseClick_NonLeftButtonIgnored(t *testing.T) {
	m := NewCRDsModel("ns")
	m.SetSize(120, 30)
	m.crds = []service.CRD{{Name: "a.io"}, {Name: "b.io"}}
	m.crdsTable.SetRows(m.crdsRows())
	clickY := m.tableFirstRowY() + 1
	out, _ := m.Update(tea.MouseClickMsg{X: 10, Y: clickY, Button: tea.MouseRight})
	if got := out.crdsTable.Cursor(); got != 0 {
		t.Errorf("right-click should not move cursor; got %d", got)
	}
}

func TestCRDsModel_MouseWheel_MovesActiveTableCursor(t *testing.T) {
	m := NewCRDsModel("ns")
	m.SetSize(120, 30)
	m.crds = make([]service.CRD, 10)
	for i := range m.crds {
		m.crds[i] = service.CRD{Name: "x"}
	}
	m.crdsTable.SetRows(m.crdsRows())

	out, _ := m.Update(tea.MouseWheelMsg{Button: tea.MouseWheelDown})
	if got := out.crdsTable.Cursor(); got != tableWheelStep {
		t.Errorf("wheel down should advance crdsTable cursor by %d; got %d", tableWheelStep, got)
	}
}

func TestCRDsModel_SetSize_PreservesCursors(t *testing.T) {
	m := NewCRDsModel("ns")
	m.SetSize(120, 30)
	m.crds = []service.CRD{{Name: "a"}, {Name: "b"}, {Name: "c"}}
	m.crdsTable.SetRows(m.crdsRows())
	m.crdsTable.SetCursor(2)

	m.instances = []service.CRDInstance{{Name: "x"}, {Name: "y"}}
	m.instanceTable.SetRows(m.instanceRows())
	m.instanceTable.SetCursor(1)

	m.SetSize(200, 50)

	if got := m.crdsTable.Cursor(); got != 2 {
		t.Errorf("resize must preserve crdsTable cursor; got %d, want 2", got)
	}
	if got := m.instanceTable.Cursor(); got != 1 {
		t.Errorf("resize must preserve instanceTable cursor; got %d, want 1", got)
	}
	if got := len(m.crdsTable.Rows()); got != 3 {
		t.Errorf("resize must keep crdsTable rows; got %d", got)
	}
}
