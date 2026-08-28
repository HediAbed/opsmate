package model

import (
	"strings"
	"testing"

	"charm.land/bubbles/v2/viewport"
)

func TestClampModalWidth(t *testing.T) {
	cases := []struct {
		name     string
		desired  int
		terminal int
		want     int
	}{
		{"wide terminal fits desired", 60, 200, 60},
		{"exact fit including gutters", 60, 64, 60},
		{"clamps to terminal minus gutters", 60, 50, 46},
		{"terminal below usable returns desired untouched", 60, 8, 60},
		{"terminal at usable boundary clamps", 60, 16, 12},
		{"terminal one above boundary clamps to 13", 60, 17, 13},
		{"terminal one below boundary returns desired", 60, 15, 60},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := clampModalWidth(c.desired, c.terminal); got != c.want {
				t.Errorf("clampModalWidth(%d,%d)=%d want %d", c.desired, c.terminal, got, c.want)
			}
		})
	}
}

func TestViewportScrollPct_Clamps(t *testing.T) {
	v := viewport.New()
	v.SetWidth(40)
	v.SetHeight(10)
	v.SetContent("one\n")
	if got := viewportScrollPct(v); got < 0 || got > 100 {
		t.Errorf("viewportScrollPct must clamp to [0,100]; got %d", got)
	}
}

func TestViewportScrollDirection(t *testing.T) {
	v := viewport.New()
	v.SetWidth(40)
	v.SetHeight(3)
	v.SetContent("only one\nshort\nfile")
	if got := viewportScrollDirection(v); got != "" {
		t.Errorf("non-overflowing content must yield empty direction; got %q", got)
	}

	v.SetContent(strings.Repeat("line\n", 30))
	v.GotoTop()
	if got := viewportScrollDirection(v); got != "▼" {
		t.Errorf("at top with more below should be ▼; got %q", got)
	}
	v.GotoBottom()
	if got := viewportScrollDirection(v); got != "▲" {
		t.Errorf("at bottom with more above should be ▲; got %q", got)
	}
	v.SetYOffset(10)
	if got := viewportScrollDirection(v); got != "▲▼" {
		t.Errorf("middle should be ▲▼; got %q", got)
	}
}

func TestPopupScrollIndicator_EmptyWhenFits(t *testing.T) {
	v := viewport.New()
	v.SetWidth(40)
	v.SetHeight(20)
	v.SetContent("line\n")
	if got := popupScrollIndicator(v); got != "" {
		t.Errorf("scroll indicator on non-overflowing content should be empty, got %q", got)
	}
}

func TestPopupScrollIndicator_AtTopShowsMoreBelow(t *testing.T) {
	v := viewport.New()
	v.SetWidth(40)
	v.SetHeight(3)
	v.SetContent(strings.Repeat("line\n", 30))
	v.GotoTop()
	got := popupScrollIndicator(v)
	if !strings.Contains(got, "▼") {
		t.Errorf("at top should include ▼ glyph; got %q", got)
	}
	if !strings.Contains(got, "more below") {
		t.Errorf("at top should mention 'more below'; got %q", got)
	}
}

func TestPopupScrollIndicator_AtBottomShowsMoreAbove(t *testing.T) {
	v := viewport.New()
	v.SetWidth(40)
	v.SetHeight(3)
	v.SetContent(strings.Repeat("line\n", 30))
	v.GotoBottom()
	got := popupScrollIndicator(v)
	if !strings.Contains(got, "▲") {
		t.Errorf("at bottom should include ▲ glyph; got %q", got)
	}
	if !strings.Contains(got, "more above") {
		t.Errorf("at bottom should mention 'more above'; got %q", got)
	}
}

func TestPopupScrollIndicator_MiddleShowsBothGlyphs(t *testing.T) {
	v := viewport.New()
	v.SetWidth(40)
	v.SetHeight(3)
	v.SetContent(strings.Repeat("line\n", 30))
	v.SetYOffset(10)
	got := popupScrollIndicator(v)
	if !strings.Contains(got, "▲▼") {
		t.Errorf("mid-scroll should include both glyphs; got %q", got)
	}
}

func TestAppendHelpHint_DropsHintWhenOverflows(t *testing.T) {
	helpBar := "h:help q:quit"
	if got := appendHelpHint(helpBar, "30%", 5); got != helpBar {
		t.Errorf("hint must be dropped when combined width exceeds w; got %q", got)
	}
}

func TestAppendHelpHint_AppendsAfterHelpBarWhenFits(t *testing.T) {
	helpBar := "h:help"
	got := appendHelpHint(helpBar, "30%", 200)
	helpIdx := strings.Index(got, "h:help")
	hintIdx := strings.Index(got, "30%")
	if helpIdx < 0 || hintIdx < 0 {
		t.Fatalf("both helpBar and hint must appear; got %q", got)
	}
	if hintIdx < helpIdx {
		t.Errorf("hint must come after helpBar (order matters); got %q", got)
	}
}

func TestTableCursorLabel(t *testing.T) {
	cases := []struct {
		cursor, total int
		want          string
	}{
		{0, 0, ""},
		{0, 1, "1/1"},
		{3, 10, "4/10"},
		{99, 100, "100/100"},
	}
	for _, c := range cases {
		if got := tableCursorLabel(c.cursor, c.total); got != c.want {
			t.Errorf("tableCursorLabel(%d,%d)=%q want %q", c.cursor, c.total, got, c.want)
		}
	}
}

func TestRenderBrowserTabStrip_FitsAllOnWideTerminal(t *testing.T) {
	got := renderBrowserTabStrip("pods", 500)
	for _, rt := range allResourceTypes {
		if !strings.Contains(got, strings.ToUpper(rt)) {
			t.Errorf("wide terminal must render every tab; missing %q", rt)
		}
	}
	if strings.Contains(got, "‹") || strings.Contains(got, "›") {
		t.Error("wide terminal must not show overflow hints when all tabs fit")
	}
	prev := -1
	for _, rt := range allResourceTypes {
		idx := strings.Index(got, strings.ToUpper(rt))
		if idx < prev {
			t.Errorf("tabs must render in declared order; %q out of order", rt)
		}
		prev = idx
	}
}

func TestRenderBrowserTabStrip_NarrowWindowsAroundActive(t *testing.T) {
	got := renderBrowserTabStrip("hpas", 30)
	if !strings.Contains(got, "HPAS") {
		t.Errorf("active tab must always be visible; got %q", got)
	}
	if !strings.Contains(got, "‹") {
		t.Errorf("hidden tabs to the left must be signaled with ‹; got %q", got)
	}
	if !strings.Contains(got, "›") {
		t.Errorf("hidden tabs to the right must be signaled with ›; got %q", got)
	}
	if strings.Contains(got, "PODS") && strings.Contains(got, "RBAC") {
		t.Errorf("narrow terminal must hide some tabs; both PODS and RBAC appear in %q", got)
	}
}

func TestRenderBrowserTabStrip_ReturnsEmptyForTooNarrow(t *testing.T) {
	if got := renderBrowserTabStrip("pods", browserTabStripMinViable-1); got != "" {
		t.Errorf("width below browserTabStripMinViable should render empty; got %q", got)
	}
	if got := renderBrowserTabStrip("pods", browserTabStripMinViable); got == "" {
		t.Error("width at browserTabStripMinViable should render at least the active tab; got empty")
	}
}

func TestRenderBrowserTabStrip_ActiveFirstShowsRightHintOnly(t *testing.T) {
	got := renderBrowserTabStrip(allResourceTypes[0], 30)
	if strings.Contains(got, "‹") {
		t.Errorf("when the first tab is active there's nothing further left; got %q", got)
	}
	if !strings.Contains(got, "›") {
		t.Errorf("right hint should still appear when more tabs exist to the right; got %q", got)
	}
}

func TestRenderBrowserTabStrip_ActiveLastShowsLeftHintOnly(t *testing.T) {
	got := renderBrowserTabStrip(allResourceTypes[len(allResourceTypes)-1], 30)
	if strings.Contains(got, "›") {
		t.Errorf("when the last tab is active there's nothing further right; got %q", got)
	}
	if !strings.Contains(got, "‹") {
		t.Errorf("left hint should appear when more tabs exist to the left; got %q", got)
	}
}

func TestRenderBrowserTabStrip_MidTabRoughlyAt80Cols(t *testing.T) {
	got := renderBrowserTabStrip("services", 80)
	if !strings.Contains(got, "SERVICES") {
		t.Errorf("active tab must be visible at 80 cols; got %q", got)
	}
}

func TestDashboardOverflows_FalseOnSmallContent(t *testing.T) {
	var m DashboardModel
	m.bodyView = viewport.New()
	m.bodyView.SetWidth(40)
	m.bodyView.SetHeight(10)
	m.bodyView.SetContent("one line")
	if m.bodyOverflows() {
		t.Error("single line should not be reported as overflow")
	}
}

func TestDashboardOverflows_TrueOnLargeContent(t *testing.T) {
	var m DashboardModel
	m.bodyView = viewport.New()
	m.bodyView.SetWidth(40)
	m.bodyView.SetHeight(3)
	m.bodyView.SetContent(strings.Repeat("line\n", 30))
	if !m.bodyOverflows() {
		t.Error("30 lines in a 3-row viewport must report overflow")
	}
}

func TestDashboardOverflows_FalseOnZeroHeight(t *testing.T) {
	var m DashboardModel
	m.bodyView = viewport.New()
	m.bodyView.SetWidth(40)
	m.bodyView.SetHeight(0)
	m.bodyView.SetContent(strings.Repeat("line\n", 30))
	if m.bodyOverflows() {
		t.Error("zero-height viewport is uninitialised; must not report overflow")
	}
}

func TestDashboardScrollHint(t *testing.T) {
	v := viewport.New()
	v.SetWidth(40)
	v.SetHeight(3)
	v.SetContent("short")
	if got := dashboardScrollHint(v); got != "" {
		t.Errorf("non-overflow yields empty hint; got %q", got)
	}
	v.SetContent(strings.Repeat("line\n", 30))
	v.GotoTop()
	if got := dashboardScrollHint(v); !strings.Contains(got, "▼") {
		t.Errorf("top should show ▼; got %q", got)
	}
	v.GotoBottom()
	if got := dashboardScrollHint(v); !strings.Contains(got, "▲") {
		t.Errorf("bottom should show ▲; got %q", got)
	}
}

func TestLogsModel_SoftWrapEnabled(t *testing.T) {
	m := NewLogsModel("ns")
	if !m.logView.SoftWrap {
		t.Error("logs viewport must have SoftWrap=true so long log lines don't clip silently")
	}
}
