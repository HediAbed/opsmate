package service

import "testing"

func TestFormatLabelMap_Empty(t *testing.T) {
	if got := formatLabelMap(nil); got != "" {
		t.Errorf("nil label map should render empty; got %q", got)
	}
	if got := formatLabelMap(map[string]string{}); got != "" {
		t.Errorf("empty label map should render empty; got %q", got)
	}
}

func TestFormatLabelMap_SortedDeterministic(t *testing.T) {
	in := map[string]string{"app": "web", "tier": "frontend", "env": "prod"}
	got := formatLabelMap(in)
	want := "app=web,env=prod,tier=frontend"
	if got != want {
		t.Errorf("formatLabelMap sorted output mismatch:\n got %q\nwant %q", got, want)
	}
}

func TestJoinOrNone_EmptyReturnsAngleNone(t *testing.T) {
	if got := joinOrNone(nil); got != "<none>" {
		t.Errorf("nil should yield <none>; got %q", got)
	}
	if got := joinOrNone([]string{}); got != "<none>" {
		t.Errorf("empty should yield <none>; got %q", got)
	}
}

func TestJoinOrNone_JoinsWithCommas(t *testing.T) {
	if got := joinOrNone([]string{"a", "b"}); got != "a,b" {
		t.Errorf("expected a,b; got %q", got)
	}
}
