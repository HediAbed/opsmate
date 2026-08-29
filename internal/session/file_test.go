package session

import (
	"os"
	"testing"
)

func readTestFile(t *testing.T, path string) string {
	t.Helper()
	// #nosec G304 -- tests only pass paths created inside isolated temporary directories.
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read test file: %v", err)
	}
	return string(data)
}
