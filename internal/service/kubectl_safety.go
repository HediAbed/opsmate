package service

import (
	"errors"
	"fmt"
	"strings"

	"github.com/google/shlex"
)

type CommandRisk int

const (
	RiskUnknown CommandRisk = iota
	RiskReadOnly
	RiskMutating
	RiskDestructive
)

func ClassifyKubectlCommand(cmdStr string) (CommandRisk, string) {
	args, err := shlex.Split(cmdStr)
	if err != nil || len(args) < 2 || args[0] != "kubectl" {
		return RiskUnknown, "Cannot classify"
	}
	if _, err := validateKubectl(cmdStr); err == nil {
		return RiskReadOnly, "Read-only"
	}
	verb := args[1]
	switch verb {
	case "scale", "rollout", "label", "annotate", "set", "patch", "apply", "edit":
		return RiskMutating, "Mutates cluster state"
	case "delete", "drain", "cordon", "uncordon", "taint", "evict":
		return RiskDestructive, "Destructive — IRREVERSIBLE"
	}
	return RiskUnknown, "Unknown verb"
}

// ErrEmptyCommand is returned when validateKubectl receives a blank string.
var ErrEmptyCommand = errors.New("empty command")

// ErrForbiddenCommand is returned when a command is rejected by the read-only
// kubectl policy. Provider-proposed commands still require this boundary even
// after user approval because flags can change the target cluster or identity.
var ErrForbiddenCommand = errors.New("command not allowed by read-only policy")

var ErrSensitiveDataCommand = errors.New("command may expose sensitive cluster data")

var ErrCommandScope = errors.New("command exceeds namespace scope")

type CommandPolicyError struct {
	Detail string
	Err    error
}

func (e *CommandPolicyError) Error() string {
	if e.Detail != "" {
		return "kubectl policy: " + e.Detail
	}
	if e.Err != nil {
		return "kubectl policy: " + e.Err.Error()
	}
	return "kubectl policy: unknown error"
}

func (e *CommandPolicyError) Unwrap() error {
	return e.Err
}

// readOnlyTopLevel lists kubectl subcommands that are read-only and can be
// executed without further checks. Mutating operations (scale/delete/restart)
// must go through the dedicated UI actions, which carry their own confirm
// dialogs.
var readOnlyTopLevel = map[string]struct{}{
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

// readOnlyCompound lists compound subcommands (`kubectl config ...`,
// `kubectl auth ...`) whose read-only verbs are safe. The outer key is the
// first sub-command; the inner set holds allowed second-level verbs.
var readOnlyCompound = map[string]map[string]struct{}{
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

// forbiddenFlags are global flags that can retarget kubectl at a different
// cluster, identity, or authentication source. Allowing them would let a
// crafted command bypass the safety policy entirely, so we reject regardless
// of the subcommand.
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

const compoundCommandParts = 2

// validateKubectl tokenises a shell-quoted command string and returns the
// arguments to pass to exec.Command (without the leading "kubectl") if and
// only if the command is accepted by the read-only policy.
func validateKubectl(cmdStr string) ([]string, error) {
	tokens, err := shlex.Split(cmdStr)
	if err != nil {
		return nil, fmt.Errorf("%w: parse error: %w", ErrForbiddenCommand, err)
	}
	if len(tokens) == 0 {
		return nil, ErrEmptyCommand
	}
	if tokens[0] != "kubectl" {
		return nil, fmt.Errorf("%w: must start with 'kubectl'", ErrForbiddenCommand)
	}

	args := tokens[1:]
	if err := checkSensitiveDataAccess(args); err != nil {
		return nil, err
	}
	if err := checkSubcommand(args); err != nil {
		return nil, err
	}
	if err := checkFlags(args); err != nil {
		return nil, err
	}
	return args, nil
}

func checkSensitiveDataAccess(args []string) error {
	if containsKubectlFlag(args, "--raw") {
		return sensitiveDataPolicyError("raw API reads are disabled")
	}
	if len(args) == 0 || (args[0] != "get" && args[0] != "describe") {
		return nil
	}
	if containsManifestReadFlag(args) {
		return sensitiveDataPolicyError("manifest-based reads are disabled")
	}
	resource := kubectlResourceArgument(args[1:])
	if containsSecretResource(resource) {
		return sensitiveDataPolicyError("reading Secret resources is disabled")
	}
	return nil
}

func containsKubectlFlag(args []string, flag string) bool {
	for _, argument := range args {
		if argument == flag || strings.HasPrefix(argument, flag+"=") {
			return true
		}
	}
	return false
}

func containsManifestReadFlag(args []string) bool {
	for _, argument := range args {
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

func kubectlResourceArgument(args []string) string {
	for index := 0; index < len(args); index++ {
		argument := args[index]
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
func checkSubcommand(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("%w: missing subcommand", ErrForbiddenCommand)
	}
	sub := args[0]

	if _, ok := readOnlyTopLevel[sub]; ok {
		if sub == "cluster-info" && len(args) > 1 && args[1] == "dump" {
			return sensitiveDataPolicyError("cluster-wide diagnostic dumps are disabled")
		}
		return nil
	}
	if verbs, ok := readOnlyCompound[sub]; ok {
		if len(args) < compoundCommandParts {
			return fmt.Errorf("%w: %q requires a read-only verb", ErrForbiddenCommand, sub)
		}
		if _, allowed := verbs[args[1]]; allowed {
			return nil
		}
		return fmt.Errorf("%w: %q %q is not a read-only verb", ErrForbiddenCommand, sub, args[1])
	}
	return fmt.Errorf("%w: %q is a mutating or unsupported subcommand", ErrForbiddenCommand, sub)
}

func scopeKubectlCommand(command, namespace string) (string, error) {
	if strings.TrimSpace(namespace) == "" {
		return "", commandScopeError("namespace is required")
	}
	args, err := validateKubectl(command)
	if err != nil {
		return "", err
	}
	foundNamespace, found, err := explicitCommandNamespace(args)
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

func explicitCommandNamespace(args []string) (namespace string, found bool, returnErr error) {
	for index := 0; index < len(args); index++ {
		argument := args[index]
		if isAllNamespacesFlag(argument) {
			return "", false, commandScopeError("all-namespaces access is disabled")
		}
		candidate, consumedNext, isNamespaceFlag, err := parseNamespaceFlag(args, index)
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

func parseNamespaceFlag(args []string, index int) (value string, consumedNext bool, found bool, err error) {
	argument := args[index]
	switch {
	case argument == "-n" || argument == "--namespace":
		if index+1 >= len(args) {
			return "", false, false, commandScopeError("namespace flag requires a value")
		}
		return args[index+1], true, true, nil
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
func checkFlags(args []string) error {
	for _, a := range args {
		if strings.HasPrefix(a, forbiddenServerShortFlag) && !strings.HasPrefix(a, "--") {
			return fmt.Errorf("%w: flag %q may redirect cluster target or auth", ErrForbiddenCommand, a)
		}
		for _, bad := range forbiddenFlags {
			if a == bad || strings.HasPrefix(a, bad+"=") {
				return fmt.Errorf("%w: flag %q may redirect cluster target or auth", ErrForbiddenCommand, a)
			}
		}
	}
	return nil
}
