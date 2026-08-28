package model

import (
	"reflect"
	"testing"
)

func TestFormatPVCAccessModes_AbbreviatesKnownModes(t *testing.T) {
	got := formatPVCAccessModes([]string{
		"ReadWriteOnce",
		"ReadOnlyMany",
		"ReadWriteMany",
		"ReadWriteOncePod",
	})
	want := []string{"RWO", "ROX", "RWX", "RWOP"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestFormatPVCAccessModes_PreservesUnknownModes(t *testing.T) {
	got := formatPVCAccessModes([]string{"ReadWriteOnce", "MadeUpMode"})
	want := []string{"RWO", "MadeUpMode"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("unknown mode should pass through unchanged; got %+v want %+v", got, want)
	}
}

func TestFormatPVCAccessModes_EmptyInputYieldsEmptyOutput(t *testing.T) {
	if got := formatPVCAccessModes(nil); len(got) != 0 {
		t.Errorf("nil should yield empty slice; got %+v", got)
	}
}
