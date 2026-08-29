package analysis

import (
	"errors"
	"reflect"
	"testing"
)

func TestValidateKubectl_Empty(t *testing.T) {
	_, err := validateKubectl("")
	if !errors.Is(err, ErrEmptyCommand) {
		t.Errorf("empty input should return ErrEmptyCommand, got %v", err)
	}
}

func TestCommandPolicyErrorFormatsAndUnwraps(t *testing.T) {
	sentinel := errors.New("failed")
	tests := []struct {
		err  *CommandPolicyError
		want string
	}{
		{err: &CommandPolicyError{Detail: "blocked", Err: sentinel}, want: "kubectl policy: blocked"},
		{err: &CommandPolicyError{Err: sentinel}, want: "kubectl policy: failed"},
		{err: &CommandPolicyError{}, want: "kubectl policy: unknown error"},
	}
	for _, test := range tests {
		if got := test.err.Error(); got != test.want {
			t.Errorf("error = %q, want %q", got, test.want)
		}
	}
	if !errors.Is(&CommandPolicyError{Err: sentinel}, sentinel) {
		t.Fatal("policy error did not unwrap its cause")
	}
}

func TestValidateKubectl_WhitespaceOnly(t *testing.T) {
	_, err := validateKubectl("   \t\n")
	if !errors.Is(err, ErrEmptyCommand) {
		t.Errorf("whitespace-only input should return ErrEmptyCommand, got %v", err)
	}
}

func TestValidateKubectl_UnmatchedQuote(t *testing.T) {
	_, err := validateKubectl(`kubectl get pods -l 'unbalanced`)
	if !errors.Is(err, ErrForbiddenCommand) {
		t.Errorf("shlex parse error should surface as ErrForbiddenCommand, got %v", err)
	}
}

func TestValidateKubectl_NonKubectlPrefixRejected(t *testing.T) {
	tests := []string{
		"rm -rf /",
		"bash -c 'echo pwned'",
		"sudo kubectl get pods",
		"/usr/bin/kubectl get pods",
		"kubectl.exe get pods",
	}
	for _, in := range tests {
		t.Run(in, func(t *testing.T) {
			_, err := validateKubectl(in)
			if !errors.Is(err, ErrForbiddenCommand) {
				t.Errorf("validateKubectl(%q) err = %v; want ErrForbiddenCommand", in, err)
			}
		})
	}
}

func TestValidateKubectl_MissingSubcommand(t *testing.T) {
	_, err := validateKubectl("kubectl")
	if !errors.Is(err, ErrForbiddenCommand) {
		t.Errorf("bare 'kubectl' should be rejected, got %v", err)
	}
}

func TestValidateKubectl_MutatingSubcommandsRejected(t *testing.T) {
	mutating := []string{
		"kubectl apply -f pod.yaml",
		"kubectl delete pod foo",
		"kubectl patch pod foo -p '{}'",
		"kubectl replace -f pod.yaml",
		"kubectl create deployment app --image=nginx",
		"kubectl scale deploy app --replicas=0",
		"kubectl run busybox --image=busybox",
		"kubectl exec -it pod -- sh",
		"kubectl attach pod",
		"kubectl cp /etc/passwd pod:/tmp/pwn",
		"kubectl proxy",
		"kubectl port-forward pod 8080:80",
		"kubectl edit pod foo",
		"kubectl debug pod foo",
		"kubectl label pod foo a=b",
		"kubectl annotate pod foo a=b",
		"kubectl rollout restart deploy/app",
		"kubectl taint node n1 key=value:NoSchedule",
		"kubectl drain node1",
		"kubectl certificate approve csr-1",
	}
	for _, cmd := range mutating {
		t.Run(cmd, func(t *testing.T) {
			_, err := validateKubectl(cmd)
			if !errors.Is(err, ErrForbiddenCommand) {
				t.Errorf("validateKubectl(%q) err = %v; want ErrForbiddenCommand", cmd, err)
			}
		})
	}
}

func TestValidateKubectl_ReadOnlySubcommandsAccepted(t *testing.T) {
	cases := []struct {
		in       string
		wantArgs []string
	}{
		{"kubectl get pods", []string{"get", "pods"}},
		{"kubectl describe pod foo", []string{"describe", "pod", "foo"}},
		{"kubectl logs foo -n default", []string{"logs", "foo", "-n", "default"}},
		{"kubectl top pods", []string{"top", "pods"}},
		{"kubectl explain pod.spec", []string{"explain", "pod.spec"}},
		{"kubectl version --client", []string{"version", "--client"}},
		{"kubectl api-resources", []string{"api-resources"}},
		{"kubectl api-versions", []string{"api-versions"}},
		{"kubectl cluster-info", []string{"cluster-info"}},
		{"kubectl events", []string{"events"}},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			args, err := validateKubectl(tc.in)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !reflect.DeepEqual(args, tc.wantArgs) {
				t.Errorf("args = %v; want %v", args, tc.wantArgs)
			}
		})
	}
}

func TestValidateKubectl_CompoundConfigReadOnlyAccepted(t *testing.T) {
	cases := []string{
		"kubectl config get-contexts",
		"kubectl config current-context",
		"kubectl config get-clusters",
		"kubectl config get-users",
	}
	for _, cmd := range cases {
		t.Run(cmd, func(t *testing.T) {
			if _, err := validateKubectl(cmd); err != nil {
				t.Errorf("%q should be allowed, got: %v", cmd, err)
			}
		})
	}
}

func TestValidateKubectl_RejectsSensitiveReads(t *testing.T) {
	tests := []string{
		"kubectl get secret database -o yaml",
		"kubectl get secrets -A -o json",
		"kubectl get -n default secret/database",
		"kubectl describe secrets database",
		"kubectl get secrets.v1. database -o yaml",
		"kubectl config view --raw",
		"kubectl config view --raw=true",
		"kubectl get --raw /api/v1/namespaces/default/secrets",
		"kubectl get --raw=/api/v1/namespaces/default/secrets",
		"kubectl get -f secret.yaml -o yaml",
		"kubectl get --filename=secret.yaml -o yaml",
		"kubectl get -k overlays/production -o yaml",
		"kubectl get --kustomize=overlays/production -o yaml",
	}
	for _, command := range tests {
		t.Run(command, func(t *testing.T) {
			_, err := validateKubectl(command)
			if !errors.Is(err, ErrSensitiveDataCommand) || !errors.Is(err, ErrForbiddenCommand) {
				t.Fatalf("error = %v, want sensitive-data policy error", err)
			}
			var policyErr *CommandPolicyError
			if !errors.As(err, &policyErr) {
				t.Fatalf("error = %#v, want CommandPolicyError", err)
			}
		})
	}
}

func TestValidateKubectl_RejectsConfigView(t *testing.T) {
	if _, err := validateKubectl("kubectl config view --minify"); !errors.Is(err, ErrForbiddenCommand) {
		t.Fatalf("config view error = %v, want ErrForbiddenCommand", err)
	}
}

func TestValidateKubectl_RejectsDiagnosticDumpAndDiff(t *testing.T) {
	tests := []struct {
		command string
		wantErr error
	}{
		{command: "kubectl cluster-info dump", wantErr: ErrSensitiveDataCommand},
		{command: "kubectl diff", wantErr: ErrForbiddenCommand},
	}
	for _, test := range tests {
		t.Run(test.command, func(t *testing.T) {
			if _, err := validateKubectl(test.command); !errors.Is(err, test.wantErr) {
				t.Fatalf("validateKubectl error = %v, want %v", err, test.wantErr)
			}
		})
	}
}

func TestValidateKubectl_CompoundConfigMutatingRejected(t *testing.T) {
	cases := []string{
		"kubectl config set-context foo",
		"kubectl config use-context foo",
		"kubectl config delete-context foo",
		"kubectl config rename-context a b",
		"kubectl config",
	}
	for _, cmd := range cases {
		t.Run(cmd, func(t *testing.T) {
			_, err := validateKubectl(cmd)
			if !errors.Is(err, ErrForbiddenCommand) {
				t.Errorf("%q should be rejected, got: %v", cmd, err)
			}
		})
	}
}

func TestValidateKubectl_CompoundAuth(t *testing.T) {
	if _, err := validateKubectl("kubectl auth can-i get pods"); err != nil {
		t.Errorf("auth can-i should be allowed, got: %v", err)
	}
	if _, err := validateKubectl("kubectl auth reconcile -f x.yaml"); !errors.Is(err, ErrForbiddenCommand) {
		t.Errorf("auth reconcile should be rejected, got: %v", err)
	}
}

func TestValidateKubectl_ForbiddenFlagsRejected(t *testing.T) {
	cases := []string{
		"kubectl get pods --kubeconfig=/tmp/evil",
		"kubectl get pods --kubeconfig /tmp/evil",
		"kubectl get pods --server=https://evil.example",
		"kubectl get pods --token=abcd",
		"kubectl get pods --as=system:admin",
		"kubectl get pods --as-group=admins",
		"kubectl get pods --as-user-extra=department=security",
		"kubectl get pods --cache-dir=/tmp/cache",
		"kubectl get pods --context=prod",
		"kubectl get pods --cluster=prod",
		"kubectl get pods --user=admin",
		"kubectl get pods --insecure-skip-tls-verify",
		"kubectl get pods --client-certificate=/tmp/c.crt",
		"kubectl get pods --client-key=/tmp/c.key",
		"kubectl get pods --certificate-authority=/tmp/ca.crt",
		"kubectl get pods --kuberc=/tmp/preferences",
		"kubectl get pods --profile=cpu",
		"kubectl get pods --profile-output=/tmp/profile",
		"kubectl get pods --tls-server-name=other.example",
	}
	for _, cmd := range cases {
		t.Run(cmd, func(t *testing.T) {
			_, err := validateKubectl(cmd)
			if !errors.Is(err, ErrForbiddenCommand) {
				t.Errorf("%q should be rejected, got: %v", cmd, err)
			}
		})
	}
}

func TestValidateKubectl_ServerShortFlagRejected(t *testing.T) {
	tests := []string{
		"kubectl get pods -s https://other.example",
		"kubectl get pods -s=https://other.example",
		"kubectl get pods -shttps://other.example",
	}
	for _, command := range tests {
		t.Run(command, func(t *testing.T) {
			_, err := validateKubectl(command)
			if !errors.Is(err, ErrForbiddenCommand) {
				t.Fatalf("validateKubectl(%q) error = %v, want ErrForbiddenCommand", command, err)
			}
		})
	}
}

func TestValidateKubectl_SelectorLongFlagAccepted(t *testing.T) {
	args, err := validateKubectl("kubectl get pods --selector app=web")
	if err != nil {
		t.Fatalf("selector must remain allowed: %v", err)
	}
	want := []string{"get", "pods", "--selector", "app=web"}
	if !reflect.DeepEqual(args, want) {
		t.Fatalf("args = %v, want %v", args, want)
	}
}

func TestValidateKubectl_QuotedArgsPreserved(t *testing.T) {
	args, err := validateKubectl(`kubectl get pods -l "app=web shop"`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []string{"get", "pods", "-l", "app=web shop"}
	if !reflect.DeepEqual(args, want) {
		t.Errorf("args = %v; want %v", args, want)
	}
}

func TestScopeKubectlCommand_AddsActiveNamespace(t *testing.T) {
	command, err := scopeKubectlCommand("kubectl get pods", "operations")
	if err != nil {
		t.Fatalf("scopeKubectlCommand: %v", err)
	}
	args, err := validateKubectl(command)
	if err != nil {
		t.Fatalf("scoped command did not pass validation: %v", err)
	}
	namespace, found, err := explicitCommandNamespace(args)
	if err != nil {
		t.Fatalf("read scoped namespace: %v", err)
	}
	if !found || namespace != "operations" {
		t.Fatalf("scoped namespace = %q, %v; want operations, true", namespace, found)
	}
}

func TestScopeKubectlCommand_PreservesMatchingNamespace(t *testing.T) {
	const command = "kubectl get pods --namespace operations"
	got, err := scopeKubectlCommand(command, "operations")
	if err != nil {
		t.Fatalf("scopeKubectlCommand: %v", err)
	}
	if got != command {
		t.Fatalf("scoped command = %q, want %q", got, command)
	}
}

func TestScopeKubectlCommand_RejectsScopeChanges(t *testing.T) {
	tests := []struct {
		name      string
		command   string
		namespace string
	}{
		{name: "missing active namespace", command: "kubectl get pods"},
		{name: "different namespace", command: "kubectl get pods -n other", namespace: "operations"},
		{name: "all namespaces", command: "kubectl get pods -A", namespace: "operations"},
		{name: "conflicting flags", command: "kubectl get pods -n operations --namespace=other", namespace: "operations"},
		{name: "missing flag value", command: "kubectl get pods -n", namespace: "operations"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := scopeKubectlCommand(test.command, test.namespace)
			if !errors.Is(err, ErrCommandScope) || !errors.Is(err, ErrForbiddenCommand) {
				t.Fatalf("scopeKubectlCommand error = %v, want command-scope policy error", err)
			}
		})
	}
}

func TestScopeKubectlCommand_QuotesNamespaceAsOneArgument(t *testing.T) {
	const namespace = "operations'; --server=other.example"
	command, err := scopeKubectlCommand("kubectl get pods", namespace)
	if err != nil {
		t.Fatalf("scopeKubectlCommand: %v", err)
	}
	args, err := validateKubectl(command)
	if err != nil {
		t.Fatalf("validate scoped command: %v", err)
	}
	got, found, err := explicitCommandNamespace(args)
	if err != nil {
		t.Fatalf("read scoped namespace: %v", err)
	}
	if !found || got != namespace {
		t.Fatalf("namespace = %q, %v; want %q, true", got, found, namespace)
	}
}

func TestKubectlResourceArgumentSkipsFlagForms(t *testing.T) {
	if got := kubectlResourceArgument([]string{"--output=json", "--watch"}); got != "" {
		t.Fatalf("resource = %q, want empty", got)
	}
	if kubectlFlagConsumesValue("--watch") {
		t.Fatal("boolean flag unexpectedly consumes a value")
	}
}

func TestNamespaceFlagParsesAttachedForms(t *testing.T) {
	tests := []struct {
		argument string
		want     string
	}{
		{argument: "-n=operations", want: "operations"},
		{argument: "-noperations", want: "operations"},
	}
	for _, test := range tests {
		value, consumedNext, found, err := parseNamespaceFlag([]string{test.argument}, 0)
		if err != nil || consumedNext || !found || value != test.want {
			t.Fatalf("parse %q = (%q, %t, %t, %v)", test.argument, value, consumedNext, found, err)
		}
	}
}

func TestNamespaceMergeRejectsEmptyAndAcceptsDuplicate(t *testing.T) {
	if _, _, err := mergeCommandNamespace("", false, ""); !errors.Is(err, ErrCommandScope) {
		t.Fatalf("empty namespace error = %v", err)
	}
	value, found, err := mergeCommandNamespace("operations", true, "operations")
	if err != nil || !found || value != "operations" {
		t.Fatalf("duplicate namespace = (%q, %t, %v)", value, found, err)
	}
}
