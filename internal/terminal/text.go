package terminal

import (
	"strings"
	"unicode/utf8"

	"github.com/charmbracelet/x/ansi"
)

const (
	maximumC1Control = 0x9f
	maximumLineRunes = 512
)

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

func SanitizeLine(value string) string {
	line := strings.Join(strings.Fields(SanitizeText(value)), " ")
	characters := []rune(line)
	if len(characters) <= maximumLineRunes {
		return line
	}
	return string(characters[:maximumLineRunes-1]) + "…"
}

func SanitizeLines(lines []string) []string {
	sanitized := make([]string, len(lines))
	for index, line := range lines {
		sanitized[index] = SanitizeText(line)
	}
	return sanitized
}

func TruncateRunes(value string, limit int, suffix string) string {
	characters := []rune(value)
	if len(characters) <= limit {
		return value
	}
	if limit <= 0 {
		return suffix
	}
	return strings.TrimSpace(string(characters[:limit])) + suffix
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
			writeInvalidTerminalByte(&normalized, value[0])
			value = value[1:]
			continue
		}
		normalized.WriteString(value[:size])
		value = value[size:]
	}
	return normalized.String()
}

func writeInvalidTerminalByte(normalized *strings.Builder, value byte) {
	if value > maximumC1Control {
		normalized.WriteRune(utf8.RuneError)
	}
}
