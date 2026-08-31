package component

import "charm.land/lipgloss/v2"

type Panel struct {
	style lipgloss.Style
}

func NewPanel(style lipgloss.Style) Panel {
	return Panel{style: style}
}

func (p Panel) ContentSize(outer Size) Size {
	return Size{
		Width:  max(0, outer.Width-p.style.GetHorizontalFrameSize()),
		Height: max(0, outer.Height-p.style.GetVerticalFrameSize()),
	}
}

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
