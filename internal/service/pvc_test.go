package service

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestPVCFromRaw_BoundReportsStatusCapacity(t *testing.T) {
	var item rawPVCItem
	const payload = `{
		"metadata": {"name": "data", "namespace": "ns1", "creationTimestamp": "2026-01-01T00:00:00Z"},
		"spec": {
			"volumeName": "pv-7",
			"storageClassName": "gp3",
			"accessModes": ["ReadWriteOnce"],
			"resources": {"requests": {"storage": "5Gi"}}
		},
		"status": {
			"phase": "Bound",
			"capacity": {"storage": "10Gi"}
		}
	}`
	if err := json.Unmarshal([]byte(payload), &item); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	got := pvcFromRaw(item)
	if got.Name != "data" || got.Namespace != "ns1" {
		t.Errorf("identity wrong: %+v", got)
	}
	if got.Status != "Bound" {
		t.Errorf("status = %q, want Bound", got.Status)
	}
	if got.Volume != "pv-7" {
		t.Errorf("volume = %q, want pv-7", got.Volume)
	}
	if got.Capacity != "10Gi" {
		t.Errorf("Bound PVC must report status.capacity (10Gi); got %q", got.Capacity)
	}
	if !reflect.DeepEqual(got.AccessModes, []string{"ReadWriteOnce"}) {
		t.Errorf("access modes = %+v, want [ReadWriteOnce]", got.AccessModes)
	}
	if got.StorageClass != "gp3" {
		t.Errorf("storage class = %q, want gp3", got.StorageClass)
	}
}

func TestPVCFromRaw_PendingFallsBackToRequestedSize(t *testing.T) {
	var item rawPVCItem
	const payload = `{
		"metadata": {"name": "wait", "namespace": "ns1"},
		"spec": {
			"accessModes": ["ReadWriteMany"],
			"resources": {"requests": {"storage": "20Gi"}}
		},
		"status": {"phase": "Pending"}
	}`
	if err := json.Unmarshal([]byte(payload), &item); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	got := pvcFromRaw(item)
	if got.Status != "Pending" {
		t.Errorf("status = %q, want Pending", got.Status)
	}
	if got.Capacity != "20Gi" {
		t.Errorf("Pending PVC should fall back to spec.resources.requests.storage (20Gi); got %q", got.Capacity)
	}
}

func TestPVCFromRaw_BothCapacitySourcesEmptyYieldsEmptyColumn(t *testing.T) {
	var item rawPVCItem
	const payload = `{
		"metadata": {"name": "ghost", "namespace": "ns1"},
		"spec": {"accessModes": ["ReadWriteOnce"]},
		"status": {"phase": "Pending"}
	}`
	if err := json.Unmarshal([]byte(payload), &item); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	got := pvcFromRaw(item)
	if got.Capacity != "" {
		t.Errorf("with no status.capacity AND no spec request, capacity must be empty; got %q", got.Capacity)
	}
}

func TestDecodePVCObject_RoundTripsAndRejectsMalformed(t *testing.T) {
	const payload = `{
		"metadata": {"name": "data", "namespace": "ns1"},
		"spec": {"accessModes": ["ReadWriteOnce"]},
		"status": {"phase": "Bound", "capacity": {"storage": "1Gi"}}
	}`
	got, err := decodePVCObject(json.RawMessage(payload))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Name != "data" || got.Capacity != "1Gi" {
		t.Errorf("decoded fields wrong: %+v", got)
	}
	if _, err := decodePVCObject(json.RawMessage("{not json")); err == nil {
		t.Error("expected error on malformed JSON")
	}
}

func TestFetchPVCs_HappyPath(t *testing.T) {
	const stdout = `{
		"items": [
			{
				"metadata": {"name": "data", "namespace": "ns1"},
				"spec": {"accessModes": ["ReadWriteOnce"]},
				"status": {"phase": "Bound", "capacity": {"storage": "1Gi"}}
			}
		]
	}`
	withFakePathKubectl(t, `printf '%s' '`+stdout+`'`)

	cmd := FetchPVCs("ns1")
	msg, ok := cmd().(PVCsMsg)
	if !ok {
		t.Fatalf("expected PVCsMsg, got %T", cmd())
	}
	if msg.Err != nil {
		t.Fatalf("unexpected err: %v", msg.Err)
	}
	if len(msg.PVCs) != 1 || msg.PVCs[0].Status != "Bound" {
		t.Errorf("pvcs payload wrong: %+v", msg.PVCs)
	}
}

func TestFetchPVCs_PropagatesKubectlError(t *testing.T) {
	withFakePathKubectl(t, `printf 'pvc boom' 1>&2; exit 1`)

	msg, ok := FetchPVCs("ns1")().(PVCsMsg)
	if !ok {
		t.Fatalf("expected PVCsMsg, got %T", FetchPVCs("ns1")())
	}
	if msg.Err == nil {
		t.Fatal("expected kubectl error")
	}
	if len(msg.PVCs) != 0 {
		t.Errorf("error path should not return rows; got %d", len(msg.PVCs))
	}
}
