package service

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

var ErrInvalidDotEnvLine = errors.New("invalid dotenv line")

type DotEnvError struct {
	Path  string
	Line  int
	Stage string
	Err   error
}

func (e *DotEnvError) Error() string {
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
	return e.Err
}

func LoadDotEnvFromExecutableDir() error {
	executable, executableErr := os.Executable()
	return loadDotEnvNearExecutable(executable, executableErr, os.Stat, LoadDotEnv)
}

func loadDotEnvNearExecutable(
	executable string,
	executableErr error,
	stat func(string) (os.FileInfo, error),
	load func(string) error,
) error {
	if executableErr == nil {
		candidate := filepath.Join(filepath.Dir(executable), ".env")
		_, statErr := stat(candidate)
		switch {
		case statErr == nil:
			return load(candidate)
		case !errors.Is(statErr, os.ErrNotExist):
			return &DotEnvError{Path: candidate, Stage: "stat", Err: statErr}
		}
	}
	return load(".env")
}

func LoadDotEnv(path string) (returnErr error) {
	// #nosec G304 -- path is the configuration file explicitly selected by the caller.
	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return &DotEnvError{Path: path, Stage: "open", Err: err}
	}
	return loadDotEnvReader(path, file)
}

func loadDotEnvReader(path string, reader io.ReadCloser) (returnErr error) {
	defer func() {
		if closeErr := reader.Close(); closeErr != nil && returnErr == nil {
			returnErr = &DotEnvError{Path: path, Stage: "close", Err: closeErr}
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
		return &DotEnvError{Path: path, Line: lineNumber, Stage: "read", Err: scanErr}
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
		return &DotEnvError{Path: path, Line: lineNumber, Stage: "parse", Err: err}
	}
	if skip {
		return nil
	}
	if _, exists := lookup(key); exists {
		return nil
	}
	if err := set(key, value); err != nil {
		return &DotEnvError{Path: path, Line: lineNumber, Stage: "set", Err: err}
	}
	return nil
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
	if len(value) < 2 || value[len(value)-1] != quote {
		return "", "", false, fmt.Errorf("%w: unmatched quote", ErrInvalidDotEnvLine)
	}
	return key, value[1 : len(value)-1], false, nil
}
