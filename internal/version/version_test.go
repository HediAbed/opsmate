package version

import (
	"runtime/debug"
	"testing"
)

func TestCurrentUsesInjectedRelease(t *testing.T) {
	if value := resolve(" 0.1.0 ", unavailableBuildInfo); value != "0.1.0" {
		t.Fatalf("Current() = %q, want 0.1.0", value)
	}
}

func TestCurrentUsesModuleVersion(t *testing.T) {
	readBuildInfo := func() (*debug.BuildInfo, bool) {
		return &debug.BuildInfo{Main: debug.Module{Version: "v1.2.3"}}, true
	}
	if value := resolve("", readBuildInfo); value != "1.2.3" {
		t.Fatalf("Current() = %q, want 1.2.3", value)
	}
}

func TestCurrentFallsBackToDevelopment(t *testing.T) {
	testCases := []struct {
		name      string
		build     *debug.BuildInfo
		available bool
	}{
		{name: "missing build information"},
		{name: "nil build information", available: true},
		{name: "empty module version", build: &debug.BuildInfo{}, available: true},
		{name: "development module", build: &debug.BuildInfo{Main: debug.Module{Version: "(devel)"}}, available: true},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			readBuildInfo := func() (*debug.BuildInfo, bool) {
				return testCase.build, testCase.available
			}
			if value := resolve("", readBuildInfo); value != development {
				t.Fatalf("Current() = %q, want %q", value, development)
			}
		})
	}
}

func unavailableBuildInfo() (*debug.BuildInfo, bool) {
	return nil, false
}
