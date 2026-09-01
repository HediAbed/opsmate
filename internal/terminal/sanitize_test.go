package terminal

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestSanitizeTerminalTextRemovesControlSequences(t *testing.T) {
	input := "before\x1b]52;c;payload\a\x1b[2Jafter\rhidden\x00"
	got := SanitizeText(input)
	if got != "beforeafterhidden" {
		t.Fatalf("sanitized text = %q, want %q", got, "beforeafterhidden")
	}
}

func TestSanitizeTerminalTextPreservesFormatting(t *testing.T) {
	input := "first\nsecond\tvalue"
	if got := SanitizeText(input); got != input {
		t.Fatalf("sanitized text = %q, want %q", got, input)
	}
}

func TestSanitizeTerminalLinesPreservesLineCount(t *testing.T) {
	got := SanitizeLines([]string{"safe", "\x1b[31mred\x1b[0m"})
	if len(got) != 2 || got[0] != "safe" || got[1] != "red" {
		t.Fatalf("sanitized lines = %#v", got)
	}
}

func TestSanitizeTerminalLineNormalizesAndTruncatesLongText(t *testing.T) {
	input := "  " + strings.Repeat("é", maximumLineRunes+10) + "\n trailing"
	got := SanitizeLine(input)

	if utf8.RuneCountInString(got) != maximumLineRunes {
		t.Fatalf("line length = %d runes, want %d", utf8.RuneCountInString(got), maximumLineRunes)
	}
	if !strings.HasSuffix(got, "…") {
		t.Fatalf("line = %q, want ellipsis suffix", got)
	}
}
