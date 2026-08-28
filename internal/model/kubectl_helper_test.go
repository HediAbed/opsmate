//go:build !windows

package model

import (
	"os"
	"path/filepath"
	"testing"
)

func installFakeKubectl(t *testing.T, script string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "kubectl")
	writeTestExecutable(t, path, script)
	prev := os.Getenv("PATH")
	t.Cleanup(func() { _ = os.Setenv("PATH", prev) })
	if err := os.Setenv("PATH", dir+string(os.PathListSeparator)+prev); err != nil {
		t.Fatalf("set PATH: %v", err)
	}
	return path
}
