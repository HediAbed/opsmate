package component

const modalSideGutterWidth = 2

const modalSideCount = 2

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
