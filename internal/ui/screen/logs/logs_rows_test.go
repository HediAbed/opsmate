package logs

import "testing"

const (
	rowIndexProbeWidth = 10
	shortProbeLine     = "short"
	wrappingProbeLine  = "0123456789012345678901234"
)

func TestRenderedRowCountFallsBackToOneRowWithoutAWidth(t *testing.T) {
	if got := renderedRowCount(wrappingProbeLine, 0); got != singleRenderedRow {
		t.Fatalf("rows without width = %d, want %d", got, singleRenderedRow)
	}
}

func TestRenderedRowCountCountsWrappedRows(t *testing.T) {
	tests := map[string]struct {
		line string
		want int
	}{
		"fits exactly":    {line: "0123456789", want: 1},
		"shorter":         {line: shortProbeLine, want: 1},
		"wraps partially": {line: wrappingProbeLine, want: 3},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			if got := renderedRowCount(test.line, rowIndexProbeWidth); got != test.want {
				t.Fatalf("rows = %d, want %d", got, test.want)
			}
		})
	}
}

func TestLineRowIndexReportsRowsPerLine(t *testing.T) {
	index := newLineRowIndex([]string{shortProbeLine, wrappingProbeLine, shortProbeLine}, rowIndexProbeWidth)
	if index.totalRows != 5 {
		t.Fatalf("total rows = %d, want 5", index.totalRows)
	}
	if got := index.rowOf(1); got != 1 {
		t.Fatalf("row of second line = %d, want 1", got)
	}
	if got := index.rowsIn(1); got != 3 {
		t.Fatalf("rows in second line = %d, want 3", got)
	}
}

func TestLineRowIndexClampsOutOfRangeLines(t *testing.T) {
	index := newLineRowIndex([]string{shortProbeLine}, rowIndexProbeWidth)
	if got := index.rowOf(-1); got != 0 {
		t.Fatalf("row of negative line = %d, want 0", got)
	}
	if got := index.rowOf(99); got != 0 {
		t.Fatalf("row of missing line = %d, want 0", got)
	}
	if got := index.rowsIn(-1); got != singleRenderedRow {
		t.Fatalf("rows in negative line = %d, want %d", got, singleRenderedRow)
	}
	if got := index.rowsIn(99); got != singleRenderedRow {
		t.Fatalf("rows in missing line = %d, want %d", got, singleRenderedRow)
	}
}

func TestLineAtMapsRowsBackToLines(t *testing.T) {
	index := newLineRowIndex([]string{shortProbeLine, wrappingProbeLine, shortProbeLine}, rowIndexProbeWidth)
	tests := map[string]struct {
		row  int
		want int
	}{
		"first row":         {row: 0, want: 0},
		"negative row":      {row: -5, want: 0},
		"inside wrapped":    {row: 2, want: 1},
		"after wrapped":     {row: 4, want: 2},
		"beyond last row":   {row: 99, want: 2},
		"start of wrapping": {row: 1, want: 1},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			if got := index.lineAt(test.row); got != test.want {
				t.Fatalf("lineAt(%d) = %d, want %d", test.row, got, test.want)
			}
		})
	}
}

func TestLineAtOnAnEmptyBufferSelectsNothing(t *testing.T) {
	index := newLineRowIndex(nil, rowIndexProbeWidth)
	if got := index.lineAt(3); got != noLineSelected {
		t.Fatalf("lineAt on empty buffer = %d, want %d", got, noLineSelected)
	}
}
