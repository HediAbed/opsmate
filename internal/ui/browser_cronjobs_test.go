package ui

import (
	"testing"

	"github.com/HediAbed/opsmate/internal/cluster"
)

func TestCurrentResourceRows_CronJobs_RendersAllColumns(t *testing.T) {
	m := newTestBrowserModel("ops")
	m.SetSize(200, 40)
	m.SetResourceType("cronjobs")
	m.cronjobs = []cluster.CronJob{
		{
			Name:         "nightly",
			Namespace:    "ops",
			Schedule:     "0 2 * * *",
			Suspend:      true,
			Active:       3,
			LastSchedule: "5m",
			Age:          "2d",
		},
	}

	rows := m.currentResourceRows()
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	row := rows[0]
	specCols := resourceColSpecs["cronjobs"]
	if len(row) != len(specCols) {
		t.Fatalf("row has %d cells, tablespec declares %d columns", len(row), len(specCols))
	}

	want := []string{"nightly", "0 2 * * *", "True", "3", "5m", "2d"}
	for i, w := range want {
		if row[i] != w {
			t.Errorf("col %d (%s) = %q, want %q", i, specCols[i].Title, row[i], w)
		}
	}
}

func TestCurrentResourceRows_CronJobs_SuspendFalseRendersFalse(t *testing.T) {
	m := newTestBrowserModel("ops")
	m.SetSize(200, 40)
	m.SetResourceType("cronjobs")
	m.cronjobs = []cluster.CronJob{{Name: "n", Schedule: "@hourly"}}

	rows := m.currentResourceRows()
	if rows[0][2] != "False" {
		t.Errorf("default suspend should render as False; got %q", rows[0][2])
	}
}
