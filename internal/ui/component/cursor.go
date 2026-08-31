package component

import "fmt"

func TableCursorLabel(cursor, total int) string {
	if total <= 0 {
		return ""
	}
	clamped := min(max(cursor, 0), total-1)
	return fmt.Sprintf("%d/%d", clamped+1, total)
}
