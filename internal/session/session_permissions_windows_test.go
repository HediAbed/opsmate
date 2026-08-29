//go:build windows

package session

import (
	"os"
	"testing"
)

func assertPrivateSessionPermissions(t *testing.T, directoryInfo, fileInfo os.FileInfo) {
	t.Helper()
	if !directoryInfo.IsDir() {
		t.Fatalf("session directory mode = %v, want directory", directoryInfo.Mode())
	}
	if !fileInfo.Mode().IsRegular() {
		t.Fatalf("session file mode = %v, want regular file", fileInfo.Mode())
	}
}
