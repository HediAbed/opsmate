package ui

import (
	"errors"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/HediAbed/opsmate/internal/kube"
)

func TestHelmModel_HelmReleasesMsg_PopulatesTableAndClearsLoading(t *testing.T) {
	m := newTestHelmModel("ns")
	m.SetSize(120, 40)
	m.loading = true

	out, _ := m.Update(helmReleasesMsg{
		Releases: []kube.HelmRelease{
			{Name: "ingress-nginx", Namespace: "ingress", Revision: 5, Status: "deployed", ChartName: "ingress-nginx", ChartVersion: "4.10.0", AppVersion: "1.10.0"},
		},
	})
	if out.loading {
		t.Error("loading should clear after fetch")
	}
	if len(out.releases) != 1 {
		t.Fatalf("expected 1 release, got %d", len(out.releases))
	}
	rows := out.releaseTable.Rows()
	if len(rows) != 1 || rows[0][0] != "ingress-nginx" {
		t.Errorf("table rows wrong: %+v", rows)
	}
	if rows[0][2] != "5" {
		t.Errorf("revision should render as decimal; got %q", rows[0][2])
	}
}

func TestHelmModel_ErrorRendersBanner(t *testing.T) {
	m := newTestHelmModel("ns")
	m.SetSize(120, 40)

	sentinel := errors.New("release list unavailable")
	out, _ := m.Update(helmReleasesMsg{Err: sentinel})
	if !errors.Is(out.err, sentinel) {
		t.Fatalf("error = %v, want sentinel", out.err)
	}
	view := stripAnsiForTest(out.View())
	if !strings.Contains(view, "release list unavailable") {
		t.Errorf("error banner missing cause; view:\n%s", view)
	}
}

func TestHelmModel_GenericErrorRendersStderrInBanner(t *testing.T) {
	m := newTestHelmModel("ns")
	m.SetSize(120, 40)

	out, _ := m.Update(helmReleasesMsg{Err: errors.New("release nope not found")})
	view := stripAnsiForTest(out.View())
	if !strings.Contains(view, "release nope not found") {
		t.Errorf("generic error banner should preserve helm stderr; view:\n%s", view)
	}
}

func TestHelmModel_RKeyTriggersRefresh(t *testing.T) {
	m := newTestHelmModel("ns")
	m.SetSize(120, 40)
	m.loading = false

	out, cmd := m.Update(tea.KeyPressMsg{Code: 'r', Text: "r"})
	if !out.loading {
		t.Error("'r' should set loading=true")
	}
	if cmd == nil {
		t.Error("'r' should return a non-nil refresh cmd")
	}
}

func TestHelmModel_SetNamespace_DropsCacheAndReturnsFetchCmd(t *testing.T) {
	m := newTestHelmModel("ns-a")
	m.releases = []kube.HelmRelease{{Name: "stale"}}
	m.loading = false

	cmd := m.SetNamespace("ns-b")
	if m.namespace != "ns-b" {
		t.Errorf("namespace = %q, want ns-b", m.namespace)
	}
	if m.releases != nil {
		t.Error("namespace switch must drop the stale cache")
	}
	if !m.loading {
		t.Error("namespace switch must set loading=true so the refetch shows the spinner")
	}
	if cmd == nil {
		t.Error("namespace switch must return a fetch cmd; otherwise the spinner sits forever")
	}
}

func TestHelmModel_SetNamespace_SameNamespaceIsNoOp(t *testing.T) {
	m := newTestHelmModel("ns")
	m.releases = []kube.HelmRelease{{Name: "keep"}}
	m.loading = false

	if cmd := m.SetNamespace("ns"); cmd != nil {
		t.Error("SetNamespace to the same value must not return a cmd (avoid wasted refetch)")
	}
	if len(m.releases) == 0 {
		t.Error("same-namespace SetNamespace must not clear the cache")
	}
	if m.loading {
		t.Error("same-namespace SetNamespace must not flip loading back on")
	}
}

func TestHelmModel_Init_ReturnsNonNilFetchCmd(t *testing.T) {
	m := newTestHelmModel("ns")
	if m.Init() == nil {
		t.Error("Init must return a non-nil cmd")
	}
}

func TestHelmModel_Deactivate_IsIdempotent(_ *testing.T) {
	m := newTestHelmModel("ns")
	m.Deactivate()
	m.Deactivate()
}

func TestHelmModel_Activate_SetsLoadingAndReturnsFetchCmd(t *testing.T) {
	m := newTestHelmModel("ns")
	m.SetSize(120, 40)
	m.loading = false

	cmd := m.Activate()
	if !m.loading {
		t.Error("Activate must set loading=true to show the spinner during refetch")
	}
	if cmd == nil {
		t.Error("Activate must return a non-nil refresh cmd")
	}
}

func TestHelmModel_AnalysisOverlayBounds_AccountsForChromeAndClampsToMin(t *testing.T) {
	m := newTestHelmModel("ns")
	m.SetSize(120, 40)

	topOff, panelH, bottomOff := m.AnalysisOverlayBounds(40)
	if topOff <= 0 {
		t.Errorf("topOffset must include the title bar height; got %d", topOff)
	}
	if bottomOff <= 0 {
		t.Errorf("bottomOffset must include the help bar height; got %d", bottomOff)
	}
	if panelH != 40-topOff-bottomOff {
		t.Errorf("panelHeight = %d; want totalHeight - topOff - bottomOff = %d", panelH, 40-topOff-bottomOff)
	}

	_, smallPanel, _ := m.AnalysisOverlayBounds(2)
	if smallPanel < analysisOverlayMinimumPanelHeight {
		t.Errorf("panelHeight should clamp up to aiOverlayMinPanelHeight (%d); got %d", analysisOverlayMinimumPanelHeight, smallPanel)
	}
}

func TestHelmModel_AnalysisOverlayBounds_GrowsBottomOffsetWhenErrorBannerVisible(t *testing.T) {
	noBanner := newTestHelmModel("ns")
	noBanner.SetSize(120, 40)
	_, _, bottomNoBanner := noBanner.AnalysisOverlayBounds(40)

	withBanner := newTestHelmModel("ns")
	withBanner.SetSize(120, 40)
	withBanner.err = errors.New("release list unavailable")
	_, _, bottomWithBanner := withBanner.AnalysisOverlayBounds(40)

	if bottomWithBanner <= bottomNoBanner {
		t.Errorf("error banner should grow bottomOffset; without=%d with=%d", bottomNoBanner, bottomWithBanner)
	}
}

func TestHelmModel_View_EmptyListShowsFriendlyMessage(t *testing.T) {
	m := newTestHelmModel("ns")
	m.SetSize(120, 40)
	m.loading = false
	m.releases = nil

	view := stripAnsiForTest(m.View())
	if !strings.Contains(view, "No helm releases found.") {
		t.Errorf("empty list should render a friendly placeholder; view:\n%s", view)
	}
}

func TestHelmModel_Update_ArrowKeyForwardsToTable(t *testing.T) {
	m := newTestHelmModel("ns")
	m.SetSize(120, 40)
	m.loading = false
	m.releases = []kube.HelmRelease{
		{Name: "a", Namespace: "ns"},
		{Name: "b", Namespace: "ns"},
	}
	m.releaseTable.SetRows(m.currentRows())
	m.releaseTable.SetCursor(0)

	out, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyDown, Text: "down"})
	if out.releaseTable.Cursor() != 1 {
		t.Errorf("down arrow should advance the table cursor to 1; got %d", out.releaseTable.Cursor())
	}
}

func TestHelmModel_Update_PopulatesStatusOnSuccess(t *testing.T) {
	m := newTestHelmModel("ns")
	m.SetSize(120, 40)

	out, _ := m.Update(helmReleasesMsg{Releases: []kube.HelmRelease{{Name: "only"}}})
	if out.statusMsg == "" {
		t.Error("successful fetch should set a status message so the title bar reports the count")
	}
	view := stripAnsiForTest(out.View())
	if !strings.Contains(view, "1 release") {
		t.Errorf("title bar should mention 'Loaded 1 release' (singular for count=1); view:\n%s", view)
	}
}

func TestHelmModel_SelectedRelease_ReturnsZeroValueWhenEmpty(t *testing.T) {
	m := newTestHelmModel("ns")
	m.SetSize(120, 40)
	if got := m.SelectedRelease(); got.Name != "" {
		t.Errorf("empty model should yield zero release; got %+v", got)
	}
}

func TestHelmModel_SelectedRelease_ResolvesByNameAndNamespace(t *testing.T) {
	m := newTestHelmModel("ns")
	m.SetSize(120, 40)
	m.releases = []kube.HelmRelease{
		{Name: "a", Namespace: "x"},
		{Name: "a", Namespace: "y"},
		{Name: "b", Namespace: "x"},
	}
	m.releaseTable.SetRows(m.currentRows())
	m.releaseTable.SetCursor(1)

	got := m.SelectedRelease()
	if got.Name != "a" || got.Namespace != "y" {
		t.Errorf("selected release should disambiguate by namespace; got %+v", got)
	}
}

func TestHelmModel_DefaultKeyForwardsToTable(_ *testing.T) {
	m := newTestHelmModel("ns")
	m.SetSize(120, 40)
	m.releases = []kube.HelmRelease{{Name: "a"}, {Name: "b"}}
	m.releaseTable.SetRows(m.currentRows())
	_, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyDown, Text: "down"})
}

func TestHelmModel_SpinnerTick_ReturnsCmd(t *testing.T) {
	m := newTestHelmModel("ns")
	_, cmd := m.Update(m.spinner.Tick())
	if cmd == nil {
		t.Error("spinner tick should produce next tick")
	}
}

func TestHelmModel_HasInputFocus_AlwaysFalseToday(t *testing.T) {
	m := newTestHelmModel("ns")
	if m.HasInputFocus() {
		t.Error("HelmModel currently has no text inputs; HasInputFocus must be false")
	}
}

func TestHelmModel_MouseClick_SelectsRowByY(t *testing.T) {
	m := newTestHelmModel("ns")
	m.SetSize(120, 30)
	m.releases = []kube.HelmRelease{
		{Name: "a", Namespace: "ns"},
		{Name: "b", Namespace: "ns"},
		{Name: "c", Namespace: "ns"},
	}
	m.releaseTable.SetRows(m.currentRows())

	clickY := m.tableFirstRowY() + 2
	out, _ := m.Update(tea.MouseClickMsg{X: 10, Y: clickY, Button: tea.MouseLeft})
	if got := out.releaseTable.Cursor(); got != 2 {
		t.Errorf("click on third row should set cursor=2; got %d", got)
	}
}

func TestHelmModel_MouseClick_OutOfRangeIsNoOp(t *testing.T) {
	m := newTestHelmModel("ns")
	m.SetSize(120, 30)
	m.releases = []kube.HelmRelease{{Name: "a", Namespace: "ns"}}
	m.releaseTable.SetRows(m.currentRows())
	out, _ := m.Update(tea.MouseClickMsg{X: 10, Y: 999, Button: tea.MouseLeft})
	if got := out.releaseTable.Cursor(); got != 0 {
		t.Errorf("out-of-range click should not move cursor; got %d", got)
	}
}

func TestHelmModel_MouseClick_NonLeftButtonIgnored(t *testing.T) {
	m := newTestHelmModel("ns")
	m.SetSize(120, 30)
	m.releases = []kube.HelmRelease{{Name: "a"}, {Name: "b"}}
	m.releaseTable.SetRows(m.currentRows())
	clickY := m.tableFirstRowY() + 1
	for _, b := range []tea.MouseButton{tea.MouseRight, tea.MouseMiddle} {
		out, _ := m.Update(tea.MouseClickMsg{X: 10, Y: clickY, Button: b})
		if got := out.releaseTable.Cursor(); got != 0 {
			t.Errorf("button=%v should not move cursor; got %d", b, got)
		}
	}
}

func TestHelmModel_MouseWheel_MovesCursor(t *testing.T) {
	m := newTestHelmModel("ns")
	m.SetSize(120, 30)
	m.releases = make([]kube.HelmRelease, 10)
	for i := range m.releases {
		m.releases[i] = kube.HelmRelease{Name: "r", Namespace: "ns"}
	}
	m.releaseTable.SetRows(m.currentRows())

	out, _ := m.Update(tea.MouseWheelMsg{Button: tea.MouseWheelDown})
	if got := out.releaseTable.Cursor(); got != tableWheelStep {
		t.Errorf("wheel down should advance cursor by %d; got %d", tableWheelStep, got)
	}
	out, _ = out.Update(tea.MouseWheelMsg{Button: tea.MouseWheelUp})
	if got := out.releaseTable.Cursor(); got != 0 {
		t.Errorf("wheel up should walk cursor back to 0; got %d", got)
	}
}

func TestHelmModel_SetSize_PreservesCursorAndRows(t *testing.T) {
	m := newTestHelmModel("ns")
	m.SetSize(120, 30)
	m.releases = []kube.HelmRelease{
		{Name: "alpha", Namespace: "ns"},
		{Name: "beta", Namespace: "ns"},
		{Name: "gamma", Namespace: "ns"},
	}
	m.releaseTable.SetRows(m.currentRows())
	m.releaseTable.SetCursor(2)

	m.SetSize(200, 50)

	if got := m.releaseTable.Cursor(); got != 2 {
		t.Errorf("resize must not reset cursor; got %d, want 2", got)
	}
	if got := len(m.releaseTable.Rows()); got != 3 {
		t.Errorf("resize must not drop rows; got %d, want 3", got)
	}
}

func TestFormatHelmReleaseTime(t *testing.T) {
	if formatted := formatHelmReleaseTime(time.Time{}); formatted != "-" {
		t.Fatalf("zero release time = %q, want dash", formatted)
	}
	updatedAt := time.Date(2026, time.August, 29, 8, 15, 0, 0, time.UTC)
	if formatted := formatHelmReleaseTime(updatedAt); formatted != "2026-08-29 08:15 UTC" {
		t.Fatalf("release time = %q", formatted)
	}
}
