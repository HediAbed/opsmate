package dashboard

import (
	"strings"
	"testing"

	"charm.land/bubbles/v2/viewport"
)

func TestDashboardBodyOverflowDetection(t *testing.T) {
	tests := []struct {
		name    string
		height  int
		content string
		want    bool
	}{
		{name: "content fits", height: 10, content: "one line", want: false},
		{name: "content overflows", height: 3, content: strings.Repeat("line\n", 30), want: true},
		{name: "zero height", height: 0, content: strings.Repeat("line\n", 30), want: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			model := DashboardModel{bodyView: viewport.New()}
			model.bodyView.SetWidth(40)
			model.bodyView.SetHeight(test.height)
			model.bodyView.SetContent(test.content)
			if got := model.bodyOverflows(); got != test.want {
				t.Errorf("bodyOverflows() = %t, want %t", got, test.want)
			}
		})
	}
}

func TestDashboardScrollHintTracksViewportPosition(t *testing.T) {
	view := viewport.New()
	view.SetWidth(40)
	view.SetHeight(3)
	view.SetContent("short")
	if got := dashboardScrollHint(view); got != "" {
		t.Errorf("non-overflow hint = %q", got)
	}
	view.SetContent(strings.Repeat("line\n", 30))
	view.GotoTop()
	if got := dashboardScrollHint(view); !strings.Contains(got, "▼") {
		t.Errorf("top hint = %q", got)
	}
	view.GotoBottom()
	if got := dashboardScrollHint(view); !strings.Contains(got, "▲") {
		t.Errorf("bottom hint = %q", got)
	}
}
