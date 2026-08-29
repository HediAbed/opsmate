package analysis

import (
	"errors"
	"fmt"
	"strings"

	"github.com/HediAbed/opsmate/failure"
	"github.com/google/shlex"
)

type CommandRisk uint8

const (
	RiskUnknown CommandRisk = iota
	RiskReadOnly
	RiskMutating
	RiskDestructive
)

func ClassifyKubectlCommand(command string) (CommandRisk, string) {
	arguments, err := shlex.Split(command)
	if err != nil || len(arguments) < 2 || arguments[0] != "kubectl" {
		return RiskUnknown, "Cannot classify"
	}
	if _, err := validateKubectl(command); err == nil {
		return RiskReadOnly, "Read-only"
	}
	verb := arguments[1]
	switch verb {
	case "scale", "rollout", "label", "annotate", "set", "patch", "apply", "edit":
		return RiskMutating, "Mutates cluster state"
	case "delete", "drain", "cordon", "uncordon", "taint", "evict":
		return RiskDestructive, "Destructive: IRREVERSIBLE"
	}
	return RiskUnknown, "Unknown verb"
}

// ErrEmptyCommand is returned when validateKubectl receives a blank string.
var ErrEmptyCommand = errors.New("empty command")

// ErrForbiddenCommand is returned when a command is rejected by the read-only
// kubectl policy. The policy prevents provider suggestions from changing the
// target cluster or identity when a user copies them into a terminal.
var ErrForbiddenCommand = errors.New("command not allowed by read-only policy")

var ErrSensitiveDataCommand = errors.New("command may expose sensitive cluster data")

var ErrCommandScope = errors.New("command exceeds namespace scope")

type CommandPolicyError struct {
	Detail string
	Err    error
}

func (e *CommandPolicyError) Error() string {
	if e == nil {
		return "kubectl policy: unknown error"
	}
	if e.Detail != "" {
		return "kubectl policy: " + e.Detail
	}
	if e.Err != nil {
		return "kubectl policy: " + e.Err.Error()
	}
	return "kubectl policy: unknown error"
}

func (e *CommandPolicyError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

func (e *CommandPolicyError) FailureCode() failure.Code {
	if e == nil || e.Err == nil {
		return failure.CodeUnknown
	}
	if errors.Is(e.Err, ErrEmptyCommand) {
		return failure.CodeInvalidArgument
	}
	if errors.Is(e.Err, ErrForbiddenCommand) ||
		errors.Is(e.Err, ErrSensitiveDataCommand) ||
		errors.Is(e.Err, ErrCommandScope) {
		return failure.CodePermissionDenied
	}
	return failure.CodeUnknown
}

// readOnlySubcommands lists kubectl subcommands accepted as command
// suggestions. Mutating operations use the dedicated UI actions, which carry
// their own confirmation dialogs.
var readOnlySubcommands = map[string]struct{}{
	"get":           {},
	"describe":      {},
	"logs":          {},
	"top":           {},
	"explain":       {},
	"version":       {},
	"api-resources": {},
	"api-versions":  {},
	"cluster-info":  {},
	"events":        {},
}

// readOnlyCompoundSubcommands lists compound commands accepted as
// suggestions. The outer key is the first subcommand. The inner set holds
// allowed second-level verbs.
var readOnlyCompoundSubcommands = map[string]map[string]struct{}{
	"config": {
		"current-context": {},
		"get-clusters":    {},
		"get-contexts":    {},
		"get-users":       {},
	},
	"auth": {
		"can-i":  {},
		"whoami": {},
	},
}

// forbiddenFlags can retarget kubectl at a different cluster, identity, or
// authentication source. Suggestions containing one are always rejected.
var forbiddenFlags = []string{
	"--kubeconfig",
	"--as",
	"--as-group",
	"--as-uid",
	"--as-user-extra",
	"--cache-dir",
	"--server",
	"--token",
	"--insecure-skip-tls-verify",
	"--cluster",
	"--user",
	"--context",
	"--client-certificate",
	"--client-key",
	"--certificate-authority",
	"--kuberc",
	"--password",
	"--profile",
	"--profile-output",
	"--tls-server-name",
	"--username",
}

// kubectl exposes -s as the short form of --server. Attached values such as
// -s=https://other.example are valid too, so every single-dash token beginning
// with -s must be rejected. Long options such as --selector remain unaffected.
const forbiddenServerShortFlag = "-s"

const minimumCompoundCommandParts = 2

// validateKubectl returns validated arguments without the executable name.
func validateKubectl(command string) ([]string, error) {
	tokens, err := shlex.Split(command)
	if err != nil {
		return nil, fmt.Errorf("%w: parse error: %w", ErrForbiddenCommand, err)
	}
	if len(tokens) == 0 {
		return nil, ErrEmptyCommand
	}
	if tokens[0] != "kubectl" {
		return nil, fmt.Errorf("%w: must start with 'kubectl'", ErrForbiddenCommand)
	}

	arguments := tokens[1:]
	if err := checkSensitiveDataAccess(arguments); err != nil {
		return nil, err
	}
	if err := checkSubcommand(arguments); err != nil {
		return nil, err
	}
	if err := checkFlags(arguments); err != nil {
		return nil, err
	}
	return arguments, nil
}

func checkSensitiveDataAccess(arguments []string) error {
	if containsKubectlFlag(arguments, "--raw") {
		return sensitiveDataPolicyError("raw API reads are disabled")
	}
	if len(arguments) == 0 || (arguments[0] != "get" && arguments[0] != "describe") {
		return nil
	}
	if containsManifestReadFlag(arguments) {
		return sensitiveDataPolicyError("manifest-based reads are disabled")
	}
	resource := kubectlResourceArgument(arguments[1:])
	if containsSecretResource(resource) {
		return sensitiveDataPolicyError("reading Secret resources is disabled")
	}
	return nil
}

func containsKubectlFlag(arguments []string, flag string) bool {
	for _, argument := range arguments {
		if argument == flag || strings.HasPrefix(argument, flag+"=") {
			return true
		}
	}
	return false
}

func containsManifestReadFlag(arguments []string) bool {
	for _, argument := range arguments {
		flag, _, _ := strings.Cut(argument, "=")
		switch flag {
		case "--filename", "-f", "--kustomize", "-k":
			return true
		}
	}
	return false
}

func containsSecretResource(resourceArgument string) bool {
	for _, resourceName := range strings.Split(resourceArgument, ",") {
		resourceName, _, _ = strings.Cut(strings.ToLower(resourceName), "/")
		resourceName, _, _ = strings.Cut(resourceName, ".")
		if resourceName == "secret" || resourceName == "secrets" {
			return true
		}
	}
	return false
}

func kubectlResourceArgument(arguments []string) string {
	for index := 0; index < len(arguments); index++ {
		argument := arguments[index]
		if !strings.HasPrefix(argument, "-") {
			return argument
		}
		if strings.Contains(argument, "=") || !kubectlFlagConsumesValue(argument) {
			continue
		}
		index++
	}
	return ""
}

func kubectlFlagConsumesValue(flag string) bool {
	switch flag {
	case "-n", "--namespace", "-o", "--output", "-l", "--selector", "--field-selector",
		"--sort-by", "--chunk-size", "--request-timeout", "--template", "--subresource":
		return true
	default:
		return false
	}
}

func sensitiveDataPolicyError(detail string) error {
	return &CommandPolicyError{
		Detail: detail,
		Err:    errors.Join(ErrForbiddenCommand, ErrSensitiveDataCommand),
	}
}

// checkSubcommand validates that the first non-kubectl token is a known
// read-only subcommand, and for compound commands (`config`, `auth`), that
// the second token is also in the safe set.
func checkSubcommand(arguments []string) error {
	if len(arguments) == 0 {
		return fmt.Errorf("%w: missing subcommand", ErrForbiddenCommand)
	}
	subcommand := arguments[0]

	if _, allowed := readOnlySubcommands[subcommand]; allowed {
		if subcommand == "cluster-info" && len(arguments) > 1 && arguments[1] == "dump" {
			return sensitiveDataPolicyError("cluster-wide diagnostic dumps are disabled")
		}
		return nil
	}
	if verbs, allowed := readOnlyCompoundSubcommands[subcommand]; allowed {
		if len(arguments) < minimumCompoundCommandParts {
			return fmt.Errorf("%w: %q requires a read-only verb", ErrForbiddenCommand, subcommand)
		}
		if _, verbAllowed := verbs[arguments[1]]; verbAllowed {
			return nil
		}
		return fmt.Errorf("%w: %q %q is not a read-only verb", ErrForbiddenCommand, subcommand, arguments[1])
	}
	return fmt.Errorf("%w: %q is a mutating or unsupported subcommand", ErrForbiddenCommand, subcommand)
}

func scopeKubectlCommand(command, namespace string) (string, error) {
	if strings.TrimSpace(namespace) == "" {
		return "", commandScopeError("namespace is required")
	}
	arguments, err := validateKubectl(command)
	if err != nil {
		return "", err
	}
	foundNamespace, found, err := explicitCommandNamespace(arguments)
	if err != nil {
		return "", err
	}
	if found {
		if foundNamespace != namespace {
			return "", commandScopeError(fmt.Sprintf(
				"namespace %q does not match active namespace %q",
				foundNamespace,
				namespace,
			))
		}
		return command, nil
	}
	return strings.TrimSpace(command) + " --namespace=" + quoteShellWord(namespace), nil
}

func explicitCommandNamespace(arguments []string) (namespace string, found bool, returnErr error) {
	for index := 0; index < len(arguments); index++ {
		argument := arguments[index]
		if isAllNamespacesFlag(argument) {
			return "", false, commandScopeError("all-namespaces access is disabled")
		}
		candidate, consumedNext, isNamespaceFlag, err := parseNamespaceFlag(arguments, index)
		if err != nil {
			return "", false, err
		}
		if !isNamespaceFlag {
			continue
		}
		if consumedNext {
			index++
		}
		namespace, found, err = mergeCommandNamespace(namespace, found, candidate)
		if err != nil {
			return "", false, err
		}
	}
	return namespace, found, nil
}

func isAllNamespacesFlag(argument string) bool {
	return argument == "-A" || argument == "--all-namespaces" ||
		strings.HasPrefix(argument, "-A=") || strings.HasPrefix(argument, "--all-namespaces=")
}

func parseNamespaceFlag(arguments []string, index int) (value string, consumedNext bool, found bool, err error) {
	argument := arguments[index]
	switch {
	case argument == "-n" || argument == "--namespace":
		if index+1 >= len(arguments) {
			return "", false, false, commandScopeError("namespace flag requires a value")
		}
		return arguments[index+1], true, true, nil
	case strings.HasPrefix(argument, "--namespace="):
		return strings.TrimPrefix(argument, "--namespace="), false, true, nil
	case strings.HasPrefix(argument, "-n="):
		return strings.TrimPrefix(argument, "-n="), false, true, nil
	case strings.HasPrefix(argument, "-n") && !strings.HasPrefix(argument, "--"):
		return strings.TrimPrefix(argument, "-n"), false, true, nil
	default:
		return "", false, false, nil
	}
}

func mergeCommandNamespace(current string, found bool, candidate string) (string, bool, error) {
	if candidate == "" {
		return "", false, commandScopeError("namespace flag requires a value")
	}
	if found && candidate != current {
		return "", false, commandScopeError("conflicting namespace flags")
	}
	return candidate, true, nil
}

func commandScopeError(detail string) error {
	return &CommandPolicyError{Detail: detail, Err: errors.Join(ErrForbiddenCommand, ErrCommandScope)}
}

func quoteShellWord(value string) string {
	return "'" + strings.ReplaceAll(value, "'", `'"'"'`) + "'"
}

// checkFlags rejects arguments that match any of the forbiddenFlags, either
// as a bare token ("--server") or in the attached form ("--server=URL").
func checkFlags(arguments []string) error {
	for _, argument := range arguments {
		if strings.HasPrefix(argument, forbiddenServerShortFlag) && !strings.HasPrefix(argument, "--") {
			return fmt.Errorf("%w: flag %q may redirect cluster target or auth", ErrForbiddenCommand, argument)
		}
		for _, forbiddenFlag := range forbiddenFlags {
			if argument == forbiddenFlag || strings.HasPrefix(argument, forbiddenFlag+"=") {
				return fmt.Errorf("%w: flag %q may redirect cluster target or auth", ErrForbiddenCommand, argument)
			}
		}
	}
	return nil
}
