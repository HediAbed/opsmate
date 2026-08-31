package terminal

import (
	"strings"
	"unicode/utf8"

	"github.com/charmbracelet/x/ansi"
)

const maximumC1Control = 0x9f

func SanitizeText(value string) string {
	value = ansi.Strip(normalizeTerminalUTF8(value))
	var sanitized strings.Builder
	sanitized.Grow(len(value))
	for _, character := range value {
		if isSafeTerminalRune(character) {
			sanitized.WriteRune(character)
		}
	}
	return sanitized.String()
}

func isSafeTerminalRune(character rune) bool {
	if character == '\n' || character == '\t' {
		return true
	}
	return character >= ' ' && (character < 0x7f || character > maximumC1Control)
}

func normalizeTerminalUTF8(value string) string {
	var normalized strings.Builder
	normalized.Grow(len(value))
	for len(value) > 0 {
		character, size := utf8.DecodeRuneInString(value)
		if character == utf8.RuneError && size == 1 {
			if value[0] > maximumC1Control {
				normalized.WriteRune(utf8.RuneError)
			}
			value = value[1:]
			continue
		}
		normalized.WriteString(value[:size])
		value = value[size:]
	}
	return normalized.String()
}
