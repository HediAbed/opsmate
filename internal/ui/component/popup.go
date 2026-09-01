package component

import (
	"charm.land/bubbles/v2/viewport"

	"github.com/HediAbed/opsmate/internal/ui/theme"
)

func PopupScrollIndicator(view viewport.Model) string {
	indicator := ViewportScrollIndicator(view)
	if indicator == "" {
		return ""
	}
	return theme.Dim.Render(indicator)
}
