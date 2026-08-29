package tui

import "charm.land/lipgloss/v2"

// Panel sizes and renders content inside the frame of a lipgloss
// style. All chrome arithmetic derives from the style's own frame
// metrics, so layout math cannot drift from the rendered result.
type Panel struct {
	style lipgloss.Style
}

// NewPanel builds a panel whose chrome is the border, padding, and
// margin frame of style.
func NewPanel(style lipgloss.Style) Panel {
	return Panel{style: style}
}

// ContentSize returns the space left for content when the panel fills
// outer, clamped at zero when outer cannot hold the frame.
func (p Panel) ContentSize(outer Size) Size {
	return Size{
		Width:  max(0, outer.Width-p.style.GetHorizontalFrameSize()),
		Height: max(0, outer.Height-p.style.GetVerticalFrameSize()),
	}
}

// Render draws content framed and sized to fill outer, the complete
// panel extent including margins. Overflowing content is clipped so
// the result never exceeds outer. Outer sizes without room for the
// frame plus one content cell render nothing, because a partial frame
// would spill past the terminal edge.
func (p Panel) Render(outer Size, content string) string {
	inner := p.ContentSize(outer)
	if inner.Width == 0 || inner.Height == 0 {
		return ""
	}
	return p.style.
		Width(outer.Width - p.style.GetHorizontalMargins()).
		Height(outer.Height - p.style.GetVerticalMargins()).
		MaxWidth(outer.Width).
		MaxHeight(outer.Height).
		Render(content)
}
