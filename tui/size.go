// Package tui provides theme-agnostic building blocks for responsive
// terminal interfaces: cell sizes, framed panels whose chrome derives
// from Lip Gloss frame metrics, modal width fitting, viewport scroll
// indicators, table cursor labels, and width-aware hint composition.
// Colors and styling decisions stay with callers.
package tui

// Size is a rectangular extent in terminal cells.
type Size struct {
	Width  int
	Height int
}
