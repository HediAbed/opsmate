package theme

import "testing"

func TestBoxContentWidth_SubtractsBorderAndPadding(t *testing.T) {
	if got := BoxContentWidth(100); got != 96 {
		t.Errorf("BoxContentWidth(100) = %d, want 96", got)
	}
	if got := BoxContentWidth(4); got != 0 {
		t.Errorf("BoxContentWidth(4) = %d, want 0", got)
	}
	if got := BoxContentWidth(0); got != -4 {
		t.Errorf("BoxContentWidth(0) = %d, want -4", got)
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
