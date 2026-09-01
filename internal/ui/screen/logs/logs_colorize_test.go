package logs

import (
	"strconv"
	"testing"
)

func TestColorizeLines_CacheReusesRender(t *testing.T) {
	m := newTestLogsModel("default")
	lines := []string{"first error here", "second normal line"}

	first := m.colorizeLines(lines)
	if got := len(m.colorizeCache); got != 2 {
		t.Errorf("expected 2 cached entries after first colorize, got %d", got)
	}

	second := m.colorizeLines(lines)
	if first != second {
		t.Error("repeated colorize on identical lines must produce identical output")
	}
	if got := len(m.colorizeCache); got != 2 {
		t.Errorf("cache should not grow on repeated render of same lines, got %d", got)
	}
}

func TestColorizeLines_NewLinesAddToCache(t *testing.T) {
	m := newTestLogsModel("default")
	m.colorizeLines([]string{"alpha"})
	if got := len(m.colorizeCache); got != 1 {
		t.Fatalf("expected 1 cached entry, got %d", got)
	}
	m.colorizeLines([]string{"alpha", "beta"})
	if got := len(m.colorizeCache); got != 2 {
		t.Errorf("expected 2 cached entries after adding 'beta', got %d", got)
	}
}

func TestColorizeLines_CacheEvictsAtCap(t *testing.T) {
	m := newTestLogsModel("default")
	lines := make([]string, maxColorizeCacheSize+10)
	for i := range lines {
		lines[i] = "unique-line-" + strconv.Itoa(i)
	}
	m.colorizeLines(lines)
	if got := len(m.colorizeCache); got > maxColorizeCacheSize {
		t.Errorf("cache size %d exceeds cap %d", got, maxColorizeCacheSize)
	}
}

func TestSetPod_ResetsColorizeCache(t *testing.T) {
	m := newTestLogsModel("default")
	m.colorizeLines([]string{"line"})
	if len(m.colorizeCache) == 0 {
		t.Fatal("precondition: cache must contain entry before reset")
	}
	m.SetPod("new-pod")
	if len(m.colorizeCache) != 0 {
		t.Errorf("SetPod must clear colorize cache; size=%d", len(m.colorizeCache))
	}
}

func TestSetNamespace_ResetsColorizeCache(t *testing.T) {
	m := newTestLogsModel("default")
	m.colorizeLines([]string{"line"})
	m.SetNamespace("other")
	if len(m.colorizeCache) != 0 {
		t.Errorf("SetNamespace must clear colorize cache; size=%d", len(m.colorizeCache))
	}
}

func TestRenderedLine_RespectsSeverity(t *testing.T) {
	m := newTestLogsModel("default")
	cases := map[string]lineSeverity{
		"FATAL: process exited":      sevCritical,
		"error connecting to db":     sevError,
		"warn: slow query":           sevWarn,
		"  at com.example.Foo.bar()": sevStack,
		"DEBUG: connection pool":     sevDebug,
		"normal output":              sevNone,
	}
	for line, want := range cases {
		got := m.renderedLine(line)
		if got.severity != want {
			t.Errorf("line %q: severity=%v, want %v", line, got.severity, want)
		}
	}
}
