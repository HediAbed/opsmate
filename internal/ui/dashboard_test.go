package ui

import (
	"strings"
	"testing"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/HediAbed/opsmate/internal/cluster"
)

func TestSortedEventsNewestFirst_WarningsFirst(t *testing.T) {
	base := time.Date(2026, 4, 17, 12, 0, 0, 0, time.UTC)
	events := []cluster.Event{
		{Type: "Normal", Reason: "n-new", LastTimestamp: base.Add(10 * time.Second)},
		{Type: "Warning", Reason: "w-old", LastTimestamp: base},
		{Type: "Normal", Reason: "n-old", LastTimestamp: base.Add(-time.Minute)},
		{Type: "Warning", Reason: "w-new", LastTimestamp: base.Add(30 * time.Second)},
	}
	got := sortedEventsNewestFirst(events)
	wantReasons := []string{"w-new", "w-old", "n-new", "n-old"}
	if len(got) != len(wantReasons) {
		t.Fatalf("len = %d; want %d", len(got), len(wantReasons))
	}
	for i, reason := range wantReasons {
		if got[i].Reason != reason {
			t.Errorf("position %d: got %q; want %q", i, got[i].Reason, reason)
		}
	}
}

func TestSortedEventsNewestFirst_EmptyInput(t *testing.T) {
	if got := sortedEventsNewestFirst(nil); len(got) != 0 {
		t.Errorf("nil input → %d events; want 0", len(got))
	}
}

func TestSortedEventsNewestFirst_OnlyNormals(t *testing.T) {
	base := time.Now()
	events := []cluster.Event{
		{Type: "Normal", Reason: "b", LastTimestamp: base.Add(-time.Second)},
		{Type: "Normal", Reason: "a", LastTimestamp: base},
	}
	got := sortedEventsNewestFirst(events)
	if got[0].Reason != "a" || got[1].Reason != "b" {
		t.Errorf("want [a, b]; got [%s, %s]", got[0].Reason, got[1].Reason)
	}
}

func TestNewDashboardModel_Defaults(t *testing.T) {
	m := newTestDashboardModel("default")
	if m.namespace != "default" {
		t.Errorf("namespace = %q; want %q", m.namespace, "default")
	}
	if !m.loading {
		t.Error("new dashboard should be loading")
	}
	if m.err != nil {
		t.Errorf("new dashboard should have nil error, got: %v", m.err)
	}
	if len(m.pods) != 0 {
		t.Errorf("new dashboard should have 0 pods, got: %d", len(m.pods))
	}
}

func TestNewDashboardModel_CustomNamespace(t *testing.T) {
	m := newTestDashboardModel("kube-system")
	if m.namespace != "kube-system" {
		t.Errorf("namespace = %q; want %q", m.namespace, "kube-system")
	}
}

func TestDashboard_SetSize(t *testing.T) {
	m := newTestDashboardModel("default")
	m.SetSize(120, 40)
	if m.width != 120 {
		t.Errorf("width = %d; want 120", m.width)
	}
	if m.height != 40 {
		t.Errorf("height = %d; want 40", m.height)
	}
}

func TestDashboard_SetSize_Small(t *testing.T) {
	m := newTestDashboardModel("default")
	m.SetSize(10, 5)
	if m.width != 10 {
		t.Errorf("width = %d; want 10", m.width)
	}
}

func TestDashboard_Namespace(t *testing.T) {
	m := newTestDashboardModel("prod")
	if m.Namespace() != "prod" {
		t.Errorf("Namespace() = %q; want %q", m.Namespace(), "prod")
	}
}

func TestDashboard_SetNamespace(t *testing.T) {
	m := newTestDashboardModel("default")
	cmd := m.SetNamespace("staging")
	if m.namespace != "staging" {
		t.Errorf("namespace after SetNamespace = %q; want %q", m.namespace, "staging")
	}
	if !m.loading {
		t.Error("should be loading after SetNamespace")
	}
	if cmd == nil {
		t.Error("SetNamespace should return a non-nil command")
	}
	if m.pods != nil {
		t.Error("pods should be nil after SetNamespace")
	}
}

func TestDashboard_SelectedPod_Empty(t *testing.T) {
	m := newTestDashboardModel("default")
	if got := m.SelectedPod(); got != "" {
		t.Errorf("SelectedPod on empty table = %q; want empty", got)
	}
}

func TestDashboard_View_ZeroWidth(t *testing.T) {
	m := newTestDashboardModel("default")
	got := m.View()
	if got != "Initializing dashboard..." {
		t.Errorf("View with zero width = %q; want placeholder", got)
	}
}

func TestDashboard_View_WithSize(t *testing.T) {
	m := newTestDashboardModel("default")
	m.SetSize(100, 30)
	got := m.View()
	if got == "" {
		t.Error("View with size should not be empty")
	}
	if !strings.Contains(got, "CLUSTER MONITOR") {
		t.Error("View should contain CLUSTER MONITOR title")
	}
}

func TestDashboard_View_WithPods(t *testing.T) {
	m := newTestDashboardModel("default")
	m.SetSize(120, 40)
	m.pods = []cluster.Pod{
		{Name: "nginx-abc", Status: "Running", Ready: "1/1", Restarts: 0, Age: "2d"},
		{Name: "redis-xyz", Status: "CrashLoopBackOff", Ready: "0/1", Restarts: 50, Age: "5d"},
	}
	m.loading = false
	m.rebuildTableRows()

	got := m.View()
	if !strings.Contains(got, "ALERTS") {
		t.Error("View with CrashLoopBackOff pod should show ALERTS section")
	}

	for _, want := range []string{"nginx-abc", "redis-xyz"} {
		if !strings.Contains(got, want) {
			t.Errorf("View should render pod %q in the table region", want)
		}
	}
}

func TestDashboard_PodTableWidthAlignsWithSiblings(t *testing.T) {
	m := newTestDashboardModel("default")
	m.SetSize(160, 50)
	m.pods = []cluster.Pod{
		{Name: "alpha", Status: "Running", Ready: "1/1", Restarts: 0, Age: "1d"},
		{Name: "beta", Status: "CrashLoopBackOff", Ready: "0/1", Restarts: 9, Age: "1d"},
	}
	m.deployments = []cluster.Deployment{
		{Name: "web", Ready: "1/1", UpToDate: 1, Available: 1, Age: "1d"},
	}
	m.events = []cluster.Event{
		{Type: "Warning", Reason: "BackOff", Object: "Pod/beta", Message: "x"},
	}
	m.loading = false
	m.rebuildTableRows()

	lines := strings.Split(m.View(), "\n")
	maxLine := 0
	for _, line := range lines {
		if w := lipgloss.Width(line); w > maxLine {
			maxLine = w
		}
	}
	if maxLine > 160 {
		t.Errorf("dashboard rendered %d cells wide; want <= 160", maxLine)
	}
}

func TestDashboard_View_NoAlerts_AllRunning(t *testing.T) {
	m := newTestDashboardModel("default")
	m.SetSize(120, 40)
	m.pods = []cluster.Pod{
		{Name: "nginx-abc", Status: "Running", Ready: "1/1", Restarts: 0, Age: "2d"},
		{Name: "redis-xyz", Status: "Running", Ready: "1/1", Restarts: 2, Age: "5d"},
	}
	m.loading = false
	m.rebuildTableRows()

	got := m.View()
	if strings.Contains(got, "ALERTS") {
		t.Error("View with all healthy pods should NOT show ALERTS section")
	}
}

func TestDashboard_View_WithDeployments(t *testing.T) {
	m := newTestDashboardModel("default")
	m.SetSize(120, 40)
	m.deployments = []cluster.Deployment{
		{Name: "web-app", Ready: "3/3", UpToDate: 3, Available: 3, Age: "10d"},
		{Name: "api-server", Ready: "0/2", UpToDate: 2, Available: 0, Age: "1d"},
	}
	m.loading = false

	got := m.View()
	if !strings.Contains(got, "DEPLOYMENT HEALTH") {
		t.Error("View with deployments should show DEPLOYMENT HEALTH section")
	}
}

func TestDashboard_View_WithEvents(t *testing.T) {
	m := newTestDashboardModel("default")
	m.SetSize(120, 40)
	m.events = []cluster.Event{
		{Type: "Warning", Reason: "BackOff", Object: "Pod/bad", Message: "Back-off restarting"},
		{Type: "Normal", Reason: "Pulled", Object: "Pod/good", Message: "Successfully pulled image"},
	}
	m.loading = false

	got := m.View()
	if !strings.Contains(got, "RECENT EVENTS") {
		t.Error("View with events should show RECENT EVENTS section")
	}
}

func TestDashboard_View_WarningsFirst(t *testing.T) {
	m := newTestDashboardModel("default")
	m.SetSize(120, 40)
	m.events = []cluster.Event{
		{Type: "Normal", Reason: "Pulled", Object: "Pod/good", Message: "Image pulled"},
		{Type: "Warning", Reason: "BackOff", Object: "Pod/bad", Message: "Back-off restarting"},
	}
	m.loading = false

	got := m.View()
	warnIdx := strings.Index(got, "BackOff")
	normIdx := strings.Index(got, "Pulled")
	if warnIdx == -1 || normIdx == -1 {
		t.Error("View should contain both event reasons")
	} else if warnIdx > normIdx {
		t.Error("Warning events should appear before Normal events")
	}
}

func TestDashboard_View_SmallHeight(t *testing.T) {
	m := newTestDashboardModel("default")
	m.SetSize(80, 5)
	m.pods = []cluster.Pod{
		{Name: "pod1", Status: "Running", Ready: "1/1", Age: "1d"},
		{Name: "pod2", Status: "Running", Ready: "1/1", Age: "2d"},
	}
	m.loading = false
	m.rebuildTableRows()

	got := m.View()
	if got == "" {
		t.Error("View should produce output even at small height")
	}
}

func TestRenderBar_Zero(t *testing.T) {
	bar := renderBar(0, 10, lipgloss.Color("#00FFFF"), lipgloss.Color("#555555"))
	if !strings.Contains(bar, "░") {
		t.Error("renderBar(0) should contain empty blocks")
	}
	if strings.Contains(bar, "█") {
		t.Error("renderBar(0) should not contain filled blocks")
	}
}

func TestRenderBar_Full(t *testing.T) {
	bar := renderBar(1.0, 10, lipgloss.Color("#00FFFF"), lipgloss.Color("#555555"))
	if !strings.Contains(bar, "█") {
		t.Error("renderBar(1.0) should contain filled blocks")
	}
	if strings.Contains(bar, "░") {
		t.Error("renderBar(1.0) should not contain empty blocks")
	}
}

func TestRenderBar_Half(t *testing.T) {
	bar := renderBar(0.5, 10, lipgloss.Color("#00FFFF"), lipgloss.Color("#555555"))
	if !strings.Contains(bar, "█") {
		t.Error("renderBar(0.5) should contain filled blocks")
	}
	if !strings.Contains(bar, "░") {
		t.Error("renderBar(0.5) should contain empty blocks")
	}
}

func TestRenderBar_NegativeClamp(t *testing.T) {
	bar := renderBar(-0.5, 10, lipgloss.Color("#00FFFF"), lipgloss.Color("#555555"))
	if bar == "" {
		t.Error("renderBar(-0.5) should not be empty")
	}
}

func TestRenderBar_OverOneClamp(t *testing.T) {
	bar := renderBar(1.5, 10, lipgloss.Color("#00FFFF"), lipgloss.Color("#555555"))
	if bar == "" {
		t.Error("renderBar(1.5) should not be empty")
	}
}

func TestDashPodColumns_ReturnsSevenColumns(t *testing.T) {
	cols := dashPodColumns(100)
	if len(cols) != 7 {
		t.Errorf("dashPodColumns should return 7 columns, got %d", len(cols))
	}
}

func TestDashPodColumns_NameIsFirst(t *testing.T) {
	cols := dashPodColumns(100)
	if cols[0].Title != "NAME" {
		t.Errorf("first column should be NAME, got %q", cols[0].Title)
	}
}

func TestDashPodColumns_TotalFitsWidth(t *testing.T) {
	for _, w := range []int{49, 60, 80, 120, 200} {
		cols := dashPodColumns(w)
		total := 0
		for _, c := range cols {
			total += c.Width
		}
		rendered := total + len(cols)*2
		if rendered > w {
			t.Errorf("dashPodColumns(%d): rendered width %d (cols %d + padding %d) exceeds content area",
				w, rendered, total, len(cols)*2)
		}
	}
}

func TestDashPodColumns_MinWidth(t *testing.T) {
	cols := dashPodColumns(10)
	for _, c := range cols {
		if c.Width < 1 {
			t.Errorf("column %q has width %d; should be at least 1", c.Title, c.Width)
		}
	}
}

func TestDashboard_RenderAlerts_NoAlerts(t *testing.T) {
	m := newTestDashboardModel("default")
	m.SetSize(100, 30)
	m.pods = []cluster.Pod{
		{Name: "healthy", Status: "Running", Ready: "1/1", Restarts: 0},
	}
	got := m.renderAlerts(100)
	if got != "" {
		t.Error("renderAlerts should return empty for all-healthy pods")
	}
}

func TestDashboard_RenderAlerts_CrashLoop(t *testing.T) {
	m := newTestDashboardModel("default")
	m.SetSize(100, 30)
	m.pods = []cluster.Pod{
		{Name: "crasher", Status: "CrashLoopBackOff", Ready: "0/1", Restarts: 100},
	}
	got := m.renderAlerts(100)
	if got == "" {
		t.Error("renderAlerts should return non-empty for CrashLoopBackOff pod")
	}
}

func TestDashboard_RenderAlerts_HighRestarts(t *testing.T) {
	m := newTestDashboardModel("default")
	m.SetSize(100, 30)
	m.pods = []cluster.Pod{
		{Name: "restarty", Status: "Running", Ready: "1/1", Restarts: 50},
	}
	got := m.renderAlerts(100)
	if got == "" {
		t.Error("renderAlerts should flag pods with >10 restarts")
	}
}

func TestDashboard_RenderAlerts_LowRestarts(t *testing.T) {
	m := newTestDashboardModel("default")
	m.SetSize(100, 30)
	m.pods = []cluster.Pod{
		{Name: "normal", Status: "Running", Ready: "1/1", Restarts: 5},
	}
	got := m.renderAlerts(100)
	if got != "" {
		t.Error("renderAlerts should not flag pods with <=10 restarts")
	}
}

func TestDashboard_RenderOverviewRow(t *testing.T) {
	m := newTestDashboardModel("default")
	m.SetSize(100, 30)
	m.pods = []cluster.Pod{
		{Name: "p1", Status: "Running"},
		{Name: "p2", Status: "Running"},
		{Name: "p3", Status: "Pending"},
		{Name: "p4", Status: "CrashLoopBackOff"},
	}
	m.deployments = []cluster.Deployment{
		{Name: "d1"},
	}

	got := m.renderOverviewRow(100)
	if !strings.Contains(got, "4") {
		t.Error("overview should show total pod count")
	}
}

func TestDashboard_MergeMetrics(t *testing.T) {
	m := newTestDashboardModel("default")
	m.pods = []cluster.Pod{
		{Name: "pod1", Status: "Running"},
		{Name: "pod2", Status: "Running"},
	}
	m.metrics = []cluster.PodMetric{
		{Name: "pod1", CPU: "100m", Memory: "256Mi"},
	}
	m.mergeMetrics()

	if m.pods[0].CPU != "100m" {
		t.Errorf("pod1 CPU = %q; want %q", m.pods[0].CPU, "100m")
	}
	if m.pods[0].Memory != "256Mi" {
		t.Errorf("pod1 Memory = %q; want %q", m.pods[0].Memory, "256Mi")
	}
	if m.pods[1].CPU != "" {
		t.Errorf("pod2 CPU = %q; want empty", m.pods[1].CPU)
	}
}

func TestDashboard_RebuildTableRows(t *testing.T) {
	m := newTestDashboardModel("default")
	m.SetSize(100, 30)
	m.pods = []cluster.Pod{
		{Name: "pod1", Status: "Running", Ready: "1/1", Restarts: 0, Age: "1d", CPU: "50m", Memory: "128Mi"},
		{Name: "pod2", Status: "Pending", Ready: "0/1", Restarts: 3, Age: "5m"},
	}
	m.rebuildTableRows()

	rows := m.podTable.Rows()
	if len(rows) != 2 {
		t.Fatalf("table should have 2 rows, got %d", len(rows))
	}
	if rows[0][0] != "pod1" {
		t.Errorf("row 0 name = %q; want %q", rows[0][0], "pod1")
	}
	if rows[1][5] != "-" {
		t.Errorf("row 1 CPU should be '-' when empty, got %q", rows[1][5])
	}
}

func TestDashboard_RenderDeploymentHealth(t *testing.T) {
	m := newTestDashboardModel("default")
	m.SetSize(120, 40)
	m.deployments = []cluster.Deployment{
		{Name: "web", Ready: "3/3", UpToDate: 3, Available: 3, Age: "10d"},
		{Name: "api", Ready: "0/2", UpToDate: 2, Available: 0, Age: "1d"},
	}

	got := m.renderDeploymentHealth(120)
	if !strings.Contains(got, "DEPLOYMENT HEALTH") {
		t.Error("should contain header")
	}
	if !strings.Contains(got, "web") {
		t.Error("should contain deployment name")
	}
	if !strings.Contains(got, "3/3") {
		t.Error("should contain ready count")
	}
}

func TestDashboard_RenderDeploymentHealth_MaxSix(t *testing.T) {
	m := newTestDashboardModel("default")
	m.SetSize(120, 40)
	for i := 0; i < 10; i++ {
		m.deployments = append(m.deployments, cluster.Deployment{
			Name: "deploy-" + strings.Repeat("x", 5), Ready: "1/1", Age: "1d",
		})
	}

	got := m.renderDeploymentHealth(120)
	if !strings.Contains(got, "+4 more") {
		t.Error("should show +N more when >6 deployments")
	}
}

func TestParseMilli_Millicores(t *testing.T) {
	tests := []struct {
		input string
		want  int
	}{
		{"100m", 100},
		{"0m", 0},
		{"1500m", 1500},
		{"", 0},
		{"-", 0},
		{"2", 2000},
		{"1", 1000},
	}
	for _, tt := range tests {
		got := parseMilli(tt.input)
		if got != tt.want {
			t.Errorf("parseMilli(%q) = %d; want %d", tt.input, got, tt.want)
		}
	}
}

func TestDashboard_RenderTopConsumers_NoMetrics(t *testing.T) {
	m := newTestDashboardModel("default")
	m.SetSize(120, 40)
	got := m.renderTopConsumers(116)
	if got != "" {
		t.Error("renderTopConsumers should return empty when no metrics")
	}
}

func TestDashboard_RenderTopConsumers_WithMetrics(t *testing.T) {
	m := newTestDashboardModel("default")
	m.SetSize(120, 40)
	m.pods = []cluster.Pod{
		{Name: "pod-a", CPU: "500m", Memory: "256Mi"},
		{Name: "pod-b", CPU: "100m", Memory: "128Mi"},
		{Name: "pod-c", CPU: "800m", Memory: "512Mi"},
	}
	m.metrics = []cluster.PodMetric{
		{Name: "pod-a", CPU: "500m", Memory: "256Mi"},
		{Name: "pod-b", CPU: "100m", Memory: "128Mi"},
		{Name: "pod-c", CPU: "800m", Memory: "512Mi"},
	}

	got := m.renderTopConsumers(116)
	if got == "" {
		t.Fatal("renderTopConsumers should not be empty with metrics")
	}
	if !strings.Contains(got, "TOP RESOURCE CONSUMERS") {
		t.Error("should contain header")
	}
	if !strings.Contains(got, "pod-c") {
		t.Error("should contain highest CPU pod")
	}
}

func TestDashboard_RenderOverviewRow_NoPods(t *testing.T) {
	m := newTestDashboardModel("default")
	m.SetSize(120, 40)
	got := m.renderOverviewRow(120)
	if got == "" {
		t.Error("renderOverviewRow should not be empty even with no pods")
	}
}

func TestDashboard_RenderOverviewRow_WithPods(t *testing.T) {
	m := newTestDashboardModel("default")
	m.SetSize(120, 40)
	m.pods = []cluster.Pod{
		{Name: "a", Status: "Running"},
		{Name: "b", Status: "Running"},
		{Name: "c", Status: "Pending"},
	}
	got := m.renderOverviewRow(120)
	plain := stripANSI(got)
	if !strings.Contains(plain, "Pods:3") {
		t.Errorf("should show Pods:3, got: %q", plain)
	}
	if !strings.Contains(plain, "Running:2") {
		t.Errorf("should show Running:2, got: %q", plain)
	}
}

func TestDashboard_HealthAnalysis_Defaults(t *testing.T) {
	m := newTestDashboardModel("default")
	if m.showHealthAnalysis {
		t.Error("health analysis should be hidden by default")
	}
	if m.healthAnalysisLoading {
		t.Error("health analysis should not be loading by default")
	}
	if m.healthAnalysisSummary != "" {
		t.Errorf("health analysis summary should be empty by default, got %q", m.healthAnalysisSummary)
	}
}

func TestDashboard_RenderHealthAnalysis_Loading(t *testing.T) {
	m := newTestDashboardModel("default")
	m.SetSize(120, 40)
	m.showHealthAnalysis = true
	m.healthAnalysisLoading = true
	got := m.renderHealthAnalysis(m.innerW())
	if got == "" {
		t.Error("health analysis should not be empty when loading")
	}
	plain := stripANSI(got)
	if !strings.Contains(plain, "Analyzing") {
		t.Errorf("loading state should contain 'Analyzing', got: %q", plain)
	}
}

func TestDashboard_RenderHealthAnalysis_WithSummary(t *testing.T) {
	m := newTestDashboardModel("default")
	m.SetSize(120, 40)
	m.showHealthAnalysis = true
	m.healthAnalysisSummary = "Cluster is healthy. All pods running."
	got := m.renderHealthAnalysis(m.innerW())
	plain := stripANSI(got)
	if !strings.Contains(plain, "CLUSTER HEALTH") {
		t.Errorf("health analysis should contain the header, got: %q", plain)
	}
	if !strings.Contains(plain, "Cluster is healthy") {
		t.Errorf("health analysis should contain summary text, got: %q", plain)
	}
}

func TestDashboard_RenderHealthAnalysis_Hidden(t *testing.T) {
	m := newTestDashboardModel("default")
	m.SetSize(120, 40)
	m.showHealthAnalysis = false
	got := m.View()
	plain := stripANSI(got)
	if strings.Contains(plain, "CLUSTER HEALTH") {
		t.Error("view should not contain health analysis when it is hidden")
	}
}
