package model

import (
	"testing"

	"github.com/HediAbed/opsmate/internal/service"
)

func TestCurrentResourceRows_HPAs_RendersAllColumns(t *testing.T) {
	m := NewBrowserModel("ns")
	m.SetSize(220, 40)
	m.SetResourceType("hpas")
	m.hpas = []service.HPA{
		{
			Name:      "web-hpa",
			Namespace: "ns",
			Reference: service.ScaleTargetRef{Kind: "Deployment", Name: "web"},
			Targets: []service.HPAMetricPair{
				{Current: "35%", Target: "80%"},
				{Current: "50", Target: "100"},
			},
			MinReplicas: 2,
			MaxReplicas: 10,
			Replicas:    4,
			Age:         "5d",
		},
	}

	rows := m.currentResourceRows()
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	row := rows[0]
	specCols := resourceColSpecs["hpas"]
	if len(row) != len(specCols) {
		t.Fatalf("row has %d cells, tablespec declares %d columns", len(row), len(specCols))
	}

	want := []string{"web-hpa", "Deployment/web", "35%/80%,50/100", "2", "10", "4", "5d"}
	for i, w := range want {
		if row[i] != w {
			t.Errorf("col %d (%s) = %q, want %q", i, specCols[i].Title, row[i], w)
		}
	}
}

func TestCurrentResourceRows_HPAs_SingleTargetRendersWithoutComma(t *testing.T) {
	m := NewBrowserModel("ns")
	m.SetSize(220, 40)
	m.SetResourceType("hpas")
	m.hpas = []service.HPA{{
		Name:        "h",
		Targets:     []service.HPAMetricPair{{Current: "35%", Target: "80%"}},
		MinReplicas: 1,
		MaxReplicas: 1,
	}}
	row := m.currentResourceRows()[0]
	if row[2] != "35%/80%" {
		t.Errorf("single target should render verbatim; got %q", row[2])
	}
}
