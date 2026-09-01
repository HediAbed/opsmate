package browser

import (
	"slices"
	"strings"
)

const labelSelectorAll = "<all>"

var pvcAccessModeShortNames = map[string]string{
	"ReadWriteOnce":    "RWO",
	"ReadOnlyMany":     "ROX",
	"ReadWriteMany":    "RWX",
	"ReadWriteOncePod": "RWOP",
}

func formatPVCAccessModes(modes []string) []string {
	formatted := make([]string, len(modes))
	for index, mode := range modes {
		if short, ok := pvcAccessModeShortNames[mode]; ok {
			formatted[index] = short
			continue
		}
		formatted[index] = mode
	}
	return formatted
}

func formatBoolColumn(value bool) string {
	if value {
		return "True"
	}
	return "False"
}

func formatLabelSelector(labels map[string]string) string {
	if len(labels) == 0 {
		return labelSelectorAll
	}
	keys := make([]string, 0, len(labels))
	for key := range labels {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	parts := make([]string, 0, len(labels))
	for _, key := range keys {
		parts = append(parts, key+"="+labels[key])
	}
	return strings.Join(parts, ",")
}
