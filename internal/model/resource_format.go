package model

import (
	"slices"
	"strings"
)

const labelSelectorAll = "<all>"

// pvcAccessModeShortNames mirrors kubectl's access-mode abbreviations.
var pvcAccessModeShortNames = map[string]string{
	"ReadWriteOnce":    "RWO",
	"ReadOnlyMany":     "ROX",
	"ReadWriteMany":    "RWX",
	"ReadWriteOncePod": "RWOP",
}

func formatPVCAccessModes(modes []string) []string {
	out := make([]string, len(modes))
	for i, m := range modes {
		if short, ok := pvcAccessModeShortNames[m]; ok {
			out[i] = short
			continue
		}
		out[i] = m
	}
	return out
}

func formatBoolColumn(b bool) string {
	if b {
		return "True"
	}
	return "False"
}

// formatLabelSelector returns labels in stable key order.
func formatLabelSelector(labels map[string]string) string {
	if len(labels) == 0 {
		return labelSelectorAll
	}
	keys := make([]string, 0, len(labels))
	for k := range labels {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	parts := make([]string, 0, len(labels))
	for _, k := range keys {
		parts = append(parts, k+"="+labels[k])
	}
	return strings.Join(parts, ",")
}
