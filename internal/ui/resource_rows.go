package ui

import (
	"strconv"
	"strings"

	"charm.land/bubbles/v2/table"
)

func podRows(model *BrowserModel) []table.Row {
	rows := make([]table.Row, 0, len(model.pods))
	for _, item := range model.pods {
		rows = append(rows, table.Row{
			model.displayIdentity(resourceIdentity{Kind: resourceKindPod, Namespace: item.Namespace, Name: item.Name}), item.Status, item.Ready, strconv.Itoa(item.Restarts), item.Age, item.Node,
		})
	}
	return rows
}

func deploymentRows(model *BrowserModel) []table.Row {
	rows := make([]table.Row, 0, len(model.deployments))
	for _, item := range model.deployments {
		rows = append(rows, table.Row{
			model.displayIdentity(resourceIdentity{Kind: resourceKindDeployment, Namespace: item.Namespace, Name: item.Name}), item.Ready, strconv.Itoa(item.UpToDate), strconv.Itoa(item.Available), item.Age,
		})
	}
	return rows
}

func serviceRows(model *BrowserModel) []table.Row {
	rows := make([]table.Row, 0, len(model.services))
	for _, item := range model.services {
		rows = append(rows, table.Row{
			model.displayIdentity(resourceIdentity{Kind: resourceKindService, Namespace: item.Namespace, Name: item.Name}), item.Type, item.ClusterIP, item.Ports, item.Age,
		})
	}
	return rows
}

func statefulSetRows(model *BrowserModel) []table.Row {
	rows := make([]table.Row, 0, len(model.statefulsets))
	for _, item := range model.statefulsets {
		rows = append(rows, table.Row{
			model.displayIdentity(resourceIdentity{Kind: resourceKindStatefulSet, Namespace: item.Namespace, Name: item.Name}), item.Ready, strconv.Itoa(item.Replicas), item.Age,
		})
	}
	return rows
}

func daemonSetRows(model *BrowserModel) []table.Row {
	rows := make([]table.Row, 0, len(model.daemonsets))
	for _, item := range model.daemonsets {
		rows = append(rows, table.Row{
			model.displayIdentity(resourceIdentity{Kind: resourceKindDaemonSet, Namespace: item.Namespace, Name: item.Name}),
			strconv.Itoa(item.Desired),
			strconv.Itoa(item.Current),
			strconv.Itoa(item.Ready),
			strconv.Itoa(item.Available),
			item.Age,
		})
	}
	return rows
}

func configMapRows(model *BrowserModel) []table.Row {
	rows := make([]table.Row, 0, len(model.configmaps))
	for _, item := range model.configmaps {
		rows = append(rows, table.Row{
			model.displayIdentity(resourceIdentity{Kind: resourceKindConfigMap, Namespace: item.Namespace, Name: item.Name}), strconv.Itoa(item.Data), item.Age,
		})
	}
	return rows
}

func nodeRows(model *BrowserModel) []table.Row {
	rows := make([]table.Row, 0, len(model.nodes))
	for _, item := range model.nodes {
		rows = append(rows, table.Row{
			model.displayIdentity(resourceIdentity{Kind: resourceKindNode, Name: item.Name}), item.Status, item.Roles, item.Version, item.Age,
		})
	}
	return rows
}

func jobRows(model *BrowserModel) []table.Row {
	rows := make([]table.Row, 0, len(model.jobs))
	for _, item := range model.jobs {
		rows = append(rows, table.Row{
			model.displayIdentity(resourceIdentity{Kind: resourceKindJob, Namespace: item.Namespace, Name: item.Name}), item.Completions, item.Duration, item.Status, item.Age,
		})
	}
	return rows
}

func ingressRows(model *BrowserModel) []table.Row {
	rows := make([]table.Row, 0, len(model.ingresses))
	for _, item := range model.ingresses {
		rows = append(rows, table.Row{
			model.displayIdentity(resourceIdentity{Kind: resourceKindIngress, Namespace: item.Namespace, Name: item.Name}), item.Class, item.Hosts, item.Address, item.Ports, item.Age,
		})
	}
	return rows
}

func networkPolicyRows(model *BrowserModel) []table.Row {
	rows := make([]table.Row, 0, len(model.networkpolicies))
	for _, item := range model.networkpolicies {
		rows = append(rows, table.Row{
			model.displayIdentity(resourceIdentity{Kind: resourceKindNetworkPolicy, Namespace: item.Namespace, Name: item.Name}),
			formatLabelSelector(item.PodSelector),
			strings.Join(item.PolicyTypes, ","),
			item.Age,
		})
	}
	return rows
}

func pvcRows(model *BrowserModel) []table.Row {
	rows := make([]table.Row, 0, len(model.pvcs))
	for _, item := range model.pvcs {
		rows = append(rows, table.Row{
			model.displayIdentity(resourceIdentity{Kind: resourceKindPVC, Namespace: item.Namespace, Name: item.Name}),
			item.Status,
			item.Volume,
			item.Capacity,
			strings.Join(formatPVCAccessModes(item.AccessModes), ","),
			item.StorageClass,
			item.Age,
		})
	}
	return rows
}

func cronJobRows(model *BrowserModel) []table.Row {
	rows := make([]table.Row, 0, len(model.cronjobs))
	for _, item := range model.cronjobs {
		rows = append(rows, table.Row{
			model.displayIdentity(resourceIdentity{Kind: resourceKindCronJob, Namespace: item.Namespace, Name: item.Name}),
			item.Schedule,
			formatBoolColumn(item.Suspend),
			strconv.Itoa(item.Active),
			item.LastSchedule,
			item.Age,
		})
	}
	return rows
}

func hpaRows(model *BrowserModel) []table.Row {
	rows := make([]table.Row, 0, len(model.hpas))
	for _, item := range model.hpas {
		targets := make([]string, 0, len(item.Targets))
		for _, target := range item.Targets {
			targets = append(targets, target.String())
		}
		rows = append(rows, table.Row{
			model.displayIdentity(resourceIdentity{Kind: resourceKindHPA, Namespace: item.Namespace, Name: item.Name}),
			item.Reference.String(),
			strings.Join(targets, ","),
			strconv.Itoa(item.MinReplicas),
			strconv.Itoa(item.MaxReplicas),
			strconv.Itoa(item.Replicas),
			item.Age,
		})
	}
	return rows
}

func secretRows(model *BrowserModel) []table.Row {
	rows := make([]table.Row, 0, len(model.secrets))
	for _, item := range model.secrets {
		rows = append(rows, table.Row{
			model.displayIdentity(resourceIdentity{Kind: resourceKindSecret, Namespace: item.Namespace, Name: item.Name}), item.Type, strconv.Itoa(item.Data), item.Age,
		})
	}
	return rows
}

func replicaSetRows(model *BrowserModel) []table.Row {
	rows := make([]table.Row, 0, len(model.replicasets))
	for _, item := range model.replicasets {
		rows = append(rows, table.Row{
			model.displayIdentity(resourceIdentity{Kind: resourceKindReplicaSet, Namespace: item.Namespace, Name: item.Name}),
			strconv.Itoa(item.Desired),
			strconv.Itoa(item.Current),
			strconv.Itoa(item.Ready),
			item.Age,
		})
	}
	return rows
}

func rbacRows(model *BrowserModel) []table.Row {
	rows := make([]table.Row, 0, len(model.rbac))
	for _, item := range model.rbac {
		rows = append(rows, table.Row{
			item.Kind, model.displayIdentity(resourceIdentity{Kind: strings.ToLower(item.Kind), Namespace: item.Namespace, Name: item.Name}), strconv.Itoa(item.Count), item.Scope, item.Age,
		})
	}
	return rows
}

func podRowsWide(model *BrowserModel) []table.Row {
	rows := make([]table.Row, 0, len(model.pods))
	for _, item := range model.pods {
		rows = append(rows, table.Row{
			model.displayIdentity(resourceIdentity{Kind: resourceKindPod, Namespace: item.Namespace, Name: item.Name}), item.Status, item.Ready, strconv.Itoa(item.Restarts), item.Age, item.IP, item.Node,
		})
	}
	return rows
}

func deploymentRowsWide(model *BrowserModel) []table.Row {
	rows := make([]table.Row, 0, len(model.deployments))
	for _, item := range model.deployments {
		rows = append(rows, table.Row{
			model.displayIdentity(resourceIdentity{Kind: resourceKindDeployment, Namespace: item.Namespace, Name: item.Name}),
			item.Ready,
			strconv.Itoa(item.UpToDate),
			strconv.Itoa(item.Available),
			item.Age,
			strings.Join(item.Containers, ","),
			strings.Join(item.Images, ","),
			item.Selector,
		})
	}
	return rows
}

func serviceRowsWide(model *BrowserModel) []table.Row {
	rows := make([]table.Row, 0, len(model.services))
	for _, item := range model.services {
		rows = append(rows, table.Row{
			model.displayIdentity(resourceIdentity{Kind: resourceKindService, Namespace: item.Namespace, Name: item.Name}), item.Type, item.ClusterIP, item.ExternalIP, item.Ports, item.Age, item.Selector,
		})
	}
	return rows
}

func nodeRowsWide(model *BrowserModel) []table.Row {
	rows := make([]table.Row, 0, len(model.nodes))
	for _, item := range model.nodes {
		rows = append(rows, table.Row{
			model.displayIdentity(resourceIdentity{Kind: resourceKindNode, Name: item.Name}),
			item.Status,
			item.Roles,
			item.Version,
			item.Age,
			item.InternalIP,
			item.OSImage,
			item.Kernel,
			item.Runtime,
		})
	}
	return rows
}
