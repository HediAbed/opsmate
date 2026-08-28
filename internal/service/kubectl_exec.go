package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Timeouts for kubectl operations. Picked so that a stalled cluster (dead VPN,
// unreachable API server) surfaces as an error in the UI instead of hanging
// the Bubble Tea event loop forever.
const (
	KubectlReadTimeout    = 15 * time.Second // list/get/top/jsonpath
	KubectlDetailTimeout  = 30 * time.Second // describe, yaml
	KubectlActionTimeout  = 30 * time.Second // scale, delete, restart
	KubectlLogsTimeout    = 60 * time.Second // logs can be large
	maxKubectlOutput      = 32 * 1024 * 1024
	maxKubectlErrorOutput = 1024 * 1024
)

// ErrKubectlTimeout is returned (via KubectlError) when a kubectl command
// exceeds its deadline. Callers can use errors.Is to detect it.
var ErrKubectlTimeout = errors.New("kubectl command timed out")

var ErrKubectlOutputLimit = errors.New("kubectl output exceeded safety limit")

// ErrNamespaceRequired is returned by the single-resource helpers
// (describe, logs, scale, delete, …) when an empty namespace is passed.
// These commands cannot meaningfully target "all namespaces" — kubectl
// would reject `-n ""` with a confusing error, so we surface a typed
// signal the UI can translate into a clear message.
var ErrNamespaceRequired = errors.New("namespace is required for this operation")

// requireNamespace validates that namespace is non-empty for commands that
// cannot target --all-namespaces. It returns a KubectlError wrapping
// ErrNamespaceRequired so callers can use errors.Is.
func requireNamespace(subcommand, namespace string) error {
	if namespace == "" {
		return &KubectlError{
			Subcommand: subcommand,
			Stderr:     "namespace is required",
			Err:        ErrNamespaceRequired,
		}
	}
	return nil
}

// KubectlError is returned by every kubectl exec helper. It preserves the
// invoked sub-command, any captured stderr, and the underlying error so that
// the UI can show the real kubectl message instead of a generic wrap.
type KubectlError struct {
	Subcommand string
	Stderr     string
	Err        error
}

func (e *KubectlError) Error() string {
	prefix := "kubectl"
	if e.Subcommand != "" {
		prefix = "kubectl " + e.Subcommand
	}
	if e.Stderr != "" {
		return prefix + ": " + e.Stderr
	}
	if e.Err != nil {
		return prefix + ": " + e.Err.Error()
	}
	return prefix + ": unknown error"
}

func (e *KubectlError) Unwrap() error { return e.Err }

// subcommand returns the first positional argument for error reporting.
func subcommand(args []string) string {
	if len(args) == 0 {
		return ""
	}
	return args[0]
}

// runKubectl executes `kubectl <args>` with a context timeout. Stdout and
// stderr are captured separately so stdout can be JSON-decoded without
// corruption while stderr is still available for diagnostics.
func runKubectl(timeout time.Duration, args ...string) ([]byte, error) {
	return runKubectlContext(context.Background(), timeout, args...)
}

func runKubectlContext(parent context.Context, timeout time.Duration, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()

	stdout := newLimitedBuffer(maxKubectlOutput)
	stderr := newLimitedBuffer(maxKubectlErrorOutput)
	cmd := newExternalCommandContext(ctx, kubectlBinary, args...)
	cmd.Stdout = stdout
	cmd.Stderr = stderr

	if err := cmd.Run(); err != nil {
		return nil, kubectlExecutionError(ctx, timeout, args, stderr.String(), err)
	}
	if stdout.Truncated() {
		return nil, &KubectlError{Subcommand: subcommand(args), Err: ErrKubectlOutputLimit}
	}
	return stdout.Bytes(), nil
}

func kubectlExecutionError(ctx context.Context, timeout time.Duration, args []string, stderr string, runErr error) error {
	contextErr := ctx.Err()
	if errors.Is(contextErr, context.DeadlineExceeded) {
		return &KubectlError{
			Subcommand: subcommand(args),
			Stderr:     fmt.Sprintf("timeout after %s", timeout),
			Err:        ErrKubectlTimeout,
		}
	}
	if contextErr != nil {
		runErr = contextErr
	}
	return &KubectlError{
		Subcommand: subcommand(args),
		Stderr:     strings.TrimSpace(stderr),
		Err:        runErr,
	}
}

type kubectlListItem interface {
	rawPod | rawIngressItem | rawNetworkPolicyItem | rawPVCItem | rawCronJobItem |
		rawHPAItem | rawSecretItem | rawReplicaSetItem | rawCRDItem | rawCRDInstance
}

type kubectlProjectedResource interface {
	Pod | Ingress | NetworkPolicy | PersistentVolumeClaim | CronJob | HPA | Secret |
		ReplicaSet | CRD | CRDInstance
}

type kubectlWatchItem interface {
	rawPod | rawIngressItem | rawNetworkPolicyItem | rawPVCItem | rawCronJobItem |
		rawHPAItem | rawSecretItem | rawReplicaSetItem
}

type kubectlJSONPayload interface {
	podList | deploymentList | eventList | serviceList | statefulSetList | daemonSetList |
		configMapList | nodeList | jobList | rbacList | crdList
}

func listKubectlItems[Raw kubectlListItem](resource, namespace string) ([]Raw, error) {
	var payload struct {
		Items []Raw `json:"items"`
	}
	args := listArgsJSON(resource, namespace)
	raw, err := runKubectl(KubectlReadTimeout, args...)
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, kubectlJSONError(args, err)
	}
	return payload.Items, nil
}

func projectListItems[Raw kubectlListItem, Domain kubectlProjectedResource](items []Raw, project func(Raw) Domain) []Domain {
	out := make([]Domain, 0, len(items))
	for _, item := range items {
		out = append(out, project(item))
	}
	return out
}

func decodeWatchObject[Raw kubectlWatchItem, Domain WatchResource](raw json.RawMessage, project func(Raw) Domain) (Domain, error) {
	var item Raw
	var zero Domain
	if err := json.Unmarshal(raw, &item); err != nil {
		return zero, err
	}
	return project(item), nil
}

func runKubectlJSON[T kubectlJSONPayload](timeout time.Duration, args ...string) (T, error) {
	var output T
	raw, err := runKubectl(timeout, args...)
	if err != nil {
		return output, err
	}
	if err := json.Unmarshal(raw, &output); err != nil {
		return output, kubectlJSONError(args, err)
	}
	return output, nil
}

func kubectlJSONError(args []string, err error) error {
	return &KubectlError{
		Subcommand: subcommand(args),
		Stderr:     "parse json: " + err.Error(),
		Err:        err,
	}
}

// runKubectlText executes kubectl and returns the combined stdout+stderr as a
// string. Intended for describe/logs/yaml/action commands where kubectl may
// write informational text to either stream and the user wants to see
// everything. On error, the output is still returned so it can be displayed.
func runKubectlText(timeout time.Duration, args ...string) (string, error) {
	return runKubectlTextContext(context.Background(), timeout, args...)
}

func runKubectlTextContext(parent context.Context, timeout time.Duration, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()

	output := newLimitedBuffer(maxKubectlOutput)
	cmd := newExternalCommandContext(ctx, kubectlBinary, args...)
	cmd.Stdout = output
	cmd.Stderr = output
	runErr := cmd.Run()
	text := stripANSI(output.String())
	if output.Truncated() {
		return text, &KubectlError{
			Subcommand: subcommand(args),
			Stderr:     "output exceeded safety limit",
			Err:        ErrKubectlOutputLimit,
		}
	}
	if runErr != nil {
		return text, kubectlExecutionError(ctx, timeout, args, text, runErr)
	}
	return text, nil
}
