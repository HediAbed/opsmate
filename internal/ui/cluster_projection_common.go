package ui

import (
	"fmt"
	"maps"
	"slices"
	"strings"
	"time"
)

func projectSlice[Source, Target any](items []Source, now time.Time, project func(Source, time.Time) Target) []Target {
	projected := make([]Target, 0, len(items))
	for _, item := range items {
		projected = append(projected, project(item, now))
	}
	return projected
}

func projectedAge(now, timestamp time.Time) string {
	if timestamp.IsZero() {
		return projectedUnknown
	}
	return projectedDuration(now.Sub(timestamp))
}

func projectedDuration(duration time.Duration) string {
	if duration < 0 {
		duration = 0
	}
	switch {
	case duration < time.Minute:
		return fmt.Sprintf("%ds", int(duration.Seconds()))
	case duration < time.Hour:
		return fmt.Sprintf("%dm", int(duration.Minutes()))
	case duration < projectedDay:
		return fmt.Sprintf("%dh", int(duration.Hours()))
	default:
		return fmt.Sprintf("%dd", int(duration/projectedDay))
	}
}

func projectedLabelMap(labels map[string]string) string {
	if len(labels) == 0 {
		return ""
	}
	keys := slices.Sorted(maps.Keys(labels))
	values := make([]string, 0, len(keys))
	for _, key := range keys {
		values = append(values, key+"="+labels[key])
	}
	return strings.Join(values, ",")
}

func appendUnique(values []string, candidate string) []string {
	if slices.Contains(values, candidate) {
		return values
	}
	return append(values, candidate)
}

func joinProjectedValues(values []string) string {
	if len(values) == 0 {
		return projectedNone
	}
	return strings.Join(values, ",")
}
