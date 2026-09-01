package screen

import "testing"

func TestResourceKindValidity(t *testing.T) {
	validKinds := []ResourceKind{
		ResourceKindPod,
		ResourceKindDeployment,
		ResourceKindService,
		ResourceKindStatefulSet,
		ResourceKindDaemonSet,
		ResourceKindConfigMap,
		ResourceKindNode,
		ResourceKindJob,
		ResourceKindIngress,
		ResourceKindNetworkPolicy,
		ResourceKindPVC,
		ResourceKindCronJob,
		ResourceKindHPA,
		ResourceKindSecret,
		ResourceKindReplicaSet,
		ResourceKindRBAC,
	}
	for _, kind := range validKinds {
		if !kind.Valid() {
			t.Errorf("registered resource kind %q is invalid", kind)
		}
	}
	if ResourceKind("unknown").Valid() {
		t.Error("unknown resource kind must be invalid")
	}
}

func TestSearchItemValidity(t *testing.T) {
	if !(SearchItem{Kind: ResourceKindPod, Name: "api"}).Valid() {
		t.Error("canonical kind with a name must be valid")
	}
	if (SearchItem{Kind: ResourceKindPod}).Valid() {
		t.Error("search item without a name must be invalid")
	}
	if (SearchItem{Kind: ResourceKind("unknown"), Name: "api"}).Valid() {
		t.Error("search item with an unknown kind must be invalid")
	}
}
