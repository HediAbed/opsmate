package service

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

const (
	providerWorkspacePattern = "opsmate-provider-*"
	providerPromptFilename   = "system-prompt.txt"
	maxProviderCLIErrorBytes = 64 * 1024
	providerPromptFileMode   = 0o600
)

type ClaudeCLIProvider struct{}

type providerWorkspaceOperations struct {
	create func(string, string) (string, error)
	write  func(string, []byte, os.FileMode) error
	remove func(string) error
}

func NewClaudeCLIProvider() *ClaudeCLIProvider {
	return &ClaudeCLIProvider{}
}

func (*ClaudeCLIProvider) Name() string { return "Claude CLI" }

func (c *ClaudeCLIProvider) Chat(
	ctx context.Context,
	systemPrompt string,
	userMessage string,
) (response string, returnErr error) {
	operations := providerWorkspaceOperations{
		create: os.MkdirTemp,
		write:  os.WriteFile,
		remove: os.RemoveAll,
	}
	return c.chatWithWorkspace(ctx, systemPrompt, userMessage, operations)
}

func (c *ClaudeCLIProvider) chatWithWorkspace(
	ctx context.Context,
	systemPrompt string,
	userMessage string,
	operations providerWorkspaceOperations,
) (response string, returnErr error) {
	workspace, err := operations.create("", providerWorkspacePattern)
	if err != nil {
		return "", c.operationError("create workspace", err)
	}
	defer func() {
		if cleanupErr := operations.remove(workspace); cleanupErr != nil {
			returnErr = errors.Join(returnErr, c.operationError("remove workspace", cleanupErr))
		}
	}()

	promptPath := filepath.Join(workspace, providerPromptFilename)
	if err := operations.write(promptPath, []byte(systemPrompt), providerPromptFileMode); err != nil {
		return "", c.operationError("write system prompt", err)
	}

	stdout := newLimitedBuffer(maxProviderResponseBytes)
	stderr := newLimitedBuffer(maxProviderCLIErrorBytes)
	command := newExternalCommandContext(ctx, "claude", claudeCLIArguments(promptPath)...)
	command.Dir = workspace
	command.Stdin = strings.NewReader(userMessage)
	command.Stdout = stdout
	command.Stderr = stderr
	if err := command.Run(); err != nil {
		if ctx.Err() != nil {
			err = ctx.Err()
		}
		return "", &ProviderError{
			Provider:  c.Name(),
			Operation: "run CLI",
			Detail:    truncateText(strings.TrimSpace(stripANSI(stderr.String())), maxProviderErrorDetailRunes),
			Err:       err,
		}
	}
	if stdout.Truncated() {
		return "", c.operationError("read CLI response", ErrProviderResponseTooLarge)
	}

	response = strings.TrimSpace(stripANSI(stdout.String()))
	if response == "" {
		return "", c.operationError("read CLI response", ErrProviderEmptyResponse)
	}
	return response, nil
}

func claudeCLIArguments(promptPath string) []string {
	return []string{
		"-p",
		"--output-format", "text",
		"--no-session-persistence",
		"--system-prompt-file", promptPath,
		"--tools", "",
		"--disallowedTools", "mcp__*",
		"--strict-mcp-config",
		"--mcp-config", "{}",
		"--disable-slash-commands",
		"--setting-sources", "",
	}
}

func (c *ClaudeCLIProvider) operationError(operation string, err error) error {
	if err == nil {
		err = errors.New("unknown failure")
	}
	return &ProviderError{Provider: c.Name(), Operation: operation, Err: err}
}
