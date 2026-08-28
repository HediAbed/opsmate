package service

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestCRDFromRaw_PopulatesEveryColumn(t *testing.T) {
	const payload = `{
		"metadata": {"name": "certificates.cert-manager.io", "creationTimestamp": "2026-01-01T00:00:00Z"},
		"spec": {
			"group": "cert-manager.io",
			"names": {"kind": "Certificate", "plural": "certificates", "singular": "certificate"},
			"scope": "Namespaced",
			"versions": [
				{"name": "v1", "served": true, "storage": true},
				{"name": "v1alpha2", "served": false, "storage": false}
			]
		}
	}`
	var item rawCRDItem
	if err := json.Unmarshal([]byte(payload), &item); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	got := crdFromRaw(item)
	if got.Name != "certificates.cert-manager.io" || got.Group != "cert-manager.io" {
		t.Errorf("identity wrong: %+v", got)
	}
	if got.Kind != "Certificate" || got.Plural != "certificates" || got.Singular != "certificate" {
		t.Errorf("names not propagated: %+v", got)
	}
	if got.Scope != "Namespaced" {
		t.Errorf("scope = %q, want Namespaced", got.Scope)
	}
	if !reflect.DeepEqual(got.Versions, []string{"v1"}) {
		t.Errorf("served versions only; got %+v", got.Versions)
	}
	if got.Resource != "certificates.cert-manager.io" {
		t.Errorf("resource arg = %q, want certificates.cert-manager.io", got.Resource)
	}
}

func TestServedVersionNames_DropsUnservedEntries(t *testing.T) {
	in := []rawCRDVersion{
		{Name: "v1beta1", Served: false},
		{Name: "v1", Served: true},
		{Name: "v2", Served: true},
	}
	got := servedVersionNames(in)
	if !reflect.DeepEqual(got, []string{"v1", "v2"}) {
		t.Errorf("served filter wrong; got %+v want [v1 v2]", got)
	}
}

func TestCRDResourceArg_EmptyInputsReturnEmpty(t *testing.T) {
	if got := crdResourceArg("", "g"); got != "" {
		t.Errorf("empty plural should yield empty resource; got %q", got)
	}
	if got := crdResourceArg("p", ""); got != "" {
		t.Errorf("empty group should yield empty resource; got %q", got)
	}
}

func TestJoinVersions_HandlesEmptyAndMulti(t *testing.T) {
	if got := JoinVersions(nil); got != "" {
		t.Errorf("nil should yield empty; got %q", got)
	}
	if got := JoinVersions([]string{"v1", "v1beta1"}); got != "v1,v1beta1" {
		t.Errorf("multi join wrong; got %q", got)
	}
}

func TestFetchCRDs_HappyPathAndError(t *testing.T) {
	const stdout = `{"items":[
		{"metadata":{"name":"x.example.com"},"spec":{"group":"example.com","names":{"plural":"x","kind":"X"},"scope":"Cluster","versions":[{"name":"v1","served":true,"storage":true}]}}
	]}`
	withFakePathKubectl(t, `printf '%s' '`+stdout+`'`)
	msg, ok := FetchCRDs()().(CRDsMsg)
	if !ok {
		t.Fatalf("expected CRDsMsg, got %T", FetchCRDs()())
	}
	if msg.Err != nil || len(msg.CRDs) != 1 {
		t.Fatalf("happy path wrong: err=%v crds=%+v", msg.Err, msg.CRDs)
	}
	if msg.CRDs[0].Resource != "x.example.com" {
		t.Errorf("resource not built; got %+v", msg.CRDs[0])
	}

	withFakePathKubectl(t, `printf 'crd boom' 1>&2; exit 1`)
	errMsg, _ := FetchCRDs()().(CRDsMsg)
	if errMsg.Err == nil {
		t.Error("expected kubectl error")
	}
}

func TestListCRDInstances_HappyPathPropagatesResourceDiscriminator(t *testing.T) {
	const stdout = `{"items":[
		{"metadata":{"name":"sample-cert","namespace":"web","creationTimestamp":"2026-01-01T00:00:00Z"}},
		{"metadata":{"name":"api-cert","namespace":"api","creationTimestamp":"2026-01-01T00:00:00Z"}}
	]}`
	withFakePathKubectl(t, `printf '%s' '`+stdout+`'`)
	msg, ok := ListCRDInstances("certificates.cert-manager.io", "")().(CRDInstancesMsg)
	if !ok {
		t.Fatalf("expected CRDInstancesMsg, got %T", ListCRDInstances("certificates.cert-manager.io", "")())
	}
	if msg.Err != nil {
		t.Fatalf("unexpected err: %v", msg.Err)
	}
	if msg.Resource != "certificates.cert-manager.io" {
		t.Errorf("Resource discriminator missing; got %q", msg.Resource)
	}
	if len(msg.Instances) != 2 {
		t.Errorf("expected 2 instances; got %d", len(msg.Instances))
	}
}

func TestListCRDInstances_ErrorAlsoCarriesResourceDiscriminator(t *testing.T) {
	withFakePathKubectl(t, `printf 'no such resource' 1>&2; exit 1`)
	msg, _ := ListCRDInstances("missing.example.com", "ns")().(CRDInstancesMsg)
	if msg.Err == nil {
		t.Fatal("expected error")
	}
	if msg.Resource != "missing.example.com" {
		t.Errorf("error path must still set Resource so the model can drop stale fetches; got %q", msg.Resource)
	}
	if !strings.Contains(msg.Err.Error(), "no such resource") {
		t.Errorf("error should preserve stderr; got %v", msg.Err)
	}
}

func TestListCRDInstances_ErrorIsTypedCRDError(t *testing.T) {
	withFakePathKubectl(t, `printf 'denied' 1>&2; exit 1`)
	msg, _ := ListCRDInstances("things.example.com", "ns")().(CRDInstancesMsg)
	var crdErr *CRDError
	if !errors.As(msg.Err, &crdErr) {
		t.Fatalf("error should wrap *CRDError; got %T", msg.Err)
	}
	if crdErr.Operation != "list-instances" {
		t.Errorf("Operation = %q, want list-instances", crdErr.Operation)
	}
	if crdErr.Resource != "things.example.com" {
		t.Errorf("Resource = %q, want things.example.com", crdErr.Resource)
	}
}

func TestFetchCRDs_ErrorIsTypedCRDError(t *testing.T) {
	withFakePathKubectl(t, `printf 'denied' 1>&2; exit 1`)
	msg, _ := FetchCRDs()().(CRDsMsg)
	var crdErr *CRDError
	if !errors.As(msg.Err, &crdErr) {
		t.Fatalf("error should wrap *CRDError; got %T", msg.Err)
	}
	if crdErr.Operation != "list" {
		t.Errorf("Operation = %q, want list", crdErr.Operation)
	}
}
