package ui

import (
	"context"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/HediAbed/opsmate/internal/kube"
)

type testContextManager struct {
	connect        func(context.Context, string) error
	contexts       func(context.Context) ([]kube.ContextInfo, error)
	currentContext func(context.Context) (string, error)
	namespaces     func(context.Context) ([]string, error)
}

type testClusterOperations struct {
	testResourceInspector
	testPodReader
	testResourceWriter
	shellRequest       kube.ShellRequest
	shellSession       kube.ShellSession
	shellErr           error
	portForwardRequest kube.PortForwardRequest
	portForward        kube.PortForward
	portForwardErr     error
	stoppedForwardID   string
	stopForwardErr     error
	stopAllForwardErr  error
	portForwards       []kube.PortForwardSession
	helmReleases       []kube.HelmRelease
	helmValues         string
	helmErr            error
}

type testHelmCommands struct {
	listReleases func(string) tea.Cmd
	getValues    func(kube.HelmReleaseReference) tea.Cmd
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

type testPortForward struct {
	session kube.PortForwardSession
	exit    chan kube.PortForwardExit
}

func (s *testShellSession) Identity() kube.ShellIdentity {
	return s.identity
}

func (s *testShellSession) Send(line string) error {
	s.sent = append(s.sent, line)
	return s.sendErr
}

func (s *testShellSession) Output() <-chan kube.ShellOutput {
	return s.output
}

func (s *testShellSession) Exit() <-chan kube.ShellExit {
	return s.exit
}

func (s *testShellSession) Interrupt() error {
	s.interrupted = true
	return s.interruptErr
}

func (s *testShellSession) Close() {
	s.closed = true
}

func (s *testPortForward) Session() kube.PortForwardSession {
	return s.session
}

func (s *testPortForward) Exit() <-chan kube.PortForwardExit {
	return s.exit
}

func (o *testClusterOperations) StartShell(_ context.Context, request kube.ShellRequest) (kube.ShellSession, error) {
	o.shellRequest = request
	return o.shellSession, o.shellErr
}

func (o *testClusterOperations) StartPortForward(_ context.Context, request kube.PortForwardRequest) (kube.PortForward, error) {
	o.portForwardRequest = request
	return o.portForward, o.portForwardErr
}

func (o *testClusterOperations) StopPortForward(_ context.Context, sessionID string) error {
	o.stoppedForwardID = sessionID
	return o.stopForwardErr
}

func (o *testClusterOperations) StopAllPortForwards(context.Context) error {
	return o.stopAllForwardErr
}

func (o *testClusterOperations) PortForwards() []kube.PortForwardSession {
	return append([]kube.PortForwardSession(nil), o.portForwards...)
}

func (o *testClusterOperations) ListHelmReleases(context.Context, string) ([]kube.HelmRelease, error) {
	return append([]kube.HelmRelease(nil), o.helmReleases...), o.helmErr
}

func (o *testClusterOperations) HelmReleaseValues(context.Context, kube.HelmReleaseReference) (string, error) {
	return o.helmValues, o.helmErr
}

func (c *testHelmCommands) ListReleases(namespace string) tea.Cmd {
	if c != nil && c.listReleases != nil {
		return c.listReleases(namespace)
	}
	return func() tea.Msg { return helmReleasesMsg{} }
}

func (c *testHelmCommands) GetValues(reference kube.HelmReleaseReference) tea.Cmd {
	if c != nil && c.getValues != nil {
		return c.getValues(reference)
	}
	return func() tea.Msg {
		return helmValuesMsg{Release: reference.Name, Namespace: reference.Namespace}
	}
}

func (m *testContextManager) Connect(ctx context.Context, name string) error {
	if m.connect == nil {
		return nil
	}
	return m.connect(ctx, name)
}

func (m *testContextManager) Contexts(ctx context.Context) ([]kube.ContextInfo, error) {
	if m.contexts == nil {
		return nil, nil
	}
	return m.contexts(ctx)
}

func (m *testContextManager) CurrentContext(ctx context.Context) (string, error) {
	if m.currentContext == nil {
		return "test", nil
	}
	return m.currentContext(ctx)
}

func (m *testContextManager) Namespaces(ctx context.Context) ([]string, error) {
	if m.namespaces == nil {
		return nil, nil
	}
	return m.namespaces(ctx)
}

func newTestRootModel(t *testing.T, namespace string) RootModel {
	t.Helper()
	resources := &testResourceReader{}
	operations := &testClusterOperations{}
	root, err := NewRootModel(namespace, RuntimeDependencies{
		Context:           context.Background(),
		ClusterContext:    &testContextManager{},
		ClusterResources:  resources,
		ClusterSnapshots:  &snapshotCollectorStub{},
		ClusterObserver:   &testResourceObserver{},
		ClusterOperations: operations,
		HelmReleases:      operations,
	})
	if err != nil {
		t.Fatalf("NewRootModel() error = %v", err)
	}
	return root
}

func newTestBrowserModel(namespace string) BrowserModel {
	return NewBrowserModel(namespace, newTestClusterCommands(), newTestClusterOperations())
}

func newTestBrowserWithClusterOperations(namespace string, operations *testClusterOperations) BrowserModel {
	adapter := newNativeClusterOperations(context.Background(), operations, operations, operations, operations)
	return NewBrowserModel(namespace, newTestClusterCommands(), adapter)
}

func newTestDashboardModel(namespace string) DashboardModel {
	return NewDashboardModel(namespace, newTestClusterCommands())
}

func newTestLogsModel(namespace string) LogsModel {
	return NewLogsModel(namespace, newTestClusterCommands(), newTestClusterOperations())
}

func newTestCRDsModel(namespace string) CRDsModel {
	return NewCRDsModel(namespace, newTestClusterCommands())
}

func newTestHelmModel(namespace string) HelmModel {
	return NewHelmModel(namespace, &testHelmCommands{})
}

func newTestClusterCommands() clusterCommands {
	return newNativeClusterCommands(context.Background(), &testResourceReader{}, &testResourceObserver{})
}

func newTestClusterOperations() clusterOperations {
	operations := &testClusterOperations{}
	return newNativeClusterOperations(context.Background(), operations, operations, operations, operations)
}

func testModelPortForwardSession(t *testing.T, sessionID, pod string, localPort, remotePort int) kube.PortForwardSession {
	t.Helper()
	local, err := kube.NewNetworkPort(localPort)
	if err != nil {
		t.Fatalf("NewNetworkPort(%d) error = %v", localPort, err)
	}
	remote, err := kube.NewNetworkPort(remotePort)
	if err != nil {
		t.Fatalf("NewNetworkPort(%d) error = %v", remotePort, err)
	}
	return kube.PortForwardSession{
		ID:         sessionID,
		Pod:        kube.PodReference{Namespace: "default", Name: pod},
		LocalPort:  local,
		RemotePort: remote,
		Status:     kube.PortForwardRunning,
	}
}

func installTestRootOperations(root *RootModel, operations *testClusterOperations) {
	adapter := newNativeClusterOperations(context.Background(), operations, operations, operations, operations)
	root.operations = adapter
	root.browser.operations = adapter
	root.logs.operations = adapter
}
