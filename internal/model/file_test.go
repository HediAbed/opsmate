package model

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
	if err := publishTestExecutable(path, contents); err != nil {
		t.Fatalf("publish test executable: %v", err)
	}
}

func publishTestExecutable(path, contents string) error {
	syscall.ForkLock.RLock()
	defer syscall.ForkLock.RUnlock()

	stagingPath := path + ".pending"
	if err := os.WriteFile(stagingPath, []byte(contents), testPrivateFileMode); err != nil {
		return err
	}
	if err := os.Chmod(stagingPath, testExecutableFileMode); err != nil {
		return err
	}
	if err := os.Rename(stagingPath, path); err != nil {
		return err
	}
	return nil
}
