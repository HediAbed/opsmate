package main

import (
	_ "embed"
	"strings"
)

//go:embed VERSION
var embeddedVersion string

func currentVersion() string {
	return strings.TrimSpace(embeddedVersion)
}
