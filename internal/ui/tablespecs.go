package ui

import (
	"charm.land/bubbles/v2/table"
)

type colSpec struct {
	Title string
	Width int
	Flex  int
	Min   int
}

const (
	tableHeightHint       = 10
	tableCellPadding      = 2
	tableHeaderChromeRows = 3
	tableWheelStep        = 3

	columnWidthNarrow    = 6
	columnWidthCompact   = 8
	columnWidthFlag      = 9
	columnWidthStandard  = 10
	columnWidthMedium    = 12
	columnWidthWide      = 14
	columnWidthExtraWide = 16
	columnWidthKind      = 22

	columnFlexMinimal   = 10
	columnFlexSmall     = 15
	columnFlexModest    = 20
	columnFlexQuarter   = 25
	columnFlexSecondary = 30
	columnFlexMedium    = 35
	columnFlexStrong    = 40
	columnFlexHalf      = 50
	columnFlexPrimary   = 60
	columnFlexFull      = 100

	columnMinimumCompact  = 8
	columnMinimumStandard = 10
	columnMinimumReadable = 12
	columnMinimumExpanded = 15
	columnMinimumWide     = 16
	columnMinimumName     = 20
)

var resourceColSpecs = map[string][]colSpec{
	resourceTypePods: {
		{Title: "NAME", Flex: columnFlexPrimary, Min: columnMinimumCompact},
		{Title: "STATUS", Width: columnWidthMedium},
		{Title: "READY", Width: columnWidthCompact},
		{Title: "RESTARTS", Width: columnWidthStandard},
		{Title: "AGE", Width: columnWidthNarrow},
		{Title: "NODE", Flex: columnFlexStrong, Min: columnMinimumCompact},
	},
	resourceTypeDeployments: {
		{Title: "NAME", Flex: columnFlexFull, Min: columnMinimumName},
		{Title: "READY", Width: columnWidthStandard},
		{Title: "UP-TO-DATE", Width: columnWidthMedium},
		{Title: "AVAILABLE", Width: columnWidthMedium},
		{Title: "AGE", Width: columnWidthCompact},
	},
	resourceTypeServices: {
		{Title: "NAME", Flex: columnFlexPrimary, Min: columnMinimumName},
		{Title: "TYPE", Width: columnWidthMedium},
		{Title: "CLUSTER-IP", Width: columnWidthExtraWide},
		{Title: "PORTS", Flex: columnFlexStrong, Min: columnMinimumExpanded},
		{Title: "AGE", Width: columnWidthCompact},
	},
	resourceTypeStatefulSets: {
		{Title: "NAME", Flex: columnFlexFull, Min: columnMinimumName},
		{Title: "READY", Width: columnWidthMedium},
		{Title: "REPLICAS", Width: columnWidthMedium},
		{Title: "AGE", Width: columnWidthCompact},
	},
	resourceTypeDaemonSets: {
		{Title: "NAME", Flex: columnFlexFull, Min: columnMinimumName},
		{Title: "DESIRED", Width: columnWidthStandard},
		{Title: "CURRENT", Width: columnWidthStandard},
		{Title: "READY", Width: columnWidthStandard},
		{Title: "AVAILABLE", Width: columnWidthMedium},
		{Title: "AGE", Width: columnWidthCompact},
	},
	resourceTypeConfigMaps: {
		{Title: "NAME", Flex: columnFlexFull, Min: columnMinimumName},
		{Title: "DATA", Width: columnWidthStandard},
		{Title: "AGE", Width: columnWidthStandard},
	},
	resourceTypeNodes: {
		{Title: "NAME", Flex: columnFlexPrimary, Min: columnMinimumName},
		{Title: "STATUS", Width: columnWidthMedium},
		{Title: "ROLES", Flex: columnFlexStrong, Min: columnMinimumReadable},
		{Title: "VERSION", Width: columnWidthExtraWide},
		{Title: "AGE", Width: columnWidthCompact},
	},
	resourceTypeJobs: {
		{Title: "NAME", Flex: columnFlexFull, Min: columnMinimumName},
		{Title: "COMPLETIONS", Width: columnWidthWide},
		{Title: "DURATION", Width: columnWidthMedium},
		{Title: "STATUS", Width: columnWidthMedium},
		{Title: "AGE", Width: columnWidthCompact},
	},
	resourceTypeIngresses: {
		{Title: "NAME", Flex: columnFlexHalf, Min: columnMinimumWide},
		{Title: "CLASS", Width: columnWidthMedium},
		{Title: "HOSTS", Flex: columnFlexSecondary, Min: columnMinimumReadable},
		{Title: "ADDRESS", Flex: columnFlexModest, Min: columnMinimumReadable},
		{Title: "PORTS", Width: columnWidthStandard},
		{Title: "AGE", Width: columnWidthCompact},
	},
	resourceTypeNetworkPolicies: {
		{Title: "NAME", Flex: columnFlexHalf, Min: columnMinimumWide},
		{Title: "POD-SELECTOR", Flex: columnFlexMedium, Min: columnMinimumReadable},
		{Title: "POLICY-TYPES", Flex: columnFlexSmall, Min: columnMinimumReadable},
		{Title: "AGE", Width: columnWidthCompact},
	},
	resourceTypePVCs: {
		{Title: "NAME", Flex: columnFlexStrong, Min: columnMinimumWide},
		{Title: "STATUS", Width: columnWidthStandard},
		{Title: "VOLUME", Flex: columnFlexSecondary, Min: columnMinimumWide},
		{Title: "CAPACITY", Width: columnWidthStandard},
		{Title: "ACCESS-MODES", Width: columnWidthWide},
		{Title: "STORAGE-CLASS", Flex: columnFlexSecondary, Min: columnMinimumReadable},
		{Title: "AGE", Width: columnWidthCompact},
	},
	resourceTypeCronJobs: {
		{Title: "NAME", Flex: columnFlexHalf, Min: columnMinimumWide},
		{Title: "SCHEDULE", Flex: columnFlexSecondary, Min: columnMinimumReadable},
		{Title: "SUSPEND", Width: columnWidthFlag},
		{Title: "ACTIVE", Width: columnWidthCompact},
		{Title: "LAST-SCHEDULE", Flex: columnFlexModest, Min: columnMinimumReadable},
		{Title: "AGE", Width: columnWidthCompact},
	},
	resourceTypeHPAs: {
		{Title: "NAME", Flex: columnFlexMedium, Min: columnMinimumWide},
		{Title: "REFERENCE", Flex: columnFlexSecondary, Min: columnMinimumWide},
		{Title: "TARGETS", Flex: columnFlexMedium, Min: columnMinimumReadable},
		{Title: "MIN", Width: columnWidthNarrow},
		{Title: "MAX", Width: columnWidthNarrow},
		{Title: "REPLICAS", Width: columnWidthStandard},
		{Title: "AGE", Width: columnWidthCompact},
	},
	resourceTypeSecrets: {
		{Title: "NAME", Flex: columnFlexPrimary, Min: columnMinimumName},
		{Title: "TYPE", Flex: columnFlexSecondary, Min: columnMinimumReadable},
		{Title: "DATA", Width: columnWidthCompact},
		{Title: "AGE", Width: columnWidthCompact},
	},
	resourceTypeReplicaSets: {
		{Title: "NAME", Flex: columnFlexFull, Min: columnMinimumName},
		{Title: "DESIRED", Width: columnWidthStandard},
		{Title: "CURRENT", Width: columnWidthStandard},
		{Title: "READY", Width: columnWidthCompact},
		{Title: "AGE", Width: columnWidthCompact},
	},
	resourceTypeRBAC: {
		{Title: "KIND", Width: columnWidthKind},
		{Title: "NAME", Flex: columnFlexPrimary, Min: columnMinimumName},
		{Title: "COUNT", Width: columnWidthCompact},
		{Title: "SCOPE", Width: columnWidthMedium},
		{Title: "AGE", Width: columnWidthCompact},
	},
}

var resourceColSpecsWide = map[string][]colSpec{
	resourceTypePods: {
		{Title: "NAME", Flex: columnFlexHalf, Min: columnMinimumCompact},
		{Title: "STATUS", Width: columnWidthMedium},
		{Title: "READY", Width: columnWidthCompact},
		{Title: "RESTARTS", Width: columnWidthStandard},
		{Title: "AGE", Width: columnWidthNarrow},
		{Title: "IP", Width: columnWidthExtraWide},
		{Title: "NODE", Flex: columnFlexHalf, Min: columnMinimumCompact},
	},
	resourceTypeDeployments: {
		{Title: "NAME", Flex: columnFlexMedium, Min: columnMinimumWide},
		{Title: "READY", Width: columnWidthStandard},
		{Title: "UP-TO-DATE", Width: columnWidthMedium},
		{Title: "AVAILABLE", Width: columnWidthMedium},
		{Title: "AGE", Width: columnWidthCompact},
		{Title: "CONTAINERS", Flex: columnFlexModest, Min: columnMinimumStandard},
		{Title: "IMAGES", Flex: columnFlexSecondary, Min: columnMinimumExpanded},
		{Title: "SELECTOR", Flex: columnFlexSmall, Min: columnMinimumStandard},
	},
	resourceTypeServices: {
		{Title: "NAME", Flex: columnFlexStrong, Min: columnMinimumWide},
		{Title: "TYPE", Width: columnWidthMedium},
		{Title: "CLUSTER-IP", Width: columnWidthExtraWide},
		{Title: "EXTERNAL-IP", Flex: columnFlexModest, Min: columnMinimumReadable},
		{Title: "PORTS", Flex: columnFlexQuarter, Min: columnMinimumReadable},
		{Title: "AGE", Width: columnWidthCompact},
		{Title: "SELECTOR", Flex: columnFlexSmall, Min: columnMinimumStandard},
	},
	resourceTypeNodes: {
		{Title: "NAME", Flex: columnFlexStrong, Min: columnMinimumWide},
		{Title: "STATUS", Width: columnWidthStandard},
		{Title: "ROLES", Flex: columnFlexSmall, Min: columnMinimumStandard},
		{Title: "VERSION", Width: columnWidthWide},
		{Title: "AGE", Width: columnWidthCompact},
		{Title: "INTERNAL-IP", Width: columnWidthExtraWide},
		{Title: "OS-IMAGE", Flex: columnFlexQuarter, Min: columnMinimumReadable},
		{Title: "KERNEL", Flex: columnFlexMinimal, Min: columnMinimumCompact},
		{Title: "RUNTIME", Flex: columnFlexMinimal, Min: columnMinimumCompact},
	},
}

func selectColSpecs(resourceType string, wide bool) ([]colSpec, bool) {
	if wide {
		if specs, ok := resourceColSpecsWide[resourceType]; ok {
			return specs, true
		}
	}
	specs, ok := resourceColSpecs[resourceType]
	return specs, ok
}

func buildResourceTable(width int, specs []colSpec) table.Model {
	resourceTable := table.New(
		table.WithColumns(computeColumns(width, specs)),
		table.WithFocused(true),
		table.WithHeight(tableHeightHint),
		table.WithWidth(width),
	)
	resourceTable.SetStyles(browserTableStyles())
	return resourceTable
}

func computeColumns(tableWidth int, specs []colSpec) []table.Column {
	available := tableWidth - tableCellPadding*len(specs)
	if available < len(specs) {
		available = len(specs)
	}

	totals := measureColumnSpecs(specs)
	columns := initializeColumns(specs)
	if totals.fixedWidth > available {
		shrinkFixedToFit(columns, specs, available)
		return columns
	}
	allocateFlexColumns(columns, specs, available-totals.fixedWidth, totals)
	return columns
}

type columnSpecTotals struct {
	fixedWidth  int
	flexWeight  int
	flexMinimum int
	flexCount   int
}

func measureColumnSpecs(specs []colSpec) columnSpecTotals {
	var totals columnSpecTotals
	for _, spec := range specs {
		if spec.Width > 0 {
			totals.fixedWidth += spec.Width
			continue
		}
		totals.flexWeight += spec.Flex
		totals.flexMinimum += spec.Min
		totals.flexCount++
	}
	return totals
}

func initializeColumns(specs []colSpec) []table.Column {
	columns := make([]table.Column, len(specs))
	for index, spec := range specs {
		columns[index] = table.Column{Title: spec.Title, Width: max(0, spec.Width)}
	}
	return columns
}

func allocateFlexColumns(columns []table.Column, specs []colSpec, available int, totals columnSpecTotals) {
	applyMinimum := available >= totals.flexMinimum
	allocated, flexSeen := 0, 0
	for index, spec := range specs {
		if spec.Width != 0 {
			continue
		}
		flexSeen++
		if flexSeen == totals.flexCount {
			columns[index].Width = max(0, available-allocated)
			continue
		}
		minimum := 0
		if applyMinimum {
			minimum = spec.Min
		}
		width := flexWidth(available, spec.Flex, totals.flexWeight, minimum)
		allocated += width
		columns[index].Width = width
	}
}

func shrinkFixedToFit(cols []table.Column, specs []colSpec, available int) {
	total := 0
	for index := range cols {
		if specs[index].Width == 0 {
			cols[index].Width = 0
			continue
		}
		total += cols[index].Width
	}
	if total == 0 {
		return
	}
	used := 0
	lastIdx := -1
	for index := range cols {
		if specs[index].Width == 0 {
			continue
		}
		width := cols[index].Width * available / total
		if width < 1 {
			width = 1
		}
		cols[index].Width = width
		used += width
		lastIdx = index
	}
	if lastIdx >= 0 {
		cols[lastIdx].Width += available - used
		if cols[lastIdx].Width < 1 {
			cols[lastIdx].Width = 1
		}
	}
}

func flexWidth(remaining, weight, totalWeight, minimum int) int {
	width := 0
	if totalWeight > 0 {
		width = remaining * weight / totalWeight
	}
	if width < minimum {
		width = minimum
	}
	return width
}
