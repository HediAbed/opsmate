package component

import "testing"

func TestNounForCount(t *testing.T) {
	tests := []struct {
		name  string
		count int
		want  string
	}{
		{name: "zero", count: 0, want: "items"},
		{name: "one", count: 1, want: "item"},
		{name: "many", count: 2, want: "items"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := NounForCount("item", "items", test.count); got != test.want {
				t.Errorf("NounForCount count %d = %q, want %q", test.count, got, test.want)
			}
		})
	}
}
