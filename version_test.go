package opsmate

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

var semanticVersionPattern = regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-(0|[1-9][0-9]*|[0-9A-Za-z-]*[A-Za-z-][0-9A-Za-z-]*)(\.(0|[1-9][0-9]*|[0-9A-Za-z-]*[A-Za-z-][0-9A-Za-z-]*))*)?(\+[0-9A-Za-z-]+(\.[0-9A-Za-z-]+)*)?$`)

func TestVersionReadsVersionFile(t *testing.T) {
	versionFile, err := os.ReadFile("VERSION")
	if err != nil {
		t.Fatalf("read VERSION: %v", err)
	}
	expectedVersion := strings.TrimSpace(string(versionFile))
	if actualVersion := Version(); actualVersion != expectedVersion {
		t.Fatalf("Version() = %q, want %q", actualVersion, expectedVersion)
	}
	if !semanticVersionPattern.MatchString(expectedVersion) {
		t.Fatalf("VERSION %q is not a semantic version", expectedVersion)
	}
}
