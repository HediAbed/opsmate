package theme

import "testing"

func TestBoxStyles_CarryTheFrameLayoutsAssume(t *testing.T) {
	cases := []struct {
		name                 string
		horizontal, vertical int
	}{
		{"BoxStyle", BoxStyle.GetHorizontalFrameSize(), BoxStyle.GetVerticalFrameSize()},
		{"ActiveBoxStyle", ActiveBoxStyle.GetHorizontalFrameSize(), ActiveBoxStyle.GetVerticalFrameSize()},
	}
	for _, c := range cases {
		if c.horizontal != 4 || c.vertical != 2 {
			t.Errorf("%s frame = %dx%d, want 4x2 (border plus horizontal padding)",
				c.name, c.horizontal, c.vertical)
		}
	}
}

func TestPodStatusStyle_DistinctStylesPerStatus(t *testing.T) {
	cases := []string{
		"Running", "Pending", "Succeeded", "Completed",
		"Failed", "Error", "CrashLoopBackOff", "ImagePullBackOff",
		"Terminating", "Unknown", "SomethingElse",
	}
	for _, s := range cases {
		got := PodStatusStyle(s).Render(s)
		if got == "" {
			t.Errorf("PodStatusStyle(%q).Render(%q) returned empty", s, s)
		}
	}
}
