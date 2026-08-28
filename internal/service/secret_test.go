package service

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestSecretFromRaw_BuildsRowFromMetadataAndKeyCount(t *testing.T) {
	const payload = `{
		"metadata": {"name": "creds", "namespace": "ns", "creationTimestamp": "2026-01-01T00:00:00Z"},
		"type": "kubernetes.io/tls",
		"data": {"tls.crt": "AAAA", "tls.key": "BBBB"}
	}`
	var item rawSecretItem
	if err := json.Unmarshal([]byte(payload), &item); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	got := secretFromRaw(item)
	if got.Name != "creds" || got.Namespace != "ns" {
		t.Errorf("identity wrong: %+v", got)
	}
	if got.Type != "kubernetes.io/tls" {
		t.Errorf("type = %q, want kubernetes.io/tls", got.Type)
	}
	if got.Data != 2 {
		t.Errorf("data count = %d, want 2", got.Data)
	}
}

func TestRawSecretItem_DropsValuePayload(t *testing.T) {
	const payload = `{
		"metadata": {"name": "creds", "namespace": "ns"},
		"type": "Opaque",
		"data": {"password": "eA=="}
	}`
	var item rawSecretItem
	if err := json.Unmarshal([]byte(payload), &item); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if _, ok := item.Data["password"]; !ok {
		t.Fatal("expected key 'password' in decoded map")
	}
	valueType := reflect.TypeOf(item.Data).Elem()
	if valueType.Size() != 0 {
		t.Errorf("rawSecretItem.Data values must be zero-sized to prevent payload retention; got size=%d for %s",
			valueType.Size(), valueType.String())
	}
}

func TestSecretFromRaw_TypeFieldIsCarriedThrough(t *testing.T) {
	cases := []string{
		"Opaque",
		"kubernetes.io/service-account-token",
		"kubernetes.io/dockerconfigjson",
		"kubernetes.io/basic-auth",
		"helm.sh/release.v1",
	}
	for _, want := range cases {
		var item rawSecretItem
		item.Type = want
		if got := secretFromRaw(item).Type; got != want {
			t.Errorf("type = %q, want %q", got, want)
		}
	}
}

func TestDecodeSecretObject_RoundTripsAndRejectsMalformed(t *testing.T) {
	const payload = `{"metadata":{"name":"s","namespace":"ns"},"type":"Opaque","data":{"k":"v"}}`
	got, err := decodeSecretObject(json.RawMessage(payload))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Data != 1 || got.Type != "Opaque" {
		t.Errorf("decoded fields wrong: %+v", got)
	}
	if _, err := decodeSecretObject(json.RawMessage("{bad")); err == nil {
		t.Error("expected error on malformed JSON")
	}
}

func TestFetchSecrets_HappyPathAndError(t *testing.T) {
	const stdout = `{
		"items": [
			{"metadata":{"name":"s","namespace":"ns"},"type":"Opaque","data":{"a":"VkFM","b":"VkFM"}}
		]
	}`
	withFakePathKubectl(t, `printf '%s' '`+stdout+`'`)
	msg, ok := FetchSecrets("ns")().(SecretsMsg)
	if !ok {
		t.Fatalf("expected SecretsMsg, got %T", FetchSecrets("ns")())
	}
	if msg.Err != nil || len(msg.Secrets) != 1 {
		t.Fatalf("happy path wrong: err=%v secrets=%+v", msg.Err, msg.Secrets)
	}
	if msg.Secrets[0].Data != 2 {
		t.Errorf("data count from list payload should be 2; got %d", msg.Secrets[0].Data)
	}

	withFakePathKubectl(t, `printf 'secret boom' 1>&2; exit 1`)
	errMsg, _ := FetchSecrets("ns")().(SecretsMsg)
	if errMsg.Err == nil {
		t.Error("expected kubectl error")
	}
}
