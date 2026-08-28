package service

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func newFakeKubectl(t *testing.T, script string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "kubectl")
	contents := "#!/bin/sh\n" + script + "\n"
	writeTestExecutable(t, path, contents)
	return path
}

func withFakeKubectl(t *testing.T, path string) {
	t.Helper()
	t.Setenv("PATH", filepath.Dir(path)+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func TestStartShellSession_RequiresNamespaceAndPod(t *testing.T) {
	if _, err := StartShellSession("", "pod", ""); err == nil {
		t.Error("empty namespace must error")
	}
	if _, err := StartShellSession("ns", "", ""); err == nil {
		t.Error("empty pod must error")
	}
}

func TestStartShellSession_StreamsStdoutLines(t *testing.T) {
	path := newFakeKubectl(t, `printf "alpha\nbeta\n"; cat`)
	withFakeKubectl(t, path)

	s, err := StartShellSession("ns", "pod", "")
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	defer s.Close()

	got := drainLines(t, s.Output(), 2, 2*time.Second)
	if got[0].Line != "alpha" || got[1].Line != "beta" {
		t.Errorf("unexpected lines: %+v", got)
	}
	if got[0].Stderr || got[1].Stderr {
		t.Error("stdout lines should not be flagged as stderr")
	}
}

func TestStartShellSession_StreamsStderrLines(t *testing.T) {
	path := newFakeKubectl(t, `printf "boom\n" 1>&2; cat`)
	withFakeKubectl(t, path)

	s, err := StartShellSession("ns", "pod", "")
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	defer s.Close()

	got := drainLines(t, s.Output(), 1, 2*time.Second)
	if got[0].Line != "boom" || !got[0].Stderr {
		t.Errorf("expected stderr 'boom', got %+v", got[0])
	}
}

func TestStartShellSession_SendForwardsToStdin(t *testing.T) {
	path := newFakeKubectl(t, `while IFS= read -r line; do echo "got:$line"; done`)
	withFakeKubectl(t, path)

	s, err := StartShellSession("ns", "pod", "")
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	defer s.Close()

	if err := s.Send("hello"); err != nil {
		t.Fatalf("send: %v", err)
	}
	got := drainLines(t, s.Output(), 1, 2*time.Second)
	if got[0].Line != "got:hello" {
		t.Errorf("expected echoed line, got %q", got[0].Line)
	}
}

func TestStartShellSession_CloseTerminates(t *testing.T) {
	path := newFakeKubectl(t, `cat`)
	withFakeKubectl(t, path)

	s, err := StartShellSession("ns", "pod", "")
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	s.Close()

	select {
	case <-s.Exit():
	case <-time.After(2 * time.Second):
		t.Fatal("Close did not terminate the session within 2s")
	}
}

func TestStartShellSession_SendAfterCloseFails(t *testing.T) {
	path := newFakeKubectl(t, `cat`)
	withFakeKubectl(t, path)

	s, err := StartShellSession("ns", "pod", "")
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	s.Close()
	err = s.Send("ignored")
	if err == nil {
		t.Fatal("Send after Close must error")
	}
	if !errors.Is(err, ErrShellSessionClosed) {
		t.Errorf("Send after Close must return ErrShellSessionClosed; got %v", err)
	}
}

func TestStartShellSession_NamespaceErrorIsTyped(t *testing.T) {
	if _, err := StartShellSession("", "pod", ""); !errors.Is(err, ErrShellNamespaceRequired) {
		t.Errorf("empty namespace must return ErrShellNamespaceRequired; got %v", err)
	}
	if _, err := StartShellSession("ns", "", ""); !errors.Is(err, ErrShellPodRequired) {
		t.Errorf("empty pod must return ErrShellPodRequired; got %v", err)
	}
}

func TestStartShellSession_RealShellRoundTrip(t *testing.T) {
	path := newFakeKubectl(t, `exec /bin/sh`)
	withFakeKubectl(t, path)

	s, err := StartShellSession("ns", "pod", "")
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	defer s.Close()

	if err := s.Send("echo first"); err != nil {
		t.Fatalf("send 1: %v", err)
	}
	if err := s.Send("X=42; echo X=$X"); err != nil {
		t.Fatalf("send 2: %v", err)
	}
	if err := s.Send("cd /tmp; pwd"); err != nil {
		t.Fatalf("send 3: %v", err)
	}
	if err := s.Send("echo X-still=$X"); err != nil {
		t.Fatalf("send 4: %v", err)
	}

	got := drainLines(t, s.Output(), 4, 3*time.Second)
	want := []string{"first", "X=42", "/tmp", "X-still=42"}
	for i, w := range want {
		if got[i].Line != w {
			t.Errorf("line %d: got %q want %q", i, got[i].Line, w)
		}
		if got[i].Stderr {
			t.Errorf("line %d should be stdout, got stderr: %q", i, got[i].Line)
		}
	}
}

func TestStartShellSession_PassesContainerArg(t *testing.T) {
	path := newFakeKubectl(t, `printf '%s\n' "$@"`)
	withFakeKubectl(t, path)

	s, err := StartShellSession("ns", "pod", "side")
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	defer s.Close()

	got := drainLinesUntilExit(s.Output(), 2*time.Second)
	joined := joinLines(got)
	if !strings.Contains(joined, "-c\nside") {
		t.Errorf("expected -c side in args, got:\n%s", joined)
	}
}

func drainLines(t *testing.T, ch <-chan ShellOutput, n int, timeout time.Duration) []ShellOutput {
	t.Helper()
	var out []ShellOutput
	deadline := time.After(timeout)
	for len(out) < n {
		select {
		case line, ok := <-ch:
			if !ok {
				t.Fatalf("channel closed before %d lines (got %d)", n, len(out))
			}
			out = append(out, line)
		case <-deadline:
			t.Fatalf("timed out waiting for %d lines (got %d)", n, len(out))
		}
	}
	return out
}

func drainLinesUntilExit(ch <-chan ShellOutput, timeout time.Duration) []ShellOutput {
	var out []ShellOutput
	deadline := time.After(timeout)
	for {
		select {
		case line, ok := <-ch:
			if !ok {
				return out
			}
			out = append(out, line)
		case <-deadline:
			return out
		}
	}
}

func joinLines(lines []ShellOutput) string {
	var b strings.Builder
	for _, l := range lines {
		b.WriteString(l.Line)
		b.WriteByte('\n')
	}
	return b.String()
}
