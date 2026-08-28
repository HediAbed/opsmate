package service

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRBACFromRaw_RoleCountsRules(t *testing.T) {
	const payload = `{
		"kind": "Role",
		"metadata": {"name": "reader", "namespace": "ns"},
		"rules": [{"apiGroups": [""], "resources": ["pods"], "verbs": ["get"]},
		          {"apiGroups": [""], "resources": ["services"], "verbs": ["list"]}]
	}`
	var item rawRBACItem
	if err := json.Unmarshal([]byte(payload), &item); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	got := rbacFromRaw(item)
	if got.Kind != "Role" || got.Count != 2 {
		t.Errorf("Role with 2 rules wrong: %+v", got)
	}
	if got.Scope != rbacScopeNamespace {
		t.Errorf("Role scope = %q, want %q", got.Scope, rbacScopeNamespace)
	}
}

func TestRBACFromRaw_RoleBindingCountsSubjects(t *testing.T) {
	const payload = `{
		"kind": "RoleBinding",
		"metadata": {"name": "rb", "namespace": "ns"},
		"subjects": [
			{"kind": "User", "name": "alice"},
			{"kind": "User", "name": "bob"},
			{"kind": "ServiceAccount", "name": "ci"}
		]
	}`
	var item rawRBACItem
	if err := json.Unmarshal([]byte(payload), &item); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	got := rbacFromRaw(item)
	if got.Count != 3 {
		t.Errorf("RoleBinding with 3 subjects: count = %d, want 3", got.Count)
	}
}

func TestRBACFromRaw_ClusterScopedKindsRenderClusterScope(t *testing.T) {
	for _, kind := range []string{"ClusterRole", "ClusterRoleBinding"} {
		var item rawRBACItem
		item.Kind = kind
		got := rbacFromRaw(item)
		if got.Scope != rbacScopeCluster {
			t.Errorf("%s should be Cluster scope; got %q", kind, got.Scope)
		}
	}
}

func TestRBACFromRaw_ServiceAccountIsNamespaceScopedAndCountsZero(t *testing.T) {
	var item rawRBACItem
	item.Kind = "ServiceAccount"
	item.Metadata.Namespace = "ns"
	got := rbacFromRaw(item)
	if got.Scope != rbacScopeNamespace {
		t.Errorf("SA scope = %q, want Namespace", got.Scope)
	}
	if got.Count != 0 {
		t.Errorf("SA count = %d, want 0", got.Count)
	}
}

func TestRBACFromRaw_UnknownKindFallsBackToNamespaceScope(t *testing.T) {
	var item rawRBACItem
	item.Kind = "MysteryRBACKind"
	got := rbacFromRaw(item)
	if got.Scope != rbacScopeNamespace {
		t.Errorf("unknown kind should default to Namespace scope; got %q", got.Scope)
	}
}

func TestFetchRBAC_HappyPathAggregatesAcrossKinds(t *testing.T) {
	const stdout = `{
		"items": [
			{"kind":"ServiceAccount","metadata":{"name":"sa","namespace":"ns"}},
			{"kind":"Role","metadata":{"name":"r","namespace":"ns"},"rules":[{}]},
			{"kind":"ClusterRole","metadata":{"name":"cr"},"rules":[{},{}]}
		]
	}`
	withFakePathKubectl(t, `printf '%s' '`+stdout+`'`)

	msg, ok := FetchRBAC("ns")().(RBACMsg)
	if !ok {
		t.Fatalf("expected RBACMsg, got %T", FetchRBAC("ns")())
	}
	if msg.Err != nil {
		t.Fatalf("unexpected err: %v", msg.Err)
	}
	if len(msg.RBAC) != 3 {
		t.Fatalf("expected 3 rbac rows, got %d", len(msg.RBAC))
	}
	if msg.RBAC[2].Kind != "ClusterRole" || msg.RBAC[2].Scope != rbacScopeCluster {
		t.Errorf("cluster-scoped kind not flagged: %+v", msg.RBAC[2])
	}
	if msg.RBAC[1].Count != 1 {
		t.Errorf("Role count not propagated: %+v", msg.RBAC[1])
	}
}

func TestFetchRBAC_PropagatesKubectlError(t *testing.T) {
	withFakePathKubectl(t, `printf 'rbac boom' 1>&2; exit 1`)
	msg, _ := FetchRBAC("ns")().(RBACMsg)
	if msg.Err == nil {
		t.Error("expected kubectl error")
	}
}

func TestFetchRBAC_ArgvAggregatesEveryRBACKind(t *testing.T) {
	argvPath := withArgvCapturingFakeKubectl(t, `printf '{"items":[]}'`)
	if _, ok := FetchRBAC("ns")().(RBACMsg); !ok {
		t.Fatal("expected RBACMsg")
	}
	got := readArgv(t, argvPath)
	for _, kind := range []string{"serviceaccount", "role", "rolebinding", "clusterrole", "clusterrolebinding"} {
		if !strings.Contains(got, kind) {
			t.Errorf("kubectl argv missing %q; full argv: %q", kind, got)
		}
	}
}

func TestFetchRBAC_EmptyNamespaceUsesAllNamespacesFlag(t *testing.T) {
	argvPath := withArgvCapturingFakeKubectl(t, `printf '{"items":[]}'`)
	if _, ok := FetchRBAC("")().(RBACMsg); !ok {
		t.Fatal("expected RBACMsg")
	}
	got := readArgv(t, argvPath)
	if !strings.Contains(got, "--all-namespaces") {
		t.Errorf("empty namespace should use --all-namespaces; argv: %q", got)
	}
	if strings.Contains(got, "-n ") {
		t.Errorf("empty namespace must not pass -n; argv: %q", got)
	}
}

func TestFetchRBAC_NamespaceUsesDashN(t *testing.T) {
	argvPath := withArgvCapturingFakeKubectl(t, `printf '{"items":[]}'`)
	if _, ok := FetchRBAC("kube-system")().(RBACMsg); !ok {
		t.Fatal("expected RBACMsg")
	}
	got := readArgv(t, argvPath)
	if !strings.Contains(got, "-n kube-system") {
		t.Errorf("non-empty namespace should pass -n kube-system; argv: %q", got)
	}
	if strings.Contains(got, "--all-namespaces") {
		t.Errorf("non-empty namespace must not pass --all-namespaces; argv: %q", got)
	}
}

func withArgvCapturingFakeKubectl(t *testing.T, script string) string {
	t.Helper()
	dir := t.TempDir()
	argvPath := filepath.Join(dir, "argv.txt")
	binPath := filepath.Join(dir, "kubectl")
	contents := "#!/bin/sh\necho \"$@\" > " + argvPath + "\n" + script + "\n"
	writeTestExecutable(t, binPath, contents)
	prev := os.Getenv("PATH")
	if err := os.Setenv("PATH", dir+string(os.PathListSeparator)+prev); err != nil {
		t.Fatalf("setenv PATH: %v", err)
	}
	t.Cleanup(func() { _ = os.Setenv("PATH", prev) })
	return argvPath
}

func readArgv(t *testing.T, path string) string {
	t.Helper()
	return strings.TrimSpace(readTestFile(t, path))
}
