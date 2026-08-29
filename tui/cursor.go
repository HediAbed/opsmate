package tui

import "fmt"

// TableCursorLabel formats a one-based "cursor/total" position label.
// Tables without rows produce an empty label, and cursors outside
// [0, total) clamp into range so a stale cursor never reports an
// impossible position.
func TableCursorLabel(cursor, total int) string {
	if total <= 0 {
		return ""
	}
	clamped := min(max(cursor, 0), total-1)
	return fmt.Sprintf("%d/%d", clamped+1, total)
}
