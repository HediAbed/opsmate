package service

import (
	"strings"
	"testing"
)

func TestSanitizeKubectlStderr_AwsSsoExpiry(t *testing.T) {
	raw := `aws: [ERROR]: Error when retrieving token from sso: Token has expired and refresh failed
E0508 15:20:26.553933 4052334 memcache.go:265] "Unhandled Error" err="couldn't get current server API group list: Get \"https://example.eks.amazonaws.com/api?timeout=32s\": getting credentials: exec: executable aws failed with exit code 255"

aws: [ERROR]: Error when retrieving token from sso: Token has expired and refresh failed
E0508 15:20:26.978369 4052334 memcache.go:265] "Unhandled Error" err="..."
aws: [ERROR]: Error when retrieving token from sso: Token has expired and refresh failed
E0508 15:20:27.387041 4052334 memcache.go:265] "Unhandled Error" err="..."
Unable to connect to the server: getting credentials: exec: executable aws failed with exit code 255`

	got := SanitizeKubectlStderr(raw)
	lines := strings.Split(got, "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %d:\n%s", len(lines), got)
	}
	if !strings.Contains(lines[0], "Token has expired") {
		t.Errorf("first line should surface the SSO message, got %q", lines[0])
	}
	if !strings.Contains(lines[1], "Unable to connect") {
		t.Errorf("last line should surface the final summary, got %q", lines[1])
	}
}

func TestSanitizeKubectlStderr_DropsKlogLines(t *testing.T) {
	raw := `E0508 15:20:26.553933 4052334 memcache.go:265] "Unhandled Error" err="..."
W0508 15:20:27.000000 1234567 informer.go:42] cache resync
the real error message`
	got := SanitizeKubectlStderr(raw)
	if got != "the real error message" {
		t.Errorf("expected klog lines stripped, got %q", got)
	}
}

func TestSanitizeKubectlStderr_DedupesConsecutive(t *testing.T) {
	raw := "connection refused\nconnection refused\nconnection refused"
	got := SanitizeKubectlStderr(raw)
	if got != "connection refused" {
		t.Errorf("consecutive duplicates should collapse to one line, got %q", got)
	}
}

func TestSanitizeKubectlStderr_EmptyInputReturnsEmpty(t *testing.T) {
	if got := SanitizeKubectlStderr(""); got != "" {
		t.Errorf("empty input should return empty, got %q", got)
	}
}

func TestSanitizeKubectlStderr_AllNoiseFallsBackToTruncatedRaw(t *testing.T) {
	raw := `E0508 15:20:26.553933 4052334 memcache.go:265] "Unhandled Error" err="..."
W0508 15:20:27.000000 1234567 informer.go:42] cache resync`
	got := SanitizeKubectlStderr(raw)
	if got == "" {
		t.Error("when every line is noise, fall back to truncated raw rather than empty banner")
	}
}

func TestSanitizeKubectlStderr_TruncatesLongLines(t *testing.T) {
	long := strings.Repeat("x", maxSanitizedChars*2)
	got := SanitizeKubectlStderr(long)
	if len([]rune(got)) > maxSanitizedChars {
		t.Errorf("line length %d exceeds cap %d", len([]rune(got)), maxSanitizedChars)
	}
	if !strings.HasSuffix(got, "…") {
		t.Error("truncated line should end with …")
	}
}

func TestSanitizeKubectlStderr_RemovesTerminalControlsAndRepairsUTF8(t *testing.T) {
	raw := "\x1b]52;c;payload\a\x1b[2Jbefore\x00\x9fafter\xff"
	got := SanitizeKubectlStderr(raw)
	if got != "beforeafter�" {
		t.Fatalf("sanitized stderr = %q, want %q", got, "beforeafter�")
	}
}

func TestSanitizeKubectlStderr_TruncatesOnRuneBoundary(t *testing.T) {
	got := SanitizeKubectlStderr(strings.Repeat("🙂", maxSanitizedChars+1))
	if !strings.HasSuffix(got, "…") {
		t.Fatalf("sanitized stderr = %q, want truncation marker", got)
	}
	if len([]rune(got)) != maxSanitizedChars {
		t.Fatalf("sanitized stderr has %d runes, want %d", len([]rune(got)), maxSanitizedChars)
	}
}

func TestSanitizeKubectlStderr_KeepsSingleMeaningfulLineAsIs(t *testing.T) {
	raw := "Error from server (NotFound): pods \"web-abc\" not found"
	got := SanitizeKubectlStderr(raw)
	if got != raw {
		t.Errorf("single short meaningful line should pass through, got %q", got)
	}
}

func TestSanitizeKubectlStderr_FirstAndLastFromManyLines(t *testing.T) {
	raw := "first line\nmiddle 1\nmiddle 2\nmiddle 3\nlast line"
	got := SanitizeKubectlStderr(raw)
	lines := strings.Split(got, "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines (first+last), got %d", len(lines))
	}
	if lines[0] != "first line" || lines[1] != "last line" {
		t.Errorf("expected first+last, got %q + %q", lines[0], lines[1])
	}
}
