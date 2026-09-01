package browser

import "testing"

func TestFormatBoolColumn_RendersTitleCase(t *testing.T) {
	if got := formatBoolColumn(true); got != "True" {
		t.Errorf("true = %q, want True", got)
	}
	if got := formatBoolColumn(false); got != "False" {
		t.Errorf("false = %q, want False", got)
	}
}
