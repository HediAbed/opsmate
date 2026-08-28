package model

import (
	"testing"

	"github.com/HediAbed/opsmate/internal/service"
)

func TestCurrentResourceCount_HandlesEveryRegisteredKind(t *testing.T) {
	for _, kind := range allResourceTypes {
		t.Run(kind, func(t *testing.T) {
			m := NewBrowserModel("ns")
			m.SetResourceType(kind)
			seedOneResource(&m, kind)
			if got := m.currentResourceCount(); got != 1 {
				t.Errorf("kind %q: count = %d, want 1 — likely missing branch in currentResourceCount", kind, got)
			}
		})
	}
}

func seedOneResource(m *BrowserModel, kind string) {
	if seedPrimaryBrowserResource(m, kind) {
		return
	}
	seedExtendedBrowserResource(m, kind)
}

func seedPrimaryBrowserResource(m *BrowserModel, kind string) bool {
	switch kind {
	case "pods":
		m.pods = []service.Pod{{Name: "p"}}
	case "deployments":
		m.deployments = []service.Deployment{{Name: "d"}}
	case "services":
		m.services = []service.Service{{Name: "s"}}
	case "statefulsets":
		m.statefulsets = []service.StatefulSet{{Name: "ss"}}
	case "daemonsets":
		m.daemonsets = []service.DaemonSet{{Name: "ds"}}
	case "configmaps":
		m.configmaps = []service.ConfigMap{{Name: "cm"}}
	case "nodes":
		m.nodes = []service.Node{{Name: "n"}}
	case "jobs":
		m.jobs = []service.Job{{Name: "j"}}
	default:
		return false
	}
	return true
}

func seedExtendedBrowserResource(m *BrowserModel, kind string) {
	switch kind {
	case "ingresses":
		m.ingresses = []service.Ingress{{Name: "i"}}
	case "networkpolicies":
		m.networkpolicies = []service.NetworkPolicy{{Name: "np"}}
	case "pvcs":
		m.pvcs = []service.PersistentVolumeClaim{{Name: "v"}}
	case "cronjobs":
		m.cronjobs = []service.CronJob{{Name: "cj"}}
	case "hpas":
		m.hpas = []service.HPA{{Name: "h"}}
	case "secrets":
		m.secrets = []service.Secret{{Name: "sec"}}
	case "replicasets":
		m.replicasets = []service.ReplicaSet{{Name: "rs"}}
	case "rbac":
		m.rbac = []service.RBAC{{Name: "r"}}
	}
}
