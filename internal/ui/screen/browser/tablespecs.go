package browser

import (
	"charm.land/bubbles/v2/table"
	"github.com/HediAbed/opsmate/internal/ui/component"
)

type colSpec = component.ColumnSpec

const (
	tableHeaderChromeRows = component.TableHeaderRows

	columnWidthNarrow    = component.ColumnWidthNarrow
	columnWidthCompact   = component.ColumnWidthCompact
	columnWidthFlag      = component.ColumnWidthFlag
	columnWidthStandard  = component.ColumnWidthStandard
	columnWidthMedium    = component.ColumnWidthMedium
	columnWidthWide      = component.ColumnWidthWide
	columnWidthExtraWide = component.ColumnWidthExtraWide
	columnWidthKind      = component.ColumnWidthKind

	columnFlexMinimal   = component.ColumnFlexMinimal
	columnFlexSmall     = component.ColumnFlexSmall
	columnFlexModest    = component.ColumnFlexModest
	columnFlexQuarter   = component.ColumnFlexQuarter
	columnFlexSecondary = component.ColumnFlexSecondary
	columnFlexMedium    = component.ColumnFlexMedium
	columnFlexStrong    = component.ColumnFlexStrong
	columnFlexHalf      = component.ColumnFlexHalf
	columnFlexPrimary   = component.ColumnFlexPrimary
	columnFlexFull      = component.ColumnFlexFull

	columnMinimumCompact  = component.ColumnMinimumCompact
	columnMinimumStandard = component.ColumnMinimumStandard
	columnMinimumReadable = component.ColumnMinimumReadable
	columnMinimumExpanded = component.ColumnMinimumExpanded
	columnMinimumWide     = component.ColumnMinimumWide
	columnMinimumName     = component.ColumnMinimumName
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
	return component.NewTable(width, specs)
}

func computeColumns(tableWidth int, specs []colSpec) []table.Column {
	return component.Columns(tableWidth, specs)
}
