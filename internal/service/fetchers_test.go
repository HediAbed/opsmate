package service

import (
	"strings"
	"testing"
)

func TestFetchIngresses_HappyPathAndError(t *testing.T) {
	const stdout = `{"items":[
		{"metadata":{"name":"i","namespace":"ns"},"spec":{"rules":[{"host":"x"}]}}
	]}`
	withFakePathKubectl(t, `printf '%s' '`+stdout+`'`)
	msg, _ := FetchIngresses("ns")().(IngressesMsg)
	if msg.Err != nil || len(msg.Ingresses) != 1 || msg.Ingresses[0].Hosts != "x" {
		t.Errorf("ingress projection wrong: %+v", msg)
	}
	withFakePathKubectl(t, `exit 1`)
	if errMsg, _ := FetchIngresses("ns")().(IngressesMsg); errMsg.Err == nil {
		t.Error("expected error")
	}
}

func TestFetchContexts_HappyPathAndError(t *testing.T) {
	withFakePathKubectl(t, `printf '%s\n' "*       active     cluster1   user1   ns1" "        idle       cluster2   user2   ns2"`)
	msg, _ := FetchContexts()().(ContextsMsg)
	if msg.Err != nil {
		t.Fatalf("err: %v", msg.Err)
	}
	if len(msg.Contexts) != 2 {
		t.Errorf("expected 2 contexts, got %d", len(msg.Contexts))
	}
	withFakePathKubectl(t, `exit 1`)
	if errMsg, _ := FetchContexts()().(ContextsMsg); errMsg.Err == nil {
		t.Error("expected error")
	}
}

func TestFetchCurrentContext_HappyPathAndError(t *testing.T) {
	withFakePathKubectl(t, `printf 'my-cluster'`)
	msg, _ := FetchCurrentContext()().(CurrentContextMsg)
	if msg.Err != nil || msg.Name != "my-cluster" {
		t.Errorf("current context wrong: %+v", msg)
	}
	withFakePathKubectl(t, `exit 1`)
	if errMsg, _ := FetchCurrentContext()().(CurrentContextMsg); errMsg.Err == nil {
		t.Error("expected error")
	}
}

func TestSwitchContext_HappyPathAndError(t *testing.T) {
	withFakePathKubectl(t, `exit 0`)
	msg, _ := SwitchContext("target")().(ContextSwitchedMsg)
	if msg.Err != nil {
		t.Errorf("happy path err: %v", msg.Err)
	}
	withFakePathKubectl(t, `printf 'denied' 1>&2; exit 1`)
	if errMsg, _ := SwitchContext("target")().(ContextSwitchedMsg); errMsg.Err == nil {
		t.Error("expected error")
	}
}

func TestFetchNamespaces_HappyPathAndError(t *testing.T) {
	withFakePathKubectl(t, `printf 'default kube-system kube-public'`)
	msg, _ := FetchNamespaces()().(NamespacesMsg)
	if msg.Err != nil {
		t.Fatalf("err: %v", msg.Err)
	}
	if len(msg.Namespaces) != 3 {
		t.Errorf("expected 3 namespaces, got %d", len(msg.Namespaces))
	}
	withFakePathKubectl(t, `exit 1`)
	if errMsg, _ := FetchNamespaces()().(NamespacesMsg); errMsg.Err == nil {
		t.Error("expected error")
	}
}

func TestFetchEvents_HappyPathAndError(t *testing.T) {
	const stdout = `{"items":[
		{"metadata":{"name":"failed-event","namespace":"ns","uid":"event-uid"},"type":"Warning","reason":"Failed","message":"crash","involvedObject":{"name":"p","kind":"Pod"}}
	]}`
	withFakePathKubectl(t, `printf '%s' '`+stdout+`'`)
	msg, _ := FetchEvents("ns")().(EventsMsg)
	if msg.Err != nil {
		t.Fatalf("err: %v", msg.Err)
	}
	if len(msg.Events) != 1 || msg.Events[0].Reason != "Failed" {
		t.Errorf("event projection wrong: %+v", msg.Events)
	}
	if msg.Events[0].Name != "failed-event" || msg.Events[0].UID != "event-uid" || msg.Events[0].Namespace != "ns" {
		t.Errorf("event identity was not preserved: %+v", msg.Events[0])
	}
	withFakePathKubectl(t, `exit 1`)
	if errMsg, _ := FetchEvents("ns")().(EventsMsg); errMsg.Err == nil {
		t.Error("expected error")
	}
}

func TestFetchPodMetrics_HappyPathAndError(t *testing.T) {
	withFakePathKubectl(t, `printf '%s\n' "p1 100m 50Mi"`)
	msg, _ := FetchPodMetrics("ns")().(MetricsMsg)
	if msg.Err != nil {
		t.Fatalf("fetch pod metrics: %v", msg.Err)
	}
	want := PodMetric{Namespace: "ns", Name: "p1", CPU: "100m", Memory: "50Mi"}
	if len(msg.PodMetrics) != 1 || msg.PodMetrics[0] != want {
		t.Fatalf("pod metrics = %+v, want %+v", msg.PodMetrics, want)
	}

	withFakePathKubectl(t, `printf 'metrics unavailable' >&2; exit 1`)
	errorMsg := FetchPodMetrics("ns")().(MetricsMsg)
	if errorMsg.Err == nil || !strings.Contains(errorMsg.Err.Error(), "metrics unavailable") {
		t.Fatalf("error = %v, want metrics failure", errorMsg.Err)
	}
}

func TestFetchPods_HappyPathProjectsContainerStatuses(t *testing.T) {
	const stdout = `{"items":[
		{"metadata":{"name":"p","namespace":"ns"},"status":{"phase":"Running","containerStatuses":[{"ready":true,"restartCount":2},{"ready":false,"restartCount":0}]},"spec":{"nodeName":"n1"}}
	]}`
	withFakePathKubectl(t, `printf '%s' '`+stdout+`'`)
	msg, ok := FetchPods("ns")().(PodsMsg)
	if !ok {
		t.Fatalf("expected PodsMsg, got %T", FetchPods("ns")())
	}
	if msg.Err != nil {
		t.Fatalf("unexpected err: %v", msg.Err)
	}
	if len(msg.Pods) != 1 {
		t.Fatalf("got %d pods, want 1", len(msg.Pods))
	}
	p := msg.Pods[0]
	if p.Status != "Running" {
		t.Errorf("status = %q", p.Status)
	}
	if p.Ready != "1/2" {
		t.Errorf("ready = %q, want 1/2", p.Ready)
	}
	if p.Restarts != 2 {
		t.Errorf("restarts = %d, want 2", p.Restarts)
	}
	if p.Node != "n1" {
		t.Errorf("node = %q, want n1", p.Node)
	}
}

func TestFetchPods_PropagatesKubectlError(t *testing.T) {
	withFakePathKubectl(t, `printf 'pods boom' 1>&2; exit 1`)
	msg, _ := FetchPods("ns")().(PodsMsg)
	if msg.Err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(msg.Err.Error(), "pods boom") {
		t.Errorf("error should preserve stderr; got %v", msg.Err)
	}
}

func TestFetchDeployments_HappyPath(t *testing.T) {
	const stdout = `{"items":[
		{"metadata":{"name":"d","namespace":"ns"},"status":{"readyReplicas":2,"updatedReplicas":3,"availableReplicas":2,"replicas":3}}
	]}`
	withFakePathKubectl(t, `printf '%s' '`+stdout+`'`)
	msg, _ := FetchDeployments("ns")().(DeploymentsMsg)
	if msg.Err != nil {
		t.Fatalf("err: %v", msg.Err)
	}
	if len(msg.Deployments) != 1 || msg.Deployments[0].Ready != "2/3" {
		t.Errorf("deployment projection wrong: %+v", msg.Deployments)
	}
	if msg.Deployments[0].UpToDate != 3 {
		t.Errorf("UpToDate = %d, want 3", msg.Deployments[0].UpToDate)
	}
}

func TestFetchDeployments_Error(t *testing.T) {
	withFakePathKubectl(t, `printf 'denied' 1>&2; exit 1`)
	msg, _ := FetchDeployments("ns")().(DeploymentsMsg)
	if msg.Err == nil {
		t.Error("expected error")
	}
}

func TestFetchServices_HappyPath(t *testing.T) {
	const stdout = `{"items":[
		{"metadata":{"name":"s","namespace":"ns"},"spec":{"type":"ClusterIP","clusterIP":"10.0.0.1","ports":[{"port":80,"protocol":"TCP"},{"port":443,"protocol":"TCP"}]}}
	]}`
	withFakePathKubectl(t, `printf '%s' '`+stdout+`'`)
	msg, _ := FetchServices("ns")().(ServicesMsg)
	if msg.Err != nil {
		t.Fatalf("err: %v", msg.Err)
	}
	if len(msg.Services) != 1 || msg.Services[0].ClusterIP != "10.0.0.1" {
		t.Errorf("service projection wrong: %+v", msg.Services)
	}
	if msg.Services[0].Ports != "80/TCP,443/TCP" {
		t.Errorf("ports join wrong: %q", msg.Services[0].Ports)
	}
}

func TestFetchServices_Error(t *testing.T) {
	withFakePathKubectl(t, `exit 1`)
	msg, _ := FetchServices("ns")().(ServicesMsg)
	if msg.Err == nil {
		t.Error("expected error")
	}
}

func TestFetchStatefulSets_HappyPathAndError(t *testing.T) {
	const stdout = `{"items":[{"metadata":{"name":"ss","namespace":"ns"},"status":{"readyReplicas":1,"replicas":3}}]}`
	withFakePathKubectl(t, `printf '%s' '`+stdout+`'`)
	msg, _ := FetchStatefulSets("ns")().(StatefulSetsMsg)
	if msg.Err != nil || msg.StatefulSets[0].Ready != "1/3" {
		t.Errorf("statefulset projection wrong: %+v", msg)
	}
	withFakePathKubectl(t, `exit 1`)
	if errMsg, _ := FetchStatefulSets("ns")().(StatefulSetsMsg); errMsg.Err == nil {
		t.Error("expected error")
	}
}

func TestFetchDaemonSets_HappyPathAndError(t *testing.T) {
	const stdout = `{"items":[{"metadata":{"name":"d","namespace":"ns"},"status":{"desiredNumberScheduled":3,"currentNumberScheduled":2,"numberReady":2,"numberAvailable":2}}]}`
	withFakePathKubectl(t, `printf '%s' '`+stdout+`'`)
	msg, _ := FetchDaemonSets("ns")().(DaemonSetsMsg)
	if msg.Err != nil {
		t.Fatalf("err: %v", msg.Err)
	}
	d := msg.DaemonSets[0]
	if d.Desired != 3 || d.Current != 2 || d.Ready != 2 {
		t.Errorf("daemonset projection wrong: %+v", d)
	}
	withFakePathKubectl(t, `exit 1`)
	if errMsg, _ := FetchDaemonSets("ns")().(DaemonSetsMsg); errMsg.Err == nil {
		t.Error("expected error")
	}
}

func TestFetchConfigMaps_HappyPathAndError(t *testing.T) {
	const stdout = `{"items":[{"metadata":{"name":"c","namespace":"ns"},"data":{"a":"1","b":"2","c":"3"}}]}`
	withFakePathKubectl(t, `printf '%s' '`+stdout+`'`)
	msg, _ := FetchConfigMaps("ns")().(ConfigMapsMsg)
	if msg.Err != nil || msg.ConfigMaps[0].Data != 3 {
		t.Errorf("configmap data count wrong: %+v", msg)
	}
	withFakePathKubectl(t, `exit 1`)
	if errMsg, _ := FetchConfigMaps("ns")().(ConfigMapsMsg); errMsg.Err == nil {
		t.Error("expected error")
	}
}

func TestFetchNodes_HappyPathAndError(t *testing.T) {
	const stdout = `{"items":[
		{"metadata":{"name":"n1","labels":{"node-role.kubernetes.io/control-plane":""}},"status":{"conditions":[{"type":"Ready","status":"True"}],"nodeInfo":{"kubeletVersion":"v1.30.0"}}}
	]}`
	withFakePathKubectl(t, `printf '%s' '`+stdout+`'`)
	msg, _ := FetchNodes()().(NodesMsg)
	if msg.Err != nil {
		t.Fatalf("err: %v", msg.Err)
	}
	if len(msg.Nodes) != 1 || msg.Nodes[0].Status != "Ready" {
		t.Errorf("node projection wrong: %+v", msg.Nodes)
	}
	if !strings.Contains(msg.Nodes[0].Roles, "control-plane") {
		t.Errorf("roles should extract control-plane; got %q", msg.Nodes[0].Roles)
	}
	if msg.Nodes[0].Version != "v1.30.0" {
		t.Errorf("version = %q", msg.Nodes[0].Version)
	}

	withFakePathKubectl(t, `exit 1`)
	if errMsg, _ := FetchNodes()().(NodesMsg); errMsg.Err == nil {
		t.Error("expected error")
	}
}

func TestFetchJobs_HappyPathAndError(t *testing.T) {
	const stdout = `{"items":[
		{"metadata":{"name":"j","namespace":"ns"},"spec":{"completions":1},"status":{"succeeded":1,"startTime":"2026-01-01T00:00:00Z","completionTime":"2026-01-01T00:01:00Z"}}
	]}`
	withFakePathKubectl(t, `printf '%s' '`+stdout+`'`)
	msg, _ := FetchJobs("ns")().(JobsMsg)
	if msg.Err != nil {
		t.Fatalf("err: %v", msg.Err)
	}
	if len(msg.Jobs) != 1 {
		t.Fatalf("expected 1 job, got %d", len(msg.Jobs))
	}
	withFakePathKubectl(t, `exit 1`)
	if errMsg, _ := FetchJobs("ns")().(JobsMsg); errMsg.Err == nil {
		t.Error("expected error")
	}
}
