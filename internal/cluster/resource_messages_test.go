package cluster

import "testing"

func TestScaleTargetReferenceString(t *testing.T) {
	tests := []struct {
		name      string
		reference ScaleTargetRef
		want      string
	}{
		{name: "complete", reference: ScaleTargetRef{Kind: "Deployment", Name: "api"}, want: "Deployment/api"},
		{name: "missing kind", reference: ScaleTargetRef{Name: "api"}},
		{name: "missing name", reference: ScaleTargetRef{Kind: "Deployment"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := test.reference.String(); got != test.want {
				t.Fatalf("String() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestHPAMetricPairString(t *testing.T) {
	tests := []struct {
		name string
		pair HPAMetricPair
		want string
	}{
		{name: "both", pair: HPAMetricPair{Current: "40%", Target: "80%"}, want: "40%/80%"},
		{name: "target", pair: HPAMetricPair{Target: "80%"}, want: "80%"},
		{name: "current", pair: HPAMetricPair{Current: "40%"}, want: "40%"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := test.pair.String(); got != test.want {
				t.Fatalf("String() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestJoinVersions(t *testing.T) {
	if got := JoinVersions([]string{"v1", "v2"}); got != "v1,v2" {
		t.Fatalf("JoinVersions() = %q", got)
	}
}
