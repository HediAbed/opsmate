package service

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadDotEnv_BasicKeyValue(t *testing.T) {
	envFile := writeDotEnvFile(t, "TEST_DOTENV_KEY=hello\n")
	unsetEnvironment(t, "TEST_DOTENV_KEY")

	if err := LoadDotEnv(envFile); err != nil {
		t.Fatalf("load env: %v", err)
	}

	got := os.Getenv("TEST_DOTENV_KEY")
	if got != "hello" {
		t.Errorf("TEST_DOTENV_KEY = %q; want %q", got, "hello")
	}
}

func TestLoadDotEnv_SkipsComments(t *testing.T) {
	envFile := writeDotEnvFile(t, "# This is a comment\nTEST_DOTENV_COMMENT=value\n")
	unsetEnvironment(t, "TEST_DOTENV_COMMENT")
	if err := LoadDotEnv(envFile); err != nil {
		t.Fatalf("load env: %v", err)
	}

	got := os.Getenv("TEST_DOTENV_COMMENT")
	if got != "value" {
		t.Errorf("TEST_DOTENV_COMMENT = %q; want %q", got, "value")
	}
}

func TestLoadDotEnv_SkipsEmptyLines(t *testing.T) {
	envFile := writeDotEnvFile(t, "\n\nTEST_DOTENV_EMPTY=yes\n\n")
	unsetEnvironment(t, "TEST_DOTENV_EMPTY")
	if err := LoadDotEnv(envFile); err != nil {
		t.Fatalf("load env: %v", err)
	}

	got := os.Getenv("TEST_DOTENV_EMPTY")
	if got != "yes" {
		t.Errorf("TEST_DOTENV_EMPTY = %q; want %q", got, "yes")
	}
}

func TestLoadDotEnv_QuotedValues(t *testing.T) {
	envFile := writeDotEnvFile(t, `TEST_DOTENV_DQ="double quoted"
TEST_DOTENV_SQ='single quoted'
`)

	unsetEnvironment(t, "TEST_DOTENV_DQ")
	unsetEnvironment(t, "TEST_DOTENV_SQ")
	if err := LoadDotEnv(envFile); err != nil {
		t.Fatalf("load env: %v", err)
	}

	if got := os.Getenv("TEST_DOTENV_DQ"); got != "double quoted" {
		t.Errorf("TEST_DOTENV_DQ = %q; want %q", got, "double quoted")
	}
	if got := os.Getenv("TEST_DOTENV_SQ"); got != "single quoted" {
		t.Errorf("TEST_DOTENV_SQ = %q; want %q", got, "single quoted")
	}
}

func TestLoadDotEnv_RealEnvTakesPrecedence(t *testing.T) {
	envFile := writeDotEnvFile(t, "TEST_DOTENV_PREC=from_file\n")
	t.Setenv("TEST_DOTENV_PREC", "from_env")
	if err := LoadDotEnv(envFile); err != nil {
		t.Fatalf("load env: %v", err)
	}

	got := os.Getenv("TEST_DOTENV_PREC")
	if got != "from_env" {
		t.Errorf("TEST_DOTENV_PREC = %q; want %q (real env should take precedence)", got, "from_env")
	}
}

func TestLoadDotEnv_MissingFile(t *testing.T) {
	if err := LoadDotEnv(filepath.Join(t.TempDir(), "missing.env")); err != nil {
		t.Fatalf("missing optional file returned error: %v", err)
	}
}

func TestLoadDotEnv_InvalidLineReturnsTypedError(t *testing.T) {
	envFile := writeDotEnvFile(t, "INVALID_LINE_NO_EQUALS\nTEST_DOTENV_VALID=ok\n")
	unsetEnvironment(t, "TEST_DOTENV_VALID")
	err := LoadDotEnv(envFile)
	if !errors.Is(err, ErrInvalidDotEnvLine) {
		t.Fatalf("error = %v, want ErrInvalidDotEnvLine", err)
	}
	var dotEnvErr *DotEnvError
	if !errors.As(err, &dotEnvErr) || dotEnvErr.Line != 1 || dotEnvErr.Stage != "parse" {
		t.Fatalf("error = %#v, want line-one parse DotEnvError", err)
	}
}

func TestLoadDotEnv_WhitespaceAroundKeyValue(t *testing.T) {
	envFile := writeDotEnvFile(t, "  TEST_DOTENV_WS  =  spaced  \n")
	unsetEnvironment(t, "TEST_DOTENV_WS")
	if err := LoadDotEnv(envFile); err != nil {
		t.Fatalf("load env: %v", err)
	}

	got := os.Getenv("TEST_DOTENV_WS")
	if got != "spaced" {
		t.Errorf("TEST_DOTENV_WS = %q; want %q", got, "spaced")
	}
}

func TestLoadDotEnvFromExecutableDir_FallbackToCWD(t *testing.T) {
	t.Chdir(t.TempDir())
	if err := LoadDotEnvFromExecutableDir(); err != nil {
		t.Fatalf("load optional env: %v", err)
	}
}

func TestDotEnvErrorFormatsContextAndUnwraps(t *testing.T) {
	sentinel := errors.New("failed")
	tests := []struct {
		err  *DotEnvError
		want string
	}{
		{err: &DotEnvError{Path: ".env", Stage: "read"}, want: "dotenv .env (read): unknown error"},
		{err: &DotEnvError{Path: ".env", Line: 3, Stage: "parse", Err: sentinel}, want: "dotenv .env:3 (parse): failed"},
		{err: &DotEnvError{Path: ".env", Stage: "open", Err: sentinel}, want: "dotenv .env (open): failed"},
	}
	for _, test := range tests {
		if got := test.err.Error(); got != test.want {
			t.Errorf("error = %q, want %q", got, test.want)
		}
	}
	if !errors.Is(&DotEnvError{Err: sentinel}, sentinel) {
		t.Fatal("dotenv error did not unwrap its cause")
	}
}

func TestLoadDotEnvFromExecutableDirSelection(t *testing.T) {
	sentinel := errors.New("stat failed")
	tests := []struct {
		name          string
		executableErr error
		statErr       error
		loadErr       error
		wantPath      string
		wantErr       error
	}{
		{name: "executable file", wantPath: filepath.Join("bin", ".env")},
		{name: "missing beside executable", statErr: os.ErrNotExist, wantPath: ".env"},
		{name: "executable lookup failed", executableErr: sentinel, wantPath: ".env"},
		{name: "stat failed", statErr: sentinel, wantErr: sentinel},
		{name: "load failed", loadErr: sentinel, wantPath: filepath.Join("bin", ".env"), wantErr: sentinel},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			loadedPath := ""
			err := loadDotEnvNearExecutable(
				filepath.Join("bin", "opsmate"),
				test.executableErr,
				func(string) (os.FileInfo, error) { return nil, test.statErr },
				func(path string) error {
					loadedPath = path
					return test.loadErr
				},
			)
			if !errors.Is(err, test.wantErr) {
				t.Fatalf("error = %v, want %v", err, test.wantErr)
			}
			if loadedPath != test.wantPath {
				t.Fatalf("loaded path = %q, want %q", loadedPath, test.wantPath)
			}
		})
	}
}

func TestLoadDotEnvReportsOpenReadAndCloseFailures(t *testing.T) {
	if err := LoadDotEnv("\x00"); err == nil {
		t.Fatal("invalid path did not fail to open")
	}

	readFailure := errors.New("read failed")
	err := loadDotEnvReader("config", &failingProviderBody{readErr: readFailure})
	if !errors.Is(err, readFailure) {
		t.Fatalf("read error = %v, want sentinel", err)
	}

	closeFailure := errors.New("close failed")
	unsetEnvironment(t, "TEST_DOTENV_CLOSE")
	err = loadDotEnvReader(
		"config",
		&dotEnvReadCloser{Reader: strings.NewReader("TEST_DOTENV_CLOSE=value\n"), closeErr: closeFailure},
	)
	if !errors.Is(err, closeFailure) {
		t.Fatalf("close error = %v, want sentinel", err)
	}
}

func TestLoadDotEnvLineReportsEnvironmentFailure(t *testing.T) {
	sentinel := errors.New("set failed")
	err := loadDotEnvLineWithEnvironment(
		"config",
		2,
		"KEY=value",
		func(string) (string, bool) { return "", false },
		func(string, string) error { return sentinel },
	)
	if !errors.Is(err, sentinel) {
		t.Fatalf("error = %v, want environment failure", err)
	}
}

func TestParseDotEnvLineRejectsInvalidKeysAndQuotes(t *testing.T) {
	for _, line := range []string{"=value", "INVALID KEY=value", `KEY="unterminated`, "KEY='"} {
		if _, _, _, err := parseDotEnvLine(line); !errors.Is(err, ErrInvalidDotEnvLine) {
			t.Errorf("line %q error = %v, want invalid-line classification", line, err)
		}
	}
	key, value, skip, err := parseDotEnvLine("EMPTY=")
	if err != nil || skip || key != "EMPTY" || value != "" {
		t.Fatalf("empty value = (%q, %q, %t, %v)", key, value, skip, err)
	}
}

type dotEnvReadCloser struct {
	io.Reader
	closeErr error
}

func (reader *dotEnvReadCloser) Close() error { return reader.closeErr }

func writeDotEnvFile(t *testing.T, contents string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), ".env")
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatalf("write env: %v", err)
	}
	return path
}

func unsetEnvironment(t *testing.T, key string) {
	t.Helper()
	previous, existed := os.LookupEnv(key)
	if err := os.Unsetenv(key); err != nil {
		t.Fatalf("unset %s: %v", key, err)
	}
	t.Cleanup(func() {
		var err error
		if existed {
			err = os.Setenv(key, previous)
		} else {
			err = os.Unsetenv(key)
		}
		if err != nil {
			t.Errorf("restore %s: %v", key, err)
		}
	})
}
