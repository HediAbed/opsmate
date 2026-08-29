package analysis

import (
	"slices"
	"strconv"
	"strings"

	"github.com/HediAbed/opsmate/internal/cluster"
)

func attribute(name string, value int) screenContextAttribute {
	return screenContextAttribute{Name: name, Value: strconv.Itoa(value)}
}

func stringAttribute(name, value string) screenContextAttribute {
	return screenContextAttribute{Name: name, Value: boundedContextValue(value, maxScreenFieldRunes)}
}

func podContextRow(item cluster.Pod) screenContextRow {
	return screenContextRow{Kind: "pod", Name: boundedContextValue(item.Name, maxScreenFieldRunes), Attributes: []screenContextAttribute{
		stringAttribute("status", item.Status), stringAttribute("ready", item.Ready),
		attribute("restarts", item.Restarts), stringAttribute("age", item.Age),
		stringAttribute("cpu", item.CPU), stringAttribute("memory", item.Memory), stringAttribute("node", item.Node),
	}}
}

func deploymentContextRow(item cluster.Deployment) screenContextRow {
	return screenContextRow{Kind: "deployment", Name: boundedContextValue(item.Name, maxScreenFieldRunes), Attributes: []screenContextAttribute{
		stringAttribute("ready", item.Ready), attribute("up_to_date", item.UpToDate),
		attribute("available", item.Available), stringAttribute("age", item.Age),
	}}
}

func eventContextRow(item cluster.Event) screenContextRow {
	return screenContextRow{Kind: "event", Name: boundedContextValue(item.Object, maxScreenFieldRunes), Attributes: []screenContextAttribute{
		stringAttribute("type", item.Type), stringAttribute("reason", item.Reason),
		stringAttribute("message", item.Message), attribute("count", item.Count),
	}}
}

func serviceContextRow(item cluster.Service) screenContextRow {
	return resourceRow("service", item.Name,
		stringAttribute("type", item.Type), stringAttribute("cluster_ip", item.ClusterIP),
		stringAttribute("ports", item.Ports), stringAttribute("age", item.Age))
}

func statefulSetContextRow(item cluster.StatefulSet) screenContextRow {
	return resourceRow("statefulset", item.Name,
		stringAttribute("ready", item.Ready), attribute("replicas", item.Replicas), stringAttribute("age", item.Age))
}

func daemonSetContextRow(item cluster.DaemonSet) screenContextRow {
	return resourceRow("daemonset", item.Name,
		attribute("desired", item.Desired), attribute("current", item.Current), attribute("ready", item.Ready),
		attribute("available", item.Available), stringAttribute("age", item.Age))
}

func configMapContextRow(item cluster.ConfigMap) screenContextRow {
	return resourceRow("configmap", item.Name, attribute("data_keys", item.Data), stringAttribute("age", item.Age))
}

func nodeContextRow(item cluster.Node) screenContextRow {
	return resourceRow("node", item.Name,
		stringAttribute("status", item.Status), stringAttribute("roles", item.Roles),
		stringAttribute("version", item.Version), stringAttribute("age", item.Age))
}

func jobContextRow(item cluster.Job) screenContextRow {
	return resourceRow("job", item.Name,
		stringAttribute("completions", item.Completions), stringAttribute("duration", item.Duration),
		stringAttribute("status", item.Status), stringAttribute("age", item.Age))
}

func ingressContextRow(item cluster.Ingress) screenContextRow {
	return resourceRow("ingress", item.Name,
		stringAttribute("class", item.Class), stringAttribute("hosts", item.Hosts),
		stringAttribute("address", item.Address), stringAttribute("ports", item.Ports), stringAttribute("age", item.Age))
}

func networkPolicyContextRow(item cluster.NetworkPolicy) screenContextRow {
	return resourceRow("networkpolicy", item.Name,
		stringAttribute("selector", formatLabelSelector(item.PodSelector)),
		stringAttribute("policy_types", strings.Join(item.PolicyTypes, ",")), stringAttribute("age", item.Age))
}

func formatLabelSelector(labels map[string]string) string {
	keys := make([]string, 0, len(labels))
	for key := range labels {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, key+"="+labels[key])
	}
	return strings.Join(parts, ",")
}

func pvcContextRow(item cluster.PersistentVolumeClaim) screenContextRow {
	return resourceRow("pvc", item.Name,
		stringAttribute("status", item.Status), stringAttribute("capacity", item.Capacity),
		stringAttribute("access_modes", strings.Join(item.AccessModes, ",")),
		stringAttribute("storage_class", item.StorageClass), stringAttribute("age", item.Age))
}

func cronJobContextRow(item cluster.CronJob) screenContextRow {
	return resourceRow("cronjob", item.Name,
		stringAttribute("schedule", item.Schedule), stringAttribute("suspended", strconv.FormatBool(item.Suspend)),
		attribute("active", item.Active), stringAttribute("last_schedule", item.LastSchedule), stringAttribute("age", item.Age))
}

func hpaContextRow(item cluster.HPA) screenContextRow {
	targets := make([]string, 0, len(item.Targets))
	for _, target := range item.Targets {
		targets = append(targets, target.String())
	}
	return resourceRow("hpa", item.Name,
		stringAttribute("reference", item.Reference.String()), stringAttribute("targets", strings.Join(targets, ",")),
		attribute("min_replicas", item.MinReplicas), attribute("max_replicas", item.MaxReplicas),
		attribute("replicas", item.Replicas), stringAttribute("age", item.Age))
}

func secretContextRow(item cluster.Secret) screenContextRow {
	return resourceRow("secret", item.Name,
		stringAttribute("type", item.Type), attribute("data_keys", item.Data), stringAttribute("age", item.Age))
}

func replicaSetContextRow(item cluster.ReplicaSet) screenContextRow {
	return resourceRow("replicaset", item.Name,
		attribute("desired", item.Desired), attribute("current", item.Current),
		attribute("ready", item.Ready), stringAttribute("age", item.Age))
}

func rbacContextRow(item cluster.RBAC) screenContextRow {
	return resourceRow("rbac", item.Name,
		stringAttribute("rbac_kind", item.Kind), stringAttribute("scope", item.Scope),
		attribute("entries", item.Count), stringAttribute("age", item.Age))
}

func resourceRow(kind, name string, attributes ...screenContextAttribute) screenContextRow {
	return screenContextRow{Kind: kind, Name: boundedContextValue(name, maxScreenFieldRunes), Attributes: attributes}
}
