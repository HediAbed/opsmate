package component

import "testing"

func TestFitModalWidth(t *testing.T) {
	cases := []struct {
		name     string
		desired  int
		terminal int
		want     int
	}{
		{"zero desired", 0, 80, 0},
		{"negative desired", -5, 80, 0},
		{"zero terminal", 60, 0, 0},
		{"negative terminal", 60, -10, 0},
		{"both non-positive", -1, -1, 0},
		{"wide terminal fits desired", 60, 200, 60},
		{"exact fit including gutters", 60, 64, 60},
		{"clamps to terminal minus gutters", 60, 50, 46},
		{"narrowest terminal that keeps gutters", 60, 5, 1},
		{"too narrow for gutters claims full terminal", 60, 4, 4},
		{"single-cell terminal", 60, 1, 1},
		{"tiny desired on tiny terminal stays bounded", 2, 3, 2},
		{"desired narrower than guttered terminal", 10, 80, 10},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := FitModalWidth(c.desired, c.terminal); got != c.want {
				t.Errorf("FitModalWidth(%d,%d)=%d want %d", c.desired, c.terminal, got, c.want)
			}
		})
	}
}
