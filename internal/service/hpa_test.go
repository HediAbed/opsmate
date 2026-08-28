package service

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestScaleTargetRef_String_EmptyKindOrNameYieldsEmpty(t *testing.T) {
	if got := (ScaleTargetRef{}).String(); got != "" {
		t.Errorf("empty ScaleTargetRef should render empty; got %q", got)
	}
	if got := (ScaleTargetRef{Kind: "Deployment"}).String(); got != "" {
		t.Errorf("missing Name should render empty; got %q", got)
	}
	if got := (ScaleTargetRef{Name: "web"}).String(); got != "" {
		t.Errorf("missing Kind should render empty; got %q", got)
	}
}

func TestHPAMetricPair_String_AllFormats(t *testing.T) {
	cases := []struct {
		pair HPAMetricPair
		want string
	}{
		{HPAMetricPair{Current: "10%", Target: "80%"}, "10%/80%"},
		{HPAMetricPair{Target: "<none>"}, "<none>"},
		{HPAMetricPair{Current: "5"}, "5"},
		{HPAMetricPair{}, ""},
	}
	for _, c := range cases {
		if got := c.pair.String(); got != c.want {
			t.Errorf("(%+v).String() = %q, want %q", c.pair, got, c.want)
		}
	}
}

func TestHPAFromRaw_BuildsReferenceAndTargetsAndReplicas(t *testing.T) {
	const payload = `{
		"metadata": {"name": "web-hpa", "namespace": "ns", "creationTimestamp": "2026-01-01T00:00:00Z"},
		"spec": {
			"scaleTargetRef": {"kind": "Deployment", "name": "web"},
			"minReplicas": 2,
			"maxReplicas": 10,
			"metrics": [
				{
					"type": "Resource",
					"resource": {"name": "cpu", "target": {"type": "Utilization", "averageUtilization": 80}}
				}
			]
		},
		"status": {
			"currentReplicas": 4,
			"currentMetrics": [
				{
					"type": "Resource",
					"resource": {"name": "cpu", "current": {"averageUtilization": 35}}
				}
			]
		}
	}`
	var item rawHPAItem
	if err := json.Unmarshal([]byte(payload), &item); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	got := hpaFromRaw(item)
	if got.Reference.Kind != "Deployment" || got.Reference.Name != "web" {
		t.Errorf("reference = %+v, want {Deployment, web}", got.Reference)
	}
	if got.Reference.String() != "Deployment/web" {
		t.Errorf("reference.String() = %q, want Deployment/web", got.Reference.String())
	}
	wantTargets := []HPAMetricPair{{Current: "35%", Target: "80%"}}
	if !reflect.DeepEqual(got.Targets, wantTargets) {
		t.Errorf("targets = %+v, want %+v", got.Targets, wantTargets)
	}
	if got.MinReplicas != 2 || got.MaxReplicas != 10 || got.Replicas != 4 {
		t.Errorf("replicas wrong: min=%d max=%d cur=%d", got.MinReplicas, got.MaxReplicas, got.Replicas)
	}
}

func TestHPAFromRaw_MinReplicasNilDefaultsToOne(t *testing.T) {
	var item rawHPAItem
	item.Spec.MaxReplicas = 5
	if got := hpaFromRaw(item); got.MinReplicas != 1 {
		t.Errorf("nil minReplicas should default to 1 (kubectl default); got %d", got.MinReplicas)
	}
}

func TestHPATargets_NoMetricsRendersNoneSentinel(t *testing.T) {
	var item rawHPAItem
	got := hpaTargets(item)
	if len(got) != 1 || got[0].String() != hpaTargetUnknown {
		t.Errorf("no-metrics HPA targets should render as the %q sentinel; got %+v", hpaTargetUnknown, got)
	}
}

func TestHPATargets_StatusMissingShowsUnknownCurrent(t *testing.T) {
	const payload = `{
		"spec": {
			"metrics": [
				{"type": "Resource", "resource": {"name": "cpu", "target": {"averageUtilization": 70}}}
			]
		},
		"status": {}
	}`
	var item rawHPAItem
	if err := json.Unmarshal([]byte(payload), &item); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	got := hpaTargets(item)
	want := []HPAMetricPair{{Current: "<unknown>", Target: "70%"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("warming-up HPA should show <unknown> as current; got %+v want %+v", got, want)
	}
	if got[0].String() != "<unknown>/70%" {
		t.Errorf("rendered = %q, want <unknown>/70%%", got[0].String())
	}
}

func TestFormatHPAMetricSide_PrefersUtilizationThenValueThenUnknown(t *testing.T) {
	u := 90
	if got := formatHPAMetricSide(rawHPAMetricSide{AverageUtilization: &u}); got != "90%" {
		t.Errorf("utilization should render with %%; got %q", got)
	}
	if got := formatHPAMetricSide(rawHPAMetricSide{AverageValue: "200Mi"}); got != "200Mi" {
		t.Errorf("averageValue should pass through; got %q", got)
	}
	if got := formatHPAMetricSide(rawHPAMetricSide{}); got != hpaMetricUnknown {
		t.Errorf("empty side should render unknown sentinel; got %q", got)
	}
}

func TestDecodeHPAObject_RoundTripsAndRejectsMalformed(t *testing.T) {
	const payload = `{"metadata":{"name":"h","namespace":"ns"},"spec":{"scaleTargetRef":{"kind":"Deployment","name":"w"},"maxReplicas":5}}`
	got, err := decodeHPAObject(json.RawMessage(payload))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Reference.Kind != "Deployment" || got.Reference.Name != "w" {
		t.Errorf("reference = %+v", got.Reference)
	}
	if _, err := decodeHPAObject(json.RawMessage("{bad")); err == nil {
		t.Error("expected error on malformed JSON")
	}
}

func TestFetchHPAs_HappyPathAndError(t *testing.T) {
	const stdout = `{
		"items": [
			{"metadata":{"name":"h","namespace":"ns"},"spec":{"scaleTargetRef":{"kind":"Deployment","name":"w"},"maxReplicas":3}}
		]
	}`
	withFakePathKubectl(t, `printf '%s' '`+stdout+`'`)
	msg, ok := FetchHPAs("ns")().(HPAsMsg)
	if !ok {
		t.Fatalf("expected HPAsMsg, got %T", FetchHPAs("ns")())
	}
	if msg.Err != nil || len(msg.HPAs) != 1 {
		t.Fatalf("happy path wrong: err=%v hpas=%+v", msg.Err, msg.HPAs)
	}
	if msg.HPAs[0].MaxReplicas != 3 {
		t.Errorf("MaxReplicas not propagated: %+v", msg.HPAs[0])
	}

	withFakePathKubectl(t, `printf 'hpa boom' 1>&2; exit 1`)
	errMsg, _ := FetchHPAs("ns")().(HPAsMsg)
	if errMsg.Err == nil {
		t.Error("expected kubectl error")
	}
}
