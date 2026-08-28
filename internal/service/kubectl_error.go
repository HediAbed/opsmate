package service

import (
	"regexp"
	"strings"
)

const (
	maxSanitizedLines = 2
	maxSanitizedChars = 240
)

var (
	klogPrefix = regexp.MustCompile(`^[IWE]\d{4} \d{2}:\d{2}:\d{2}\.\d+\s+\d+\s+\S+:\d+\]`)
	retryNoise = regexp.MustCompile(`(?i)^\s*(unhandled error|err=|x509: certificate signed by unknown authority\s*$)`)
)

func SanitizeKubectlStderr(raw string) string {
	raw = stripANSI(raw)
	if strings.TrimSpace(raw) == "" {
		return ""
	}
	cleaned := dedupeConsecutive(meaningfulLines(raw))
	if len(cleaned) == 0 {
		return ellipsize(strings.TrimSpace(raw), maxSanitizedChars)
	}
	return joinSummary(cleaned)
}

func meaningfulLines(raw string) []string {
	var out []string
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if klogPrefix.MatchString(line) {
			continue
		}
		if retryNoise.MatchString(line) {
			continue
		}
		out = append(out, line)
	}
	return out
}

func dedupeConsecutive(lines []string) []string {
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		if len(out) > 0 && out[len(out)-1] == line {
			continue
		}
		out = append(out, line)
	}
	return out
}

func joinSummary(lines []string) string {
	if len(lines) > maxSanitizedLines {
		lines = []string{lines[0], lines[len(lines)-1]}
	}
	for i, line := range lines {
		lines[i] = ellipsize(line, maxSanitizedChars)
	}
	return strings.Join(lines, "\n")
}

func ellipsize(value string, limit int) string {
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit-1]) + "…"
}
