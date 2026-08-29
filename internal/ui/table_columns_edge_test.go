package ui

import (
	"testing"

	"charm.land/bubbles/v2/table"
)

func TestShrinkFixedToFitIgnoresFlexibleOnlySpecs(t *testing.T) {
	columns := []table.Column{{Width: 4}, {Width: 6}}
	specs := []colSpec{{Flex: 1}, {Flex: 1}}

	shrinkFixedToFit(columns, specs, 5)

	if columns[0].Width != 0 || columns[1].Width != 0 {
		t.Fatalf("column widths = [%d %d], want [0 0]", columns[0].Width, columns[1].Width)
	}
}

func TestShrinkFixedToFitKeepsEveryFixedColumnVisible(t *testing.T) {
	columns := []table.Column{{Width: 100}, {Width: 100}}
	specs := []colSpec{{Width: 100}, {Width: 100}}

	shrinkFixedToFit(columns, specs, 1)

	if columns[0].Width != 1 || columns[1].Width != 1 {
		t.Fatalf("column widths = [%d %d], want [1 1]", columns[0].Width, columns[1].Width)
	}
}
