package component

import (
	"slices"
	"testing"

	"charm.land/bubbles/v2/table"
	"charm.land/lipgloss/v2"

	"github.com/HediAbed/opsmate/internal/ui/theme"
)

func TestFlexWidthUsesWeightAndMinimum(t *testing.T) {
	if got := flexWidth(100, 60, 100, 0); got != 60 {
		t.Fatalf("flexWidth() = %d, want 60", got)
	}
	if got := flexWidth(20, 1, 100, 30); got != 30 {
		t.Fatalf("flexWidth() = %d, want 30", got)
	}
	if got := flexWidth(100, 0, 0, 8); got != 8 {
		t.Fatalf("flexWidth() = %d, want 8", got)
	}
}

func TestShrinkFixedToFitIgnoresFlexibleSpecs(t *testing.T) {
	columns := []table.Column{{Width: 4}, {Width: 6}}
	specs := []ColumnSpec{{Flex: 1}, {Flex: 1}}
	shrinkFixedToFit(columns, specs, 5)
	if columns[0].Width != 0 || columns[1].Width != 0 {
		t.Fatalf("column widths = [%d %d], want [0 0]", columns[0].Width, columns[1].Width)
	}
}

func TestShrinkFixedToFitKeepsFixedColumnsVisible(t *testing.T) {
	columns := []table.Column{{Width: 100}, {Width: 100}}
	specs := []ColumnSpec{{Width: 100}, {Width: 100}}
	shrinkFixedToFit(columns, specs, 1)
	if columns[0].Width != 1 || columns[1].Width != 1 {
		t.Fatalf("column widths = [%d %d], want [1 1]", columns[0].Width, columns[1].Width)
	}
}

func TestColumnsKeepsFixedWidthsWithinAvailableSpace(t *testing.T) {
	columns := Columns(30, []ColumnSpec{{Title: "a", Width: 10}, {Title: "b", Width: 10}})
	assertColumnWidths(t, columns, []int{10, 10})
}

func TestColumnsShrinksFixedWidthsExceedingAvailableSpace(t *testing.T) {
	columns := Columns(14, []ColumnSpec{{Title: "a", Width: 100}, {Title: "b", Width: 100}})
	assertColumnWidths(t, columns, []int{5, 5})
}

func TestColumnsDistributesFlexWeightsWithMinimums(t *testing.T) {
	specs := []ColumnSpec{
		{Title: "fixed", Width: 10},
		{Title: "wide", Flex: 60, Min: 20},
		{Title: "rest", Flex: 40, Min: 10},
	}
	columns := Columns(64, specs)
	assertColumnWidths(t, columns, []int{10, 28, 20})
}

func TestColumnsDropsMinimumsWhenSpaceIsTight(t *testing.T) {
	specs := []ColumnSpec{
		{Title: "a", Flex: 1, Min: 30},
		{Title: "b", Flex: 1, Min: 30},
	}
	columns := Columns(10, specs)
	assertColumnWidths(t, columns, []int{3, 3})
}

func TestColumnsClampsAvailableToColumnCount(t *testing.T) {
	specs := []ColumnSpec{{Title: "a"}, {Title: "b"}, {Title: "c"}}
	columns := Columns(1, specs)
	assertColumnWidths(t, columns, []int{0, 0, 3})
}

func TestColumnsTreatsNegativeWidthAsFlexible(t *testing.T) {
	specs := []ColumnSpec{{Title: "a", Width: -5, Flex: 1, Min: 4}}
	columns := Columns(12, specs)
	assertColumnWidths(t, columns, []int{10})
}

func assertColumnWidths(t *testing.T, columns []table.Column, want []int) {
	t.Helper()
	widths := make([]int, len(columns))
	for index, column := range columns {
		widths[index] = column.Width
	}
	if !slices.Equal(widths, want) {
		t.Fatalf("column widths = %v, want %v", widths, want)
	}
}

func TestMeasureColumnSpecsSeparatesFixedAndFlexible(t *testing.T) {
	totals := measureColumnSpecs([]ColumnSpec{
		{Width: 10},
		{Flex: 60, Min: 20},
		{Flex: 40, Min: 10},
	})
	if totals.fixedWidth != 10 || totals.flexWeight != 100 || totals.flexMinimum != 30 || totals.flexCount != 2 {
		t.Fatalf("totals = %+v, want fixed 10 weight 100 minimum 30 count 2", totals)
	}
}

func TestInitializeColumnsClampsNegativeWidths(t *testing.T) {
	columns := initializeColumns([]ColumnSpec{{Title: "a", Width: -5}, {Title: "b", Width: 10}})
	assertColumnWidths(t, columns, []int{0, 10})
	if columns[0].Title != "a" || columns[1].Title != "b" {
		t.Fatalf("column titles = [%q %q], want [a b]", columns[0].Title, columns[1].Title)
	}
}

func TestAllocateFlexColumnsSkipsFixedSpecs(t *testing.T) {
	columns := initializeColumns([]ColumnSpec{{Title: "fixed", Width: 10}, {Title: "flex", Flex: 1, Min: 5}})
	specs := []ColumnSpec{{Width: 10}, {Flex: 1, Min: 5}}
	allocateFlexColumns(columns, specs, 20, measureColumnSpecs(specs))
	assertColumnWidths(t, columns, []int{10, 20})
}

func TestNewTableBuildsFocusedTableWithSizedColumns(t *testing.T) {
	resourceTable := NewTable(64, []ColumnSpec{{Title: "NAME", Flex: 60, Min: 20}, {Title: "AGE", Width: 6}})
	if !resourceTable.Focused() {
		t.Fatal("NewTable should return a focused table")
	}
	columns := resourceTable.Columns()
	if len(columns) != 2 || columns[0].Title != "NAME" || columns[1].Title != "AGE" {
		t.Fatalf("columns = %+v", columns)
	}
	assertColumnWidths(t, columns, []int{54, 6})
}

func TestTableStylesHighlightsHeaderAndSelection(t *testing.T) {
	styles := TableStyles()
	if !styles.Header.GetBold() {
		t.Error("header should be bold")
	}
	if got := styles.Header.GetForeground(); got != theme.NeonCyan {
		t.Errorf("header foreground = %v, want %v", got, theme.NeonCyan)
	}
	if got := styles.Selected.GetBackground(); got != theme.DeepViolet {
		t.Errorf("selected background = %v, want %v", got, theme.DeepViolet)
	}
	if got := styles.Selected.GetForeground(); got != theme.White {
		t.Errorf("selected foreground = %v, want %v", got, theme.White)
	}
	if !styles.Selected.GetBold() {
		t.Error("selected row should be bold")
	}
}

func TestTableStylesHeaderShowsBottomBorder(t *testing.T) {
	border := TableStyles().Header.GetBorderStyle()
	if border != lipgloss.NormalBorder() {
		t.Errorf("header border = %v, want normal border", border)
	}
}

func TestShrinkFixedToFitSkipsFlexibleColumnsInMixedLayout(t *testing.T) {
	columns := []table.Column{{Width: 100}, {Width: 0}}
	specs := []ColumnSpec{{Width: 100}, {Flex: 1}}
	shrinkFixedToFit(columns, specs, 10)
	if columns[0].Width != 10 || columns[1].Width != 0 {
		t.Fatalf("column widths = [%d %d], want [10 0]", columns[0].Width, columns[1].Width)
	}
}
