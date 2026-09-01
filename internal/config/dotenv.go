package config

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/HediAbed/opsmate/internal/failure"
)

const (
	minimumQuotedValueLength   = 2
	dotEnvFilename             = ".env"
	applicationConfigDir       = "opsmate"
	explicitDotEnvPathEnv      = "OPSMATE_ENV_FILE"
	dotEnvPublicPermissionMask = fs.FileMode(0o077)
)

var ErrInvalidDotEnvLine = errors.New("invalid dotenv line")

type Stage string

const (
	StageStat  Stage = "stat"
	StageOpen  Stage = "open"
	StageClose Stage = "close"
	StageRead  Stage = "read"
	StageParse Stage = "parse"
	StageSet   Stage = "set"
)

type DotEnvError struct {
	Path  string
	Line  int
	Stage Stage
	Err   error
}

func (e *DotEnvError) Error() string {
	if e == nil {
		return "dotenv: unknown error"
	}
	location := e.Path
	if e.Line > 0 {
		location = fmt.Sprintf("%s:%d", location, e.Line)
	}
	if e.Err == nil {
		return fmt.Sprintf("dotenv %s (%s): unknown error", location, e.Stage)
	}
	return fmt.Sprintf("dotenv %s (%s): %v", location, e.Stage, e.Err)
}

func (e *DotEnvError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

func (e *DotEnvError) FailureCode() failure.Code {
	if e == nil || e.Err == nil {
		return failure.CodeUnknown
	}
	if errors.Is(e.Err, context.Canceled) {
		return failure.CodeCanceled
	}
	if errors.Is(e.Err, context.DeadlineExceeded) {
		return failure.CodeDeadlineExceeded
	}
	if errors.Is(e.Err, ErrInvalidDotEnvLine) {
		return failure.CodeInvalidArgument
	}
	if errors.Is(e.Err, fs.ErrPermission) {
		return failure.CodePermissionDenied
	}
	return failure.CodeInternal
}

func LoadDotEnvFromExecutableDir() error {
	if path, explicit := os.LookupEnv(explicitDotEnvPathEnv); explicit {
		return LoadDotEnv(path)
	}
	if loaded, err := loadUserConfigDotEnv(); loaded || err != nil {
		return err
	}
	return loadDotEnvNearExecutable(currentExecutable(), os.Stat, LoadDotEnv)
}

func loadUserConfigDotEnv() (bool, error) {
	configDir := userConfigDir()
	if configDir == "" {
		return false, nil
	}
	return loadExistingDotEnv(filepath.Join(configDir, applicationConfigDir, dotEnvFilename))
}

func userConfigDir() string {
	configDir, _ := os.UserConfigDir()
	return configDir
}

func loadExistingDotEnv(path string) (bool, error) {
	_, err := os.Stat(path)
	switch {
	case err == nil:
		return true, LoadDotEnv(path)
	case errors.Is(err, os.ErrNotExist):
		return false, nil
	default:
		return true, &DotEnvError{Path: path, Stage: StageStat, Err: err}
	}
}

func currentExecutable() string {
	executable, _ := os.Executable()
	return executable
}

func loadDotEnvNearExecutable(
	executable string,
	stat func(string) (os.FileInfo, error),
	load func(string) error,
) error {
	if executable == "" {
		return nil
	}
	candidate := filepath.Join(filepath.Dir(executable), dotEnvFilename)
	_, statErr := stat(candidate)
	switch {
	case statErr == nil:
		return load(candidate)
	case errors.Is(statErr, os.ErrNotExist):
		return nil
	default:
		return &DotEnvError{Path: candidate, Stage: StageStat, Err: statErr}
	}
}

func LoadDotEnv(path string) (returnErr error) {
	// #nosec G304 -- path is the configuration file explicitly selected by the caller.
	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return &DotEnvError{Path: path, Stage: StageOpen, Err: err}
	}
	if err := validateDotEnvPermissions(path, file); err != nil {
		return errors.Join(err, closeDotEnvFile(path, file))
	}
	return loadDotEnvReader(path, file)
}

func closeDotEnvFile(path string, file *os.File) error {
	if err := file.Close(); err != nil {
		return &DotEnvError{Path: path, Stage: StageClose, Err: err}
	}
	return nil
}

func loadDotEnvReader(path string, reader io.ReadCloser) (returnErr error) {
	defer func() {
		if closeErr := reader.Close(); closeErr != nil && returnErr == nil {
			returnErr = &DotEnvError{Path: path, Stage: StageClose, Err: closeErr}
		}
	}()

	scanner := bufio.NewScanner(reader)
	lineNumber := 0
	for scanner.Scan() {
		lineNumber++
		if err := loadDotEnvLine(path, lineNumber, scanner.Text()); err != nil {
			return err
		}
	}
	if scanErr := scanner.Err(); scanErr != nil {
		return &DotEnvError{Path: path, Line: lineNumber, Stage: StageRead, Err: scanErr}
	}
	return nil
}

func loadDotEnvLine(path string, lineNumber int, line string) error {
	return loadDotEnvLineWithEnvironment(path, lineNumber, line, os.LookupEnv, os.Setenv)
}

func loadDotEnvLineWithEnvironment(
	path string,
	lineNumber int,
	line string,
	lookup func(string) (string, bool),
	set func(string, string) error,
) error {
	key, value, skip, err := parseDotEnvLine(line)
	if err != nil {
		return &DotEnvError{Path: path, Line: lineNumber, Stage: StageParse, Err: err}
	}
	if skip {
		return nil
	}
	if !isAllowedDotEnvKey(key) {
		return nil
	}
	if _, exists := lookup(key); exists {
		return nil
	}
	if err := set(key, value); err != nil {
		return &DotEnvError{Path: path, Line: lineNumber, Stage: StageSet, Err: err}
	}
	return nil
}

func isAllowedDotEnvKey(key string) bool {
	switch key {
	case "OPSMATE_PROVIDER_URL", "OPSMATE_PROVIDER_MODEL", "OPSMATE_PROVIDER_API_KEY":
		return true
	default:
		return false
	}
}

func parseDotEnvLine(raw string) (key string, value string, skip bool, err error) {
	line := strings.TrimSpace(raw)
	if line == "" || strings.HasPrefix(line, "#") {
		return "", "", true, nil
	}
	separator := strings.IndexByte(line, '=')
	if separator < 1 {
		return "", "", false, ErrInvalidDotEnvLine
	}
	key = strings.TrimSpace(line[:separator])
	value = strings.TrimSpace(line[separator+1:])
	if strings.ContainsAny(key, " \t") {
		return "", "", false, fmt.Errorf("%w: invalid key %q", ErrInvalidDotEnvLine, key)
	}
	if value == "" {
		return key, value, false, nil
	}
	quote := value[0]
	if quote != '\'' && quote != '"' {
		return key, value, false, nil
	}
	if len(value) < minimumQuotedValueLength || value[len(value)-1] != quote {
		return "", "", false, fmt.Errorf("%w: unmatched quote", ErrInvalidDotEnvLine)
	}
	return key, value[1 : len(value)-1], false, nil
}
