package browser

import "testing"

func TestFormatLabelSelector_EmptyMeansAll(t *testing.T) {
	if got := formatLabelSelector(nil); got != labelSelectorAll {
		t.Errorf("nil labels = %q, want %q", got, labelSelectorAll)
	}
	if got := formatLabelSelector(map[string]string{}); got != labelSelectorAll {
		t.Errorf("empty labels = %q, want %q", got, labelSelectorAll)
	}
}

func TestFormatLabelSelector_SortsKeysDeterministically(t *testing.T) {
	labels := map[string]string{"z": "1", "a": "2", "m": "3"}
	const want = "a=2,m=3,z=1"
	if got := formatLabelSelector(labels); got != want {
		t.Errorf("formatLabelSelector(%v) = %q, want %q", labels, got, want)
	}
}

func TestFormatLabelSelector_SingleKey(t *testing.T) {
	if got := formatLabelSelector(map[string]string{"app": "api"}); got != "app=api" {
		t.Errorf("single label = %q, want app=api", got)
	}
}
