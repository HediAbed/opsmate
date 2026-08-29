package ui

import (
	"testing"

	"github.com/HediAbed/opsmate/internal/cluster"
)

func TestCurrentResourceCount_HandlesEveryRegisteredKind(t *testing.T) {
	for _, kind := range allResourceTypes {
		t.Run(kind, func(t *testing.T) {
			m := newTestBrowserModel("ns")
			m.SetResourceType(kind)
			seedOneResource(&m, kind)
			if got := m.currentResourceCount(); got != 1 {
				t.Errorf("kind %q: count = %d, want 1; likely missing branch in currentResourceCount", kind, got)
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
		m.pods = []cluster.Pod{{Name: "p"}}
	case "deployments":
		m.deployments = []cluster.Deployment{{Name: "d"}}
	case "services":
		m.services = []cluster.Service{{Name: "s"}}
	case "statefulsets":
		m.statefulsets = []cluster.StatefulSet{{Name: "ss"}}
	case "daemonsets":
		m.daemonsets = []cluster.DaemonSet{{Name: "ds"}}
	case "configmaps":
		m.configmaps = []cluster.ConfigMap{{Name: "cm"}}
	case "nodes":
		m.nodes = []cluster.Node{{Name: "n"}}
	case "jobs":
		m.jobs = []cluster.Job{{Name: "j"}}
	default:
		return false
	}
	return true
}

func seedExtendedBrowserResource(m *BrowserModel, kind string) {
	switch kind {
	case "ingresses":
		m.ingresses = []cluster.Ingress{{Name: "i"}}
	case "networkpolicies":
		m.networkpolicies = []cluster.NetworkPolicy{{Name: "np"}}
	case "pvcs":
		m.pvcs = []cluster.PersistentVolumeClaim{{Name: "v"}}
	case "cronjobs":
		m.cronjobs = []cluster.CronJob{{Name: "cj"}}
	case "hpas":
		m.hpas = []cluster.HPA{{Name: "h"}}
	case "secrets":
		m.secrets = []cluster.Secret{{Name: "sec"}}
	case "replicasets":
		m.replicasets = []cluster.ReplicaSet{{Name: "rs"}}
	case "rbac":
		m.rbac = []cluster.RBAC{{Name: "r"}}
	}
}
