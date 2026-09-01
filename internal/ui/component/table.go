package component

import (
	"charm.land/bubbles/v2/table"
	"charm.land/lipgloss/v2"

	"github.com/HediAbed/opsmate/internal/ui/theme"
)

const (
	TableCellPadding = 2
	TableHeaderRows  = 3
	TableWheelStep   = 3
	tableHeightHint  = 10

	ColumnWidthNarrow    = 6
	ColumnWidthCompact   = 8
	ColumnWidthFlag      = 9
	ColumnWidthStandard  = 10
	ColumnWidthMedium    = 12
	ColumnWidthWide      = 14
	ColumnWidthExtraWide = 16
	ColumnWidthKind      = 22

	ColumnFlexMinimal   = 10
	ColumnFlexSmall     = 15
	ColumnFlexModest    = 20
	ColumnFlexQuarter   = 25
	ColumnFlexSecondary = 30
	ColumnFlexMedium    = 35
	ColumnFlexStrong    = 40
	ColumnFlexHalf      = 50
	ColumnFlexPrimary   = 60
	ColumnFlexFull      = 100

	ColumnMinimumCompact  = 8
	ColumnMinimumStandard = 10
	ColumnMinimumReadable = 12
	ColumnMinimumExpanded = 15
	ColumnMinimumWide     = 16
	ColumnMinimumName     = 20
)

type ColumnSpec struct {
	Title string
	Width int
	Flex  int
	Min   int
}

type columnSpecTotals struct {
	fixedWidth  int
	flexWeight  int
	flexMinimum int
	flexCount   int
}

func NewTable(width int, specs []ColumnSpec) table.Model {
	resourceTable := table.New(
		table.WithColumns(Columns(width, specs)),
		table.WithFocused(true),
		table.WithHeight(tableHeightHint),
		table.WithWidth(width),
	)
	resourceTable.SetStyles(TableStyles())
	return resourceTable
}

func Columns(tableWidth int, specs []ColumnSpec) []table.Column {
	available := tableWidth - TableCellPadding*len(specs)
	if available < len(specs) {
		available = len(specs)
	}

	totals := measureColumnSpecs(specs)
	columns := initializeColumns(specs)
	if totals.fixedWidth > available {
		shrinkFixedToFit(columns, specs, available)
		return columns
	}
	allocateFlexColumns(columns, specs, available-totals.fixedWidth, totals)
	return columns
}

func TableStyles() table.Styles {
	styles := table.DefaultStyles()
	styles.Header = styles.Header.
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(theme.DimText).
		BorderBottom(true).
		Bold(true).
		Foreground(theme.NeonCyan)
	styles.Selected = styles.Selected.
		Foreground(theme.White).
		Background(theme.DeepViolet).
		Bold(true)
	return styles
}

func measureColumnSpecs(specs []ColumnSpec) columnSpecTotals {
	var totals columnSpecTotals
	for _, spec := range specs {
		if spec.Width > 0 {
			totals.fixedWidth += spec.Width
			continue
		}
		totals.flexWeight += spec.Flex
		totals.flexMinimum += spec.Min
		totals.flexCount++
	}
	return totals
}

func initializeColumns(specs []ColumnSpec) []table.Column {
	columns := make([]table.Column, len(specs))
	for index, spec := range specs {
		columns[index] = table.Column{Title: spec.Title, Width: max(0, spec.Width)}
	}
	return columns
}

func allocateFlexColumns(columns []table.Column, specs []ColumnSpec, available int, totals columnSpecTotals) {
	applyMinimum := available >= totals.flexMinimum
	allocated, flexSeen := 0, 0
	for index, spec := range specs {
		if spec.Width > 0 {
			continue
		}
		flexSeen++
		if flexSeen == totals.flexCount {
			columns[index].Width = max(0, available-allocated)
			continue
		}
		minimum := 0
		if applyMinimum {
			minimum = spec.Min
		}
		width := flexWidth(available, spec.Flex, totals.flexWeight, minimum)
		allocated += width
		columns[index].Width = width
	}
}

func shrinkFixedToFit(columns []table.Column, specs []ColumnSpec, available int) {
	total := 0
	for index := range columns {
		if specs[index].Width <= 0 {
			columns[index].Width = 0
			continue
		}
		total += columns[index].Width
	}
	if total == 0 {
		return
	}
	used := 0
	lastIndex := -1
	for index := range columns {
		if specs[index].Width <= 0 {
			continue
		}
		width := columns[index].Width * available / total
		if width < 1 {
			width = 1
		}
		columns[index].Width = width
		used += width
		lastIndex = index
	}
	if lastIndex >= 0 {
		columns[lastIndex].Width += available - used
		if columns[lastIndex].Width < 1 {
			columns[lastIndex].Width = 1
		}
	}
}

func flexWidth(remaining, weight, totalWeight, minimum int) int {
	width := 0
	if totalWeight > 0 {
		width = remaining * weight / totalWeight
	}
	if width < minimum {
		width = minimum
	}
	return width
}
