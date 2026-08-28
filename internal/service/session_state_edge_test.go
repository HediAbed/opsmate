//go:build !windows

package service

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type sessionTestFile struct {
	reader   io.Reader
	info     os.FileInfo
	readErr  error
	writeErr error
	syncErr  error
	closeErr error
	statErr  error
}

func (file *sessionTestFile) Read(data []byte) (int, error) {
	if file.readErr != nil {
		return 0, file.readErr
	}
	if file.reader == nil {
		return 0, io.EOF
	}
	return file.reader.Read(data)
}

func (file *sessionTestFile) Write(data []byte) (int, error) {
	if file.writeErr != nil {
		return 0, file.writeErr
	}
	return len(data), nil
}

func (file *sessionTestFile) Close() error { return file.closeErr }

func (file *sessionTestFile) Stat() (os.FileInfo, error) {
	return file.info, file.statErr
}

func (file *sessionTestFile) Sync() error { return file.syncErr }

type sessionTestRoot struct {
	lstatInfo      os.FileInfo
	lstatErr       error
	statInfo       os.FileInfo
	statErr        error
	openResult     sessionFile
	openErr        error
	openFileResult sessionFile
	openFileErr    error
	chmodErr       error
	closeErr       error
	removeErr      error
	renameErr      error
	removeCalls    int
}

func (root *sessionTestRoot) Chmod(string, os.FileMode) error { return root.chmodErr }
func (root *sessionTestRoot) Close() error                    { return root.closeErr }

func (root *sessionTestRoot) Lstat(string) (os.FileInfo, error) {
	return root.lstatInfo, root.lstatErr
}

func (root *sessionTestRoot) Open(string) (sessionFile, error) {
	return root.openResult, root.openErr
}

func (root *sessionTestRoot) OpenFile(string, int, os.FileMode) (sessionFile, error) {
	return root.openFileResult, root.openFileErr
}

func (root *sessionTestRoot) Remove(string) error {
	root.removeCalls++
	return root.removeErr
}

func (root *sessionTestRoot) Rename(string, string) error { return root.renameErr }

func (root *sessionTestRoot) Stat(string) (os.FileInfo, error) {
	return root.statInfo, root.statErr
}

func sessionTestFileInfo(t *testing.T, name string) os.FileInfo {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte("state"), sessionFileMode); err != nil {
		t.Fatalf("write test file: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat test file: %v", err)
	}
	return info
}

func sessionTestDirectoryInfo(t *testing.T) os.FileInfo {
	t.Helper()
	info, err := os.Stat(t.TempDir())
	if err != nil {
		t.Fatalf("stat test directory: %v", err)
	}
	return info
}

func TestSessionErrorFormatsContextAndUnwraps(t *testing.T) {
	sentinel := errors.New("failed")
	tests := []struct {
		err  *SessionError
		want string
	}{
		{err: &SessionError{}, want: "session: unknown error"},
		{err: &SessionError{Operation: "load"}, want: "session load: unknown error"},
		{err: &SessionError{Path: "state"}, want: "session state: unknown error"},
		{err: &SessionError{Operation: "read", Path: "state", Err: sentinel}, want: "session read state: failed"},
	}
	for _, test := range tests {
		if got := test.err.Error(); got != test.want {
			t.Errorf("error = %q, want %q", got, test.want)
		}
	}
	if !errors.Is(&SessionError{Err: sentinel}, sentinel) {
		t.Fatal("session error did not unwrap its cause")
	}
}

func TestSessionPathRejectsRelativeConfigDirectory(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "relative")
	if _, err := sessionPath(); err == nil {
		t.Fatal("relative configuration directory did not fail")
	}
	if _, err := LoadSession(); err == nil {
		t.Fatal("load did not propagate path resolution failure")
	}
	if err := SaveSession(SessionState{}); err == nil {
		t.Fatal("save did not propagate path resolution failure")
	}
}

func TestLoadSessionDistinguishesMissingFileAndUnsafeDirectory(t *testing.T) {
	configDirectory := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configDirectory)
	sessionDirectory := filepath.Join(configDirectory, "opsmate")
	if err := os.Mkdir(sessionDirectory, sessionDirectoryMode); err != nil {
		t.Fatalf("create session directory: %v", err)
	}
	if _, err := LoadSession(); !errors.Is(err, ErrNoSession) {
		t.Fatalf("missing state error = %v", err)
	}

	otherConfigDirectory := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", otherConfigDirectory)
	unsafePath := filepath.Join(otherConfigDirectory, "opsmate")
	if err := os.WriteFile(unsafePath, []byte("not a directory"), sessionFileMode); err != nil {
		t.Fatalf("create unsafe path: %v", err)
	}
	if _, err := LoadSession(); !errors.Is(err, ErrUnsafeSession) {
		t.Fatalf("unsafe directory error = %v", err)
	}
}

func TestSessionFileHelpersPropagateFilesystemFailures(t *testing.T) {
	directory := t.TempDir()
	root, err := os.OpenRoot(directory)
	if err != nil {
		t.Fatalf("open root: %v", err)
	}
	if err := root.Close(); err != nil {
		t.Fatalf("close root: %v", err)
	}
	closedRoot := rootedSessionDirectory{root: root}

	if _, err := openRegularSessionFile(closedRoot, sessionFileName); err == nil {
		t.Fatal("closed root did not fail file lookup")
	}
	if err := validateSessionDestination(closedRoot, sessionFileName); err == nil {
		t.Fatal("closed root did not fail destination validation")
	}
	if _, err := writeSessionTemp(closedRoot, []byte("state")); err == nil {
		t.Fatal("closed root did not fail temporary write")
	}
	if err := replaceSessionFile(closedRoot, "missing", sessionFileName); err == nil {
		t.Fatal("closed root did not fail replacement")
	}
	if err := syncSessionDirectory(closedRoot); err == nil {
		t.Fatal("closed root did not fail synchronization")
	}
}

func TestReadBoundedSessionPropagatesReaderFailure(t *testing.T) {
	sentinel := errors.New("read failed")
	if _, err := readBoundedSession(&failingProviderBody{readErr: sentinel}); !errors.Is(err, sentinel) {
		t.Fatalf("error = %v, want reader failure", err)
	}
}

func TestSaveSessionReportsDirectoryPreparationFailure(t *testing.T) {
	configFile := filepath.Join(t.TempDir(), "config-file")
	if err := os.WriteFile(configFile, []byte("file"), sessionFileMode); err != nil {
		t.Fatalf("write config file: %v", err)
	}
	t.Setenv("XDG_CONFIG_HOME", configFile)

	err := SaveSession(SessionState{})

	var sessionErr *SessionError
	if !errors.As(err, &sessionErr) || sessionErr.Operation != "prepare directory" {
		t.Fatalf("error = %#v, want directory preparation failure", err)
	}
}

func TestSaveSessionCleansTemporaryFileAfterReplaceFailure(t *testing.T) {
	configDirectory := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configDirectory)
	path, err := sessionPath()
	if err != nil {
		t.Fatalf("session path: %v", err)
	}

	err = saveSessionWithEncoder(SessionState{}, func(SessionState) ([]byte, error) {
		if err := os.Mkdir(path, sessionDirectoryMode); err != nil {
			return nil, err
		}
		return []byte(`{}`), nil
	})

	var sessionErr *SessionError
	if !errors.As(err, &sessionErr) || sessionErr.Operation != "replace" {
		t.Fatalf("error = %#v, want replacement failure", err)
	}
	entries, readErr := os.ReadDir(filepath.Dir(path))
	if readErr != nil {
		t.Fatalf("read session directory: %v", readErr)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), sessionTempPrefix) {
			t.Fatalf("temporary state remained after replacement failure: %s", entry.Name())
		}
	}
}

func TestSaveSessionReportsTemporaryWriteFailure(t *testing.T) {
	configDirectory := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configDirectory)
	path, err := sessionPath()
	if err != nil {
		t.Fatalf("session path: %v", err)
	}

	err = saveSessionWithEncoder(SessionState{}, func(SessionState) ([]byte, error) {
		if err := os.RemoveAll(filepath.Dir(path)); err != nil {
			return nil, err
		}
		return []byte(`{}`), nil
	})

	var sessionErr *SessionError
	if !errors.As(err, &sessionErr) || sessionErr.Operation != "write temporary state" {
		t.Fatalf("error = %#v, want temporary-write failure", err)
	}
}

func TestOpenOperatingSystemSessionRootReportsOpenFailure(t *testing.T) {
	missingDirectory := filepath.Join(t.TempDir(), "missing")
	if _, err := openOperatingSystemSessionRoot(missingDirectory); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("error = %v, want missing directory", err)
	}
}

func TestOpenSessionRootWithValidatesOpenedDirectory(t *testing.T) {
	expected := sessionTestDirectoryInfo(t)
	sentinel := errors.New("open failed")
	if _, err := openSessionRootWith(
		"directory",
		func(string) (os.FileInfo, error) { return expected, nil },
		func(string) (sessionRoot, error) { return nil, sentinel },
	); !errors.Is(err, sentinel) {
		t.Fatalf("open error = %v, want sentinel", err)
	}

	statFailure := errors.New("stat failed")
	closeFailure := errors.New("close failed")
	root := &sessionTestRoot{statErr: statFailure, closeErr: closeFailure}
	if _, err := openSessionRootWith(
		"directory",
		func(string) (os.FileInfo, error) { return expected, nil },
		func(string) (sessionRoot, error) { return root, nil },
	); !errors.Is(err, statFailure) || !errors.Is(err, closeFailure) || !errors.Is(err, ErrUnsafeSession) {
		t.Fatalf("stat error = %v, want stat, close, and unsafe errors", err)
	}

	differentDirectory := sessionTestDirectoryInfo(t)
	root = &sessionTestRoot{statInfo: differentDirectory}
	if _, err := openSessionRootWith(
		"directory",
		func(string) (os.FileInfo, error) { return expected, nil },
		func(string) (sessionRoot, error) { return root, nil },
	); !errors.Is(err, ErrUnsafeSession) {
		t.Fatalf("identity error = %v, want ErrUnsafeSession", err)
	}
}

func TestOpenRegularSessionFileValidatesOpenedFile(t *testing.T) {
	expected := sessionTestFileInfo(t, "expected")
	sentinel := errors.New("open failed")
	root := &sessionTestRoot{lstatInfo: expected, openErr: sentinel}
	if _, err := openRegularSessionFile(root, sessionFileName); !errors.Is(err, sentinel) {
		t.Fatalf("open error = %v, want sentinel", err)
	}

	statFailure := errors.New("stat failed")
	closeFailure := errors.New("close failed")
	root = &sessionTestRoot{
		lstatInfo:  expected,
		openResult: &sessionTestFile{statErr: statFailure, closeErr: closeFailure},
	}
	if _, err := openRegularSessionFile(root, sessionFileName); !errors.Is(err, statFailure) || !errors.Is(err, closeFailure) || !errors.Is(err, ErrUnsafeSession) {
		t.Fatalf("stat error = %v, want stat, close, and unsafe errors", err)
	}

	differentFile := sessionTestFileInfo(t, "different")
	root = &sessionTestRoot{
		lstatInfo:  expected,
		openResult: &sessionTestFile{info: differentFile},
	}
	if _, err := openRegularSessionFile(root, sessionFileName); !errors.Is(err, ErrUnsafeSession) {
		t.Fatalf("identity error = %v, want ErrUnsafeSession", err)
	}
}

func TestLoadSessionAtPathReportsDirectoryCloseFailure(t *testing.T) {
	fileInfo := sessionTestFileInfo(t, sessionFileName)
	closeFailure := errors.New("close failed")
	root := &sessionTestRoot{
		lstatInfo: fileInfo,
		openResult: &sessionTestFile{
			reader: strings.NewReader(`{"namespace":"testing"}`),
			info:   fileInfo,
		},
		closeErr: closeFailure,
	}

	state, err := loadSessionAtPath(
		filepath.Join("config", sessionFileName),
		func(string) (sessionRoot, error) { return root, nil },
	)
	if state.Namespace != "testing" || !errors.Is(err, closeFailure) {
		t.Fatalf("load = (%#v, %v), want decoded state and close failure", state, err)
	}
	var sessionErr *SessionError
	if !errors.As(err, &sessionErr) || sessionErr.Operation != "close directory" {
		t.Fatalf("error = %#v, want close-directory SessionError", err)
	}
}

func TestSaveSessionAtPathReportsOpenDirectoryFailure(t *testing.T) {
	sentinel := errors.New("failed")
	err := saveSessionWithRoot(func(string) (sessionRoot, error) { return nil, sentinel })
	assertSessionOperationError(t, err, "open directory", sentinel)
}

func TestSaveSessionAtPathReportsCloseDirectoryFailure(t *testing.T) {
	sentinel := errors.New("failed")
	root := successfulSessionTestRoot()
	root.closeErr = sentinel
	err := saveSessionWithRoot(func(string) (sessionRoot, error) { return root, nil })
	assertSessionOperationError(t, err, "close directory", sentinel)
}

func TestSaveSessionAtPathReportsReplacementFailure(t *testing.T) {
	sentinel := errors.New("failed")
	root := successfulSessionTestRoot()
	root.renameErr = sentinel
	root.removeErr = os.ErrNotExist
	err := saveSessionWithRoot(func(string) (sessionRoot, error) { return root, nil })
	assertSessionOperationError(t, err, "replace", sentinel)
	if errors.Is(err, os.ErrNotExist) {
		t.Fatalf("cleanup absence leaked into replacement error: %v", err)
	}
}

func saveSessionWithRoot(openRoot func(string) (sessionRoot, error)) error {
	return saveSessionAtPath(
		SessionState{},
		filepath.Join("config", sessionFileName),
		func(SessionState) ([]byte, error) { return []byte(`{}`), nil },
		func(string) error { return nil },
		openRoot,
	)
}

func assertSessionOperationError(t *testing.T, err error, operation string, cause error) {
	t.Helper()
	var sessionErr *SessionError
	if !errors.As(err, &sessionErr) {
		t.Fatalf("error = %#v, want SessionError", err)
	}
	if sessionErr.Operation != operation || !errors.Is(err, cause) {
		t.Fatalf("error = %#v, want %q failure", err, operation)
	}
}

func successfulSessionTestRoot() *sessionTestRoot {
	return &sessionTestRoot{
		lstatErr:       os.ErrNotExist,
		openFileResult: &sessionTestFile{},
		openResult:     &sessionTestFile{},
	}
}

func TestPrepareSessionDirectoryWithReportsFilesystemFailures(t *testing.T) {
	directoryInfo := sessionTestDirectoryInfo(t)
	sentinel := errors.New("failed")
	create := func(string, os.FileMode) error { return nil }
	stat := func(string) (os.FileInfo, error) { return directoryInfo, nil }

	if err := prepareSessionDirectoryWith(
		"directory",
		create,
		func(string) (os.FileInfo, error) { return nil, sentinel },
		func(string) (sessionRoot, error) { return nil, nil },
	); !errors.Is(err, sentinel) {
		t.Fatalf("lstat error = %v, want sentinel", err)
	}
	if err := prepareSessionDirectoryWith(
		"directory",
		create,
		stat,
		func(string) (sessionRoot, error) { return nil, sentinel },
	); !errors.Is(err, sentinel) {
		t.Fatalf("open error = %v, want sentinel", err)
	}

	closeFailure := errors.New("close failed")
	root := &sessionTestRoot{statErr: sentinel, closeErr: closeFailure}
	if err := prepareSessionDirectoryWith(
		"directory",
		create,
		stat,
		func(string) (sessionRoot, error) { return root, nil },
	); !errors.Is(err, sentinel) || !errors.Is(err, closeFailure) || !errors.Is(err, ErrUnsafeSession) {
		t.Fatalf("stat error = %v, want stat, close, and unsafe errors", err)
	}

	differentDirectory := sessionTestDirectoryInfo(t)
	root = &sessionTestRoot{statInfo: differentDirectory}
	if err := prepareSessionDirectoryWith(
		"directory",
		create,
		stat,
		func(string) (sessionRoot, error) { return root, nil },
	); !errors.Is(err, ErrUnsafeSession) {
		t.Fatalf("identity error = %v, want ErrUnsafeSession", err)
	}
}

func TestWriteSessionTempReportsFileLifecycleFailures(t *testing.T) {
	writeFailure := errors.New("write failed")
	closeFailure := errors.New("close failed")
	removeFailure := errors.New("remove failed")
	root := &sessionTestRoot{
		openFileResult: &sessionTestFile{writeErr: writeFailure, closeErr: closeFailure},
		removeErr:      removeFailure,
	}
	if _, err := writeSessionTemp(root, []byte("state")); !errors.Is(err, writeFailure) || !errors.Is(err, closeFailure) || !errors.Is(err, removeFailure) {
		t.Fatalf("write error = %v, want write, close, and cleanup failures", err)
	}

	syncFailure := errors.New("sync failed")
	root = &sessionTestRoot{
		openFileResult: &sessionTestFile{syncErr: syncFailure, closeErr: closeFailure},
	}
	if _, err := writeSessionTemp(root, []byte("state")); !errors.Is(err, syncFailure) || !errors.Is(err, closeFailure) {
		t.Fatalf("sync error = %v, want sync and close failures", err)
	}

	root = &sessionTestRoot{
		openFileResult: &sessionTestFile{closeErr: closeFailure},
		removeErr:      os.ErrNotExist,
	}
	if _, err := writeSessionTemp(root, []byte("state")); !errors.Is(err, closeFailure) {
		t.Fatalf("close error = %v, want close failure", err)
	}
	if root.removeCalls != 1 {
		t.Fatalf("cleanup calls = %d, want 1", root.removeCalls)
	}
}
