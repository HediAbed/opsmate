package service

import (
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
)

const (
	contextNamespaceColumnIndex  = 3
	maximumRecentEvents          = 50
	namespacedMetricFieldCount   = 3
	allNamespaceMetricFieldCount = 4
	dayDuration                  = 24 * time.Hour
)

type NamespacesMsg struct {
	Namespaces []string
	Err        error
}

type Pod struct {
	Name       string
	Namespace  string
	Status     string
	Ready      string
	Restarts   int
	Age        string
	CPU        string
	Memory     string
	Node       string
	IP         string
	Containers []string
}

type PodsMsg struct {
	Pods []Pod
	Err  error
}

type Deployment struct {
	Name       string
	Namespace  string
	Ready      string
	UpToDate   int
	Available  int
	Age        string
	Containers []string
	Images     []string
	Selector   string
}

type DeploymentsMsg struct {
	Deployments []Deployment
	Err         error
}

type Event struct {
	Name          string
	UID           string
	Namespace     string
	Type          string
	Reason        string
	Object        string
	Message       string
	Age           string
	Count         int
	LastTimestamp time.Time
}

type EventsMsg struct {
	Events []Event
	Err    error
}

type DescribeMsg struct {
	Output string
	Err    error
}

type LogsMsg struct {
	Lines []string
	Err   error
}

type CommandResultMsg struct {
	Output string
	Err    error
}

type MetricsMsg struct {
	PodMetrics []PodMetric
	Err        error
}

type PodMetric struct {
	Name      string
	Namespace string
	CPU       string
	Memory    string
}

type Service struct {
	Name       string
	Namespace  string
	Type       string
	ClusterIP  string
	ExternalIP string
	Ports      string
	Age        string
	Selector   string
}

type ServicesMsg struct {
	Services []Service
	Err      error
}

type StatefulSet struct {
	Name      string
	Namespace string
	Ready     string
	Replicas  int
	Age       string
}

type StatefulSetsMsg struct {
	StatefulSets []StatefulSet
	Err          error
}

type DaemonSet struct {
	Name      string
	Namespace string
	Desired   int
	Current   int
	Ready     int
	Available int
	Age       string
}

type DaemonSetsMsg struct {
	DaemonSets []DaemonSet
	Err        error
}

type ConfigMap struct {
	Name      string
	Namespace string
	Data      int
	Age       string
}

type ConfigMapsMsg struct {
	ConfigMaps []ConfigMap
	Err        error
}

type KubeContext struct {
	Name      string
	Cluster   string
	Namespace string
	Current   bool
}

type ContextsMsg struct {
	Contexts []KubeContext
	Err      error
}

type ContextSwitchedMsg struct {
	Name string
	Err  error
}

func FetchContexts() tea.Cmd {
	return func() tea.Msg {
		out, err := runKubectl(KubectlReadTimeout, "config", "get-contexts", "--no-headers")
		if err != nil {
			return ContextsMsg{Err: err}
		}
		return ContextsMsg{Contexts: parseContextsOutput(string(out))}
	}
}

// parseContextsOutput accepts [*] NAME CLUSTER AUTHINFO [NAMESPACE] rows.
func parseContextsOutput(out string) []KubeContext {
	lines := strings.Split(strings.TrimSpace(out), "\n")
	contexts := make([]KubeContext, 0, len(lines))
	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		current := false
		if fields[0] == "*" {
			current = true
			fields = fields[1:]
		}
		if len(fields) == 0 {
			continue
		}
		ctx := KubeContext{Name: fields[0], Current: current}
		if len(fields) > 1 {
			ctx.Cluster = fields[1]
		}
		if len(fields) > contextNamespaceColumnIndex {
			ctx.Namespace = fields[contextNamespaceColumnIndex]
		}
		contexts = append(contexts, ctx)
	}
	return contexts
}

func SwitchContext(name string) tea.Cmd {
	return func() tea.Msg {
		if _, err := runKubectl(KubectlActionTimeout, "config", "use-context", name); err != nil {
			return ContextSwitchedMsg{Err: err}
		}
		return ContextSwitchedMsg{Name: name}
	}
}

type CurrentContextMsg struct {
	Name string
	Err  error
}

func FetchCurrentContext() tea.Cmd {
	return func() tea.Msg {
		out, err := runKubectl(KubectlReadTimeout, "config", "current-context")
		if err != nil {
			return CurrentContextMsg{Err: err}
		}
		return CurrentContextMsg{Name: strings.TrimSpace(string(out))}
	}
}

func listArgsJSON(kind, namespace string) []string {
	args := append([]string{"get", kind}, namespaceArgs(namespace)...)
	return append(args, "-o", "json")
}

func FetchNamespaces() tea.Cmd {
	return func() tea.Msg {
		out, err := runKubectl(KubectlReadTimeout,
			"get", "namespaces", "-o", "jsonpath={.items[*].metadata.name}")
		if err != nil {
			return NamespacesMsg{Err: err}
		}
		return NamespacesMsg{Namespaces: strings.Fields(string(out))}
	}
}

type rawContainerStatus struct {
	Ready        bool `json:"ready"`
	RestartCount int  `json:"restartCount"`
	State        struct {
		Waiting *struct {
			Reason string `json:"reason"`
		} `json:"waiting"`
		Terminated *struct {
			ExitCode int    `json:"exitCode"`
			Reason   string `json:"reason"`
		} `json:"terminated"`
	} `json:"state"`
}

type rawPod struct {
	Metadata struct {
		Name              string     `json:"name"`
		Namespace         string     `json:"namespace"`
		CreationTimestamp time.Time  `json:"creationTimestamp"`
		DeletionTimestamp *time.Time `json:"deletionTimestamp"`
	} `json:"metadata"`
	Status struct {
		Phase                 string               `json:"phase"`
		PodIP                 string               `json:"podIP"`
		InitContainerStatuses []rawContainerStatus `json:"initContainerStatuses"`
		ContainerStatuses     []rawContainerStatus `json:"containerStatuses"`
	} `json:"status"`
	Spec struct {
		NodeName   string `json:"nodeName"`
		Containers []struct {
			Name string `json:"name"`
		} `json:"containers"`
	} `json:"spec"`
}

type podList struct {
	Items []rawPod `json:"items"`
}

func FetchPods(namespace string) tea.Cmd {
	return func() tea.Msg {
		pods, err := listPodsSync(namespace)
		if err != nil {
			return PodsMsg{Err: err}
		}
		return PodsMsg{Pods: pods}
	}
}

func listPodsSync(namespace string) ([]Pod, error) {
	raw, err := runKubectlJSON[podList](KubectlReadTimeout, listArgsJSON("pods", namespace)...)
	if err != nil {
		return nil, err
	}
	return projectListItems(raw.Items, podFromRaw), nil
}

func podFromRaw(item rawPod) Pod {
	ready := 0
	restarts := 0
	for _, status := range item.Status.ContainerStatuses {
		if status.Ready {
			ready++
		}
		restarts += status.RestartCount
	}
	containers := make([]string, 0, len(item.Spec.Containers))
	for _, container := range item.Spec.Containers {
		containers = append(containers, container.Name)
	}
	return Pod{
		Name:       item.Metadata.Name,
		Namespace:  item.Metadata.Namespace,
		Status:     podDisplayStatus(item),
		Ready:      fmt.Sprintf("%d/%d", ready, len(item.Status.ContainerStatuses)),
		Restarts:   restarts,
		Age:        formatAge(time.Since(item.Metadata.CreationTimestamp)),
		Node:       item.Spec.NodeName,
		IP:         item.Status.PodIP,
		Containers: containers,
	}
}

func podDisplayStatus(item rawPod) string {
	if item.Metadata.DeletionTimestamp != nil {
		return "Terminating"
	}
	if reason := initContainerStatus(item.Status.InitContainerStatuses); reason != "" {
		return reason
	}
	if reason := containerStatusReason(item.Status.ContainerStatuses); reason != "" {
		return reason
	}
	if item.Status.Phase == "" {
		return "Unknown"
	}
	return item.Status.Phase
}

func initContainerStatus(statuses []rawContainerStatus) string {
	for index, status := range statuses {
		if status.State.Waiting != nil && status.State.Waiting.Reason != "" {
			return "Init:" + status.State.Waiting.Reason
		}
		if status.State.Terminated != nil && status.State.Terminated.ExitCode != 0 {
			reason := status.State.Terminated.Reason
			if reason == "" {
				reason = fmt.Sprintf("ExitCode%d", status.State.Terminated.ExitCode)
			}
			return "Init:" + reason
		}
		if status.State.Terminated == nil {
			return fmt.Sprintf("Init:%d/%d", index, len(statuses))
		}
	}
	return ""
}

func containerStatusReason(statuses []rawContainerStatus) string {
	for _, status := range statuses {
		if status.State.Waiting != nil && status.State.Waiting.Reason != "" {
			return status.State.Waiting.Reason
		}
	}
	return ""
}

type deploymentList struct {
	Items []struct {
		Metadata struct {
			Name              string    `json:"name"`
			Namespace         string    `json:"namespace"`
			CreationTimestamp time.Time `json:"creationTimestamp"`
		} `json:"metadata"`
		Spec struct {
			Selector struct {
				MatchLabels map[string]string `json:"matchLabels"`
			} `json:"selector"`
			Template struct {
				Spec struct {
					Containers []struct {
						Name  string `json:"name"`
						Image string `json:"image"`
					} `json:"containers"`
				} `json:"spec"`
			} `json:"template"`
		} `json:"spec"`
		Status struct {
			ReadyReplicas     int `json:"readyReplicas"`
			Replicas          int `json:"replicas"`
			UpdatedReplicas   int `json:"updatedReplicas"`
			AvailableReplicas int `json:"availableReplicas"`
		} `json:"status"`
	} `json:"items"`
}

func FetchDeployments(namespace string) tea.Cmd {
	return func() tea.Msg {
		raw, err := runKubectlJSON[deploymentList](KubectlReadTimeout, listArgsJSON("deployments", namespace)...)
		if err != nil {
			return DeploymentsMsg{Err: err}
		}
		deps := make([]Deployment, 0, len(raw.Items))
		for _, item := range raw.Items {
			containers := make([]string, 0, len(item.Spec.Template.Spec.Containers))
			images := make([]string, 0, len(item.Spec.Template.Spec.Containers))
			for _, c := range item.Spec.Template.Spec.Containers {
				containers = append(containers, c.Name)
				images = append(images, c.Image)
			}
			deps = append(deps, Deployment{
				Name:       item.Metadata.Name,
				Namespace:  item.Metadata.Namespace,
				Ready:      fmt.Sprintf("%d/%d", item.Status.ReadyReplicas, item.Status.Replicas),
				UpToDate:   item.Status.UpdatedReplicas,
				Available:  item.Status.AvailableReplicas,
				Age:        formatAge(time.Since(item.Metadata.CreationTimestamp)),
				Containers: containers,
				Images:     images,
				Selector:   formatLabelMap(item.Spec.Selector.MatchLabels),
			})
		}
		return DeploymentsMsg{Deployments: deps}
	}
}

type eventList struct {
	Items []struct {
		Metadata struct {
			Namespace string `json:"namespace"`
			Name      string `json:"name"`
			UID       string `json:"uid"`
		} `json:"metadata"`
		Type           string `json:"type"`
		Reason         string `json:"reason"`
		Message        string `json:"message"`
		Count          int    `json:"count"`
		InvolvedObject struct {
			Kind string `json:"kind"`
			Name string `json:"name"`
		} `json:"involvedObject"`
		LastTimestamp time.Time `json:"lastTimestamp"`
	} `json:"items"`
}

func FetchEvents(namespace string) tea.Cmd {
	return func() tea.Msg {
		args := append(listArgsJSON("events", namespace), "--sort-by=.lastTimestamp")
		raw, err := runKubectlJSON[eventList](KubectlReadTimeout, args...)
		if err != nil {
			return EventsMsg{Err: err}
		}

		events := make([]Event, 0, len(raw.Items))
		for _, item := range raw.Items {
			events = append(events, Event{
				Name:          item.Metadata.Name,
				UID:           item.Metadata.UID,
				Namespace:     item.Metadata.Namespace,
				Type:          item.Type,
				Reason:        item.Reason,
				Object:        item.InvolvedObject.Kind + "/" + item.InvolvedObject.Name,
				Message:       item.Message,
				Age:           formatAge(time.Since(item.LastTimestamp)),
				Count:         item.Count,
				LastTimestamp: item.LastTimestamp,
			})
		}
		if len(events) > maximumRecentEvents {
			events = events[len(events)-maximumRecentEvents:]
		}
		for i, j := 0, len(events)-1; i < j; i, j = i+1, j-1 {
			events[i], events[j] = events[j], events[i]
		}
		return EventsMsg{Events: events}
	}
}

func DescribeResource(namespace, resourceType, name string) tea.Cmd {
	return func() tea.Msg {
		if err := requireNamespace("describe", namespace); err != nil {
			return DescribeMsg{Err: err}
		}
		out, err := runKubectlText(KubectlDetailTimeout,
			"describe", resourceType, name, "-n", namespace)
		if err != nil {
			return DescribeMsg{Err: err}
		}
		return DescribeMsg{Output: out}
	}
}

// FetchContainerLogs fetches logs for a specific container in a pod. An empty
// container name targets the default container.
func FetchContainerLogs(namespace, podName, container string, tailLines int) tea.Cmd {
	return func() tea.Msg {
		if err := requireNamespace("logs", namespace); err != nil {
			return LogsMsg{Err: err}
		}
		args := []string{"logs", podName, "-n", namespace, "--tail=" + strconv.Itoa(tailLines)}
		if container != "" {
			args = append(args, "-c", container)
		}
		out, err := runKubectlText(KubectlLogsTimeout, args...)
		if err != nil {
			return LogsMsg{Err: err}
		}
		return LogsMsg{Lines: strings.Split(strings.TrimSpace(out), "\n")}
	}
}

// ContainersMsg carries the list of container names for a pod.
type ContainersMsg struct {
	Containers []string
	Err        error
}

// FetchContainers lists the containers in a pod.
func FetchContainers(namespace, podName string) tea.Cmd {
	return func() tea.Msg {
		if err := requireNamespace("get pod", namespace); err != nil {
			return ContainersMsg{Err: err}
		}
		out, err := runKubectl(KubectlReadTimeout,
			"get", "pod", podName, "-n", namespace,
			"-o", "jsonpath={.spec.containers[*].name}")
		if err != nil {
			return ContainersMsg{Err: err}
		}
		return ContainersMsg{Containers: strings.Fields(strings.TrimSpace(string(out)))}
	}
}

func ScaleResource(namespace, resourceType, name string, replicas int) tea.Cmd {
	return func() tea.Msg {
		if err := requireNamespace("scale", namespace); err != nil {
			return CommandResultMsg{Err: err}
		}
		out, err := runKubectlText(KubectlActionTimeout,
			"scale", resourceType, name,
			"-n", namespace, "--replicas="+strconv.Itoa(replicas))
		if err != nil {
			return CommandResultMsg{Output: out, Err: err}
		}
		return CommandResultMsg{Output: out}
	}
}

func DeleteResource(namespace, resourceType, name string) tea.Cmd {
	return func() tea.Msg {
		if err := requireNamespace("delete", namespace); err != nil {
			return CommandResultMsg{Err: err}
		}
		out, err := runKubectlText(KubectlActionTimeout,
			"delete", resourceType, name, "-n", namespace)
		if err != nil {
			return CommandResultMsg{Output: out, Err: err}
		}
		return CommandResultMsg{Output: out}
	}
}

// DeleteResources deletes same-kind resources in one command.
func DeleteResources(namespace, resourceType string, names []string) tea.Cmd {
	return func() tea.Msg {
		if err := requireNamespace("delete", namespace); err != nil {
			return CommandResultMsg{Err: err}
		}
		if len(names) == 0 {
			return CommandResultMsg{Err: errors.New("delete: no resources selected")}
		}
		args := append([]string{"delete", resourceType}, names...)
		args = append(args, "-n", namespace)
		out, err := runKubectlText(KubectlActionTimeout, args...)
		if err != nil {
			return CommandResultMsg{Output: out, Err: err}
		}
		return CommandResultMsg{Output: out}
	}
}

// YAMLMsg carries the YAML output for a resource.
type YAMLMsg struct {
	Output string
	Err    error
}

// GetYAML fetches the YAML manifest for a resource.
func GetYAML(namespace, resourceType, name string) tea.Cmd {
	return func() tea.Msg {
		if err := requireNamespace("get -o yaml", namespace); err != nil {
			return YAMLMsg{Err: err}
		}
		resource := strings.ToLower(strings.TrimSpace(resourceType))
		if resource == "secret" || resource == "secrets" {
			return YAMLMsg{Err: sensitiveDataPolicyError("reading Secret resources is disabled")}
		}
		out, err := runKubectlText(KubectlDetailTimeout,
			"get", resourceType, name, "-n", namespace, "-o", "yaml")
		if err != nil {
			return YAMLMsg{Err: err}
		}
		return YAMLMsg{Output: out}
	}
}

// RestartRollout performs a rolling restart of a deployment or statefulset.
func RestartRollout(namespace, resourceType, name string) tea.Cmd {
	return func() tea.Msg {
		if err := requireNamespace("rollout restart", namespace); err != nil {
			return CommandResultMsg{Err: err}
		}
		out, err := runKubectlText(KubectlActionTimeout,
			"rollout", "restart", resourceType+"/"+name, "-n", namespace)
		if err != nil {
			return CommandResultMsg{Output: out, Err: err}
		}
		return CommandResultMsg{Output: out}
	}
}

// RestartRollouts restarts same-kind workloads in one command.
func RestartRollouts(namespace, resourceType string, names []string) tea.Cmd {
	return func() tea.Msg {
		if err := requireNamespace("rollout restart", namespace); err != nil {
			return CommandResultMsg{Err: err}
		}
		if len(names) == 0 {
			return CommandResultMsg{Err: errors.New("rollout restart: no resources selected")}
		}
		args := []string{"rollout", "restart"}
		for _, n := range names {
			args = append(args, resourceType+"/"+n)
		}
		args = append(args, "-n", namespace)
		out, err := runKubectlText(KubectlActionTimeout, args...)
		if err != nil {
			return CommandResultMsg{Output: out, Err: err}
		}
		return CommandResultMsg{Output: out}
	}
}

func ExecuteCommand(cmdStr string) tea.Cmd {
	return func() tea.Msg {
		args, err := validateKubectl(cmdStr)
		if err != nil {
			return CommandResultMsg{Err: err}
		}
		out, runErr := runKubectlText(KubectlActionTimeout, args...)
		if runErr != nil {
			return CommandResultMsg{Output: out, Err: runErr}
		}
		return CommandResultMsg{Output: out}
	}
}

func FetchPodMetrics(namespace string) tea.Cmd {
	return func() tea.Msg {
		args := append([]string{"top", "pods"}, namespaceArgs(namespace)...)
		args = append(args, "--no-headers")
		out, err := runKubectl(KubectlReadTimeout, args...)
		if err != nil {
			return MetricsMsg{Err: fmt.Errorf("%w (metrics-server may not be installed)", err)}
		}
		return MetricsMsg{PodMetrics: parsePodMetrics(string(out), namespace)}
	}
}

// parsePodMetrics handles namespace-scoped and all-namespace output.
func parsePodMetrics(out, namespace string) []PodMetric {
	lines := strings.Split(strings.TrimSpace(out), "\n")
	metrics := make([]PodMetric, 0, len(lines))
	for _, line := range lines {
		fields := strings.Fields(line)
		if namespace == "" {
			if len(fields) >= allNamespaceMetricFieldCount {
				metrics = append(metrics, PodMetric{
					Namespace: fields[0], Name: fields[1], CPU: fields[2], Memory: fields[3],
				})
			}
			continue
		}
		if len(fields) >= namespacedMetricFieldCount {
			metrics = append(metrics, PodMetric{
				Namespace: namespace, Name: fields[0], CPU: fields[1], Memory: fields[2],
			})
		}
	}
	return metrics
}

type serviceList struct {
	Items []struct {
		Metadata struct {
			Name              string    `json:"name"`
			Namespace         string    `json:"namespace"`
			CreationTimestamp time.Time `json:"creationTimestamp"`
		} `json:"metadata"`
		Spec struct {
			Type        string            `json:"type"`
			ClusterIP   string            `json:"clusterIP"`
			ExternalIPs []string          `json:"externalIPs"`
			Selector    map[string]string `json:"selector"`
			Ports       []struct {
				Port     int    `json:"port"`
				Protocol string `json:"protocol"`
			} `json:"ports"`
		} `json:"spec"`
		Status struct {
			LoadBalancer struct {
				Ingress []struct {
					IP       string `json:"ip"`
					Hostname string `json:"hostname"`
				} `json:"ingress"`
			} `json:"loadBalancer"`
		} `json:"status"`
	} `json:"items"`
}

func FetchServices(namespace string) tea.Cmd {
	return func() tea.Msg {
		svcs, err := listServicesSync(namespace)
		if err != nil {
			return ServicesMsg{Err: err}
		}
		return ServicesMsg{Services: svcs}
	}
}

func listServicesSync(namespace string) ([]Service, error) {
	raw, err := runKubectlJSON[serviceList](KubectlReadTimeout, listArgsJSON("services", namespace)...)
	if err != nil {
		return nil, err
	}
	svcs := make([]Service, 0, len(raw.Items))
	for _, item := range raw.Items {
		ports := make([]string, 0, len(item.Spec.Ports))
		for _, p := range item.Spec.Ports {
			ports = append(ports, fmt.Sprintf("%d/%s", p.Port, p.Protocol))
		}
		external := item.Spec.ExternalIPs
		for _, lb := range item.Status.LoadBalancer.Ingress {
			if lb.IP != "" {
				external = append(external, lb.IP)
				continue
			}
			if lb.Hostname != "" {
				external = append(external, lb.Hostname)
			}
		}
		svcs = append(svcs, Service{
			Name:       item.Metadata.Name,
			Namespace:  item.Metadata.Namespace,
			Type:       item.Spec.Type,
			ClusterIP:  item.Spec.ClusterIP,
			ExternalIP: joinOrNone(external),
			Ports:      strings.Join(ports, ","),
			Age:        formatAge(time.Since(item.Metadata.CreationTimestamp)),
			Selector:   formatLabelMap(item.Spec.Selector),
		})
	}
	return svcs, nil
}

type statefulSetList struct {
	Items []struct {
		Metadata struct {
			Name              string    `json:"name"`
			Namespace         string    `json:"namespace"`
			CreationTimestamp time.Time `json:"creationTimestamp"`
		} `json:"metadata"`
		Status struct {
			ReadyReplicas int `json:"readyReplicas"`
			Replicas      int `json:"replicas"`
		} `json:"status"`
	} `json:"items"`
}

func FetchStatefulSets(namespace string) tea.Cmd {
	return func() tea.Msg {
		raw, err := runKubectlJSON[statefulSetList](KubectlReadTimeout, listArgsJSON("statefulsets", namespace)...)
		if err != nil {
			return StatefulSetsMsg{Err: err}
		}
		sets := make([]StatefulSet, 0, len(raw.Items))
		for _, item := range raw.Items {
			sets = append(sets, StatefulSet{
				Name:      item.Metadata.Name,
				Namespace: item.Metadata.Namespace,
				Ready:     fmt.Sprintf("%d/%d", item.Status.ReadyReplicas, item.Status.Replicas),
				Replicas:  item.Status.Replicas,
				Age:       formatAge(time.Since(item.Metadata.CreationTimestamp)),
			})
		}
		return StatefulSetsMsg{StatefulSets: sets}
	}
}

type daemonSetList struct {
	Items []struct {
		Metadata struct {
			Name              string    `json:"name"`
			Namespace         string    `json:"namespace"`
			CreationTimestamp time.Time `json:"creationTimestamp"`
		} `json:"metadata"`
		Status struct {
			DesiredNumberScheduled int `json:"desiredNumberScheduled"`
			CurrentNumberScheduled int `json:"currentNumberScheduled"`
			NumberReady            int `json:"numberReady"`
			NumberAvailable        int `json:"numberAvailable"`
		} `json:"status"`
	} `json:"items"`
}

func FetchDaemonSets(namespace string) tea.Cmd {
	return func() tea.Msg {
		raw, err := runKubectlJSON[daemonSetList](KubectlReadTimeout, listArgsJSON("daemonsets", namespace)...)
		if err != nil {
			return DaemonSetsMsg{Err: err}
		}
		sets := make([]DaemonSet, 0, len(raw.Items))
		for _, item := range raw.Items {
			sets = append(sets, DaemonSet{
				Name:      item.Metadata.Name,
				Namespace: item.Metadata.Namespace,
				Desired:   item.Status.DesiredNumberScheduled,
				Current:   item.Status.CurrentNumberScheduled,
				Ready:     item.Status.NumberReady,
				Available: item.Status.NumberAvailable,
				Age:       formatAge(time.Since(item.Metadata.CreationTimestamp)),
			})
		}
		return DaemonSetsMsg{DaemonSets: sets}
	}
}

type configMapList struct {
	Items []struct {
		Metadata struct {
			Name              string    `json:"name"`
			Namespace         string    `json:"namespace"`
			CreationTimestamp time.Time `json:"creationTimestamp"`
		} `json:"metadata"`
		Data map[string]string `json:"data"`
	} `json:"items"`
}

func FetchConfigMaps(namespace string) tea.Cmd {
	return func() tea.Msg {
		raw, err := runKubectlJSON[configMapList](KubectlReadTimeout, listArgsJSON("configmaps", namespace)...)
		if err != nil {
			return ConfigMapsMsg{Err: err}
		}
		cms := make([]ConfigMap, 0, len(raw.Items))
		for _, item := range raw.Items {
			cms = append(cms, ConfigMap{
				Name:      item.Metadata.Name,
				Namespace: item.Metadata.Namespace,
				Data:      len(item.Data),
				Age:       formatAge(time.Since(item.Metadata.CreationTimestamp)),
			})
		}
		return ConfigMapsMsg{ConfigMaps: cms}
	}
}

type Node struct {
	Name       string
	Status     string
	Roles      string
	Version    string
	Age        string
	InternalIP string
	OSImage    string
	Kernel     string
	Runtime    string
}

type NodesMsg struct {
	Nodes []Node
	Err   error
}

type nodeCondition struct {
	Type   string `json:"type"`
	Status string `json:"status"`
}

type nodeList struct {
	Items []struct {
		Metadata struct {
			Name              string            `json:"name"`
			Labels            map[string]string `json:"labels"`
			CreationTimestamp time.Time         `json:"creationTimestamp"`
		} `json:"metadata"`
		Status struct {
			Conditions []nodeCondition `json:"conditions"`
			Addresses  []struct {
				Type    string `json:"type"`
				Address string `json:"address"`
			} `json:"addresses"`
			NodeInfo struct {
				KubeletVersion          string `json:"kubeletVersion"`
				OSImage                 string `json:"osImage"`
				KernelVersion           string `json:"kernelVersion"`
				ContainerRuntimeVersion string `json:"containerRuntimeVersion"`
			} `json:"nodeInfo"`
		} `json:"status"`
	} `json:"items"`
}

func FetchNodes() tea.Cmd {
	return func() tea.Msg {
		raw, err := runKubectlJSON[nodeList](KubectlReadTimeout, "get", "nodes", "-o", "json")
		if err != nil {
			return NodesMsg{Err: err}
		}
		nodes := make([]Node, 0, len(raw.Items))
		for _, item := range raw.Items {
			internal := ""
			for _, a := range item.Status.Addresses {
				if a.Type == "InternalIP" {
					internal = a.Address
					break
				}
			}
			nodes = append(nodes, Node{
				Name:       item.Metadata.Name,
				Status:     nodeStatusFromConditions(item.Status.Conditions),
				Roles:      nodeRolesFromLabels(item.Metadata.Labels),
				Version:    item.Status.NodeInfo.KubeletVersion,
				Age:        formatAge(time.Since(item.Metadata.CreationTimestamp)),
				InternalIP: internal,
				OSImage:    item.Status.NodeInfo.OSImage,
				Kernel:     item.Status.NodeInfo.KernelVersion,
				Runtime:    item.Status.NodeInfo.ContainerRuntimeVersion,
			})
		}
		return NodesMsg{Nodes: nodes}
	}
}

func nodeStatusFromConditions(conditions []nodeCondition) string {
	for _, c := range conditions {
		if c.Type == "Ready" {
			if c.Status == "True" {
				return "Ready"
			}
			return "NotReady"
		}
	}
	return "Unknown"
}

func nodeRolesFromLabels(labels map[string]string) string {
	const prefix = "node-role.kubernetes.io/"
	roles := make([]string, 0, len(labels))
	for k := range labels {
		if strings.HasPrefix(k, prefix) {
			if role := strings.TrimPrefix(k, prefix); role != "" {
				roles = append(roles, role)
			}
		}
	}
	if len(roles) == 0 {
		return "<none>"
	}
	return strings.Join(roles, ",")
}

type Job struct {
	Name        string
	Namespace   string
	Completions string
	Duration    string
	Status      string
	Age         string
}

type JobsMsg struct {
	Jobs []Job
	Err  error
}

type jobList struct {
	Items []struct {
		Metadata struct {
			Name              string    `json:"name"`
			Namespace         string    `json:"namespace"`
			CreationTimestamp time.Time `json:"creationTimestamp"`
		} `json:"metadata"`
		Spec struct {
			Completions *int `json:"completions"`
		} `json:"spec"`
		Status struct {
			Succeeded      int        `json:"succeeded"`
			Failed         int        `json:"failed"`
			Active         int        `json:"active"`
			StartTime      *time.Time `json:"startTime"`
			CompletionTime *time.Time `json:"completionTime"`
		} `json:"status"`
	} `json:"items"`
}

func FetchJobs(namespace string) tea.Cmd {
	return func() tea.Msg {
		raw, err := runKubectlJSON[jobList](KubectlReadTimeout, listArgsJSON("jobs", namespace)...)
		if err != nil {
			return JobsMsg{Err: err}
		}
		jobs := make([]Job, 0, len(raw.Items))
		for _, item := range raw.Items {
			completions := 1
			if item.Spec.Completions != nil {
				completions = *item.Spec.Completions
			}
			status := "Running"
			switch {
			case item.Status.Succeeded >= completions:
				status = "Complete"
			case item.Status.Failed > 0 && item.Status.Active == 0:
				status = "Failed"
			}
			duration := "-"
			if item.Status.StartTime != nil {
				end := time.Now()
				if item.Status.CompletionTime != nil {
					end = *item.Status.CompletionTime
				}
				duration = formatAge(end.Sub(*item.Status.StartTime))
			}
			jobs = append(jobs, Job{
				Name:        item.Metadata.Name,
				Namespace:   item.Metadata.Namespace,
				Completions: fmt.Sprintf("%d/%d", item.Status.Succeeded, completions),
				Duration:    duration,
				Status:      status,
				Age:         formatAge(time.Since(item.Metadata.CreationTimestamp)),
			})
		}
		return JobsMsg{Jobs: jobs}
	}
}

type Ingress struct {
	Name      string
	Namespace string
	Class     string
	Hosts     string
	Address   string
	Ports     string
	Age       string
}

type IngressesMsg struct {
	Ingresses []Ingress
	Err       error
}

type rawIngressItem struct {
	Metadata struct {
		Name              string    `json:"name"`
		Namespace         string    `json:"namespace"`
		CreationTimestamp time.Time `json:"creationTimestamp"`
	} `json:"metadata"`
	Spec struct {
		IngressClassName *string `json:"ingressClassName"`
		TLS              []struct {
			Hosts []string `json:"hosts"`
		} `json:"tls"`
		Rules []struct {
			Host string `json:"host"`
		} `json:"rules"`
	} `json:"spec"`
	Status struct {
		LoadBalancer struct {
			Ingress []struct {
				IP       string `json:"ip"`
				Hostname string `json:"hostname"`
			} `json:"ingress"`
		} `json:"loadBalancer"`
	} `json:"status"`
}

func FetchIngresses(namespace string) tea.Cmd {
	return func() tea.Msg {
		items, err := listKubectlItems[rawIngressItem]("ingresses", namespace)
		if err != nil {
			return IngressesMsg{Err: err}
		}
		return IngressesMsg{Ingresses: projectListItems(items, ingressFromRaw)}
	}
}

func ingressFromRaw(item rawIngressItem) Ingress {
	class := ""
	if item.Spec.IngressClassName != nil {
		class = *item.Spec.IngressClassName
	}
	return Ingress{
		Name:      item.Metadata.Name,
		Namespace: item.Metadata.Namespace,
		Class:     class,
		Hosts:     joinIngressHosts(item),
		Address:   joinIngressAddresses(item),
		Ports:     ingressPorts(item),
		Age:       formatAge(time.Since(item.Metadata.CreationTimestamp)),
	}
}

func joinIngressHosts(item rawIngressItem) string {
	hosts := make([]string, 0, len(item.Spec.Rules))
	seen := make(map[string]struct{}, len(item.Spec.Rules))
	for _, rule := range item.Spec.Rules {
		if rule.Host == "" {
			continue
		}
		if _, dup := seen[rule.Host]; dup {
			continue
		}
		seen[rule.Host] = struct{}{}
		hosts = append(hosts, rule.Host)
	}
	return strings.Join(hosts, ",")
}

func joinIngressAddresses(item rawIngressItem) string {
	addrs := make([]string, 0, len(item.Status.LoadBalancer.Ingress))
	for _, lb := range item.Status.LoadBalancer.Ingress {
		switch {
		case lb.Hostname != "":
			addrs = append(addrs, lb.Hostname)
		case lb.IP != "":
			addrs = append(addrs, lb.IP)
		}
	}
	return strings.Join(addrs, ",")
}

func ingressPorts(item rawIngressItem) string {
	if len(item.Spec.TLS) > 0 {
		return ingressHTTPSPorts
	}
	return ingressHTTPPorts
}

const (
	ingressHTTPPorts  = "80"
	ingressHTTPSPorts = "80, 443"
)

type NetworkPolicy struct {
	Name        string
	Namespace   string
	PodSelector map[string]string
	PolicyTypes []string
	Age         string
}

type NetworkPoliciesMsg struct {
	NetworkPolicies []NetworkPolicy
	Err             error
}

type rawNetworkPolicyItem struct {
	Metadata struct {
		Name              string    `json:"name"`
		Namespace         string    `json:"namespace"`
		CreationTimestamp time.Time `json:"creationTimestamp"`
	} `json:"metadata"`
	Spec struct {
		PodSelector struct {
			MatchLabels map[string]string `json:"matchLabels"`
		} `json:"podSelector"`
		PolicyTypes []string `json:"policyTypes"`
	} `json:"spec"`
}

func FetchNetworkPolicies(namespace string) tea.Cmd {
	return func() tea.Msg {
		items, err := listKubectlItems[rawNetworkPolicyItem]("networkpolicies", namespace)
		if err != nil {
			return NetworkPoliciesMsg{Err: err}
		}
		return NetworkPoliciesMsg{NetworkPolicies: projectListItems(items, networkPolicyFromRaw)}
	}
}

func networkPolicyFromRaw(item rawNetworkPolicyItem) NetworkPolicy {
	return NetworkPolicy{
		Name:        item.Metadata.Name,
		Namespace:   item.Metadata.Namespace,
		PodSelector: item.Spec.PodSelector.MatchLabels,
		PolicyTypes: item.Spec.PolicyTypes,
		Age:         formatAge(time.Since(item.Metadata.CreationTimestamp)),
	}
}

type PersistentVolumeClaim struct {
	Name         string
	Namespace    string
	Status       string
	Volume       string
	Capacity     string
	AccessModes  []string
	StorageClass string
	Age          string
}

type PVCsMsg struct {
	PVCs []PersistentVolumeClaim
	Err  error
}

type rawPVCItem struct {
	Metadata struct {
		Name              string    `json:"name"`
		Namespace         string    `json:"namespace"`
		CreationTimestamp time.Time `json:"creationTimestamp"`
	} `json:"metadata"`
	Spec struct {
		VolumeName       string   `json:"volumeName"`
		StorageClassName string   `json:"storageClassName"`
		AccessModes      []string `json:"accessModes"`
		Resources        struct {
			Requests struct {
				Storage string `json:"storage"`
			} `json:"requests"`
		} `json:"resources"`
	} `json:"spec"`
	Status struct {
		Phase    string `json:"phase"`
		Capacity struct {
			Storage string `json:"storage"`
		} `json:"capacity"`
	} `json:"status"`
}

func FetchPVCs(namespace string) tea.Cmd {
	return func() tea.Msg {
		items, err := listKubectlItems[rawPVCItem]("pvc", namespace)
		if err != nil {
			return PVCsMsg{Err: err}
		}
		return PVCsMsg{PVCs: projectListItems(items, pvcFromRaw)}
	}
}

func pvcFromRaw(item rawPVCItem) PersistentVolumeClaim {
	return PersistentVolumeClaim{
		Name:         item.Metadata.Name,
		Namespace:    item.Metadata.Namespace,
		Status:       item.Status.Phase,
		Volume:       item.Spec.VolumeName,
		Capacity:     pvcCapacity(item),
		AccessModes:  item.Spec.AccessModes,
		StorageClass: item.Spec.StorageClassName,
		Age:          formatAge(time.Since(item.Metadata.CreationTimestamp)),
	}
}

// pvcCapacity falls back to the requested size for unbound claims.
func pvcCapacity(item rawPVCItem) string {
	if item.Status.Capacity.Storage != "" {
		return item.Status.Capacity.Storage
	}
	return item.Spec.Resources.Requests.Storage
}

type CronJob struct {
	Name         string
	Namespace    string
	Schedule     string
	Suspend      bool
	Active       int
	LastSchedule string
	Age          string
}

type CronJobsMsg struct {
	CronJobs []CronJob
	Err      error
}

type rawCronJobItem struct {
	Metadata struct {
		Name              string    `json:"name"`
		Namespace         string    `json:"namespace"`
		CreationTimestamp time.Time `json:"creationTimestamp"`
	} `json:"metadata"`
	Spec struct {
		Schedule string `json:"schedule"`
		Suspend  *bool  `json:"suspend"`
	} `json:"spec"`
	Status struct {
		LastScheduleTime *time.Time `json:"lastScheduleTime"`
		Active           []struct {
			Name string `json:"name"`
		} `json:"active"`
	} `json:"status"`
}

func FetchCronJobs(namespace string) tea.Cmd {
	return func() tea.Msg {
		items, err := listKubectlItems[rawCronJobItem]("cronjobs", namespace)
		if err != nil {
			return CronJobsMsg{Err: err}
		}
		return CronJobsMsg{CronJobs: projectListItems(items, cronJobFromRaw)}
	}
}

func cronJobFromRaw(item rawCronJobItem) CronJob {
	return CronJob{
		Name:         item.Metadata.Name,
		Namespace:    item.Metadata.Namespace,
		Schedule:     item.Spec.Schedule,
		Suspend:      item.Spec.Suspend != nil && *item.Spec.Suspend,
		Active:       len(item.Status.Active),
		LastSchedule: cronJobLastScheduleAge(item.Status.LastScheduleTime),
		Age:          formatAge(time.Since(item.Metadata.CreationTimestamp)),
	}
}

const cronJobNeverScheduled = "<none>"

const (
	hpaTargetUnknown = "<none>"
	hpaMetricUnknown = "<unknown>"

	// autoscaling/v2 defaults an omitted minimum to one replica.
	hpaDefaultMinReplicas = 1
)

// ScaleTargetRef identifies an HPA target.
type ScaleTargetRef struct {
	Kind string
	Name string
}

// String returns the kubectl-style "Kind/Name" form for display.
func (r ScaleTargetRef) String() string {
	if r.Kind == "" || r.Name == "" {
		return ""
	}
	return r.Kind + "/" + r.Name
}

// HPAMetricPair contains one current and target metric reading.
type HPAMetricPair struct {
	Current string
	Target  string
}

func (p HPAMetricPair) String() string {
	if p.Current == "" {
		return p.Target
	}
	if p.Target == "" {
		return p.Current
	}
	return p.Current + "/" + p.Target
}

type HPA struct {
	Name        string
	Namespace   string
	Reference   ScaleTargetRef
	Targets     []HPAMetricPair
	MinReplicas int
	MaxReplicas int
	Replicas    int
	Age         string
}

type HPAsMsg struct {
	HPAs []HPA
	Err  error
}

type rawHPAItem struct {
	Metadata struct {
		Name              string    `json:"name"`
		Namespace         string    `json:"namespace"`
		CreationTimestamp time.Time `json:"creationTimestamp"`
	} `json:"metadata"`
	Spec struct {
		ScaleTargetRef struct {
			Kind string `json:"kind"`
			Name string `json:"name"`
		} `json:"scaleTargetRef"`
		MinReplicas *int           `json:"minReplicas"`
		MaxReplicas int            `json:"maxReplicas"`
		Metrics     []rawHPAMetric `json:"metrics"`
	} `json:"spec"`
	Status struct {
		CurrentReplicas int            `json:"currentReplicas"`
		CurrentMetrics  []rawHPAMetric `json:"currentMetrics"`
	} `json:"status"`
}

type rawHPAMetric struct {
	Type     string `json:"type"`
	Resource struct {
		Name    string           `json:"name"`
		Target  rawHPAMetricSide `json:"target"`
		Current rawHPAMetricSide `json:"current"`
	} `json:"resource"`
}

type rawHPAMetricSide struct {
	Type               string `json:"type"`
	AverageUtilization *int   `json:"averageUtilization"`
	AverageValue       string `json:"averageValue"`
}

func FetchHPAs(namespace string) tea.Cmd {
	return func() tea.Msg {
		items, err := listKubectlItems[rawHPAItem]("hpa", namespace)
		if err != nil {
			return HPAsMsg{Err: err}
		}
		return HPAsMsg{HPAs: projectListItems(items, hpaFromRaw)}
	}
}

func hpaFromRaw(item rawHPAItem) HPA {
	minReplicas := hpaDefaultMinReplicas
	if item.Spec.MinReplicas != nil {
		minReplicas = *item.Spec.MinReplicas
	}
	return HPA{
		Name:      item.Metadata.Name,
		Namespace: item.Metadata.Namespace,
		Reference: ScaleTargetRef{
			Kind: item.Spec.ScaleTargetRef.Kind,
			Name: item.Spec.ScaleTargetRef.Name,
		},
		Targets:     hpaTargets(item),
		MinReplicas: minReplicas,
		MaxReplicas: item.Spec.MaxReplicas,
		Replicas:    item.Status.CurrentReplicas,
		Age:         formatAge(time.Since(item.Metadata.CreationTimestamp)),
	}
}

// hpaTargets pairs each spec metric target with the matching status
// metric current reading. When status is missing a reading (HPA still
// warming up), current is shown as "<unknown>".
func hpaTargets(item rawHPAItem) []HPAMetricPair {
	if len(item.Spec.Metrics) == 0 {
		return []HPAMetricPair{{Current: "", Target: hpaTargetUnknown}}
	}
	out := make([]HPAMetricPair, 0, len(item.Spec.Metrics))
	for i, spec := range item.Spec.Metrics {
		current := hpaMetricUnknown
		if i < len(item.Status.CurrentMetrics) {
			current = formatHPAMetricSide(item.Status.CurrentMetrics[i].Resource.Current)
		}
		out = append(out, HPAMetricPair{
			Current: current,
			Target:  formatHPAMetricSide(spec.Resource.Target),
		})
	}
	return out
}

func formatHPAMetricSide(side rawHPAMetricSide) string {
	if side.AverageUtilization != nil {
		return strconv.Itoa(*side.AverageUtilization) + "%"
	}
	if side.AverageValue != "" {
		return side.AverageValue
	}
	return hpaMetricUnknown
}

// Secret excludes values and retains only the key count.
type Secret struct {
	Name      string
	Namespace string
	Type      string
	Data      int
	Age       string
}

type SecretsMsg struct {
	Secrets []Secret
	Err     error
}

// rawSecretItem decodes keys without retaining secret values.
type rawSecretItem struct {
	Metadata struct {
		Name              string    `json:"name"`
		Namespace         string    `json:"namespace"`
		CreationTimestamp time.Time `json:"creationTimestamp"`
	} `json:"metadata"`
	Type string                        `json:"type"`
	Data map[string]secretValueDiscard `json:"data"`
}

type secretValueDiscard struct{}

func (secretValueDiscard) UnmarshalJSON(_ []byte) error { return nil }

func FetchSecrets(namespace string) tea.Cmd {
	return func() tea.Msg {
		items, err := listKubectlItems[rawSecretItem]("secrets", namespace)
		if err != nil {
			return SecretsMsg{Err: err}
		}
		return SecretsMsg{Secrets: projectListItems(items, secretFromRaw)}
	}
}

func secretFromRaw(item rawSecretItem) Secret {
	return Secret{
		Name:      item.Metadata.Name,
		Namespace: item.Metadata.Namespace,
		Type:      item.Type,
		Data:      len(item.Data),
		Age:       formatAge(time.Since(item.Metadata.CreationTimestamp)),
	}
}

type ReplicaSet struct {
	Name      string
	Namespace string
	Desired   int
	Current   int
	Ready     int
	Age       string
}

type ReplicaSetsMsg struct {
	ReplicaSets []ReplicaSet
	Err         error
}

type rawReplicaSetItem struct {
	Metadata struct {
		Name              string    `json:"name"`
		Namespace         string    `json:"namespace"`
		CreationTimestamp time.Time `json:"creationTimestamp"`
	} `json:"metadata"`
	Spec struct {
		Replicas *int `json:"replicas"`
	} `json:"spec"`
	Status struct {
		Replicas      int `json:"replicas"`
		ReadyReplicas int `json:"readyReplicas"`
	} `json:"status"`
}

const replicaSetDesiredWhenUnset = 0

func FetchReplicaSets(namespace string) tea.Cmd {
	return func() tea.Msg {
		items, err := listKubectlItems[rawReplicaSetItem]("replicasets", namespace)
		if err != nil {
			return ReplicaSetsMsg{Err: err}
		}
		return ReplicaSetsMsg{ReplicaSets: projectListItems(items, replicaSetFromRaw)}
	}
}

// RBAC is a common row for accounts, roles, and bindings.
type RBAC struct {
	Kind      string
	Name      string
	Namespace string
	Count     int
	Scope     string
	Age       string
}

type RBACMsg struct {
	RBAC []RBAC
	Err  error
}

type rbacList struct {
	Items []rawRBACItem `json:"items"`
}

type rawRBACItem struct {
	Kind     string `json:"kind"`
	Metadata struct {
		Name              string    `json:"name"`
		Namespace         string    `json:"namespace"`
		CreationTimestamp time.Time `json:"creationTimestamp"`
	} `json:"metadata"`
	Rules    []rbacEntryDiscard `json:"rules"`
	Subjects []rbacEntryDiscard `json:"subjects"`
}

type rbacEntryDiscard struct{}

func (rbacEntryDiscard) UnmarshalJSON(_ []byte) error { return nil }

const (
	rbacKindServiceAccount     = "ServiceAccount"
	rbacKindRole               = "Role"
	rbacKindRoleBinding        = "RoleBinding"
	rbacKindClusterRole        = "ClusterRole"
	rbacKindClusterRoleBinding = "ClusterRoleBinding"

	rbacScopeNamespace = "Namespace"
	rbacScopeCluster   = "Cluster"
)

const rbacFetchKindList = "serviceaccount,role,rolebinding,clusterrole,clusterrolebinding"

func FetchRBAC(namespace string) tea.Cmd {
	return func() tea.Msg {
		args := append([]string{"get", rbacFetchKindList, "-o", "json"}, namespaceArgs(namespace)...)
		raw, err := runKubectlJSON[rbacList](KubectlReadTimeout, args...)
		if err != nil {
			return RBACMsg{Err: err}
		}
		out := make([]RBAC, 0, len(raw.Items))
		for _, item := range raw.Items {
			out = append(out, rbacFromRaw(item))
		}
		return RBACMsg{RBAC: out}
	}
}

func namespaceArgs(namespace string) []string {
	if namespace == "" {
		return []string{"--all-namespaces"}
	}
	return []string{"-n", namespace}
}

func rbacFromRaw(item rawRBACItem) RBAC {
	return RBAC{
		Kind:      item.Kind,
		Name:      item.Metadata.Name,
		Namespace: item.Metadata.Namespace,
		Count:     rbacCount(item),
		Scope:     rbacScope(item.Kind),
		Age:       formatAge(time.Since(item.Metadata.CreationTimestamp)),
	}
}

func rbacCount(item rawRBACItem) int {
	switch item.Kind {
	case rbacKindRole, rbacKindClusterRole:
		return len(item.Rules)
	case rbacKindRoleBinding, rbacKindClusterRoleBinding:
		return len(item.Subjects)
	}
	return 0
}

func rbacScope(kind string) string {
	switch kind {
	case rbacKindClusterRole, rbacKindClusterRoleBinding:
		return rbacScopeCluster
	}
	return rbacScopeNamespace
}

func replicaSetFromRaw(item rawReplicaSetItem) ReplicaSet {
	desired := replicaSetDesiredWhenUnset
	if item.Spec.Replicas != nil {
		desired = *item.Spec.Replicas
	}
	return ReplicaSet{
		Name:      item.Metadata.Name,
		Namespace: item.Metadata.Namespace,
		Desired:   desired,
		Current:   item.Status.Replicas,
		Ready:     item.Status.ReadyReplicas,
		Age:       formatAge(time.Since(item.Metadata.CreationTimestamp)),
	}
}

func cronJobLastScheduleAge(t *time.Time) string {
	if t == nil || t.IsZero() {
		return cronJobNeverScheduled
	}
	return formatAge(time.Since(*t))
}

// formatLabelMap returns labels in stable key order.
func formatLabelMap(labels map[string]string) string {
	if len(labels) == 0 {
		return ""
	}
	keys := make([]string, 0, len(labels))
	for k := range labels {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, k+"="+labels[k])
	}
	return strings.Join(parts, ",")
}

func joinOrNone(values []string) string {
	if len(values) == 0 {
		return "<none>"
	}
	return strings.Join(values, ",")
}

func formatAge(d time.Duration) string {
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < dayDuration:
		return fmt.Sprintf("%dh", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd", int(d/dayDuration))
	}
}
