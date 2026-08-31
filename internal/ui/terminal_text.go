package ui

import (
	"strings"

	"github.com/HediAbed/opsmate/internal/terminal"
)

const maxTerminalLineRunes = 512

func sanitizeTerminalText(value string) string {
	return terminal.SanitizeText(value)
}

func sanitizeTerminalLine(value string) string {
	line := strings.Join(strings.Fields(sanitizeTerminalText(value)), " ")
	runes := []rune(line)
	if len(runes) <= maxTerminalLineRunes {
		return line
	}
	return string(runes[:maxTerminalLineRunes-1]) + "…"
}

func sanitizeTerminalLines(lines []string) []string {
	sanitized := make([]string, len(lines))
	for index, line := range lines {
		sanitized[index] = sanitizeTerminalText(line)
	}
	return sanitized
}
