package service

import (
	"errors"
	"strings"
	"testing"
)

func TestScreenContextErrorFormatsKnownAndUnknownFailures(t *testing.T) {
	sentinel := errors.New("failed")
	tests := []struct {
		err  *ScreenContextError
		want string
	}{
		{err: &ScreenContextError{}, want: "screen context: unknown error"},
		{err: &ScreenContextError{Err: sentinel}, want: "screen context: failed"},
		{err: &ScreenContextError{Resource: BrowserPods, Err: sentinel}, want: `screen context "pods": failed`},
	}
	for _, test := range tests {
		if got := test.err.Error(); got != test.want {
			t.Errorf("error = %q, want %q", got, test.want)
		}
	}
	if !errors.Is(&ScreenContextError{Err: sentinel}, sentinel) {
		t.Fatal("screen context error did not unwrap its cause")
	}
}

func TestParseBrowserResourceKindAcceptsBothCatalogGroups(t *testing.T) {
	for _, value := range []string{string(BrowserPods), string(BrowserIngresses)} {
		resource, err := ParseBrowserResourceKind(value)
		if err != nil || string(resource) != value {
			t.Fatalf("parse %q = (%q, %v)", value, resource, err)
		}
	}
	if isSecondaryBrowserResource("unknown") {
		t.Fatal("unknown resource classified as secondary")
	}
	if isSecondaryBrowserResource(BrowserPods) {
		t.Fatal("primary resource classified as secondary")
	}
}

func TestBoundedContextWriterReportsExactCapacity(t *testing.T) {
	writer := &boundedContextWriter{remaining: 2}
	if !writer.addRecord([]byte("x")) {
		t.Fatal("record fitting exact capacity was rejected")
	}
	if !writer.Full() {
		t.Fatal("writer with no remaining capacity is not full")
	}
}

func TestDashboardContextStopsAtResourceAndEventBounds(t *testing.T) {
	longName := strings.Repeat("x", maxScreenFieldRunes)
	input := DashboardContextInput{
		Pods:        []Pod{{Name: longName}, {Name: longName}},
		Deployments: []Deployment{{Name: longName}},
		Events:      []Event{{Object: longName}, {Object: longName}},
	}
	for _, remaining := range []int{1, maxScreenFieldRunes + 100, 2*maxScreenFieldRunes + 200} {
		writer := &boundedContextWriter{remaining: remaining}
		writeDashboardResources(writer, input)
		if !writer.Full() {
			t.Fatalf("writer with capacity %d did not reach its bound", remaining)
		}
	}

	eventWriter := &boundedContextWriter{remaining: 1}
	writeDashboardResources(eventWriter, DashboardContextInput{
		Events: []Event{{Object: longName}, {Object: longName}},
	})
	if !eventWriter.Full() {
		t.Fatal("bounded dashboard events did not stop at capacity")
	}
}

func TestBrowserRowsStopWhenContextIsFull(t *testing.T) {
	writer := &boundedContextWriter{remaining: 1}
	writeRows(writer, []Pod{{Name: "pod"}, {Name: "second"}}, podContextRow)
	if !writer.Full() {
		t.Fatal("bounded browser rows did not stop at capacity")
	}
}

func TestContextValueBoundsHandleNonPositiveLimits(t *testing.T) {
	if got := boundedContextValue("value", 0); got != "" {
		t.Fatalf("bounded value = %q, want empty", got)
	}
	if got := truncateContextText("value", 0); got != "" {
		t.Fatalf("truncated value = %q, want empty", got)
	}
}
