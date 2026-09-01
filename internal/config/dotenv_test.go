package config

import (
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"testing/iotest"

	"github.com/HediAbed/opsmate/internal/failure"
)

func TestLoadDotEnv_BasicKeyValue(t *testing.T) {
	envFile := writeDotEnvFile(t, "OPSMATE_PROVIDER_MODEL=hello\n")
	unsetEnvironment(t, "OPSMATE_PROVIDER_MODEL")

	if err := LoadDotEnv(envFile); err != nil {
		t.Fatalf("load env: %v", err)
	}

	got := os.Getenv("OPSMATE_PROVIDER_MODEL")
	if got != "hello" {
		t.Errorf("OPSMATE_PROVIDER_MODEL = %q; want %q", got, "hello")
	}
}

func TestLoadDotEnv_SkipsComments(t *testing.T) {
	envFile := writeDotEnvFile(t, "# This is a comment\nOPSMATE_PROVIDER_MODEL=value\n")
	unsetEnvironment(t, "OPSMATE_PROVIDER_MODEL")
	if err := LoadDotEnv(envFile); err != nil {
		t.Fatalf("load env: %v", err)
	}

	got := os.Getenv("OPSMATE_PROVIDER_MODEL")
	if got != "value" {
		t.Errorf("OPSMATE_PROVIDER_MODEL = %q; want %q", got, "value")
	}
}

func TestLoadDotEnv_SkipsEmptyLines(t *testing.T) {
	envFile := writeDotEnvFile(t, "\n\nOPSMATE_PROVIDER_MODEL=yes\n\n")
	unsetEnvironment(t, "OPSMATE_PROVIDER_MODEL")
	if err := LoadDotEnv(envFile); err != nil {
		t.Fatalf("load env: %v", err)
	}

	got := os.Getenv("OPSMATE_PROVIDER_MODEL")
	if got != "yes" {
		t.Errorf("OPSMATE_PROVIDER_MODEL = %q; want %q", got, "yes")
	}
}

func TestLoadDotEnv_QuotedValues(t *testing.T) {
	envFile := writeDotEnvFile(t, `OPSMATE_PROVIDER_MODEL="double quoted"
OPSMATE_PROVIDER_API_KEY='single quoted'
`)

	unsetEnvironment(t, "OPSMATE_PROVIDER_MODEL")
	unsetEnvironment(t, "OPSMATE_PROVIDER_API_KEY")
	if err := LoadDotEnv(envFile); err != nil {
		t.Fatalf("load env: %v", err)
	}

	if got := os.Getenv("OPSMATE_PROVIDER_MODEL"); got != "double quoted" {
		t.Errorf("OPSMATE_PROVIDER_MODEL = %q; want %q", got, "double quoted")
	}
	if got := os.Getenv("OPSMATE_PROVIDER_API_KEY"); got != "single quoted" {
		t.Errorf("OPSMATE_PROVIDER_API_KEY = %q; want %q", got, "single quoted")
	}
}

func TestLoadDotEnv_RealEnvTakesPrecedence(t *testing.T) {
	envFile := writeDotEnvFile(t, "OPSMATE_PROVIDER_MODEL=from_file\n")
	t.Setenv("OPSMATE_PROVIDER_MODEL", "from_env")
	if err := LoadDotEnv(envFile); err != nil {
		t.Fatalf("load env: %v", err)
	}

	got := os.Getenv("OPSMATE_PROVIDER_MODEL")
	if got != "from_env" {
		t.Errorf("OPSMATE_PROVIDER_MODEL = %q; want %q (real env should take precedence)", got, "from_env")
	}
}

func TestLoadDotEnv_MissingFile(t *testing.T) {
	if err := LoadDotEnv(filepath.Join(t.TempDir(), "missing.env")); err != nil {
		t.Fatalf("missing optional file returned error: %v", err)
	}
}

func TestLoadDotEnv_InvalidLineReturnsTypedError(t *testing.T) {
	envFile := writeDotEnvFile(t, "INVALID_LINE_NO_EQUALS\nOPSMATE_PROVIDER_MODEL=ok\n")
	unsetEnvironment(t, "OPSMATE_PROVIDER_MODEL")
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
	envFile := writeDotEnvFile(t, "  OPSMATE_PROVIDER_MODEL  =  spaced  \n")
	unsetEnvironment(t, "OPSMATE_PROVIDER_MODEL")
	if err := LoadDotEnv(envFile); err != nil {
		t.Fatalf("load env: %v", err)
	}

	got := os.Getenv("OPSMATE_PROVIDER_MODEL")
	if got != "spaced" {
		t.Errorf("OPSMATE_PROVIDER_MODEL = %q; want %q", got, "spaced")
	}
}

func TestLoadDotEnvFromExecutableDir_IgnoresWorkingDirectoryFile(t *testing.T) {
	redirectUserConfigDir(t)
	workingDirectory := t.TempDir()
	writeDotEnvFileIn(t, workingDirectory, "OPSMATE_PROVIDER_MODEL=from-working-directory\n")
	t.Chdir(workingDirectory)
	unsetEnvironment(t, "OPSMATE_ENV_FILE")
	unsetEnvironment(t, "OPSMATE_PROVIDER_MODEL")

	if err := LoadDotEnvFromExecutableDir(); err != nil {
		t.Fatalf("load optional env: %v", err)
	}

	if value, found := os.LookupEnv("OPSMATE_PROVIDER_MODEL"); found {
		t.Fatalf("OPSMATE_PROVIDER_MODEL = %q; the working directory must never be searched", value)
	}
}

func TestLoadDotEnvFromExecutableDir_LoadsUserConfigFile(t *testing.T) {
	writeUserConfigDotEnv(t, "OPSMATE_PROVIDER_MODEL=from-user-config\n")
	t.Chdir(t.TempDir())
	unsetEnvironment(t, "OPSMATE_ENV_FILE")
	unsetEnvironment(t, "OPSMATE_PROVIDER_MODEL")

	if err := LoadDotEnvFromExecutableDir(); err != nil {
		t.Fatalf("load optional env: %v", err)
	}

	if got := os.Getenv("OPSMATE_PROVIDER_MODEL"); got != "from-user-config" {
		t.Fatalf("OPSMATE_PROVIDER_MODEL = %q, want %q", got, "from-user-config")
	}
}

func TestLoadDotEnvFromExecutableDir_ExplicitEnvFileWins(t *testing.T) {
	writeUserConfigDotEnv(t, "OPSMATE_PROVIDER_MODEL=from-user-config\n")
	explicitFile := writeDotEnvFile(t, "OPSMATE_PROVIDER_MODEL=from-explicit-file\n")
	t.Chdir(t.TempDir())
	t.Setenv("OPSMATE_ENV_FILE", explicitFile)
	unsetEnvironment(t, "OPSMATE_PROVIDER_MODEL")

	if err := LoadDotEnvFromExecutableDir(); err != nil {
		t.Fatalf("load optional env: %v", err)
	}

	if got := os.Getenv("OPSMATE_PROVIDER_MODEL"); got != "from-explicit-file" {
		t.Fatalf("OPSMATE_PROVIDER_MODEL = %q, want %q", got, "from-explicit-file")
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

type dotEnvSelectionCase struct {
	name          string
	executableErr error
	statErr       error
	loadErr       error
	wantPath      string
	wantErr       error
}

func TestLoadDotEnvFromExecutableDirSelection(t *testing.T) {
	sentinel := errors.New("stat failed")
	tests := []dotEnvSelectionCase{
		{name: "executable file", wantPath: filepath.Join("bin", ".env")},
		{name: "missing beside executable", statErr: os.ErrNotExist},
		{name: "executable lookup failed", executableErr: sentinel},
		{name: "stat failed", statErr: sentinel, wantErr: sentinel},
		{name: "load failed", loadErr: sentinel, wantPath: filepath.Join("bin", ".env"), wantErr: sentinel},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assertDotEnvSelection(t, test)
		})
	}
}

func assertDotEnvSelection(t *testing.T, test dotEnvSelectionCase) {
	t.Helper()
	loadedPath := ""
	executable := filepath.Join("bin", "opsmate")
	if test.executableErr != nil {
		executable = ""
	}
	err := loadDotEnvNearExecutable(
		executable,
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
}

func TestLoadDotEnvReportsOpenReadAndCloseFailures(t *testing.T) {
	if err := LoadDotEnv("\x00"); err == nil {
		t.Fatal("invalid path did not fail to open")
	}

	readFailure := errors.New("read failed")
	err := loadDotEnvReader("config", io.NopCloser(iotest.ErrReader(readFailure)))
	if !errors.Is(err, readFailure) {
		t.Fatalf("read error = %v, want sentinel", err)
	}

	closeFailure := errors.New("close failed")
	unsetEnvironment(t, "OPSMATE_PROVIDER_MODEL")
	err = loadDotEnvReader(
		"config",
		&dotEnvReadCloser{Reader: strings.NewReader("OPSMATE_PROVIDER_MODEL=value\n"), closeErr: closeFailure},
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
		"OPSMATE_PROVIDER_MODEL=value",
		func(string) (string, bool) { return "", false },
		func(string, string) error { return sentinel },
	)
	if !errors.Is(err, sentinel) {
		t.Fatalf("error = %v, want environment failure", err)
	}
}

func TestParseDotEnvLineRejectsInvalidKeysAndQuotes(t *testing.T) {
	invalid := []string{
		"=value",
		"INVALID KEY=value",
		`OPSMATE_PROVIDER_MODEL="unterminated`,
		"OPSMATE_PROVIDER_MODEL='",
	}
	for _, line := range invalid {
		if _, _, _, err := parseDotEnvLine(line); !errors.Is(err, ErrInvalidDotEnvLine) {
			t.Errorf("line %q error = %v, want invalid-line classification", line, err)
		}
	}
	key, value, skip, err := parseDotEnvLine("OPSMATE_PROVIDER_API_KEY=")
	if err != nil || skip || key != "OPSMATE_PROVIDER_API_KEY" || value != "" {
		t.Fatalf("empty value = (%q, %q, %t, %v)", key, value, skip, err)
	}
}

func TestLoadDotEnv_IgnoresKeysOutsideProviderAllowlist(t *testing.T) {
	keys := []string{
		"KUBECONFIG",
		"LD_PRELOAD",
		"HTTPS_PROXY",
		"OPSMATE_ENV_FILE",
		"OPSMATE_PROVIDER_TOKEN",
	}
	for _, key := range keys {
		t.Run(key, func(t *testing.T) {
			envFile := writeDotEnvFile(t, key+"=injected\n")
			unsetEnvironment(t, key)

			_ = LoadDotEnv(envFile)

			if value, found := os.LookupEnv(key); found {
				t.Fatalf("%s = %q; dotenv files may only set documented provider keys", key, value)
			}
		})
	}
}

func TestLoadDotEnv_RejectsGroupOrWorldAccessibleFile(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("windows does not expose unix permission bits")
	}
	for _, mode := range []fs.FileMode{0o604, 0o640, 0o644, 0o660, 0o666} {
		t.Run(mode.String(), func(t *testing.T) {
			envFile := writeDotEnvFileWithMode(t, mode, "OPSMATE_PROVIDER_MODEL=from-insecure-file\n")
			unsetEnvironment(t, "OPSMATE_PROVIDER_MODEL")

			err := LoadDotEnv(envFile)
			if !errors.Is(err, fs.ErrPermission) {
				t.Fatalf("error = %v, want fs.ErrPermission", err)
			}
			if got := failure.CodeOf(err); got != failure.CodePermissionDenied {
				t.Fatalf("failure code = %q, want %q", got, failure.CodePermissionDenied)
			}
			if value, found := os.LookupEnv("OPSMATE_PROVIDER_MODEL"); found {
				t.Fatalf("OPSMATE_PROVIDER_MODEL = %q; an insecure file must not be applied", value)
			}
		})
	}
}

func TestLoadDotEnv_AllowsWindowsFilePermissions(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("only windows reports permissive bits for ordinary files")
	}
	envFile := writeDotEnvFileWithMode(t, 0o666, "OPSMATE_PROVIDER_MODEL=from-windows\n")
	unsetEnvironment(t, "OPSMATE_PROVIDER_MODEL")

	if err := LoadDotEnv(envFile); err != nil {
		t.Fatalf("windows permission bits must not block loading: %v", err)
	}
	if got := os.Getenv("OPSMATE_PROVIDER_MODEL"); got != "from-windows" {
		t.Fatalf("OPSMATE_PROVIDER_MODEL = %q, want %q", got, "from-windows")
	}
}

func TestLoadUserConfigDotEnvWithoutHome(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("windows uses AppData for the user config directory")
	}
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("HOME", "")

	loaded, err := loadUserConfigDotEnv()
	if loaded || err != nil {
		t.Fatalf("loadUserConfigDotEnv() = (%t, %v), want no file and no error", loaded, err)
	}
}

func TestLoadExistingDotEnvReportsStatFailure(t *testing.T) {
	loaded, err := loadExistingDotEnv("\x00")
	if !loaded || err == nil {
		t.Fatalf("loadExistingDotEnv() = (%t, %v), want attempted load and error", loaded, err)
	}
	var dotEnvErr *DotEnvError
	if !errors.As(err, &dotEnvErr) || dotEnvErr.Stage != StageStat {
		t.Fatalf("error = %#v, want stat DotEnvError", err)
	}
}

func TestCloseDotEnvFileReportsCloseFailure(t *testing.T) {
	file, err := os.CreateTemp(t.TempDir(), ".env")
	if err != nil {
		t.Fatalf("create env: %v", err)
	}
	path := file.Name()
	if err := file.Close(); err != nil {
		t.Fatalf("close env: %v", err)
	}

	err = closeDotEnvFile(path, file)
	var dotEnvErr *DotEnvError
	if !errors.As(err, &dotEnvErr) || dotEnvErr.Stage != StageClose {
		t.Fatalf("error = %#v, want close DotEnvError", err)
	}
}

func TestValidateDotEnvPermissionsReportsStatFailure(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("windows does not apply unix permission bits")
	}
	file, err := os.CreateTemp(t.TempDir(), ".env")
	if err != nil {
		t.Fatalf("create env: %v", err)
	}
	path := file.Name()
	if err := file.Close(); err != nil {
		t.Fatalf("close env: %v", err)
	}

	err = validateDotEnvPermissions(path, file)
	var dotEnvErr *DotEnvError
	if !errors.As(err, &dotEnvErr) || dotEnvErr.Stage != StageStat {
		t.Fatalf("error = %#v, want stat DotEnvError", err)
	}
}

type dotEnvReadCloser struct {
	io.Reader
	closeErr error
}

func (reader *dotEnvReadCloser) Close() error { return reader.closeErr }

func writeDotEnvFile(t *testing.T, contents string) string {
	t.Helper()
	return writeDotEnvFileIn(t, t.TempDir(), contents)
}

func writeDotEnvFileIn(t *testing.T, directory, contents string) string {
	t.Helper()
	path := filepath.Join(directory, ".env")
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatalf("write env: %v", err)
	}
	return path
}

func writeDotEnvFileWithMode(t *testing.T, mode fs.FileMode, contents string) string {
	t.Helper()
	path := writeDotEnvFile(t, contents)
	if err := os.Chmod(path, mode); err != nil {
		t.Fatalf("chmod env: %v", err)
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

func redirectUserConfigDir(t *testing.T) string {
	t.Helper()
	sandbox := t.TempDir()
	switch runtime.GOOS {
	case "windows":
		t.Setenv("AppData", sandbox)
	case "darwin":
		t.Setenv("HOME", sandbox)
	default:
		t.Setenv("XDG_CONFIG_HOME", sandbox)
	}
	directory, err := os.UserConfigDir()
	if err != nil {
		t.Skipf("user config directory unavailable: %v", err)
	}
	if !strings.HasPrefix(directory, sandbox) {
		t.Skipf("user config directory %q escaped the sandbox %q", directory, sandbox)
	}
	return directory
}

func writeUserConfigDotEnv(t *testing.T, contents string) {
	t.Helper()
	directory := filepath.Join(redirectUserConfigDir(t), "opsmate")
	if err := os.MkdirAll(directory, 0o700); err != nil {
		t.Fatalf("create user config directory: %v", err)
	}
	writeDotEnvFileIn(t, directory, contents)
}
