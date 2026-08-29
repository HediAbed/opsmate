package tui

// modalSideGutterWidth is kept clear on each side of a fitted modal
// so its border never touches the terminal edge.
const modalSideGutterWidth = 2

const modalSideCount = 2

// FitModalWidth shrinks desired so a modal fits inside a terminal of
// the given width, keeping a gutter on each side while the terminal
// has room for one. On terminals too narrow for the gutters the modal
// claims the full terminal width instead of vanishing. Non-positive
// inputs normalize to a zero width.
func FitModalWidth(desired, terminal int) int {
	if desired <= 0 || terminal <= 0 {
		return 0
	}
	limit := terminal - modalSideCount*modalSideGutterWidth
	if limit < 1 {
		limit = terminal
	}
	return min(desired, limit)
}
