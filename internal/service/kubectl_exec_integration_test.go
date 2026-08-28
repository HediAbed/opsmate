//go:build !windows

package service

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func installFakeKubectl(t *testing.T, script string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "kubectl")
	writeTestExecutable(t, path, script)
	prev := os.Getenv("PATH")
	t.Cleanup(func() { _ = os.Setenv("PATH", prev) })
	if err := os.Setenv("PATH", dir+string(os.PathListSeparator)+prev); err != nil {
		t.Fatalf("set PATH: %v", err)
	}
	return path
}

func TestRunKubectl_Success_ReturnsStdoutOnly(t *testing.T) {
	installFakeKubectl(t, "#!/bin/sh\necho hello-stdout\necho hello-stderr >&2\n")

	out, err := runKubectl(2*time.Second, "version")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.TrimSpace(string(out)) != "hello-stdout" {
		t.Errorf("got %q; want hello-stdout (stderr must not leak into stdout)", string(out))
	}
}

func TestRunKubectl_NonZeroExit_WrapsStderr(t *testing.T) {
	installFakeKubectl(t, "#!/bin/sh\necho 'namespace not found' >&2\nexit 1\n")

	_, err := runKubectl(2*time.Second, "get", "pods")
	if err == nil {
		t.Fatal("expected error from failing kubectl")
	}
	var ke *KubectlError
	if !errors.As(err, &ke) {
		t.Fatalf("expected *KubectlError, got %T (%v)", err, err)
	}
	if ke.Subcommand != "get" {
		t.Errorf("Subcommand = %q; want get", ke.Subcommand)
	}
	if !strings.Contains(ke.Stderr, "namespace not found") {
		t.Errorf("Stderr missing message: %q", ke.Stderr)
	}
}

func TestRunKubectl_Timeout_ReturnsTypedError(t *testing.T) {
	installFakeKubectl(t, "#!/bin/sh\nsleep 2\n")

	_, err := runKubectl(100*time.Millisecond, "get", "pods")
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if !errors.Is(err, ErrKubectlTimeout) {
		t.Errorf("expected ErrKubectlTimeout, got: %v", err)
	}
}

func TestRunKubectlJSON_DecodesStdout(t *testing.T) {
	installFakeKubectl(t, `#!/bin/sh
cat <<'EOF'
{"items":[{"metadata":{"name":"foo"}}]}
EOF`)

	raw, err := runKubectlJSON[podList](2*time.Second, "get", "pods", "-o", "json")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(raw.Items) != 1 || raw.Items[0].Metadata.Name != "foo" {
		t.Errorf("decoded = %+v; want items[0].metadata.name=foo", raw)
	}
}

func TestRunKubectlJSON_InvalidJSON_WrapsParseError(t *testing.T) {
	installFakeKubectl(t, "#!/bin/sh\necho 'not json'\n")

	_, err := runKubectlJSON[podList](2*time.Second, "get", "pods", "-o", "json")
	if err == nil {
		t.Fatal("expected parse error")
	}
	var ke *KubectlError
	if !errors.As(err, &ke) {
		t.Fatalf("expected *KubectlError, got %T", err)
	}
	if !strings.Contains(ke.Error(), "parse json") {
		t.Errorf("expected parse-json mention in %q", ke.Error())
	}
}

func TestRunKubectlText_CombinesStdoutAndStderr(t *testing.T) {
	installFakeKubectl(t, "#!/bin/sh\necho out-line\necho err-line >&2\n")

	out, err := runKubectlText(2*time.Second, "describe", "pod", "x")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "out-line") || !strings.Contains(out, "err-line") {
		t.Errorf("combined output missing expected lines: %q", out)
	}
}

func TestRunKubectlText_Timeout_IncludesDuration(t *testing.T) {
	installFakeKubectl(t, "#!/bin/sh\nsleep 2\n")

	_, err := runKubectlText(100*time.Millisecond, "logs", "foo")
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if !errors.Is(err, ErrKubectlTimeout) {
		t.Errorf("expected ErrKubectlTimeout, got: %v", err)
	}
	var ke *KubectlError
	if !errors.As(err, &ke) {
		t.Fatalf("expected *KubectlError, got %T", err)
	}
	if !strings.Contains(ke.Stderr, "timeout after") {
		t.Errorf("timeout message missing: %q", ke.Stderr)
	}
}
