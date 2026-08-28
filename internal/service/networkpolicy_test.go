package service

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestNetworkPolicyFromRaw_PreservesNativeShape(t *testing.T) {
	var item rawNetworkPolicyItem
	const payload = `{
		"metadata": {"name": "deny-all", "namespace": "default", "creationTimestamp": "2026-01-01T00:00:00Z"},
		"spec": {
			"podSelector": {"matchLabels": {"app": "api", "tier": "backend"}},
			"policyTypes": ["Ingress", "Egress"]
		}
	}`
	if err := json.Unmarshal([]byte(payload), &item); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	got := networkPolicyFromRaw(item)
	if got.Name != "deny-all" || got.Namespace != "default" {
		t.Errorf("identity fields wrong: %+v", got)
	}
	wantSelector := map[string]string{"app": "api", "tier": "backend"}
	if !reflect.DeepEqual(got.PodSelector, wantSelector) {
		t.Errorf("pod selector = %+v, want %+v", got.PodSelector, wantSelector)
	}
	wantTypes := []string{"Ingress", "Egress"}
	if !reflect.DeepEqual(got.PolicyTypes, wantTypes) {
		t.Errorf("policy types = %+v, want %+v", got.PolicyTypes, wantTypes)
	}
}

func TestDecodeNetworkPolicyObject_RejectsMalformedJSON(t *testing.T) {
	if _, err := decodeNetworkPolicyObject(json.RawMessage("{bad json")); err == nil {
		t.Fatal("expected decode error on malformed JSON")
	}
}

func TestFetchNetworkPolicies_HappyPath(t *testing.T) {
	const stdout = `{
		"items": [
			{
				"metadata": {"name": "deny-egress", "namespace": "ns1"},
				"spec": {
					"podSelector": {"matchLabels": {"app": "api"}},
					"policyTypes": ["Egress"]
				}
			}
		]
	}`
	withFakePathKubectl(t, `printf '%s' '`+stdout+`'`)

	msg, ok := FetchNetworkPolicies("ns1")().(NetworkPoliciesMsg)
	if !ok {
		t.Fatalf("expected NetworkPoliciesMsg, got %T", FetchNetworkPolicies("ns1")())
	}
	if msg.Err != nil {
		t.Fatalf("unexpected err: %v", msg.Err)
	}
	if len(msg.NetworkPolicies) != 1 {
		t.Fatalf("expected 1 networkpolicy, got %d", len(msg.NetworkPolicies))
	}
	if msg.NetworkPolicies[0].Name != "deny-egress" {
		t.Errorf("name = %q, want deny-egress", msg.NetworkPolicies[0].Name)
	}
}

func TestFetchNetworkPolicies_PropagatesKubectlError(t *testing.T) {
	withFakePathKubectl(t, `printf 'boom' 1>&2; exit 1`)

	msg, ok := FetchNetworkPolicies("ns1")().(NetworkPoliciesMsg)
	if !ok {
		t.Fatalf("expected NetworkPoliciesMsg, got %T", FetchNetworkPolicies("ns1")())
	}
	if msg.Err == nil {
		t.Fatal("expected kubectl error")
	}
	if len(msg.NetworkPolicies) != 0 {
		t.Errorf("error path should not return rows; got %d", len(msg.NetworkPolicies))
	}
}

func withFakePathKubectl(t *testing.T, script string) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "kubectl")
	writeTestExecutable(t, path, "#!/bin/sh\n"+script+"\n")
	prev := os.Getenv("PATH")
	if err := os.Setenv("PATH", dir+string(os.PathListSeparator)+prev); err != nil {
		t.Fatalf("setenv PATH: %v", err)
	}
	t.Cleanup(func() { _ = os.Setenv("PATH", prev) })
}

func TestDecodeNetworkPolicyObject_RoundTrips(t *testing.T) {
	const payload = `{
		"metadata": {"name": "allow-web", "namespace": "ns1", "creationTimestamp": "2026-01-01T00:00:00Z"},
		"spec": {
			"podSelector": {"matchLabels": {"role": "web"}},
			"policyTypes": ["Ingress"]
		}
	}`
	got, err := decodeNetworkPolicyObject(json.RawMessage(payload))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Name != "allow-web" || got.Namespace != "ns1" {
		t.Errorf("identity fields wrong: %+v", got)
	}
	if got.PodSelector["role"] != "web" || len(got.PodSelector) != 1 {
		t.Errorf("pod selector = %+v, want {role:web}", got.PodSelector)
	}
	if !reflect.DeepEqual(got.PolicyTypes, []string{"Ingress"}) {
		t.Errorf("policy types = %+v, want [Ingress]", got.PolicyTypes)
	}
}
