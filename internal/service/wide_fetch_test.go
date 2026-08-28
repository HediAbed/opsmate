//go:build !windows

package service

import "testing"

func TestFetchPods_ExtractsPodIP(t *testing.T) {
	installFakeKubectl(t, `#!/bin/sh
cat <<'EOF'
{
  "items": [
    {
      "metadata": {"name": "alpha", "namespace": "default", "creationTimestamp": "2024-01-01T00:00:00Z"},
      "status": {"phase": "Running", "podIP": "10.42.0.7"},
      "spec": {"nodeName": "n1"}
    }
  ]
}
EOF
`)
	pods, err := listPodsSync("default")
	if err != nil {
		t.Fatalf("listPodsSync: %v", err)
	}
	if len(pods) != 1 || pods[0].IP != "10.42.0.7" {
		t.Errorf("podIP not parsed; pods=%+v", pods)
	}
}

func TestFetchServices_ExtractsExternalIPAndSelector(t *testing.T) {
	installFakeKubectl(t, `#!/bin/sh
cat <<'EOF'
{
  "items": [
    {
      "metadata": {"name": "api", "namespace": "default", "creationTimestamp": "2024-01-01T00:00:00Z"},
      "spec": {
        "type": "LoadBalancer",
        "clusterIP": "10.96.0.1",
        "selector": {"app": "api", "tier": "frontend"},
        "ports": [{"port": 80, "protocol": "TCP"}]
      },
      "status": {"loadBalancer": {"ingress": [{"ip": "203.0.113.5"}]}}
    }
  ]
}
EOF
`)
	svcs, err := listServicesSync("default")
	if err != nil {
		t.Fatalf("listServicesSync: %v", err)
	}
	if len(svcs) != 1 {
		t.Fatalf("expected 1 svc, got %d", len(svcs))
	}
	if svcs[0].ExternalIP != "203.0.113.5" {
		t.Errorf("ExternalIP from LB ingress: got %q", svcs[0].ExternalIP)
	}
	if svcs[0].Selector != "app=api,tier=frontend" {
		t.Errorf("Selector sorted: got %q", svcs[0].Selector)
	}
}

func TestFetchServices_NoExternalIPRendersAngleNone(t *testing.T) {
	installFakeKubectl(t, `#!/bin/sh
cat <<'EOF'
{
  "items": [
    {
      "metadata": {"name": "internal", "namespace": "default", "creationTimestamp": "2024-01-01T00:00:00Z"},
      "spec": {"type": "ClusterIP", "clusterIP": "10.96.0.2", "ports": []}
    }
  ]
}
EOF
`)
	svcs, err := listServicesSync("default")
	if err != nil {
		t.Fatalf("listServicesSync: %v", err)
	}
	if svcs[0].ExternalIP != "<none>" {
		t.Errorf("ClusterIP service must report <none> external; got %q", svcs[0].ExternalIP)
	}
}
