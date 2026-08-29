package ui

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/HediAbed/opsmate/internal/cluster"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	metricsv1beta1 "k8s.io/metrics/pkg/apis/metrics/v1beta1"
)

func TestProjectPods(t *testing.T) {
	now := time.Date(2026, time.August, 28, 12, 0, 0, 0, time.UTC)
	pods := projectPods([]corev1.Pod{{
		ObjectMeta: metav1.ObjectMeta{Name: "web", Namespace: "shop", CreationTimestamp: metav1.NewTime(now.Add(-2 * time.Hour))},
		Spec: corev1.PodSpec{
			NodeName:   "node-a",
			Containers: []corev1.Container{{Name: "app"}, {Name: "sidecar"}},
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			PodIP: "10.0.0.4",
			ContainerStatuses: []corev1.ContainerStatus{
				{Ready: true, RestartCount: 2},
				{Ready: false, RestartCount: 1},
			},
		},
	}}, now)
	want := []cluster.Pod{{
		Name:       "web",
		Namespace:  "shop",
		Status:     "Running",
		Ready:      "1/2",
		Restarts:   3,
		Age:        "2h",
		Node:       "node-a",
		IP:         "10.0.0.4",
		Containers: []string{"app", "sidecar"},
	}}
	if !reflect.DeepEqual(pods, want) {
		t.Fatalf("projectPods() = %+v, want %+v", pods, want)
	}
}

func TestProjectedPodStatuses(t *testing.T) {
	now := time.Date(2026, time.August, 28, 12, 0, 0, 0, time.UTC)
	deletionTime := metav1.NewTime(now)
	tests := []struct {
		name string
		pod  corev1.Pod
		want string
	}{
		{name: "terminating", pod: corev1.Pod{ObjectMeta: metav1.ObjectMeta{DeletionTimestamp: &deletionTime}}, want: "Terminating"},
		{name: "init waiting", pod: podWithInitStatus(corev1.ContainerStatus{State: corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{Reason: "ImagePullBackOff"}}}), want: "Init:ImagePullBackOff"},
		{name: "init terminated reason", pod: podWithInitStatus(corev1.ContainerStatus{State: corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{ExitCode: 1, Reason: "Failed"}}}), want: "Init:Failed"},
		{name: "init terminated exit code", pod: podWithInitStatus(corev1.ContainerStatus{State: corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{ExitCode: 2}}}), want: "Init:ExitCode2"},
		{name: "init active", pod: podWithInitStatus(corev1.ContainerStatus{}), want: "Init:0/1"},
		{name: "container waiting", pod: corev1.Pod{Status: corev1.PodStatus{ContainerStatuses: []corev1.ContainerStatus{{State: corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{Reason: "CrashLoopBackOff"}}}}}}, want: "CrashLoopBackOff"},
		{name: "unknown", pod: corev1.Pod{}, want: "Unknown"},
		{name: "phase", pod: corev1.Pod{Status: corev1.PodStatus{Phase: corev1.PodPending}}, want: "Pending"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := projectedPodStatus(test.pod); got != test.want {
				t.Fatalf("projectedPodStatus() = %q, want %q", got, test.want)
			}
		})
	}
	if got := projectedInitStatus([]corev1.ContainerStatus{{State: corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{}}}}); got != "" {
		t.Fatalf("projectedInitStatus(success) = %q, want empty", got)
	}
}

func podWithInitStatus(status corev1.ContainerStatus) corev1.Pod {
	return corev1.Pod{Status: corev1.PodStatus{InitContainerStatuses: []corev1.ContainerStatus{status}}}
}

func TestProjectDeployments(t *testing.T) {
	now := time.Date(2026, time.August, 28, 12, 0, 0, 0, time.UTC)
	deployments := projectDeployments([]appsv1.Deployment{{
		ObjectMeta: metav1.ObjectMeta{Name: "api", Namespace: "shop", CreationTimestamp: metav1.NewTime(now.Add(-48 * time.Hour))},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"tier": "api", "app": "shop"}},
			Template: corev1.PodTemplateSpec{Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "api", Image: "example.invalid/api:v1"}}}},
		},
		Status: appsv1.DeploymentStatus{Replicas: 3, ReadyReplicas: 2, UpdatedReplicas: 3, AvailableReplicas: 2},
	}}, now)
	if len(deployments) != 1 || deployments[0].Ready != "2/3" || deployments[0].Selector != "app=shop,tier=api" || deployments[0].Age != "2d" || strings.Join(deployments[0].Containers, ",") != "api" || strings.Join(deployments[0].Images, ",") != "example.invalid/api:v1" {
		t.Fatalf("projectDeployments() = %+v", deployments)
	}
}

func TestProjectEventsSortsLimitsAndReadsSeries(t *testing.T) {
	now := time.Date(2026, time.August, 28, 12, 0, 0, 0, time.UTC)
	events := make([]corev1.Event, maximumProjectedEvents+1)
	for index := range events {
		observedAt := now.Add(time.Duration(index) * time.Second)
		events[index] = corev1.Event{
			ObjectMeta:     metav1.ObjectMeta{Name: "event", Namespace: "shop", UID: "uid", CreationTimestamp: metav1.NewTime(observedAt)},
			LastTimestamp:  metav1.NewTime(observedAt),
			InvolvedObject: corev1.ObjectReference{Kind: "Pod", Name: "web"},
			Type:           "Warning", Reason: "BackOff", Message: "retrying", Count: 2,
		}
	}
	projectedEvents := projectEvents(events, now.Add(time.Hour))
	if len(projectedEvents) != maximumProjectedEvents || projectedEvents[0].LastTimestamp.Before(projectedEvents[1].LastTimestamp) {
		t.Fatalf("projectEvents() did not sort and limit: first=%+v count=%d", projectedEvents[0], len(projectedEvents))
	}
	seriesTime := metav1.NewMicroTime(now.Add(-time.Minute))
	seriesEvent := corev1.Event{
		ObjectMeta:     metav1.ObjectMeta{Name: "series", Namespace: "shop", UID: "series-id"},
		Series:         &corev1.EventSeries{Count: 5, LastObservedTime: seriesTime},
		Count:          2,
		InvolvedObject: corev1.ObjectReference{Kind: "Deployment", Name: "api"},
	}
	projected := projectEvent(seriesEvent, now)
	if projected.Count != 5 || projected.Object != "Deployment/api" || projected.LastTimestamp != seriesTime.Time {
		t.Fatalf("projectEvent(series) = %+v", projected)
	}
}

func TestEventObservedAtUsesBestAvailableTimestamp(t *testing.T) {
	base := time.Date(2026, time.August, 28, 12, 0, 0, 0, time.UTC)
	micro := metav1.NewMicroTime(base.Add(time.Second))
	last := metav1.NewTime(base.Add(2 * time.Second))
	first := metav1.NewTime(base.Add(3 * time.Second))
	created := metav1.NewTime(base.Add(4 * time.Second))
	tests := []struct {
		name  string
		event corev1.Event
		want  time.Time
	}{
		{name: "series", event: corev1.Event{Series: &corev1.EventSeries{LastObservedTime: micro}}, want: micro.Time},
		{name: "event time", event: corev1.Event{EventTime: micro}, want: micro.Time},
		{name: "last", event: corev1.Event{LastTimestamp: last}, want: last.Time},
		{name: "first", event: corev1.Event{FirstTimestamp: first}, want: first.Time},
		{name: "created", event: corev1.Event{ObjectMeta: metav1.ObjectMeta{CreationTimestamp: created}}, want: created.Time},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := eventObservedAt(test.event); !got.Equal(test.want) {
				t.Fatalf("eventObservedAt() = %v, want %v", got, test.want)
			}
		})
	}
}

func TestProjectPodMetrics(t *testing.T) {
	now := time.Date(2026, time.August, 28, 12, 0, 0, 0, time.UTC)
	metrics := projectPodMetrics([]metricsv1beta1.PodMetrics{{
		ObjectMeta: metav1.ObjectMeta{Name: "api", Namespace: "shop"},
		Containers: []metricsv1beta1.ContainerMetrics{
			{Name: "api", Usage: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("125m"), corev1.ResourceMemory: resource.MustParse("10Mi")}},
			{Name: "sidecar", Usage: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("25m"), corev1.ResourceMemory: resource.MustParse("2Mi")}},
		},
	}}, now)
	if len(metrics) != 1 || metrics[0].CPU != "150m" || metrics[0].Memory != "12Mi" {
		t.Fatalf("projectPodMetrics() = %+v", metrics)
	}
	if cpu, memory := podMetricUsage(metricsv1beta1.PodMetrics{}); cpu != "0m" || memory != "0Mi" {
		t.Fatalf("podMetricUsage(empty) = (%q, %q)", cpu, memory)
	}
	if got := formatBinaryBytes(2 * 1024 * 1024); got != "2Mi" {
		t.Fatalf("formatBinaryBytes() = %q", got)
	}
}

func TestProjectServices(t *testing.T) {
	now := time.Date(2026, time.August, 28, 12, 0, 0, 0, time.UTC)
	services := projectServices([]corev1.Service{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "gateway", Namespace: "shop", CreationTimestamp: metav1.NewTime(now.Add(-time.Hour))},
			Spec: corev1.ServiceSpec{
				Type: corev1.ServiceTypeLoadBalancer, ClusterIP: "10.96.0.1",
				ExternalIPs: []string{"203.0.113.1"}, Selector: map[string]string{"app": "gateway"},
				Ports: []corev1.ServicePort{{Port: 443, Protocol: corev1.ProtocolTCP}},
			},
			Status: corev1.ServiceStatus{LoadBalancer: corev1.LoadBalancerStatus{Ingress: []corev1.LoadBalancerIngress{
				{IP: "203.0.113.1"}, {Hostname: "lb.example.invalid"}, {},
			}}},
		},
		{ObjectMeta: metav1.ObjectMeta{Name: "internal"}},
	}, now)
	if services[0].ExternalIP != "203.0.113.1,lb.example.invalid" || services[0].Ports != "443/TCP" || services[0].Selector != "app=gateway" || services[1].ExternalIP != projectedNone {
		t.Fatalf("projectServices() = %+v", services)
	}
}

func TestProjectControllerWorkloads(t *testing.T) {
	now := time.Date(2026, time.August, 28, 12, 0, 0, 0, time.UTC)
	statefulSets := projectStatefulSets([]appsv1.StatefulSet{{ObjectMeta: metav1.ObjectMeta{Name: "db"}, Status: appsv1.StatefulSetStatus{ReadyReplicas: 1, Replicas: 2}}}, now)
	daemonSets := projectDaemonSets([]appsv1.DaemonSet{{ObjectMeta: metav1.ObjectMeta{Name: "agent"}, Status: appsv1.DaemonSetStatus{DesiredNumberScheduled: 4, CurrentNumberScheduled: 3, NumberReady: 2, NumberAvailable: 2}}}, now)
	configMaps := projectConfigMaps([]corev1.ConfigMap{{ObjectMeta: metav1.ObjectMeta{Name: "settings"}, Data: map[string]string{"a": "1"}, BinaryData: map[string][]byte{"b": {1}}}}, now)
	if statefulSets[0].Ready != "1/2" || statefulSets[0].Replicas != 2 || daemonSets[0].Desired != 4 || daemonSets[0].Current != 3 || daemonSets[0].Ready != 2 || daemonSets[0].Available != 2 || configMaps[0].Data != 2 {
		t.Fatalf("controller projections = stateful=%+v daemon=%+v config=%+v", statefulSets, daemonSets, configMaps)
	}
}

func TestProjectNodes(t *testing.T) {
	now := time.Date(2026, time.August, 28, 12, 0, 0, 0, time.UTC)
	nodes := projectNodes([]corev1.Node{{
		ObjectMeta: metav1.ObjectMeta{Name: "node-a", Labels: map[string]string{
			"node-role.kubernetes.io/worker": "", "node-role.kubernetes.io/control-plane": "", "other": "value",
		}},
		Spec: corev1.NodeSpec{Unschedulable: true},
		Status: corev1.NodeStatus{
			Conditions: []corev1.NodeCondition{{Type: corev1.NodeMemoryPressure}, {Type: corev1.NodeReady, Status: corev1.ConditionTrue}},
			Addresses:  []corev1.NodeAddress{{Type: corev1.NodeHostName, Address: "node-a"}, {Type: corev1.NodeInternalIP, Address: "10.0.0.10"}},
			NodeInfo:   corev1.NodeSystemInfo{KubeletVersion: "v1.36.1", OSImage: "Linux", KernelVersion: "6.0", ContainerRuntimeVersion: "containerd://2"},
		},
	}}, now)
	if len(nodes) != 1 || nodes[0].Status != "Ready,SchedulingDisabled" || nodes[0].Roles != "control-plane,worker" || nodes[0].InternalIP != "10.0.0.10" || nodes[0].Version != "v1.36.1" {
		t.Fatalf("projectNodes() = %+v", nodes)
	}
}

func TestProjectedNodeHelpers(t *testing.T) {
	if got := projectedNodeStatus([]corev1.NodeCondition{{Type: corev1.NodeReady, Status: corev1.ConditionFalse}}); got != "NotReady" {
		t.Fatalf("projectedNodeStatus(false) = %q", got)
	}
	if got := projectedNodeStatus(nil); got != "Unknown" {
		t.Fatalf("projectedNodeStatus(nil) = %q", got)
	}
	if got := projectedNodeRoles(map[string]string{"other": "value", nodeRoleLabelPrefix: "ignored"}); got != projectedNone {
		t.Fatalf("projectedNodeRoles(no roles) = %q", got)
	}
	if got := projectedNodeInternalIP(nil); got != "" {
		t.Fatalf("projectedNodeInternalIP(nil) = %q", got)
	}
}

func TestProjectJobs(t *testing.T) {
	now := time.Date(2026, time.August, 28, 12, 0, 0, 0, time.UTC)
	start := metav1.NewTime(now.Add(-10 * time.Minute))
	completion := metav1.NewTime(now.Add(-5 * time.Minute))
	two := int32(2)
	jobs := projectJobs([]batchv1.Job{
		{ObjectMeta: metav1.ObjectMeta{Name: "complete"}, Spec: batchv1.JobSpec{Completions: &two}, Status: batchv1.JobStatus{Succeeded: 2, StartTime: &start, CompletionTime: &completion}},
		{ObjectMeta: metav1.ObjectMeta{Name: "failed"}, Status: batchv1.JobStatus{Failed: 1}},
		{ObjectMeta: metav1.ObjectMeta{Name: "running"}, Status: batchv1.JobStatus{Active: 1, StartTime: &start}},
	}, now)
	if jobs[0].Status != "Complete" || jobs[0].Completions != "2/2" || jobs[0].Duration != "5m" || jobs[1].Status != "Failed" || jobs[1].Duration != "-" || jobs[2].Status != "Running" || jobs[2].Completions != "0/1" || jobs[2].Duration != "10m" {
		t.Fatalf("projectJobs() = %+v", jobs)
	}
	future := metav1.NewTime(now.Add(time.Minute))
	if got := projectedJobDuration(batchv1.Job{Status: batchv1.JobStatus{StartTime: &future}}, now); got != "0s" {
		t.Fatalf("projectedJobDuration(future) = %q", got)
	}
}

func TestProjectedTimeAndCollectionHelpers(t *testing.T) {
	now := time.Date(2026, time.August, 28, 12, 0, 0, 0, time.UTC)
	if got := projectedAge(now, time.Time{}); got != projectedUnknown {
		t.Fatalf("projectedAge(zero) = %q", got)
	}
	tests := []struct {
		duration time.Duration
		want     string
	}{
		{duration: -time.Second, want: "0s"},
		{duration: 59 * time.Second, want: "59s"},
		{duration: 59 * time.Minute, want: "59m"},
		{duration: 23 * time.Hour, want: "23h"},
		{duration: 48 * time.Hour, want: "2d"},
	}
	for _, test := range tests {
		if got := projectedDuration(test.duration); got != test.want {
			t.Fatalf("projectedDuration(%s) = %q, want %q", test.duration, got, test.want)
		}
	}
	if got := projectedLabelMap(nil); got != "" {
		t.Fatalf("projectedLabelMap(nil) = %q", got)
	}
	if got := projectedLabelMap(map[string]string{"b": "2", "a": "1"}); got != "a=1,b=2" {
		t.Fatalf("projectedLabelMap() = %q", got)
	}
	values := appendUnique([]string{"one"}, "one")
	values = appendUnique(values, "two")
	if strings.Join(values, ",") != "one,two" {
		t.Fatalf("appendUnique() = %v", values)
	}
	if joinProjectedValues(nil) != projectedNone || joinProjectedValues(values) != "one,two" {
		t.Fatal("joinProjectedValues() did not normalize values")
	}
}

func pointerTo[T any](value T) *T {
	return &value
}
