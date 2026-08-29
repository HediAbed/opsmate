package session

import (
	"os"
	"path/filepath"
	"testing"
)

func useTestConfigDirectory(t *testing.T) string {
	t.Helper()
	return useTestConfigRoot(t, t.TempDir())
}

func useTestConfigRoot(t *testing.T, root string) string {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(root, "config"))
	t.Setenv("HOME", filepath.Join(root, "home"))
	t.Setenv("AppData", filepath.Join(root, "config"))
	configDirectory, err := os.UserConfigDir()
	if err != nil {
		t.Fatalf("resolve test config directory: %v", err)
	}
	return configDirectory
}

func useRelativeTestConfigDirectory(t *testing.T) {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", "relative")
	t.Setenv("HOME", "relative")
	t.Setenv("AppData", "relative")
}
