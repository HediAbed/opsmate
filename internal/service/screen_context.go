package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"
)

const (
	maxDashboardEvents      = 10
	maxDashboardResources   = 50
	maxBrowserResources     = 100
	maxScreenDetailRunes    = 3000
	maxScreenFieldRunes     = 1024
	maxScreenContextRunes   = 12000
	maxLogContextLines      = 50
	contextTruncationMarker = "..."
	untrustedContextStart   = "BEGIN UNTRUSTED CLUSTER SNAPSHOT\n"
	untrustedContextEnd     = "END UNTRUSTED CLUSTER SNAPSHOT"
)

var ErrUnsupportedBrowserResource = errors.New("unsupported browser resource")

type ScreenContextError struct {
	Resource BrowserResourceKind
	Err      error
}

func (e *ScreenContextError) Error() string {
	if e.Err == nil {
		return "screen context: unknown error"
	}
	if e.Resource == "" {
		return "screen context: " + e.Err.Error()
	}
	return fmt.Sprintf("screen context %q: %v", e.Resource, e.Err)
}

func (e *ScreenContextError) Unwrap() error {
	return e.Err
}

type BrowserResourceKind string

const (
	BrowserPods            BrowserResourceKind = "pods"
	BrowserDeployments     BrowserResourceKind = "deployments"
	BrowserServices        BrowserResourceKind = "services"
	BrowserStatefulSets    BrowserResourceKind = "statefulsets"
	BrowserDaemonSets      BrowserResourceKind = "daemonsets"
	BrowserConfigMaps      BrowserResourceKind = "configmaps"
	BrowserNodes           BrowserResourceKind = "nodes"
	BrowserJobs            BrowserResourceKind = "jobs"
	BrowserIngresses       BrowserResourceKind = "ingresses"
	BrowserNetworkPolicies BrowserResourceKind = "networkpolicies"
	BrowserPVCs            BrowserResourceKind = "pvcs"
	BrowserCronJobs        BrowserResourceKind = "cronjobs"
	BrowserHPAs            BrowserResourceKind = "hpas"
	BrowserSecrets         BrowserResourceKind = "secrets"
	BrowserReplicaSets     BrowserResourceKind = "replicasets"
	BrowserRBAC            BrowserResourceKind = "rbac"
)

func ParseBrowserResourceKind(value string) (BrowserResourceKind, error) {
	resource := BrowserResourceKind(value)
	if isPrimaryBrowserResource(resource) || isSecondaryBrowserResource(resource) {
		return resource, nil
	}
	return "", &ScreenContextError{Resource: resource, Err: ErrUnsupportedBrowserResource}
}

func isPrimaryBrowserResource(resource BrowserResourceKind) bool {
	switch resource {
	case BrowserPods, BrowserDeployments, BrowserServices, BrowserStatefulSets,
		BrowserDaemonSets, BrowserConfigMaps, BrowserNodes, BrowserJobs:
		return true
	case BrowserIngresses, BrowserNetworkPolicies, BrowserPVCs, BrowserCronJobs,
		BrowserHPAs, BrowserSecrets, BrowserReplicaSets, BrowserRBAC:
		return false
	}
	return false
}

func isSecondaryBrowserResource(resource BrowserResourceKind) bool {
	switch resource {
	case BrowserIngresses, BrowserNetworkPolicies, BrowserPVCs, BrowserCronJobs,
		BrowserHPAs, BrowserSecrets, BrowserReplicaSets, BrowserRBAC:
		return true
	case BrowserPods, BrowserDeployments, BrowserServices, BrowserStatefulSets,
		BrowserDaemonSets, BrowserConfigMaps, BrowserNodes, BrowserJobs:
		return false
	}
	return false
}

type DashboardContextInput struct {
	Namespace   string
	Pods        []Pod
	Deployments []Deployment
	Events      []Event
}

type BrowserContextSelection struct {
	Namespace         string
	Resource          BrowserResourceKind
	SelectedName      string
	SelectedNamespace string
	DetailContent     string
}

type BrowserSnapshot struct {
	Pods            []Pod
	Deployments     []Deployment
	Services        []Service
	StatefulSets    []StatefulSet
	DaemonSets      []DaemonSet
	ConfigMaps      []ConfigMap
	Nodes           []Node
	Jobs            []Job
	Ingresses       []Ingress
	NetworkPolicies []NetworkPolicy
	PVCs            []PersistentVolumeClaim
	CronJobs        []CronJob
	HPAs            []HPA
	Secrets         []Secret
	ReplicaSets     []ReplicaSet
	RBAC            []RBAC
}

type BrowserContextInput struct {
	selection BrowserContextSelection
	snapshot  BrowserSnapshot
}

func NewBrowserContextInput(
	selection BrowserContextSelection,
	snapshot BrowserSnapshot,
) (BrowserContextInput, error) {
	if !isPrimaryBrowserResource(selection.Resource) && !isSecondaryBrowserResource(selection.Resource) {
		return BrowserContextInput{}, &ScreenContextError{
			Resource: selection.Resource,
			Err:      ErrUnsupportedBrowserResource,
		}
	}
	return BrowserContextInput{selection: selection, snapshot: snapshot}, nil
}

type screenContextHeader struct {
	Screen            string `json:"screen"`
	Namespace         string `json:"namespace"`
	Resource          string `json:"resource,omitempty"`
	SelectedName      string `json:"selected_name,omitempty"`
	SelectedNamespace string `json:"selected_namespace,omitempty"`
	Filter            string `json:"filter,omitempty"`
	Total             int    `json:"total,omitempty"`
	Included          int    `json:"included,omitempty"`
}

type screenContextAttribute struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type screenContextRow struct {
	Kind       string                   `json:"kind"`
	Name       string                   `json:"name,omitempty"`
	Namespace  string                   `json:"namespace,omitempty"`
	Attributes []screenContextAttribute `json:"attributes,omitempty"`
}

type screenContextRecord interface {
	screenContextHeader | screenContextRow
}

type boundedContextWriter struct {
	builder   strings.Builder
	remaining int
	truncated bool
}

func newBoundedContextWriter() *boundedContextWriter {
	boundaryRunes := len([]rune(untrustedContextStart + "\n" + untrustedContextEnd))
	reservedMarkerRunes := len([]rune(contextTruncationMarker + "\n"))
	return &boundedContextWriter{remaining: maxScreenContextRunes - boundaryRunes - reservedMarkerRunes}
}

func (w *boundedContextWriter) addRecord(encoded []byte) bool {
	requiredRunes := utf8.RuneCount(encoded) + 1
	if requiredRunes > w.remaining {
		w.truncated = true
		return false
	}
	w.builder.Write(encoded)
	w.builder.WriteByte('\n')
	w.remaining -= requiredRunes
	return true
}

func (w *boundedContextWriter) Full() bool {
	return w.truncated || w.remaining == 0
}

func (w *boundedContextWriter) String() string {
	const boundaryNewlineCount = 2
	var result strings.Builder
	result.Grow(len(untrustedContextStart) + w.builder.Len() + len(untrustedContextEnd) + len(contextTruncationMarker) + boundaryNewlineCount)
	result.WriteString(untrustedContextStart)
	result.WriteString(w.builder.String())
	if w.truncated {
		result.WriteString(contextTruncationMarker)
		result.WriteByte('\n')
	}
	result.WriteString(untrustedContextEnd)
	return result.String()
}

func writeScreenContextRecord[Record screenContextRecord](writer *boundedContextWriter, record Record) {
	encoded, _ := json.Marshal(record)
	writer.addRecord(encoded)
}

func BuildDashboardContext(input DashboardContextInput) string {
	document := newBoundedContextWriter()
	writeScreenContextRecord(document, screenContextHeader{
		Screen: "namespace-dashboard", Namespace: boundedContextValue(input.Namespace, maxScreenFieldRunes),
	})
	statusCounts := dashboardStatusAttributes(input.Pods)
	writeScreenContextRecord(document, screenContextRow{Kind: "pod-summary", Attributes: statusCounts})
	writeDashboardResources(document, input)
	return document.String()
}

func dashboardStatusAttributes(pods []Pod) []screenContextAttribute {
	running, pending, failed := 0, 0, 0
	for _, pod := range pods {
		switch pod.Status {
		case "Running":
			running++
		case "Pending":
			pending++
		case "Failed", "Error", "CrashLoopBackOff", "ImagePullBackOff":
			failed++
		}
	}
	return []screenContextAttribute{
		attribute("total", len(pods)),
		attribute("running", running),
		attribute("pending", pending),
		attribute("failed", failed),
	}
}

func writeDashboardResources(document *boundedContextWriter, input DashboardContextInput) {
	remainingResources := maxDashboardResources
	podCount := min(len(input.Pods), remainingResources)
	for _, pod := range input.Pods[:podCount] {
		if document.Full() {
			return
		}
		writeScreenContextRecord(document, podContextRow(pod))
	}
	remainingResources -= podCount
	deploymentCount := min(len(input.Deployments), remainingResources)
	for _, deployment := range input.Deployments[:deploymentCount] {
		if document.Full() {
			return
		}
		writeScreenContextRecord(document, deploymentContextRow(deployment))
	}
	eventCount := min(len(input.Events), maxDashboardEvents)
	for _, event := range input.Events[:eventCount] {
		if document.Full() {
			return
		}
		writeScreenContextRecord(document, eventContextRow(event))
	}
}

func BuildBrowserContext(input BrowserContextInput) (string, error) {
	selection := input.selection
	if !isPrimaryBrowserResource(selection.Resource) && !isSecondaryBrowserResource(selection.Resource) {
		return "", &ScreenContextError{Resource: selection.Resource, Err: ErrUnsupportedBrowserResource}
	}
	document := newBoundedContextWriter()
	writeScreenContextRecord(document, screenContextHeader{
		Screen:            "kubernetes-browser",
		Namespace:         boundedContextValue(selection.Namespace, maxScreenFieldRunes),
		Resource:          string(selection.Resource),
		SelectedName:      boundedContextValue(selection.SelectedName, maxScreenFieldRunes),
		SelectedNamespace: boundedContextValue(selection.SelectedNamespace, maxScreenFieldRunes),
	})
	writeBrowserResources(document, input)
	if selection.DetailContent != "" && selection.Resource != BrowserSecrets && !document.Full() {
		writeScreenContextRecord(document, screenContextRow{
			Kind: "detail",
			Attributes: []screenContextAttribute{{
				Name: "content", Value: boundedContextValue(selection.DetailContent, maxScreenDetailRunes),
			}},
		})
	}
	return document.String(), nil
}

func writeBrowserResources(document *boundedContextWriter, input BrowserContextInput) {
	writePrimaryBrowserResources(document, input)
	writeSecondaryBrowserResources(document, input)
}

func writePrimaryBrowserResources(document *boundedContextWriter, input BrowserContextInput) {
	switch input.selection.Resource {
	case BrowserPods:
		writeRows(document, input.snapshot.Pods, podContextRow)
	case BrowserDeployments:
		writeRows(document, input.snapshot.Deployments, deploymentContextRow)
	case BrowserServices:
		writeRows(document, input.snapshot.Services, serviceContextRow)
	case BrowserStatefulSets:
		writeRows(document, input.snapshot.StatefulSets, statefulSetContextRow)
	case BrowserDaemonSets:
		writeRows(document, input.snapshot.DaemonSets, daemonSetContextRow)
	case BrowserConfigMaps:
		writeRows(document, input.snapshot.ConfigMaps, configMapContextRow)
	case BrowserNodes:
		writeRows(document, input.snapshot.Nodes, nodeContextRow)
	case BrowserJobs:
		writeRows(document, input.snapshot.Jobs, jobContextRow)
	case BrowserIngresses, BrowserNetworkPolicies, BrowserPVCs, BrowserCronJobs,
		BrowserHPAs, BrowserSecrets, BrowserReplicaSets, BrowserRBAC:
		return
	}
}

func writeSecondaryBrowserResources(document *boundedContextWriter, input BrowserContextInput) {
	switch input.selection.Resource {
	case BrowserIngresses:
		writeRows(document, input.snapshot.Ingresses, ingressContextRow)
	case BrowserNetworkPolicies:
		writeRows(document, input.snapshot.NetworkPolicies, networkPolicyContextRow)
	case BrowserPVCs:
		writeRows(document, input.snapshot.PVCs, pvcContextRow)
	case BrowserCronJobs:
		writeRows(document, input.snapshot.CronJobs, cronJobContextRow)
	case BrowserHPAs:
		writeRows(document, input.snapshot.HPAs, hpaContextRow)
	case BrowserSecrets:
		writeRows(document, input.snapshot.Secrets, secretContextRow)
	case BrowserReplicaSets:
		writeRows(document, input.snapshot.ReplicaSets, replicaSetContextRow)
	case BrowserRBAC:
		writeRows(document, input.snapshot.RBAC, rbacContextRow)
	case BrowserPods, BrowserDeployments, BrowserServices, BrowserStatefulSets,
		BrowserDaemonSets, BrowserConfigMaps, BrowserNodes, BrowserJobs:
		return
	}
}

type browserContextResource interface {
	Pod | Deployment | Service | StatefulSet | DaemonSet | ConfigMap | Node | Job | Ingress |
		NetworkPolicy | PersistentVolumeClaim | CronJob | HPA | Secret | ReplicaSet | RBAC
}

func writeRows[Resource browserContextResource](
	document *boundedContextWriter,
	resources []Resource,
	project func(Resource) screenContextRow,
) {
	resourceCount := min(len(resources), maxBrowserResources)
	for _, resource := range resources[:resourceCount] {
		if document.Full() {
			return
		}
		writeScreenContextRecord(document, project(resource))
	}
}

func BuildLogsContext(namespace, podName string, lines []string, filter string) (string, error) {
	document := newBoundedContextWriter()
	start := max(0, len(lines)-maxLogContextLines)
	writeScreenContextRecord(document, screenContextHeader{
		Screen:       "log-viewer",
		Namespace:    boundedContextValue(namespace, maxScreenFieldRunes),
		SelectedName: boundedContextValue(podName, maxScreenFieldRunes),
		Filter:       boundedContextValue(filter, maxScreenFieldRunes),
		Total:        len(lines),
		Included:     len(lines) - start,
	})
	if len(lines) == 0 {
		writeScreenContextRecord(document, screenContextRow{Kind: "empty-logs"})
	}
	for _, line := range lines[start:] {
		if document.Full() {
			break
		}
		writeScreenContextRecord(document, screenContextRow{
			Kind: "log", Attributes: []screenContextAttribute{{Name: "line", Value: boundedContextValue(line, maxScreenFieldRunes)}},
		})
	}
	return document.String(), nil
}

func boundedContextValue(value string, maxRunes int) string {
	if maxRunes <= 0 {
		return ""
	}
	var bounded strings.Builder
	bounded.Grow(min(len(value), maxRunes))
	runeCount := 0
	truncated := false
	for byteIndex, character := range value {
		if runeCount == maxRunes {
			truncated = byteIndex < len(value)
			break
		}
		bounded.WriteRune(character)
		runeCount++
	}
	sanitized := stripANSI(bounded.String())
	if truncated {
		return truncateContextText(sanitized, maxRunes-len([]rune(contextTruncationMarker))) + contextTruncationMarker
	}
	return sanitized
}

func truncateContextText(text string, maxRunes int) string {
	if maxRunes <= 0 {
		return ""
	}
	var bounded strings.Builder
	bounded.Grow(min(len(text), maxRunes))
	for _, character := range text {
		if maxRunes == 0 {
			break
		}
		bounded.WriteRune(character)
		maxRunes--
	}
	return bounded.String()
}

func attribute(name string, value int) screenContextAttribute {
	return screenContextAttribute{Name: name, Value: strconv.Itoa(value)}
}

func stringAttribute(name, value string) screenContextAttribute {
	return screenContextAttribute{Name: name, Value: boundedContextValue(value, maxScreenFieldRunes)}
}

func podContextRow(item Pod) screenContextRow {
	return screenContextRow{Kind: "pod", Name: boundedContextValue(item.Name, maxScreenFieldRunes), Attributes: []screenContextAttribute{
		stringAttribute("status", item.Status), stringAttribute("ready", item.Ready),
		attribute("restarts", item.Restarts), stringAttribute("age", item.Age),
		stringAttribute("cpu", item.CPU), stringAttribute("memory", item.Memory), stringAttribute("node", item.Node),
	}}
}

func deploymentContextRow(item Deployment) screenContextRow {
	return screenContextRow{Kind: "deployment", Name: boundedContextValue(item.Name, maxScreenFieldRunes), Attributes: []screenContextAttribute{
		stringAttribute("ready", item.Ready), attribute("up_to_date", item.UpToDate),
		attribute("available", item.Available), stringAttribute("age", item.Age),
	}}
}

func eventContextRow(item Event) screenContextRow {
	return screenContextRow{Kind: "event", Name: boundedContextValue(item.Object, maxScreenFieldRunes), Attributes: []screenContextAttribute{
		stringAttribute("type", item.Type), stringAttribute("reason", item.Reason),
		stringAttribute("message", item.Message), attribute("count", item.Count),
	}}
}

func serviceContextRow(item Service) screenContextRow {
	return resourceRow("service", item.Name,
		stringAttribute("type", item.Type), stringAttribute("cluster_ip", item.ClusterIP),
		stringAttribute("ports", item.Ports), stringAttribute("age", item.Age))
}

func statefulSetContextRow(item StatefulSet) screenContextRow {
	return resourceRow("statefulset", item.Name,
		stringAttribute("ready", item.Ready), attribute("replicas", item.Replicas), stringAttribute("age", item.Age))
}

func daemonSetContextRow(item DaemonSet) screenContextRow {
	return resourceRow("daemonset", item.Name,
		attribute("desired", item.Desired), attribute("current", item.Current), attribute("ready", item.Ready),
		attribute("available", item.Available), stringAttribute("age", item.Age))
}

func configMapContextRow(item ConfigMap) screenContextRow {
	return resourceRow("configmap", item.Name, attribute("data_keys", item.Data), stringAttribute("age", item.Age))
}

func nodeContextRow(item Node) screenContextRow {
	return resourceRow("node", item.Name,
		stringAttribute("status", item.Status), stringAttribute("roles", item.Roles),
		stringAttribute("version", item.Version), stringAttribute("age", item.Age))
}

func jobContextRow(item Job) screenContextRow {
	return resourceRow("job", item.Name,
		stringAttribute("completions", item.Completions), stringAttribute("duration", item.Duration),
		stringAttribute("status", item.Status), stringAttribute("age", item.Age))
}

func ingressContextRow(item Ingress) screenContextRow {
	return resourceRow("ingress", item.Name,
		stringAttribute("class", item.Class), stringAttribute("hosts", item.Hosts),
		stringAttribute("address", item.Address), stringAttribute("ports", item.Ports), stringAttribute("age", item.Age))
}

func networkPolicyContextRow(item NetworkPolicy) screenContextRow {
	return resourceRow("networkpolicy", item.Name,
		stringAttribute("selector", formatLabelMap(item.PodSelector)),
		stringAttribute("policy_types", strings.Join(item.PolicyTypes, ",")), stringAttribute("age", item.Age))
}

func pvcContextRow(item PersistentVolumeClaim) screenContextRow {
	return resourceRow("pvc", item.Name,
		stringAttribute("status", item.Status), stringAttribute("capacity", item.Capacity),
		stringAttribute("access_modes", strings.Join(item.AccessModes, ",")),
		stringAttribute("storage_class", item.StorageClass), stringAttribute("age", item.Age))
}

func cronJobContextRow(item CronJob) screenContextRow {
	return resourceRow("cronjob", item.Name,
		stringAttribute("schedule", item.Schedule), stringAttribute("suspended", strconv.FormatBool(item.Suspend)),
		attribute("active", item.Active), stringAttribute("last_schedule", item.LastSchedule), stringAttribute("age", item.Age))
}

func hpaContextRow(item HPA) screenContextRow {
	targets := make([]string, 0, len(item.Targets))
	for _, target := range item.Targets {
		targets = append(targets, target.String())
	}
	return resourceRow("hpa", item.Name,
		stringAttribute("reference", item.Reference.String()), stringAttribute("targets", strings.Join(targets, ",")),
		attribute("min_replicas", item.MinReplicas), attribute("max_replicas", item.MaxReplicas),
		attribute("replicas", item.Replicas), stringAttribute("age", item.Age))
}

func secretContextRow(item Secret) screenContextRow {
	return resourceRow("secret", item.Name,
		stringAttribute("type", item.Type), attribute("data_keys", item.Data), stringAttribute("age", item.Age))
}

func replicaSetContextRow(item ReplicaSet) screenContextRow {
	return resourceRow("replicaset", item.Name,
		attribute("desired", item.Desired), attribute("current", item.Current),
		attribute("ready", item.Ready), stringAttribute("age", item.Age))
}

func rbacContextRow(item RBAC) screenContextRow {
	return resourceRow("rbac", item.Name,
		stringAttribute("rbac_kind", item.Kind), stringAttribute("scope", item.Scope),
		attribute("entries", item.Count), stringAttribute("age", item.Age))
}

func resourceRow(kind, name string, attributes ...screenContextAttribute) screenContextRow {
	return screenContextRow{Kind: kind, Name: boundedContextValue(name, maxScreenFieldRunes), Attributes: attributes}
}
