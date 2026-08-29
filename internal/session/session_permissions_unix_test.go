//go:build !windows

package session

import (
	"os"
	"testing"
)

func assertPrivateSessionPermissions(t *testing.T, directoryInfo, fileInfo os.FileInfo) {
	t.Helper()
	if directoryInfo.Mode().Perm() != sessionDirectoryMode {
		t.Fatalf("directory mode = %o, want %o", directoryInfo.Mode().Perm(), sessionDirectoryMode)
	}
	if fileInfo.Mode().Perm() != sessionFileMode {
		t.Fatalf("file mode = %o, want %o", fileInfo.Mode().Perm(), sessionFileMode)
	}
}
