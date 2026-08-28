//go:build !windows

package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestParseContextsOutputSkipsBareCurrentMarker(t *testing.T) {
	contexts := parseContextsOutput("*\nvalid cluster user namespace\n")
	if len(contexts) != 1 || contexts[0].Name != "valid" {
		t.Fatalf("contexts = %#v, want only valid row", contexts)
	}
}

func TestPodProjectionHandlesContainersAndIncompleteStatuses(t *testing.T) {
	var item rawPod
	item.Spec.Containers = append(item.Spec.Containers, struct {
		Name string `json:"name"`
	}{Name: "main"})
	pod := podFromRaw(item)
	if len(pod.Containers) != 1 || pod.Containers[0] != "main" || pod.Status != "Unknown" {
		t.Fatalf("pod = %#v", pod)
	}

	var waiting rawContainerStatus
	waiting.State.Waiting = &struct {
		Reason string `json:"reason"`
	}{Reason: "Preparing"}
	if got := initContainerStatus([]rawContainerStatus{waiting}); got != "Init:Preparing" {
		t.Fatalf("waiting status = %q", got)
	}

	var terminated rawContainerStatus
	terminated.State.Terminated = &struct {
		ExitCode int    `json:"exitCode"`
		Reason   string `json:"reason"`
	}{ExitCode: 3}
	if got := initContainerStatus([]rawContainerStatus{terminated}); got != "Init:ExitCode3" {
		t.Fatalf("terminated status = %q", got)
	}
	if got := initContainerStatus([]rawContainerStatus{{}}); got != "Init:0/1" {
		t.Fatalf("active status = %q", got)
	}
}

func TestFetchDeploymentsProjectsContainerMetadata(t *testing.T) {
	withFakePathKubectl(t, `printf '%s' '{"items":[{"metadata":{"name":"web","namespace":"ns"},"spec":{"template":{"spec":{"containers":[{"name":"main","image":"web:v1"}]}}},"status":{"replicas":1}}]}'`)

	message := FetchDeployments("ns")().(DeploymentsMsg)

	if message.Err != nil || len(message.Deployments) != 1 {
		t.Fatalf("message = %#v", message)
	}
	deployment := message.Deployments[0]
	if len(deployment.Containers) != 1 || deployment.Containers[0] != "main" || deployment.Images[0] != "web:v1" {
		t.Fatalf("deployment = %#v", deployment)
	}
}

func TestFetchEventsKeepsNewestEntriesInReverseOrder(t *testing.T) {
	var output strings.Builder
	output.WriteString(`{"items":[`)
	for index := range maximumRecentEvents + 1 {
		if index > 0 {
			output.WriteByte(',')
		}
		fmt.Fprintf(&output, `{"metadata":{"name":"event-%02d"}}`, index)
	}
	output.WriteString(`]}`)
	withFakePathKubectl(t, "printf '%s' '"+output.String()+"'")

	message := FetchEvents("ns")().(EventsMsg)

	if message.Err != nil || len(message.Events) != maximumRecentEvents {
		t.Fatalf("message = %#v", message)
	}
	if message.Events[0].Name != "event-50" || message.Events[len(message.Events)-1].Name != "event-01" {
		t.Fatalf("event order = %q ... %q", message.Events[0].Name, message.Events[len(message.Events)-1].Name)
	}
}

func TestKubectlActionsPreserveExecutionFailures(t *testing.T) {
	withFakePathKubectl(t, `printf 'command failed' >&2; exit 1`)

	results := []error{
		FetchContainerLogs("ns", "pod", "", 10)().(LogsMsg).Err,
		FetchContainers("ns", "pod")().(ContainersMsg).Err,
		ScaleResource("ns", "deployment", "web", 2)().(CommandResultMsg).Err,
		DeleteResource("ns", "pod", "web")().(CommandResultMsg).Err,
		DeleteResources("ns", "pod", []string{"a", "b"})().(CommandResultMsg).Err,
		GetYAML("ns", "pod", "web")().(YAMLMsg).Err,
		RestartRollout("ns", "deployment", "web")().(CommandResultMsg).Err,
		RestartRollouts("ns", "deployment", []string{"a", "b"})().(CommandResultMsg).Err,
		ExecuteCommand("kubectl get pods")().(CommandResultMsg).Err,
	}
	for index, err := range results {
		if err == nil || !strings.Contains(err.Error(), "command failed") {
			t.Errorf("result %d error = %v", index, err)
		}
	}
}

func TestServiceFetcherProjectsHostnameLoadBalancer(t *testing.T) {
	withFakePathKubectl(t, `printf '%s' '{"items":[{"metadata":{"name":"service","namespace":"ns"},"spec":{},"status":{"loadBalancer":{"ingress":[{"hostname":"edge.invalid"}]}}}]}'`)
	services, err := listServicesSync("ns")
	if err != nil || len(services) != 1 || services[0].ExternalIP != "edge.invalid" {
		t.Fatalf("services = %#v, error = %v", services, err)
	}
}

func TestNodeFetcherProjectsInternalAddress(t *testing.T) {
	withFakePathKubectl(t, `printf '%s' '{"items":[{"metadata":{"name":"node"},"status":{"addresses":[{"type":"InternalIP","address":"10.0.0.2"}]}}]}'`)
	nodes := FetchNodes()().(NodesMsg)
	if nodes.Err != nil || len(nodes.Nodes) != 1 || nodes.Nodes[0].InternalIP != "10.0.0.2" {
		t.Fatalf("nodes = %#v", nodes)
	}
}

func TestJobFetcherProjectsFailedStatus(t *testing.T) {
	withFakePathKubectl(t, `printf '%s' '{"items":[{"metadata":{"name":"job","namespace":"ns"},"spec":{"completions":2},"status":{"failed":1,"active":0}}]}'`)
	jobs := FetchJobs("ns")().(JobsMsg)
	if jobs.Err != nil || len(jobs.Jobs) != 1 || jobs.Jobs[0].Status != "Failed" {
		t.Fatalf("jobs = %#v", jobs)
	}
}

func TestJoinIngressHostsDropsBlankHost(t *testing.T) {
	var ingress rawIngressItem
	ingress.Spec.Rules = append(ingress.Spec.Rules, struct {
		Host string `json:"host"`
	}{})
	if got := joinIngressHosts(ingress); got != "" {
		t.Fatalf("hosts = %q, want empty", got)
	}
}

func TestKubectlExecutionHelpersCoverLimitsAndCancellation(t *testing.T) {
	installFakeKubectl(t, "#!/bin/sh\nhead -c 33554433 /dev/zero\n")
	if _, err := runKubectl(time.Second, "get"); !errors.Is(err, ErrKubectlOutputLimit) {
		t.Fatalf("binary output error = %v, want output limit", err)
	}
	if _, err := runKubectlText(time.Second, "get"); !errors.Is(err, ErrKubectlOutputLimit) {
		t.Fatalf("text output error = %v, want output limit", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	sentinel := errors.New("run failed")
	err := kubectlExecutionError(ctx, time.Second, []string{"get"}, "", sentinel)
	if !errors.Is(err, context.Canceled) || errors.Is(err, sentinel) {
		t.Fatalf("canceled execution error = %v", err)
	}
}

func TestListKubectlItemsRejectsMalformedJSON(t *testing.T) {
	installFakeKubectl(t, "#!/bin/sh\nprintf 'not json'\n")
	if _, err := listKubectlItems[rawPod]("pods", "ns"); err == nil {
		t.Fatal("malformed list response did not fail")
	}
}

func TestMeaningfulLinesDropsRetryNoise(t *testing.T) {
	if lines := meaningfulLines("x509: certificate signed by unknown authority\nuseful"); len(lines) != 1 || lines[0] != "useful" {
		t.Fatalf("meaningful lines = %#v", lines)
	}
}
