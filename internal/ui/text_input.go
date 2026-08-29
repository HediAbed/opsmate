package ui

import (
	"charm.land/bubbles/v2/textinput"
	"charm.land/lipgloss/v2"
)

type textInputOptions struct {
	Prompt      string
	Placeholder string
	Width       int
	CharLimit   int
	PromptStyle lipgloss.Style
	TextStyle   lipgloss.Style
}

func newTextInput(options textInputOptions) textinput.Model {
	input := textinput.New()
	input.Prompt = options.Prompt
	input.Placeholder = options.Placeholder
	if options.CharLimit > 0 {
		input.CharLimit = options.CharLimit
	}
	if options.Width > 0 {
		input.SetWidth(options.Width)
	}
	styles := textinput.DefaultStyles(true)
	styles.Focused.Prompt = options.PromptStyle
	styles.Blurred.Prompt = options.PromptStyle
	styles.Focused.Text = options.TextStyle
	styles.Blurred.Text = options.TextStyle
	input.SetStyles(styles)
	return input
}
