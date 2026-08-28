//go:build !windows

package service

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func withFakeClaudeOnPath(t *testing.T, script string) {
	t.Helper()
	directory := t.TempDir()
	binaryPath := filepath.Join(directory, "claude")
	writeTestExecutable(t, binaryPath, "#!/bin/sh\n"+script+"\n")
	t.Setenv("PATH", directory+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func TestDetectProviderFindsConfiguredCLI(t *testing.T) {
	withFakeClaudeOnPath(t, "printf 'ok'")
	t.Setenv("GEMINI_API_KEY", "")
	t.Setenv("OLLAMA_MODEL", "")
	t.Setenv("OLLAMA_API_URL", "")
	t.Setenv("OLLAMA_ENABLED", "")
	t.Setenv("MOONSHOT_API_KEY", "")
	t.Setenv("CLAUDE_CLI", "1")

	provider, err := DetectProvider()
	if err != nil {
		t.Fatalf("detect provider: %v", err)
	}
	if provider == nil || provider.Name() != "Claude CLI" {
		t.Fatalf("provider = %v, want configured CLI provider", provider)
	}
}

func TestClaudeCLIProviderReportsWorkspaceFailures(t *testing.T) {
	provider := NewClaudeCLIProvider()
	sentinel := errors.New("workspace failed")
	noRemoval := func(string) error { return nil }

	_, err := provider.chatWithWorkspace(
		context.Background(),
		"system",
		"user",
		providerWorkspaceOperations{
			create: func(string, string) (string, error) { return "", sentinel },
			write:  os.WriteFile,
			remove: noRemoval,
		},
	)
	if !errors.Is(err, sentinel) {
		t.Fatalf("create error = %v, want sentinel", err)
	}

	workspace := t.TempDir()
	_, err = provider.chatWithWorkspace(
		context.Background(),
		"system",
		"user",
		providerWorkspaceOperations{
			create: func(string, string) (string, error) { return workspace, nil },
			write:  func(string, []byte, os.FileMode) error { return sentinel },
			remove: noRemoval,
		},
	)
	if !errors.Is(err, sentinel) {
		t.Fatalf("write error = %v, want sentinel", err)
	}
}

func TestClaudeCLIProviderJoinsWorkspaceCleanupFailure(t *testing.T) {
	withFakeClaudeOnPath(t, "printf 'response'")
	provider := NewClaudeCLIProvider()
	cleanupFailure := errors.New("cleanup failed")
	createdWorkspace := ""
	createWorkspace := func(directory, pattern string) (string, error) {
		workspace, err := os.MkdirTemp(directory, pattern)
		createdWorkspace = workspace
		return workspace, err
	}
	t.Cleanup(func() {
		if createdWorkspace != "" {
			_ = os.RemoveAll(createdWorkspace)
		}
	})

	response, err := provider.chatWithWorkspace(
		context.Background(),
		"system",
		"user",
		providerWorkspaceOperations{
			create: createWorkspace,
			write:  os.WriteFile,
			remove: func(string) error { return cleanupFailure },
		},
	)

	if response != "response" || !errors.Is(err, cleanupFailure) {
		t.Fatalf("result = (%q, %v), want response and cleanup failure", response, err)
	}
}

func TestClaudeCLIProviderOperationErrorHandlesMissingCause(t *testing.T) {
	err := NewClaudeCLIProvider().operationError("prepare", nil)
	if err == nil || !strings.Contains(err.Error(), "unknown failure") {
		t.Fatalf("error = %v, want unknown failure", err)
	}
}

type claudeIsolationCapture struct {
	workspaceRoot        string
	argumentsPath        string
	stdinPath            string
	promptCopyPath       string
	promptPathRecord     string
	workingDirectoryPath string
}

func newClaudeIsolationCapture(t *testing.T) claudeIsolationCapture {
	t.Helper()
	captureDirectory := t.TempDir()
	capture := claudeIsolationCapture{
		workspaceRoot:        t.TempDir(),
		argumentsPath:        filepath.Join(captureDirectory, "arguments"),
		stdinPath:            filepath.Join(captureDirectory, "stdin"),
		promptCopyPath:       filepath.Join(captureDirectory, "prompt"),
		promptPathRecord:     filepath.Join(captureDirectory, "prompt-path"),
		workingDirectoryPath: filepath.Join(captureDirectory, "working-directory"),
	}
	t.Setenv("TMPDIR", capture.workspaceRoot)
	t.Setenv("CAPTURE_ARGUMENTS", capture.argumentsPath)
	t.Setenv("CAPTURE_STDIN", capture.stdinPath)
	t.Setenv("CAPTURE_PROMPT", capture.promptCopyPath)
	t.Setenv("CAPTURE_PROMPT_PATH", capture.promptPathRecord)
	t.Setenv("CAPTURE_WORKING_DIRECTORY", capture.workingDirectoryPath)
	return capture
}

func TestClaudeCLIProvider_ChatIsolatesPromptAndUserData(t *testing.T) {
	capture := newClaudeIsolationCapture(t)
	withFakeClaudeOnPath(t, `
printf '%s\n' "$@" > "$CAPTURE_ARGUMENTS"
cat > "$CAPTURE_STDIN"
previous=''
for argument in "$@"; do
  if [ "$previous" = '--system-prompt-file' ]; then
    printf '%s' "$argument" > "$CAPTURE_PROMPT_PATH"
    cp -p "$argument" "$CAPTURE_PROMPT"
  fi
  previous="$argument"
done
pwd > "$CAPTURE_WORKING_DIRECTORY"
printf 'provider response'
`)

	const systemPrompt = "private system prompt"
	const userMessage = "private cluster context"
	response, err := NewClaudeCLIProvider().Chat(context.Background(), systemPrompt, userMessage)
	if err != nil {
		t.Fatalf("chat: %v", err)
	}
	if response != "provider response" {
		t.Fatalf("response = %q, want provider response", response)
	}
	assertClaudeInvocationIsolated(t, capture, systemPrompt, userMessage)
	assertClaudeWorkspaceRemoved(t, capture)
}

func assertClaudeInvocationIsolated(
	t *testing.T,
	capture claudeIsolationCapture,
	systemPrompt string,
	userMessage string,
) {
	t.Helper()
	arguments := readTestFile(t, capture.argumentsPath)
	for _, required := range []string{
		"--system-prompt-file", "--tools", "--disallowedTools", "mcp__*",
		"--strict-mcp-config", "--mcp-config", "{}", "--disable-slash-commands",
		"--setting-sources", "--no-session-persistence",
	} {
		if !strings.Contains(arguments, required) {
			t.Errorf("arguments missing %q: %q", required, arguments)
		}
	}
	if strings.Contains(arguments, systemPrompt) || strings.Contains(arguments, userMessage) {
		t.Fatalf("sensitive content leaked into argv: %q", arguments)
	}
	if got := readTestFile(t, capture.stdinPath); got != userMessage {
		t.Fatalf("stdin = %q, want user message", got)
	}
	if got := readTestFile(t, capture.promptCopyPath); got != systemPrompt {
		t.Fatalf("system prompt file = %q, want prompt", got)
	}
	promptInfo, err := os.Stat(capture.promptCopyPath)
	if err != nil {
		t.Fatalf("stat copied prompt: %v", err)
	}
	if promptInfo.Mode().Perm() != 0o600 {
		t.Fatalf("prompt mode = %o, want 600", promptInfo.Mode().Perm())
	}
}

func assertClaudeWorkspaceRemoved(t *testing.T, capture claudeIsolationCapture) {
	t.Helper()
	promptPath := readTestFile(t, capture.promptPathRecord)
	if _, err := os.Stat(promptPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("prompt path still exists: %v", err)
	}
	workingDirectory := strings.TrimSpace(readTestFile(t, capture.workingDirectoryPath))
	if workingDirectory != filepath.Dir(promptPath) {
		t.Fatalf("working directory = %q, want %q", workingDirectory, filepath.Dir(promptPath))
	}
	entries, err := os.ReadDir(capture.workspaceRoot)
	if err != nil {
		t.Fatalf("read workspace root: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("temporary workspace was not removed: %v", entries)
	}
}

func TestClaudeCLIProvider_ChatRejectsEmptyOutput(t *testing.T) {
	withFakeClaudeOnPath(t, `exit 0`)
	_, err := NewClaudeCLIProvider().Chat(context.Background(), "sys", "user")
	if !errors.Is(err, ErrProviderEmptyResponse) {
		t.Fatalf("error = %v, want ErrProviderEmptyResponse", err)
	}
}

func TestClaudeCLIProvider_ChatPreservesExitErrorAndDiagnostic(t *testing.T) {
	withFakeClaudeOnPath(t, `printf '\033[31mdenied\033[0m' >&2; exit 7`)
	_, err := NewClaudeCLIProvider().Chat(context.Background(), "sys", "user")
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("error = %v, want exec.ExitError", err)
	}
	if !strings.Contains(err.Error(), "denied") || strings.Contains(err.Error(), "\x1b") {
		t.Fatalf("diagnostic was not sanitized: %q", err)
	}
}

func TestClaudeCLIProvider_ChatHonorsCancellation(t *testing.T) {
	withFakeClaudeOnPath(t, `while :; do :; done`)
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	_, err := NewClaudeCLIProvider().Chat(ctx, "sys", "user")
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("error = %v, want context deadline", err)
	}
}

func TestClaudeCLIProvider_ChatBoundsOutput(t *testing.T) {
	outputBytes := strconv.Itoa(maxProviderResponseBytes + 1)
	withFakeClaudeOnPath(t, `yes x | head -c `+outputBytes)
	_, err := NewClaudeCLIProvider().Chat(context.Background(), "sys", "user")
	if !errors.Is(err, ErrProviderResponseTooLarge) {
		t.Fatalf("error = %v, want ErrProviderResponseTooLarge", err)
	}
}
