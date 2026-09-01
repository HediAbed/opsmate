package component

func NounForCount(singular, plural string, count int) string {
	if count == 1 {
		return singular
	}
	return plural
}
