package service

import (
	"os"
	"syscall"
	"testing"
)

const (
	testPrivateFileMode    = 0o600
	testExecutableFileMode = 0o700
)

func writeTestExecutable(t *testing.T, path, contents string) {
	t.Helper()
	syscall.ForkLock.RLock()
	defer syscall.ForkLock.RUnlock()

	stagingPath := path + ".pending"
	if err := os.WriteFile(stagingPath, []byte(contents), testPrivateFileMode); err != nil {
		t.Fatalf("write test executable: %v", err)
	}
	if err := os.Chmod(stagingPath, testExecutableFileMode); err != nil {
		t.Fatalf("make test file executable: %v", err)
	}
	if err := os.Rename(stagingPath, path); err != nil {
		t.Fatalf("publish test executable: %v", err)
	}
}

func readTestFile(t *testing.T, path string) string {
	t.Helper()
	// #nosec G304 -- tests only pass paths created inside isolated temporary directories.
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read test file: %v", err)
	}
	return string(data)
}
