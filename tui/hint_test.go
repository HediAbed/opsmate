package tui

import "testing"

func TestAppendHint_DropsHintWhenCombinedOverflows(t *testing.T) {
	bar := "h:help q:quit"
	if got := AppendHint(bar, "30%", 5); got != bar {
		t.Errorf("hint must be dropped when combined width exceeds width; got %q", got)
	}
}

func TestAppendHint_AppendsAfterBarWhenFits(t *testing.T) {
	if got := AppendHint("h:help", "30%", 200); got != "h:help 30%" {
		t.Errorf("hint must follow the bar after a space; got %q", got)
	}
}

func TestAppendHint_ExactWidthStillFits(t *testing.T) {
	if got := AppendHint("ab", "cd", 5); got != "ab cd" {
		t.Errorf("combined width equal to width must keep the hint; got %q", got)
	}
	if got := AppendHint("ab", "cd", 4); got != "ab" {
		t.Errorf("one cell short must drop the hint; got %q", got)
	}
}

func TestAppendHint_EmptyHintLeavesBarUntouched(t *testing.T) {
	if got := AppendHint("h:help", "", 200); got != "h:help" {
		t.Errorf("empty hint must return bar with no trailing space; got %q", got)
	}
}

func TestAppendHint_EmptyBarTakesFittingHintWithoutLeadingSpace(t *testing.T) {
	if got := AppendHint("", "30%", 3); got != "30%" {
		t.Errorf("empty bar must take an exactly fitting hint alone; got %q", got)
	}
}

func TestAppendHint_EmptyBarDropsOverflowingHint(t *testing.T) {
	if got := AppendHint("", "30%", 2); got != "" {
		t.Errorf("empty bar must stay empty when the hint overflows; got %q", got)
	}
}

func TestAppendHint_BothEmpty(t *testing.T) {
	if got := AppendHint("", "", 10); got != "" {
		t.Errorf("empty bar and hint must stay empty; got %q", got)
	}
}

func TestAppendHint_MeasuresStyledHintByDisplayWidth(t *testing.T) {
	styled := "\x1b[2m30%\x1b[0m"
	if got := AppendHint("help", styled, 8); got != "help "+styled {
		t.Errorf("ANSI-styled hint of display width 3 must fit in 8; got %q", got)
	}
	if got := AppendHint("help", styled, 7); got != "help" {
		t.Errorf("ANSI-styled hint must be dropped at width 7; got %q", got)
	}
}
