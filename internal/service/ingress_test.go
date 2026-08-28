package service

import (
	"encoding/json"
	"testing"
)

func TestIngressFromRaw_JoinsAllHosts(t *testing.T) {
	var item rawIngressItem
	const payload = `{
		"metadata": {"name": "web", "namespace": "default", "creationTimestamp": "2026-01-01T00:00:00Z"},
		"spec": {
			"ingressClassName": "nginx",
			"rules": [{"host": "a.example.com"}, {"host": "b.example.com"}]
		},
		"status": {"loadBalancer": {"ingress": [{"ip": "10.0.0.1"}]}}
	}`
	if err := json.Unmarshal([]byte(payload), &item); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	got := ingressFromRaw(item)
	if got.Hosts != "a.example.com,b.example.com" {
		t.Errorf("hosts = %q, want comma-joined a + b", got.Hosts)
	}
	if got.Class != "nginx" {
		t.Errorf("class = %q, want nginx", got.Class)
	}
	if got.Address != "10.0.0.1" {
		t.Errorf("address = %q, want 10.0.0.1", got.Address)
	}
}

func TestIngressFromRaw_DedupesDuplicateHosts(t *testing.T) {
	var item rawIngressItem
	const payload = `{
		"metadata": {"name": "web", "namespace": "default"},
		"spec": {
			"rules": [{"host": "x.example.com"}, {"host": "x.example.com"}, {"host": "y.example.com"}]
		}
	}`
	if err := json.Unmarshal([]byte(payload), &item); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	got := ingressFromRaw(item)
	if got.Hosts != "x.example.com,y.example.com" {
		t.Errorf("duplicate host should be dropped; got %q", got.Hosts)
	}
}

func TestIngressFromRaw_PrefersHostnameOverIPInLBStatus(t *testing.T) {
	var item rawIngressItem
	const payload = `{
		"metadata": {"name": "web", "namespace": "default"},
		"status": {"loadBalancer": {"ingress": [{"ip": "10.0.0.1", "hostname": "lb.example.com"}]}}
	}`
	if err := json.Unmarshal([]byte(payload), &item); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	got := ingressFromRaw(item)
	if got.Address != "lb.example.com" {
		t.Errorf("hostname must win over ip; got %q", got.Address)
	}
}

func TestIngressPorts_TLSSwitchesTo443(t *testing.T) {
	var withTLS, withoutTLS rawIngressItem
	withTLS.Spec.TLS = []struct {
		Hosts []string `json:"hosts"`
	}{{Hosts: []string{"x"}}}
	if got := ingressPorts(withTLS); got != ingressHTTPSPorts {
		t.Errorf("tls ingress should report 80, 443; got %q", got)
	}
	if got := ingressPorts(withoutTLS); got != ingressHTTPPorts {
		t.Errorf("non-tls ingress should report 80; got %q", got)
	}
}

func TestDecodeIngressObject_RoundTrips(t *testing.T) {
	const payload = `{
		"metadata": {"name": "web", "namespace": "default", "creationTimestamp": "2026-01-01T00:00:00Z"},
		"spec": {"ingressClassName": "alb", "rules": [{"host": "z.example.com"}]}
	}`
	got, err := decodeIngressObject(json.RawMessage(payload))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Name != "web" || got.Namespace != "default" {
		t.Errorf("identity fields wrong: %+v", got)
	}
	if got.Class != "alb" || got.Hosts != "z.example.com" {
		t.Errorf("payload fields wrong: %+v", got)
	}
}
