package service

import (
	"encoding/json"
	"testing"
)

func TestReplicaSetFromRaw_PullsDesiredFromSpecAndCountsFromStatus(t *testing.T) {
	desired := 3
	item := rawReplicaSetItem{}
	item.Metadata.Name = "web-rs-abc"
	item.Metadata.Namespace = "ns"
	item.Spec.Replicas = &desired
	item.Status.Replicas = 2
	item.Status.ReadyReplicas = 1

	got := replicaSetFromRaw(item)
	if got.Desired != 3 {
		t.Errorf("desired = %d, want 3", got.Desired)
	}
	if got.Current != 2 {
		t.Errorf("current = %d, want 2", got.Current)
	}
	if got.Ready != 1 {
		t.Errorf("ready = %d, want 1", got.Ready)
	}
}

func TestReplicaSetFromRaw_NilDesiredZeroes(t *testing.T) {
	item := rawReplicaSetItem{}
	if got := replicaSetFromRaw(item); got.Desired != 0 {
		t.Errorf("nil spec.replicas should yield 0 desired (RS scaled to zero); got %d", got.Desired)
	}
}

func TestDecodeReplicaSetObject_RoundTripsAndRejectsMalformed(t *testing.T) {
	const payload = `{"metadata":{"name":"r","namespace":"ns"},"spec":{"replicas":3},"status":{"replicas":2,"readyReplicas":1}}`
	got, err := decodeReplicaSetObject(json.RawMessage(payload))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Desired != 3 {
		t.Errorf("decoded desired = %d, want 3", got.Desired)
	}
	if got.Current != 2 {
		t.Errorf("decoded current = %d, want 2", got.Current)
	}
	if got.Ready != 1 {
		t.Errorf("decoded ready = %d, want 1", got.Ready)
	}
	if _, err := decodeReplicaSetObject(json.RawMessage("{bad")); err == nil {
		t.Error("expected error on malformed JSON")
	}
}

func TestFetchReplicaSets_HappyPathAndError(t *testing.T) {
	const stdout = `{
		"items": [
			{"metadata":{"name":"r","namespace":"ns"},"spec":{"replicas":3},"status":{"replicas":2,"readyReplicas":1}}
		]
	}`
	withFakePathKubectl(t, `printf '%s' '`+stdout+`'`)
	msg, ok := FetchReplicaSets("ns")().(ReplicaSetsMsg)
	if !ok {
		t.Fatalf("expected ReplicaSetsMsg, got %T", FetchReplicaSets("ns")())
	}
	if msg.Err != nil || len(msg.ReplicaSets) != 1 {
		t.Fatalf("happy path wrong: err=%v rs=%+v", msg.Err, msg.ReplicaSets)
	}
	rs := msg.ReplicaSets[0]
	if rs.Desired != 3 {
		t.Errorf("Desired = %d, want 3", rs.Desired)
	}
	if rs.Current != 2 {
		t.Errorf("Current = %d, want 2", rs.Current)
	}
	if rs.Ready != 1 {
		t.Errorf("Ready = %d, want 1", rs.Ready)
	}

	withFakePathKubectl(t, `printf 'rs boom' 1>&2; exit 1`)
	errMsg, _ := FetchReplicaSets("ns")().(ReplicaSetsMsg)
	if errMsg.Err == nil {
		t.Error("expected kubectl error")
	}
}
