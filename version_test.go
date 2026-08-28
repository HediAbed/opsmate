package main

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

var semanticVersionPattern = regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-(0|[1-9][0-9]*|[0-9A-Za-z-]*[A-Za-z-][0-9A-Za-z-]*)(\.(0|[1-9][0-9]*|[0-9A-Za-z-]*[A-Za-z-][0-9A-Za-z-]*))*)?(\+[0-9A-Za-z-]+(\.[0-9A-Za-z-]+)*)?$`)

func TestCurrentVersionReadsVersionFile(t *testing.T) {
	versionFile, err := os.ReadFile("VERSION")
	if err != nil {
		t.Fatalf("read VERSION: %v", err)
	}
	expectedVersion := strings.TrimSpace(string(versionFile))
	if actualVersion := currentVersion(); actualVersion != expectedVersion {
		t.Fatalf("currentVersion() = %q, want %q", actualVersion, expectedVersion)
	}
	if !semanticVersionPattern.MatchString(expectedVersion) {
		t.Fatalf("VERSION %q is not a semantic version", expectedVersion)
	}
}
