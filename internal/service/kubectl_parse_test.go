package service

import (
	"reflect"
	"testing"
	"time"
)

func TestParseContextsOutput_Empty(t *testing.T) {
	if got := parseContextsOutput(""); len(got) != 0 {
		t.Errorf("empty input → %v; want []", got)
	}
}

func TestParseContextsOutput_SingleCurrent(t *testing.T) {
	input := "*   prod   prod-cluster   admin   kube-system\n"
	got := parseContextsOutput(input)
	want := []KubeContext{
		{Name: "prod", Cluster: "prod-cluster", Namespace: "kube-system", Current: true},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v; want %+v", got, want)
	}
}

func TestParseContextsOutput_MultipleContexts(t *testing.T) {
	input := "" +
		"*   prod   prod-cluster   admin   default\n" +
		"    dev    dev-cluster    user    app-ns\n" +
		"    stage  stage-cluster  user\n"
	got := parseContextsOutput(input)
	if len(got) != 3 {
		t.Fatalf("expected 3 contexts, got %d", len(got))
	}
	if !got[0].Current || got[1].Current || got[2].Current {
		t.Errorf("only the first context should be current: %+v", got)
	}
	if got[2].Namespace != "" {
		t.Errorf("missing NAMESPACE column should leave Namespace empty, got %q", got[2].Namespace)
	}
}

func TestParsePodMetrics_SingleNamespace(t *testing.T) {
	input := "my-pod   120m   64Mi\nother    10m    8Mi\n"
	got := parsePodMetrics(input, "default")
	want := []PodMetric{
		{Name: "my-pod", Namespace: "default", CPU: "120m", Memory: "64Mi"},
		{Name: "other", Namespace: "default", CPU: "10m", Memory: "8Mi"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v; want %+v", got, want)
	}
}

func TestParsePodMetrics_AllNamespaces(t *testing.T) {
	input := "kube-system   coredns   5m   20Mi\n"
	got := parsePodMetrics(input, "")
	want := []PodMetric{{Name: "coredns", Namespace: "kube-system", CPU: "5m", Memory: "20Mi"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v; want %+v", got, want)
	}
}

func TestParsePodMetrics_SkipsMalformedLines(t *testing.T) {
	input := "short\nalso short\nok 100m 32Mi\n"
	got := parsePodMetrics(input, "default")
	if len(got) != 1 {
		t.Fatalf("expected 1 metric, got %d: %+v", len(got), got)
	}
	if got[0].Name != "ok" {
		t.Errorf("unexpected parsed row: %+v", got[0])
	}
}

func TestPodDisplayStatus_UsesContainerWaitingReason(t *testing.T) {
	var pod rawPod
	pod.Status.Phase = "Running"
	pod.Status.ContainerStatuses = make([]rawContainerStatus, 1)
	pod.Status.ContainerStatuses[0].State.Waiting = &struct {
		Reason string `json:"reason"`
	}{Reason: "CrashLoopBackOff"}

	if got := podDisplayStatus(pod); got != "CrashLoopBackOff" {
		t.Fatalf("status = %q, want CrashLoopBackOff", got)
	}
}

func TestPodDisplayStatus_UsesImagePullFailure(t *testing.T) {
	var pod rawPod
	pod.Status.Phase = "Pending"
	pod.Status.ContainerStatuses = make([]rawContainerStatus, 1)
	pod.Status.ContainerStatuses[0].State.Waiting = &struct {
		Reason string `json:"reason"`
	}{Reason: "ImagePullBackOff"}

	if got := podDisplayStatus(pod); got != "ImagePullBackOff" {
		t.Fatalf("status = %q, want ImagePullBackOff", got)
	}
}

func TestPodDisplayStatus_ReportsTerminating(t *testing.T) {
	now := time.Now()
	var pod rawPod
	pod.Metadata.DeletionTimestamp = &now
	pod.Status.Phase = "Running"

	if got := podDisplayStatus(pod); got != "Terminating" {
		t.Fatalf("status = %q, want Terminating", got)
	}
}

func TestPodDisplayStatus_ReportsFailedInitContainer(t *testing.T) {
	var pod rawPod
	pod.Status.InitContainerStatuses = make([]rawContainerStatus, 1)
	pod.Status.InitContainerStatuses[0].State.Terminated = &struct {
		ExitCode int    `json:"exitCode"`
		Reason   string `json:"reason"`
	}{ExitCode: 2, Reason: "Error"}

	if got := podDisplayStatus(pod); got != "Init:Error" {
		t.Fatalf("status = %q, want Init:Error", got)
	}
}

func TestNodeStatusFromConditions_Ready(t *testing.T) {
	got := nodeStatusFromConditions([]nodeCondition{{Type: "Ready", Status: "True"}})
	if got != "Ready" {
		t.Errorf("got %q; want Ready", got)
	}
}

func TestNodeStatusFromConditions_NotReady(t *testing.T) {
	got := nodeStatusFromConditions([]nodeCondition{
		{Type: "MemoryPressure", Status: "False"},
		{Type: "Ready", Status: "False"},
	})
	if got != "NotReady" {
		t.Errorf("got %q; want NotReady", got)
	}
}

func TestNodeStatusFromConditions_Missing(t *testing.T) {
	got := nodeStatusFromConditions([]nodeCondition{{Type: "DiskPressure", Status: "False"}})
	if got != "Unknown" {
		t.Errorf("got %q; want Unknown", got)
	}
}

func TestNodeRolesFromLabels_None(t *testing.T) {
	got := nodeRolesFromLabels(map[string]string{"foo": "bar"})
	if got != "<none>" {
		t.Errorf("got %q; want <none>", got)
	}
}

func TestNodeRolesFromLabels_Single(t *testing.T) {
	got := nodeRolesFromLabels(map[string]string{
		"node-role.kubernetes.io/control-plane": "",
	})
	if got != "control-plane" {
		t.Errorf("got %q; want control-plane", got)
	}
}

func TestNodeRolesFromLabels_IgnoresEmptyRole(t *testing.T) {
	got := nodeRolesFromLabels(map[string]string{
		"node-role.kubernetes.io/":       "",
		"node-role.kubernetes.io/worker": "",
	})
	if got != "worker" {
		t.Errorf("got %q; want worker (empty role must be skipped)", got)
	}
}
