package model

import (
	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbles/v2/viewport"
	"charm.land/lipgloss/v2"
)

type textInputOpts struct {
	Prompt      string
	Placeholder string
	Width       int
	CharLimit   int
	PromptStyle lipgloss.Style
	TextStyle   lipgloss.Style
}

func newTextInput(o textInputOpts) textinput.Model {
	ti := textinput.New()
	ti.Prompt = o.Prompt
	ti.Placeholder = o.Placeholder
	if o.CharLimit > 0 {
		ti.CharLimit = o.CharLimit
	}
	if o.Width > 0 {
		ti.SetWidth(o.Width)
	}
	styles := textinput.DefaultStyles(true)
	styles.Focused.Prompt = o.PromptStyle
	styles.Blurred.Prompt = o.PromptStyle
	styles.Focused.Text = o.TextStyle
	styles.Blurred.Text = o.TextStyle
	ti.SetStyles(styles)
	return ti
}

func newViewport(width, height int) viewport.Model {
	return viewport.New(viewport.WithWidth(width), viewport.WithHeight(height))
}
