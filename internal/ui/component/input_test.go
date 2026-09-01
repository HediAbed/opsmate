package component

import (
	"testing"

	"charm.land/bubbles/v2/textinput"
	"charm.land/lipgloss/v2"
)

func TestNewTextInputAppliesOptions(t *testing.T) {
	promptStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#00FFFF"))
	textStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#FFFFFF"))
	input := NewTextInput(TextInputOptions{
		Prompt:      "find: ",
		Placeholder: "pod name...",
		Width:       42,
		CharLimit:   64,
		PromptStyle: promptStyle,
		TextStyle:   textStyle,
	})

	if input.Prompt != "find: " {
		t.Errorf("Prompt = %q, want %q", input.Prompt, "find: ")
	}
	if input.Placeholder != "pod name..." {
		t.Errorf("Placeholder = %q, want %q", input.Placeholder, "pod name...")
	}
	if input.CharLimit != 64 {
		t.Errorf("CharLimit = %d, want 64", input.CharLimit)
	}
	if input.Width() != 42 {
		t.Errorf("Width() = %d, want 42", input.Width())
	}
	styles := input.Styles()
	if got := styles.Focused.Prompt.GetForeground(); got != lipgloss.Color("#00FFFF") {
		t.Errorf("focused prompt color = %v, want #00FFFF", got)
	}
	if got := styles.Blurred.Prompt.GetForeground(); got != lipgloss.Color("#00FFFF") {
		t.Errorf("blurred prompt color = %v, want #00FFFF", got)
	}
	if got := styles.Focused.Text.GetForeground(); got != lipgloss.Color("#FFFFFF") {
		t.Errorf("focused text color = %v, want #FFFFFF", got)
	}
	if got := styles.Blurred.Text.GetForeground(); got != lipgloss.Color("#FFFFFF") {
		t.Errorf("blurred text color = %v, want #FFFFFF", got)
	}
}

func TestNewTextInputLeavesDefaultsForZeroOptions(t *testing.T) {
	reference := textinput.New()
	input := NewTextInput(TextInputOptions{})

	if input.CharLimit != reference.CharLimit {
		t.Errorf("CharLimit = %d, want default %d", input.CharLimit, reference.CharLimit)
	}
	if input.Width() != reference.Width() {
		t.Errorf("Width() = %d, want default %d", input.Width(), reference.Width())
	}
}
