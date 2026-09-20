package logs

import (
	"sort"

	"github.com/charmbracelet/x/ansi"
)

const (
	singleRenderedRow = 1
	noLineSelected    = 0
)

type lineRowIndex struct {
	rowStarts []int
	rowCounts []int
	totalRows int
}

func newLineRowIndex(lines []string, viewportWidth int) lineRowIndex {
	index := lineRowIndex{
		rowStarts: make([]int, len(lines)),
		rowCounts: make([]int, len(lines)),
	}
	for position, line := range lines {
		index.rowStarts[position] = index.totalRows
		rows := renderedRowCount(line, viewportWidth)
		index.rowCounts[position] = rows
		index.totalRows += rows
	}
	return index
}

func renderedRowCount(line string, viewportWidth int) int {
	if viewportWidth <= 0 {
		return singleRenderedRow
	}
	width := ansi.StringWidth(line)
	if width <= viewportWidth {
		return singleRenderedRow
	}
	rows := width / viewportWidth
	if width%viewportWidth != 0 {
		rows++
	}
	return rows
}

func (index lineRowIndex) rowOf(line int) int {
	if line < 0 || line >= len(index.rowStarts) {
		return 0
	}
	return index.rowStarts[line]
}

func (index lineRowIndex) rowsIn(line int) int {
	if line < 0 || line >= len(index.rowCounts) {
		return singleRenderedRow
	}
	return index.rowCounts[line]
}

func (index lineRowIndex) lineAt(row int) int {
	if len(index.rowStarts) == 0 || row <= 0 {
		return noLineSelected
	}
	if row >= index.totalRows {
		return len(index.rowStarts) - 1
	}
	following := sort.Search(len(index.rowStarts), func(position int) bool {
		return index.rowStarts[position] > row
	})
	return following - 1
}
