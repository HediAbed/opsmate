package model

import (
	"testing"

	"github.com/HediAbed/opsmate/internal/service"
)

func TestIngressKey_NamespaceSlashName(t *testing.T) {
	if got := ingressKey(service.Ingress{Name: "n", Namespace: "ns"}); got != "ns/n" {
		t.Errorf("ingressKey = %q, want ns/n", got)
	}
}

func TestNetworkPolicyKey_NamespaceSlashName(t *testing.T) {
	if got := networkPolicyKey(service.NetworkPolicy{Name: "n", Namespace: "ns"}); got != "ns/n" {
		t.Errorf("networkPolicyKey = %q, want ns/n", got)
	}
}

func TestPVCKey_NamespaceSlashName(t *testing.T) {
	if got := pvcKey(service.PersistentVolumeClaim{Name: "n", Namespace: "ns"}); got != "ns/n" {
		t.Errorf("pvcKey = %q, want ns/n", got)
	}
}

func TestCronJobKey_NamespaceSlashName(t *testing.T) {
	if got := cronJobKey(service.CronJob{Name: "n", Namespace: "ns"}); got != "ns/n" {
		t.Errorf("cronJobKey = %q, want ns/n", got)
	}
}

func TestHPAKey_NamespaceSlashName(t *testing.T) {
	if got := hpaKey(service.HPA{Name: "n", Namespace: "ns"}); got != "ns/n" {
		t.Errorf("hpaKey = %q, want ns/n", got)
	}
}

func TestSecretKey_NamespaceSlashName(t *testing.T) {
	if got := secretKey(service.Secret{Name: "n", Namespace: "ns"}); got != "ns/n" {
		t.Errorf("secretKey = %q, want ns/n", got)
	}
}

func TestReplicaSetKey_NamespaceSlashName(t *testing.T) {
	if got := replicaSetKey(service.ReplicaSet{Name: "n", Namespace: "ns"}); got != "ns/n" {
		t.Errorf("replicaSetKey = %q, want ns/n", got)
	}
}
