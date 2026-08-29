// Package opsmate exposes the release version embedded in the executable.
package opsmate

import (
	_ "embed"
	"strings"
)

//go:embed VERSION
var embeddedVersion string

// Version returns the semantic version embedded from VERSION.
func Version() string {
	return strings.TrimSpace(embeddedVersion)
}
