package service

import (
	"errors"
	"testing"
	"time"
)

func TestFormatAge_Seconds(t *testing.T) {
	tests := []struct {
		duration time.Duration
		want     string
	}{
		{0 * time.Second, "0s"},
		{1 * time.Second, "1s"},
		{30 * time.Second, "30s"},
		{59 * time.Second, "59s"},
	}
	for _, tt := range tests {
		got := formatAge(tt.duration)
		if got != tt.want {
			t.Errorf("formatAge(%v) = %q; want %q", tt.duration, got, tt.want)
		}
	}
}

func TestFormatAge_Minutes(t *testing.T) {
	tests := []struct {
		duration time.Duration
		want     string
	}{
		{1 * time.Minute, "1m"},
		{5 * time.Minute, "5m"},
		{30 * time.Minute, "30m"},
		{59 * time.Minute, "59m"},
	}
	for _, tt := range tests {
		got := formatAge(tt.duration)
		if got != tt.want {
			t.Errorf("formatAge(%v) = %q; want %q", tt.duration, got, tt.want)
		}
	}
}

func TestFormatAge_Hours(t *testing.T) {
	tests := []struct {
		duration time.Duration
		want     string
	}{
		{1 * time.Hour, "1h"},
		{12 * time.Hour, "12h"},
		{23 * time.Hour, "23h"},
	}
	for _, tt := range tests {
		got := formatAge(tt.duration)
		if got != tt.want {
			t.Errorf("formatAge(%v) = %q; want %q", tt.duration, got, tt.want)
		}
	}
}

func TestFormatAge_Days(t *testing.T) {
	tests := []struct {
		duration time.Duration
		want     string
	}{
		{24 * time.Hour, "1d"},
		{48 * time.Hour, "2d"},
		{7 * 24 * time.Hour, "7d"},
		{30 * 24 * time.Hour, "30d"},
		{365 * 24 * time.Hour, "365d"},
	}
	for _, tt := range tests {
		got := formatAge(tt.duration)
		if got != tt.want {
			t.Errorf("formatAge(%v) = %q; want %q", tt.duration, got, tt.want)
		}
	}
}

func TestFormatAge_Boundary(t *testing.T) {
	got := formatAge(59*time.Second + 999*time.Millisecond)
	if got != "59s" {
		t.Errorf("formatAge(59.999s) = %q; want %q", got, "59s")
	}

	got = formatAge(60 * time.Second)
	if got != "1m" {
		t.Errorf("formatAge(60s) = %q; want %q", got, "1m")
	}

	got = formatAge(59*time.Minute + 59*time.Second)
	if got != "59m" {
		t.Errorf("formatAge(59m59s) = %q; want %q", got, "59m")
	}

	got = formatAge(60 * time.Minute)
	if got != "1h" {
		t.Errorf("formatAge(60m) = %q; want %q", got, "1h")
	}

	got = formatAge(23*time.Hour + 59*time.Minute)
	if got != "23h" {
		t.Errorf("formatAge(23h59m) = %q; want %q", got, "23h")
	}
}

func TestNamespaceArgs_SpecificNamespace(t *testing.T) {
	got := namespaceArgs("kube-system")
	if len(got) != 2 || got[0] != "-n" || got[1] != "kube-system" {
		t.Errorf("namespaceArgs('kube-system') = %v; want [-n kube-system]", got)
	}
}

func TestNamespaceArgs_AllNamespaces(t *testing.T) {
	got := namespaceArgs("")
	if len(got) != 1 || got[0] != "--all-namespaces" {
		t.Errorf("namespaceArgs('') = %v; want [--all-namespaces]", got)
	}
}

func TestNamespaceArgs_DefaultNamespace(t *testing.T) {
	got := namespaceArgs("default")
	if len(got) != 2 || got[0] != "-n" || got[1] != "default" {
		t.Errorf("namespaceArgs('default') = %v; want [-n default]", got)
	}
}

func TestExecuteCommand_BlocksNonKubectl(t *testing.T) {
	tests := []struct {
		name string
		cmd  string
	}{
		{"rm command", "rm -rf /"},
		{"bash command", "bash -c 'echo pwned'"},
		{"curl command", "curl http://evil.com"},
		{"cat command", "cat /etc/passwd"},
		{"python command", "python -c 'import os; os.system(\"rm -rf /\")'"},
		{"empty command", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := ExecuteCommand(tt.cmd)
			msg := cmd()
			result, ok := msg.(CommandResultMsg)
			if !ok {
				t.Fatalf("expected CommandResultMsg, got %T", msg)
			}
			if result.Err == nil {
				t.Errorf("ExecuteCommand(%q) should return error for non-kubectl command", tt.cmd)
			}
			if tt.cmd != "" && !errors.Is(result.Err, ErrForbiddenCommand) {
				t.Errorf("expected ErrForbiddenCommand, got: %v", result.Err)
			}
		})
	}
}

func TestExecuteCommand_AllowsKubectl(t *testing.T) {
	cmd := ExecuteCommand("kubectl version --client")
	msg := cmd()
	result, ok := msg.(CommandResultMsg)
	if !ok {
		t.Fatalf("expected CommandResultMsg, got %T", msg)
	}
	if result.Err != nil && errors.Is(result.Err, ErrForbiddenCommand) {
		t.Error("kubectl command should not be blocked by allowlist")
	}
}

func TestExecuteCommand_EmptyCommand(t *testing.T) {
	cmd := ExecuteCommand("")
	msg := cmd()
	result, ok := msg.(CommandResultMsg)
	if !ok {
		t.Fatalf("expected CommandResultMsg, got %T", msg)
	}
	if result.Err == nil {
		t.Error("empty command should return error")
	}
	if !errors.Is(result.Err, ErrEmptyCommand) {
		t.Errorf("expected ErrEmptyCommand, got: %v", result.Err)
	}
}
