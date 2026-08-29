package session

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadSession_MissingFileReturnsSentinel(t *testing.T) {
	useTestConfigDirectory(t)

	_, err := LoadSession()
	if !errors.Is(err, ErrNoSession) {
		t.Errorf("LoadSession on empty dir = %v; want ErrNoSession", err)
	}
}

func TestSaveThenLoadSession_Roundtrip(t *testing.T) {
	useTestConfigDirectory(t)

	want := SessionState{
		Namespace:    "kube-system",
		Screen:       2,
		ResourceType: "deployments",
	}
	if err := SaveSession(want); err != nil {
		t.Fatalf("SaveSession: %v", err)
	}

	got, err := LoadSession()
	if err != nil {
		t.Fatalf("LoadSession: %v", err)
	}
	if got != want {
		t.Errorf("round-trip = %+v; want %+v", got, want)
	}
}

func TestSaveSession_CreatesParentDirs(t *testing.T) {
	useTestConfigDirectory(t)

	if err := SaveSession(SessionState{Namespace: "ns"}); err != nil {
		t.Fatalf("SaveSession: %v", err)
	}
	path, err := sessionPath()
	if err != nil {
		t.Fatalf("session path: %v", err)
	}
	if _, err := os.Stat(filepath.Dir(path)); err != nil {
		t.Errorf("expected parent dir to be created: %v", err)
	}
}

func TestLoadSession_CorruptFileReturnsError(t *testing.T) {
	useTestConfigDirectory(t)

	path, err := sessionPath()
	if err != nil {
		t.Fatalf("session path: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), sessionDirectoryMode); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte("not json"), sessionFileMode); err != nil {
		t.Fatalf("write: %v", err)
	}

	_, err = LoadSession()
	if err == nil {
		t.Fatal("expected parse error for corrupt file")
	}
	if errors.Is(err, ErrNoSession) {
		t.Error("parse error must remain distinct from ErrNoSession")
	}
	var sessionErr *SessionError
	if !errors.As(err, &sessionErr) || sessionErr.Operation != "decode" {
		t.Fatalf("error = %#v, want decode SessionError", err)
	}
}

func TestSaveSession_AtomicReplaceDoesNotLeaveTmp(t *testing.T) {
	useTestConfigDirectory(t)

	if err := SaveSession(SessionState{Namespace: "a"}); err != nil {
		t.Fatalf("first save: %v", err)
	}
	if err := SaveSession(SessionState{Namespace: "b"}); err != nil {
		t.Fatalf("second save: %v", err)
	}

	path, err := sessionPath()
	if err != nil {
		t.Fatalf("session path: %v", err)
	}
	temporaryFiles, err := filepath.Glob(filepath.Join(filepath.Dir(path), ".state-*.tmp*"))
	if err != nil {
		t.Fatalf("glob temporary files: %v", err)
	}
	if len(temporaryFiles) != 0 {
		t.Fatalf("temporary files remain after replacement: %v", temporaryFiles)
	}
}

func TestSaveSession_AppliesPlatformStorageProtection(t *testing.T) {
	useTestConfigDirectory(t)
	if err := SaveSession(SessionState{Namespace: "private"}); err != nil {
		t.Fatalf("save session: %v", err)
	}
	path, err := sessionPath()
	if err != nil {
		t.Fatalf("session path: %v", err)
	}
	directoryInfo, err := os.Stat(filepath.Dir(path))
	if err != nil {
		t.Fatalf("stat session directory: %v", err)
	}
	fileInfo, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat session file: %v", err)
	}
	assertPrivateSessionPermissions(t, directoryInfo, fileInfo)
}

func TestLoadSession_RejectsOversizedState(t *testing.T) {
	useTestConfigDirectory(t)
	path, err := sessionPath()
	if err != nil {
		t.Fatalf("session path: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), sessionDirectoryMode); err != nil {
		t.Fatalf("create session directory: %v", err)
	}
	oversized := make([]byte, maxSessionStateBytes+1)
	if err := os.WriteFile(path, oversized, sessionFileMode); err != nil {
		t.Fatalf("write oversized state: %v", err)
	}
	_, err = LoadSession()
	if !errors.Is(err, ErrSessionTooLarge) {
		t.Fatalf("error = %v, want ErrSessionTooLarge", err)
	}
}

func TestSaveSession_RejectsNonRegularDestinationWithoutResidue(t *testing.T) {
	useTestConfigDirectory(t)
	path, err := sessionPath()
	if err != nil {
		t.Fatalf("session path: %v", err)
	}
	if err := os.MkdirAll(path, sessionDirectoryMode); err != nil {
		t.Fatalf("create unsafe destination: %v", err)
	}
	err = SaveSession(SessionState{Namespace: "private"})
	if !errors.Is(err, ErrUnsafeSession) {
		t.Fatalf("error = %v, want ErrUnsafeSession", err)
	}
	temporaryFiles, globErr := filepath.Glob(filepath.Join(filepath.Dir(path), ".state-*.tmp*"))
	if globErr != nil {
		t.Fatalf("glob temporary files: %v", globErr)
	}
	if len(temporaryFiles) != 0 {
		t.Fatalf("failed save left temporary files: %v", temporaryFiles)
	}
}

func TestSessionPersistence_RejectsSymlinkDestination(t *testing.T) {
	temporaryDirectory := t.TempDir()
	useTestConfigRoot(t, temporaryDirectory)

	path, err := sessionPath()
	if err != nil {
		t.Fatalf("session path: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), sessionDirectoryMode); err != nil {
		t.Fatalf("create session directory: %v", err)
	}
	externalPath := filepath.Join(temporaryDirectory, "external.json")
	const externalContents = `{"namespace":"external"}`
	if err := os.WriteFile(externalPath, []byte(externalContents), sessionFileMode); err != nil {
		t.Fatalf("write external state: %v", err)
	}
	if err := os.Symlink(externalPath, path); err != nil {
		t.Skipf("symbolic links are unavailable: %v", err)
	}

	if _, err := LoadSession(); !errors.Is(err, ErrUnsafeSession) {
		t.Fatalf("LoadSession error = %v, want ErrUnsafeSession", err)
	}
	if err := SaveSession(SessionState{Namespace: "replacement"}); !errors.Is(err, ErrUnsafeSession) {
		t.Fatalf("SaveSession error = %v, want ErrUnsafeSession", err)
	}
	externalData := readTestFile(t, externalPath)
	if externalData != externalContents {
		t.Fatalf("external state changed through symlink: %q", externalData)
	}
}

func TestSessionPersistence_RejectsSymlinkDirectory(t *testing.T) {
	temporaryDirectory := t.TempDir()
	configDirectory := useTestConfigRoot(t, temporaryDirectory)
	externalDirectory := filepath.Join(temporaryDirectory, "external")
	if err := os.MkdirAll(configDirectory, sessionDirectoryMode); err != nil {
		t.Fatalf("create config directory: %v", err)
	}
	if err := os.MkdirAll(externalDirectory, sessionDirectoryMode); err != nil {
		t.Fatalf("create external directory: %v", err)
	}
	if err := os.Symlink(externalDirectory, filepath.Join(configDirectory, "opsmate")); err != nil {
		t.Skipf("symbolic links are unavailable: %v", err)
	}

	if err := SaveSession(SessionState{Namespace: "private"}); !errors.Is(err, ErrUnsafeSession) {
		t.Fatalf("SaveSession error = %v, want ErrUnsafeSession", err)
	}
	if _, err := LoadSession(); !errors.Is(err, ErrUnsafeSession) {
		t.Fatalf("LoadSession error = %v, want ErrUnsafeSession", err)
	}
	if _, err := os.Stat(filepath.Join(externalDirectory, sessionFileName)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("external session state was created: %v", err)
	}
}

func TestSaveSession_ReportsEncodingFailureWithoutResidue(t *testing.T) {
	useTestConfigDirectory(t)
	encodingFailure := errors.New("encoding failed")

	err := saveSessionWithEncoder(SessionState{}, func(SessionState) ([]byte, error) {
		return nil, encodingFailure
	})
	if !errors.Is(err, encodingFailure) {
		t.Fatalf("saveSessionWithEncoder error = %v, want encoding failure", err)
	}
	var sessionErr *SessionError
	if !errors.As(err, &sessionErr) || sessionErr.Operation != "encode" {
		t.Fatalf("saveSessionWithEncoder error = %#v, want encode SessionError", err)
	}
	path, err := sessionPath()
	if err != nil {
		t.Fatalf("session path: %v", err)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("failed encoding created session state: %v", err)
	}
}
