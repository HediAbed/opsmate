package component

import "testing"

func TestTableCursorLabel(t *testing.T) {
	cases := []struct {
		name          string
		cursor, total int
		want          string
	}{
		{"empty table", 0, 0, ""},
		{"negative total", 0, -3, ""},
		{"single row", 0, 1, "1/1"},
		{"valid position", 3, 10, "4/10"},
		{"last row", 99, 100, "100/100"},
		{"negative cursor clamps to first row", -2, 4, "1/4"},
		{"cursor past end clamps to last row", 9, 5, "5/5"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := TableCursorLabel(c.cursor, c.total); got != c.want {
				t.Errorf("TableCursorLabel(%d,%d)=%q want %q", c.cursor, c.total, got, c.want)
			}
		})
	}
}
