package session

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/HediAbed/opsmate/internal/failure"
)

const (
	sessionDirectoryMode = 0o700
	sessionFileMode      = 0o600
	maxSessionStateBytes = 64 * 1024
	sessionFileName      = "state.json"
	sessionTempPrefix    = ".state-"
	sessionTempSuffix    = ".tmp"
)

type SessionState struct {
	Namespace    string `json:"namespace,omitempty"`
	Screen       int    `json:"screen,omitempty"`
	ResourceType string `json:"resourceType,omitempty"`
	Wide         bool   `json:"wide,omitempty"`
}

var (
	ErrNoSession       = errors.New("no saved session")
	ErrSessionTooLarge = errors.New("saved session exceeds safety limit")
	ErrUnsafeSession   = errors.New("saved session path is unsafe")
)

type Operation string

const (
	OperationLocateConfig        Operation = "locate config directory"
	OperationLoad                Operation = "load"
	OperationOpenDirectory       Operation = "open directory"
	OperationCloseDirectory      Operation = "close directory"
	OperationOpen                Operation = "open"
	OperationRead                Operation = "read"
	OperationDecode              Operation = "decode"
	OperationPrepareDirectory    Operation = "prepare directory"
	OperationValidateDestination Operation = "validate destination"
	OperationEncode              Operation = "encode"
	OperationWriteTemporary      Operation = "write temporary state"
	OperationReplace             Operation = "replace"
)

type SessionError struct {
	Operation Operation
	Path      string
	Err       error
}

type sessionFile interface {
	io.Reader
	io.Writer
	Close() error
	Stat() (os.FileInfo, error)
	Sync() error
}

type sessionRoot interface {
	Chmod(name string, mode os.FileMode) error
	Close() error
	Lstat(name string) (os.FileInfo, error)
	Open(name string) (sessionFile, error)
	OpenFile(name string, flag int, mode os.FileMode) (sessionFile, error)
	Remove(name string) error
	Rename(oldName, newName string) error
	Stat(name string) (os.FileInfo, error)
}

type rootedSessionDirectory struct {
	root *os.Root
}

func (directory rootedSessionDirectory) Chmod(name string, mode os.FileMode) error {
	return directory.root.Chmod(name, mode)
}

func (directory rootedSessionDirectory) Close() error {
	return directory.root.Close()
}

func (directory rootedSessionDirectory) Lstat(name string) (os.FileInfo, error) {
	return directory.root.Lstat(name)
}

func (directory rootedSessionDirectory) Open(name string) (sessionFile, error) {
	return directory.root.Open(name)
}

func (directory rootedSessionDirectory) OpenFile(name string, flag int, mode os.FileMode) (sessionFile, error) {
	return directory.root.OpenFile(name, flag, mode)
}

func (directory rootedSessionDirectory) Remove(name string) error {
	return directory.root.Remove(name)
}

func (directory rootedSessionDirectory) Rename(oldName, newName string) error {
	return directory.root.Rename(oldName, newName)
}

func (directory rootedSessionDirectory) Stat(name string) (os.FileInfo, error) {
	return directory.root.Stat(name)
}

type sessionRootOpener func(string) (sessionRoot, error)
type sessionPathInfo func(string) (os.FileInfo, error)
type sessionDirectoryCreator func(string, os.FileMode) error

func (e *SessionError) Error() string {
	if e == nil {
		return "session: unknown error"
	}
	prefix := "session"
	if e.Operation != "" {
		prefix += " " + string(e.Operation)
	}
	if e.Path != "" {
		prefix += " " + e.Path
	}
	if e.Err == nil {
		return prefix + ": unknown error"
	}
	return prefix + ": " + e.Err.Error()
}

func (e *SessionError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

func (e *SessionError) FailureCode() failure.Code {
	if e == nil || e.Err == nil {
		return failure.CodeUnknown
	}
	if errors.Is(e.Err, context.Canceled) {
		return failure.CodeCanceled
	}
	if errors.Is(e.Err, context.DeadlineExceeded) {
		return failure.CodeDeadlineExceeded
	}
	if errors.Is(e.Err, ErrNoSession) || errors.Is(e.Err, fs.ErrNotExist) {
		return failure.CodeNotFound
	}
	if errors.Is(e.Err, ErrUnsafeSession) || errors.Is(e.Err, ErrSessionTooLarge) {
		return failure.CodeInvalidArgument
	}
	if errors.Is(e.Err, fs.ErrPermission) {
		return failure.CodePermissionDenied
	}
	return failure.CodeInternal
}

func sessionPath() (string, error) {
	configDirectory, err := os.UserConfigDir()
	if err != nil {
		return "", &SessionError{Operation: OperationLocateConfig, Err: err}
	}
	return sessionPathFromConfigDirectory(configDirectory)
}

func sessionPathFromConfigDirectory(configDirectory string) (string, error) {
	if !filepath.IsAbs(configDirectory) {
		return "", &SessionError{Operation: OperationLocateConfig, Err: ErrUnsafeSession}
	}
	return filepath.Join(configDirectory, "opsmate", sessionFileName), nil
}

func LoadSession() (_ SessionState, returnErr error) {
	path, err := sessionPath()
	if err != nil {
		return SessionState{}, err
	}
	return loadSessionAtPath(path, openSessionRoot)
}

func loadSessionAtPath(path string, openRoot sessionRootOpener) (_ SessionState, returnErr error) {
	directory := filepath.Dir(path)
	root, err := openRoot(directory)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return SessionState{}, &SessionError{Operation: OperationLoad, Path: path, Err: ErrNoSession}
		}
		return SessionState{}, &SessionError{Operation: OperationOpenDirectory, Path: directory, Err: err}
	}
	defer joinSessionRootCloseError(&returnErr, root, directory)

	file, err := openRegularSessionFile(root, sessionFileName)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return SessionState{}, &SessionError{Operation: OperationLoad, Path: path, Err: ErrNoSession}
		}
		return SessionState{}, &SessionError{Operation: OperationOpen, Path: path, Err: err}
	}
	data, readErr := readBoundedSession(file)
	closeErr := file.Close()
	if readErr != nil || closeErr != nil {
		return SessionState{}, &SessionError{
			Operation: OperationRead,
			Path:      path,
			Err:       errors.Join(readErr, closeErr),
		}
	}
	var state SessionState
	if err := json.Unmarshal(data, &state); err != nil {
		return SessionState{}, &SessionError{Operation: OperationDecode, Path: path, Err: err}
	}
	return state, nil
}

func joinSessionRootCloseError(returnErr *error, root sessionRoot, directory string) {
	if closeErr := root.Close(); closeErr != nil {
		*returnErr = errors.Join(*returnErr, &SessionError{
			Operation: OperationCloseDirectory,
			Path:      directory,
			Err:       closeErr,
		})
	}
}

func openSessionRoot(directory string) (sessionRoot, error) {
	return openSessionRootWith(directory, os.Lstat, openOperatingSystemSessionRoot)
}

func openOperatingSystemSessionRoot(directory string) (sessionRoot, error) {
	root, err := os.OpenRoot(directory)
	if err != nil {
		return nil, err
	}
	return rootedSessionDirectory{root: root}, nil
}

func openSessionRootWith(
	directory string,
	lstat sessionPathInfo,
	openRoot sessionRootOpener,
) (sessionRoot, error) {
	expected, err := lstat(directory)
	if err != nil {
		return nil, err
	}
	if !expected.IsDir() {
		return nil, ErrUnsafeSession
	}
	root, err := openRoot(directory)
	if err != nil {
		return nil, err
	}
	actual, err := root.Stat(".")
	if err != nil || !os.SameFile(expected, actual) {
		return nil, errors.Join(err, ErrUnsafeSession, root.Close())
	}
	return root, nil
}

func openRegularSessionFile(root sessionRoot, name string) (sessionFile, error) {
	expected, err := root.Lstat(name)
	if err != nil {
		return nil, err
	}
	if !expected.Mode().IsRegular() {
		return nil, ErrUnsafeSession
	}
	file, err := root.Open(name)
	if err != nil {
		return nil, err
	}
	actual, err := file.Stat()
	if err != nil || !os.SameFile(expected, actual) {
		return nil, errors.Join(err, ErrUnsafeSession, file.Close())
	}
	return file, nil
}

func readBoundedSession(reader io.Reader) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(reader, maxSessionStateBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxSessionStateBytes {
		return nil, ErrSessionTooLarge
	}
	return data, nil
}

type sessionEncoder func(SessionState) ([]byte, error)

func SaveSession(state SessionState) error {
	return saveSessionWithEncoder(state, encodeSessionState)
}

func encodeSessionState(state SessionState) ([]byte, error) {
	return json.MarshalIndent(state, "", "  ")
}

func saveSessionWithEncoder(state SessionState, encode sessionEncoder) (returnErr error) {
	path, err := sessionPath()
	if err != nil {
		return err
	}
	return saveSessionAtPath(state, path, encode, prepareSessionDirectory, openSessionRoot)
}

func saveSessionAtPath(
	state SessionState,
	path string,
	encode sessionEncoder,
	prepareDirectory func(string) error,
	openRoot sessionRootOpener,
) (returnErr error) {
	directory := filepath.Dir(path)
	if err := prepareDirectory(directory); err != nil {
		return &SessionError{Operation: OperationPrepareDirectory, Path: directory, Err: err}
	}
	root, err := openRoot(directory)
	if err != nil {
		return &SessionError{Operation: OperationOpenDirectory, Path: directory, Err: err}
	}
	defer joinSessionRootCloseError(&returnErr, root, directory)
	if err := validateSessionDestination(root, sessionFileName); err != nil {
		return &SessionError{Operation: OperationValidateDestination, Path: path, Err: err}
	}
	data, err := encode(state)
	if err != nil {
		return &SessionError{Operation: OperationEncode, Path: path, Err: err}
	}
	temporaryName, err := writeSessionTemp(root, data)
	if err != nil {
		return &SessionError{Operation: OperationWriteTemporary, Path: directory, Err: err}
	}
	if err := replaceSessionFile(root, temporaryName, sessionFileName); err != nil {
		cleanupErr := root.Remove(temporaryName)
		if errors.Is(cleanupErr, os.ErrNotExist) {
			cleanupErr = nil
		}
		return &SessionError{Operation: OperationReplace, Path: path, Err: errors.Join(err, cleanupErr)}
	}
	return nil
}

func prepareSessionDirectory(directory string) error {
	return prepareSessionDirectoryWith(directory, os.MkdirAll, os.Lstat, openOperatingSystemSessionRoot)
}

func prepareSessionDirectoryWith(
	directory string,
	createDirectory sessionDirectoryCreator,
	lstat sessionPathInfo,
	openRoot sessionRootOpener,
) error {
	if err := createDirectory(directory, sessionDirectoryMode); err != nil {
		return err
	}
	expected, err := lstat(directory)
	if err != nil {
		return err
	}
	if !expected.IsDir() {
		return ErrUnsafeSession
	}
	root, err := openRoot(directory)
	if err != nil {
		return err
	}
	actual, statErr := root.Stat(".")
	if statErr != nil || !os.SameFile(expected, actual) {
		return errors.Join(statErr, ErrUnsafeSession, root.Close())
	}
	return errors.Join(root.Chmod(".", sessionDirectoryMode), root.Close())
}

func validateSessionDestination(root sessionRoot, name string) error {
	info, err := root.Lstat(name)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return ErrUnsafeSession
	}
	return nil
}

func writeSessionTemp(root sessionRoot, data []byte) (temporaryName string, returnErr error) {
	temporaryName = sessionTempPrefix + rand.Text() + sessionTempSuffix
	file, err := root.OpenFile(temporaryName, os.O_WRONLY|os.O_CREATE|os.O_EXCL, sessionFileMode)
	if err != nil {
		return "", err
	}
	defer cleanupFailedSessionTemp(&returnErr, root, temporaryName)
	if _, err := file.Write(data); err != nil {
		return temporaryName, errors.Join(err, file.Close())
	}
	if err := file.Sync(); err != nil {
		return temporaryName, errors.Join(err, file.Close())
	}
	if err := file.Close(); err != nil {
		return temporaryName, err
	}
	return temporaryName, nil
}

func cleanupFailedSessionTemp(returnErr *error, root sessionRoot, temporaryName string) {
	if *returnErr == nil {
		return
	}
	removeErr := root.Remove(temporaryName)
	if removeErr == nil || errors.Is(removeErr, os.ErrNotExist) {
		return
	}
	*returnErr = errors.Join(*returnErr, removeErr)
}

func replaceSessionFile(root sessionRoot, temporaryName, destinationName string) error {
	if err := root.Rename(temporaryName, destinationName); err != nil {
		return err
	}
	return syncSessionDirectory(root)
}
