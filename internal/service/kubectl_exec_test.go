package service

import (
	"errors"
	"strings"
	"testing"
)

func TestKubectlError_ErrorFormat_WithStderr(t *testing.T) {
	err := &KubectlError{
		Subcommand: "get",
		Stderr:     "pods is forbidden",
		Err:        errors.New("exit status 1"),
	}
	want := "kubectl get: pods is forbidden"
	if got := err.Error(); got != want {
		t.Errorf("Error() = %q; want %q", got, want)
	}
}

func TestKubectlError_ErrorFormat_NoStderr(t *testing.T) {
	err := &KubectlError{
		Subcommand: "describe",
		Err:        errors.New("exit status 2"),
	}
	want := "kubectl describe: exit status 2"
	if got := err.Error(); got != want {
		t.Errorf("Error() = %q; want %q", got, want)
	}
}

func TestKubectlError_ErrorFormat_UnknownSubcommand(t *testing.T) {
	err := &KubectlError{Err: errors.New("boom")}
	if got := err.Error(); !strings.HasPrefix(got, "kubectl: ") {
		t.Errorf("Error() = %q; want prefix %q", got, "kubectl: ")
	}
}

func TestKubectlError_ErrorFormat_NilInner(t *testing.T) {
	err := &KubectlError{Subcommand: "get"}
	if got := err.Error(); got != "kubectl get: unknown error" {
		t.Errorf("unexpected format: %q", got)
	}
}

func TestKubectlError_UnwrapsToTimeout(t *testing.T) {
	err := &KubectlError{
		Subcommand: "get",
		Stderr:     "timeout after 15s",
		Err:        ErrKubectlTimeout,
	}
	if !errors.Is(err, ErrKubectlTimeout) {
		t.Error("errors.Is did not match ErrKubectlTimeout")
	}
}

func TestSubcommand_Empty(t *testing.T) {
	if got := subcommand(nil); got != "" {
		t.Errorf("subcommand(nil) = %q; want empty", got)
	}
}

func TestSubcommand_FirstArg(t *testing.T) {
	if got := subcommand([]string{"get", "pods"}); got != "get" {
		t.Errorf("subcommand(get,pods) = %q; want %q", got, "get")
	}
}
