package version

import (
	"runtime/debug"
	"strings"
)

const development = "development"

var release string

func Current() string {
	return resolve(release, debug.ReadBuildInfo)
}

func resolve(value string, readBuildInfo func() (*debug.BuildInfo, bool)) string {
	if value = normalized(value); value != "" {
		return value
	}
	build, available := readBuildInfo()
	if !available || build == nil {
		return development
	}
	value = normalized(build.Main.Version)
	if value == "" || value == "(devel)" {
		return development
	}
	return strings.TrimPrefix(value, "v")
}

func normalized(value string) string {
	return strings.TrimSpace(value)
}
