package browser

import (
	"sync"
	"sync/atomic"

	tea "charm.land/bubbletea/v2"

	clustermodel "github.com/HediAbed/opsmate/internal/cluster"
	"github.com/HediAbed/opsmate/internal/kube"
	clusterui "github.com/HediAbed/opsmate/internal/ui/cluster"
)

type errStub string

func (err errStub) Error() string {
	return string(err)
}

type testCommands struct {
	clusterui.Commands
	observeErr error
	calls      []string
}

func messageCommand(message tea.Msg) tea.Cmd {
	return func() tea.Msg { return message }
}

func (*testCommands) FetchPods(string) tea.Cmd {
	return messageCommand(clustermodel.PodsMsg{})
}

func (*testCommands) FetchDeployments(string) tea.Cmd {
	return messageCommand(clustermodel.DeploymentsMsg{})
}

func (*testCommands) FetchEvents(string) tea.Cmd {
	return messageCommand(clustermodel.EventsMsg{})
}

func (*testCommands) FetchPodMetrics(string) tea.Cmd {
	return messageCommand(clustermodel.MetricsMsg{})
}

func (*testCommands) FetchServices(string) tea.Cmd {
	return messageCommand(clustermodel.ServicesMsg{})
}

func (*testCommands) FetchStatefulSets(string) tea.Cmd {
	return messageCommand(clustermodel.StatefulSetsMsg{})
}

func (*testCommands) FetchDaemonSets(string) tea.Cmd {
	return messageCommand(clustermodel.DaemonSetsMsg{})
}

func (*testCommands) FetchConfigMaps(string) tea.Cmd {
	return messageCommand(clustermodel.ConfigMapsMsg{})
}

func (*testCommands) FetchNodes() tea.Cmd {
	return messageCommand(clustermodel.NodesMsg{})
}

func (*testCommands) FetchJobs(string) tea.Cmd {
	return messageCommand(clustermodel.JobsMsg{})
}

func (*testCommands) FetchIngresses(string) tea.Cmd {
	return messageCommand(clustermodel.IngressesMsg{})
}

func (*testCommands) FetchNetworkPolicies(string) tea.Cmd {
	return messageCommand(clustermodel.NetworkPoliciesMsg{})
}

func (*testCommands) FetchPVCs(string) tea.Cmd {
	return messageCommand(clustermodel.PVCsMsg{})
}

func (*testCommands) FetchCronJobs(string) tea.Cmd {
	return messageCommand(clustermodel.CronJobsMsg{})
}

func (*testCommands) FetchHPAs(string) tea.Cmd {
	return messageCommand(clustermodel.HPAsMsg{})
}

func (*testCommands) FetchSecrets(string) tea.Cmd {
	return messageCommand(clustermodel.SecretsMsg{})
}

func (*testCommands) FetchReplicaSets(string) tea.Cmd {
	return messageCommand(clustermodel.ReplicaSetsMsg{})
}

func (*testCommands) FetchRBAC(string) tea.Cmd {
	return messageCommand(clustermodel.RBACMsg{})
}

func (*testCommands) FetchCRDs() tea.Cmd {
	return messageCommand(clustermodel.CRDsMsg{})
}

func (*testCommands) FetchCRDInstances(clustermodel.CRD, string) tea.Cmd {
	return messageCommand(clustermodel.CRDInstancesMsg{})
}

func observeTestResource[T interface{}](
	commands *testCommands,
	label string,
	item T,
) (clusterui.LiveSet[T], error) {
	commands.calls = append(commands.calls, label)
	if commands.observeErr != nil {
		return nil, commands.observeErr
	}
	set := newTestResourceLiveSet(clusterui.LiveState[T]{Items: []T{item}, Ready: true})
	set.changes <- struct{}{}
	return set, nil
}

func (commands *testCommands) ObservePods(string) (clusterui.LiveSet[clustermodel.Pod], error) {
	return observeTestResource(commands, "pods", clustermodel.Pod{Name: "observed-pod", Namespace: "payments"})
}

func (commands *testCommands) ObserveDeployments(string) (clusterui.LiveSet[clustermodel.Deployment], error) {
	return observeTestResource(commands, "deployments", clustermodel.Deployment{Name: "observed-deployment", Namespace: "payments"})
}

func (commands *testCommands) ObserveEvents(string) (clusterui.LiveSet[clustermodel.Event], error) {
	return observeTestResource(commands, "events", clustermodel.Event{Name: "observed-event", Namespace: "payments"})
}

func (commands *testCommands) ObserveIngresses(string) (clusterui.LiveSet[clustermodel.Ingress], error) {
	return observeTestResource(commands, "ingresses", clustermodel.Ingress{Name: "observed-ingress", Namespace: "payments"})
}

func (commands *testCommands) ObserveNetworkPolicies(string) (clusterui.LiveSet[clustermodel.NetworkPolicy], error) {
	return observeTestResource(commands, "network policies", clustermodel.NetworkPolicy{Name: "observed-network-policy", Namespace: "payments"})
}

func (commands *testCommands) ObservePersistentVolumeClaims(string) (clusterui.LiveSet[clustermodel.PersistentVolumeClaim], error) {
	return observeTestResource(commands, "persistent volume claims", clustermodel.PersistentVolumeClaim{Name: "observed-pvc", Namespace: "payments"})
}

func (commands *testCommands) ObserveCronJobs(string) (clusterui.LiveSet[clustermodel.CronJob], error) {
	return observeTestResource(commands, "cron jobs", clustermodel.CronJob{Name: "observed-cron-job", Namespace: "payments"})
}

func (commands *testCommands) ObserveHorizontalPodAutoscalers(string) (clusterui.LiveSet[clustermodel.HPA], error) {
	return observeTestResource(commands, "horizontal pod autoscalers", clustermodel.HPA{Name: "observed-hpa", Namespace: "payments"})
}

func (commands *testCommands) ObserveSecrets(string) (clusterui.LiveSet[clustermodel.Secret], error) {
	return observeTestResource(commands, "secrets", clustermodel.Secret{Name: "observed-secret", Namespace: "payments"})
}

func (commands *testCommands) ObserveReplicaSets(string) (clusterui.LiveSet[clustermodel.ReplicaSet], error) {
	return observeTestResource(commands, "replica sets", clustermodel.ReplicaSet{Name: "observed-replica-set", Namespace: "payments"})
}

type testResourceLiveSet[T interface{}] struct {
	changes  chan struct{}
	state    clusterui.LiveState[T]
	stopOnce sync.Once
	stops    atomic.Int32
}

func newTestResourceLiveSet[T interface{}](state clusterui.LiveState[T]) *testResourceLiveSet[T] {
	return &testResourceLiveSet[T]{changes: make(chan struct{}, 1), state: state}
}

func (set *testResourceLiveSet[T]) Changes() <-chan struct{} {
	return set.changes
}

func (set *testResourceLiveSet[T]) State() clusterui.LiveState[T] {
	return set.state
}

func (set *testResourceLiveSet[T]) Stop() {
	set.stopOnce.Do(func() {
		set.stops.Add(1)
		close(set.changes)
	})
}

type testClusterOperations struct {
	clusterui.Operations
	shellSession kube.ShellSession
	shellErr     error
}

func (*testClusterOperations) InspectResource(kube.ResourceReference) tea.Cmd {
	return messageCommand(clustermodel.DescribeMsg{})
}

func (*testClusterOperations) ResourceYAML(kube.ResourceReference) tea.Cmd {
	return messageCommand(clustermodel.YAMLMsg{})
}

func (*testClusterOperations) FetchPodLogs(kube.PodLogRequest) tea.Cmd {
	return messageCommand(clustermodel.LogsMsg{})
}

func (*testClusterOperations) FetchPodContainers(kube.PodReference) tea.Cmd {
	return messageCommand(clustermodel.ContainersMsg{})
}

func (*testClusterOperations) ScaleWorkload(kube.ScaleRequest) tea.Cmd {
	return messageCommand(clustermodel.MutationResultMsg{})
}

func (*testClusterOperations) DeleteResource(kube.ResourceReference) tea.Cmd {
	return messageCommand(clustermodel.MutationResultMsg{})
}

func (*testClusterOperations) DeleteResources(kube.ResourceBatch) tea.Cmd {
	return messageCommand(clustermodel.MutationResultMsg{})
}

func (*testClusterOperations) RestartWorkload(kube.WorkloadReference) tea.Cmd {
	return messageCommand(clustermodel.MutationResultMsg{})
}

func (*testClusterOperations) RestartWorkloads(kube.WorkloadBatch) tea.Cmd {
	return messageCommand(clustermodel.MutationResultMsg{})
}

func (operations *testClusterOperations) StartShell(kube.ShellRequest) (kube.ShellSession, error) {
	return operations.shellSession, operations.shellErr
}

func (*testClusterOperations) StartPortForward(kube.PortForwardRequest) tea.Cmd {
	return messageCommand(clusterui.PortForwardStartedMsg{})
}

func (*testClusterOperations) StopPortForward(string) tea.Cmd {
	return messageCommand(clusterui.PortForwardStoppedMsg{})
}

func (*testClusterOperations) PortForwards() []kube.PortForwardSession {
	return nil
}

func (*testClusterOperations) WaitForPortForwardExit(kube.PortForward) tea.Cmd {
	return nil
}

type testShellSession struct {
	identity     kube.ShellIdentity
	output       chan kube.ShellOutput
	exit         chan kube.ShellExit
	sent         []string
	sendErr      error
	interruptErr error
	interrupted  bool
	closed       bool
}

func (session *testShellSession) Identity() kube.ShellIdentity {
	return session.identity
}

func (session *testShellSession) Send(line string) error {
	session.sent = append(session.sent, line)
	return session.sendErr
}

func (session *testShellSession) Output() <-chan kube.ShellOutput {
	return session.output
}

func (session *testShellSession) Exit() <-chan kube.ShellExit {
	return session.exit
}

func (session *testShellSession) Interrupt() error {
	session.interrupted = true
	return session.interruptErr
}

func (session *testShellSession) Close() {
	session.closed = true
}

func newTestBrowserModel(namespace string) BrowserModel {
	return NewBrowserModel(namespace, &testCommands{}, &testClusterOperations{})
}

func newTestBrowserWithClusterOperations(namespace string, operations *testClusterOperations) BrowserModel {
	return NewBrowserModel(namespace, &testCommands{}, operations)
}

func newTestClusterCommands() clusterui.Commands {
	return &testCommands{}
}

func newTestClusterOperations() clusterui.Operations {
	return &testClusterOperations{}
}
