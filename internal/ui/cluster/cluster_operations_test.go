package cluster

import (
	"context"
	"errors"
	"io"
	"slices"
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/runtime/schema"

	model "github.com/HediAbed/opsmate/internal/cluster"
	"github.com/HediAbed/opsmate/internal/kube"
)

const (
	resourceTypeConfigMaps = "configmaps"
	resourceTypePods       = "pods"
)

type testResourceInspector struct {
	reference kube.ResourceReference
	content   string
	err       error
}

func (i *testResourceInspector) ResourceYAML(_ context.Context, reference kube.ResourceReference) (string, error) {
	i.reference = reference
	return i.content, i.err
}

type testPodReader struct {
	logRequest         kube.PodLogRequest
	logStream          io.ReadCloser
	logErr             error
	containerReference kube.PodReference
	containers         []string
	containerErr       error
}

func (r *testPodReader) OpenPodLogs(_ context.Context, request kube.PodLogRequest) (io.ReadCloser, error) {
	r.logRequest = request
	return r.logStream, r.logErr
}

func (r *testPodReader) PodContainers(_ context.Context, reference kube.PodReference) ([]string, error) {
	r.containerReference = reference
	return r.containers, r.containerErr
}

type testResourceWriter struct {
	scaleRequest   kube.ScaleRequest
	scaleErr       error
	deleted        kube.ResourceReference
	deleteErr      error
	deleteBatch    kube.ResourceBatch
	deleteOutcome  kube.BatchOutcome
	deleteManyErr  error
	restarted      kube.WorkloadReference
	restartErr     error
	restartBatch   kube.WorkloadBatch
	restartOutcome kube.BatchOutcome
	restartManyErr error
}

type testSessionOperations struct {
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
	portForwards       []kube.PortForwardSession
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

func (o *testSessionOperations) StartShell(_ context.Context, request kube.ShellRequest) (kube.ShellSession, error) {
	o.shellRequest = request
	return o.shellSession, o.shellErr
}

func (o *testSessionOperations) StartPortForward(_ context.Context, request kube.PortForwardRequest) (kube.PortForward, error) {
	o.portForwardRequest = request
	return o.portForward, o.portForwardErr
}

func (o *testSessionOperations) StopPortForward(_ context.Context, sessionID string) error {
	o.stoppedForwardID = sessionID
	return o.stopForwardErr
}

func (*testSessionOperations) StopAllPortForwards(context.Context) error {
	return nil
}

func (o *testSessionOperations) PortForwards() []kube.PortForwardSession {
	return append([]kube.PortForwardSession(nil), o.portForwards...)
}

func (w *testResourceWriter) Scale(_ context.Context, request kube.ScaleRequest) error {
	w.scaleRequest = request
	return w.scaleErr
}

func (w *testResourceWriter) Delete(_ context.Context, reference kube.ResourceReference) error {
	w.deleted = reference
	return w.deleteErr
}

func (w *testResourceWriter) DeleteBatch(_ context.Context, batch kube.ResourceBatch) (kube.BatchOutcome, error) {
	w.deleteBatch = batch
	return w.deleteOutcome, w.deleteManyErr
}

func (w *testResourceWriter) Restart(_ context.Context, reference kube.WorkloadReference) error {
	w.restarted = reference
	return w.restartErr
}

func (w *testResourceWriter) RestartBatch(_ context.Context, batch kube.WorkloadBatch) (kube.BatchOutcome, error) {
	w.restartBatch = batch
	return w.restartOutcome, w.restartManyErr
}

type testLogReadCloser struct {
	reader     io.Reader
	closeCalls int
	closeErr   error
}

func (s *testLogReadCloser) Read(buffer []byte) (int, error) {
	return s.reader.Read(buffer)
}

func (s *testLogReadCloser) Close() error {
	s.closeCalls++
	return s.closeErr
}

func TestNativeClusterOperationsReadResourceContent(t *testing.T) {
	reference := kube.ResourceReference{
		Resource:  schema.GroupResource{Resource: resourceTypeConfigMaps},
		Namespace: "team-a",
		Name:      "settings",
	}
	sentinel := errors.New("read failed")
	tests := []struct {
		name    string
		content string
		err     error
		inspect bool
	}{
		{name: "inspect success", content: "kind: ConfigMap", inspect: true},
		{name: "inspect failure", err: sentinel, inspect: true},
		{name: "yaml success", content: "kind: ConfigMap"},
		{name: "yaml failure", err: sentinel},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			inspector := &testResourceInspector{content: test.content, err: test.err}
			operations := NewOperations(context.Background(), inspector, &testPodReader{}, &testResourceWriter{})
			if test.inspect {
				message := operations.InspectResource(reference)().(model.DescribeMsg)
				if message.Output != test.content || !errors.Is(message.Err, test.err) {
					t.Fatalf("InspectResource() = %+v", message)
				}
			} else {
				message := operations.ResourceYAML(reference)().(model.YAMLMsg)
				if message.Output != test.content || !errors.Is(message.Err, test.err) {
					t.Fatalf("ResourceYAML() = %+v", message)
				}
			}
			if inspector.reference != reference {
				t.Fatalf("reference = %+v, want %+v", inspector.reference, reference)
			}
		})
	}
}

func TestNativeClusterOperationsFetchPodLogs(t *testing.T) {
	request := kube.PodLogRequest{
		Pod:       kube.PodReference{Namespace: "team-a", Name: "web"},
		Container: "app",
		TailLines: 200,
	}
	sentinel := errors.New("open failed")
	reader := &testPodReader{logErr: sentinel}
	operations := NewOperations(context.Background(), &testResourceInspector{}, reader, &testResourceWriter{})
	message := operations.FetchPodLogs(request)().(model.LogsMsg)
	if message.Lines != nil || !errors.Is(message.Err, sentinel) || reader.logRequest != request {
		t.Fatalf("FetchPodLogs(open failure) = %+v, request = %+v", message, reader.logRequest)
	}

	reader = &testPodReader{}
	operations = NewOperations(context.Background(), &testResourceInspector{}, reader, &testResourceWriter{})
	message = operations.FetchPodLogs(request)().(model.LogsMsg)
	if message.Lines != nil || !errors.Is(message.Err, kube.ErrPodLogStreamUnavailable) {
		t.Fatalf("FetchPodLogs(empty stream) = %+v", message)
	}

	stream := &testLogReadCloser{reader: strings.NewReader("first\nsecond\n")}
	reader = &testPodReader{logStream: stream}
	operations = NewOperations(context.Background(), &testResourceInspector{}, reader, &testResourceWriter{})
	message = operations.FetchPodLogs(request)().(model.LogsMsg)
	if message.Err != nil || !slices.Equal(message.Lines, []string{"first", "second"}) || stream.closeCalls != 1 {
		t.Fatalf("FetchPodLogs(success) = %+v, close calls = %d", message, stream.closeCalls)
	}
}

func TestNativeClusterOperationsReportsLogReadAndCloseFailures(t *testing.T) {
	closeErr := errors.New("close failed")
	oversizedLine := strings.Repeat("x", maximumLogLineBytes+1)
	stream := &testLogReadCloser{reader: strings.NewReader(oversizedLine), closeErr: closeErr}
	reader := &testPodReader{logStream: stream}
	operations := NewOperations(context.Background(), &testResourceInspector{}, reader, &testResourceWriter{})
	message := operations.FetchPodLogs(kube.PodLogRequest{Pod: kube.PodReference{Namespace: "team-a", Name: "web"}, TailLines: 1})().(model.LogsMsg)
	if message.Err == nil || !errors.Is(message.Err, closeErr) || stream.closeCalls != 1 {
		t.Fatalf("FetchPodLogs() = %+v, close calls = %d", message, stream.closeCalls)
	}
	if len(message.Lines) != 0 {
		t.Fatalf("partial lines = %v, want empty", message.Lines)
	}
}

func TestNativeClusterOperationsFetchPodContainers(t *testing.T) {
	reference := kube.PodReference{Namespace: "team-a", Name: "web"}
	sentinel := errors.New("container read failed")
	tests := []struct {
		name       string
		containers []string
		err        error
	}{
		{name: "success", containers: []string{"app", "proxy"}},
		{name: "failure", err: sentinel},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			reader := &testPodReader{containers: test.containers, containerErr: test.err}
			operations := NewOperations(context.Background(), &testResourceInspector{}, reader, &testResourceWriter{})
			message := operations.FetchPodContainers(reference)().(model.ContainersMsg)
			if !slices.Equal(message.Containers, test.containers) || !errors.Is(message.Err, test.err) || reader.containerReference != reference {
				t.Fatalf("FetchPodContainers() = %+v, reference = %+v", message, reader.containerReference)
			}
		})
	}
}

type resourceMutationCase struct {
	name            string
	run             func() model.MutationResultMsg
	wantOutput      string
	recordedRequest func() bool
}

func resourceMutationCases(operations ResourceOperations, writer *testResourceWriter) []resourceMutationCase {
	scaleRequest := kube.ScaleRequest{
		Workload: kube.WorkloadReference{Kind: kube.WorkloadDeployment, Namespace: "team-a", Name: "web"},
		Replicas: 3,
	}
	deleteReference := kube.ResourceReference{
		Resource:  schema.GroupResource{Resource: resourceTypePods},
		Namespace: "team-a",
		Name:      "web",
	}
	deleteBatch := kube.ResourceBatch{
		Resource:  schema.GroupResource{Resource: resourceTypePods},
		Namespace: "team-a",
		Names:     []string{"web", "worker"},
	}
	restartReference := kube.WorkloadReference{Kind: kube.WorkloadStatefulSet, Namespace: "team-a", Name: "database"}
	restartBatch := kube.WorkloadBatch{Kind: kube.WorkloadDeployment, Namespace: "team-a", Names: []string{"web", "api"}}
	return []resourceMutationCase{
		{
			name: "scale",
			run: func() model.MutationResultMsg {
				return operations.ScaleWorkload(scaleRequest)().(model.MutationResultMsg)
			},
			wantOutput:      "Scaled team-a/deployment/web",
			recordedRequest: func() bool { return writer.scaleRequest == scaleRequest },
		},
		{
			name: "delete",
			run: func() model.MutationResultMsg {
				return operations.DeleteResource(deleteReference)().(model.MutationResultMsg)
			},
			wantOutput:      "Deleted team-a/pods/web",
			recordedRequest: func() bool { return writer.deleted == deleteReference },
		},
		{
			name: "delete batch",
			run: func() model.MutationResultMsg {
				return operations.DeleteResources(deleteBatch)().(model.MutationResultMsg)
			},
			wantOutput:      "Deleted 2 resources",
			recordedRequest: func() bool { return slices.Equal(writer.deleteBatch.Names, deleteBatch.Names) },
		},
		{
			name: "restart",
			run: func() model.MutationResultMsg {
				return operations.RestartWorkload(restartReference)().(model.MutationResultMsg)
			},
			wantOutput:      "Restarted team-a/statefulset/database",
			recordedRequest: func() bool { return writer.restarted == restartReference },
		},
		{
			name: "restart batch",
			run: func() model.MutationResultMsg {
				return operations.RestartWorkloads(restartBatch)().(model.MutationResultMsg)
			},
			wantOutput:      "Restarted 2 workloads",
			recordedRequest: func() bool { return slices.Equal(writer.restartBatch.Names, restartBatch.Names) },
		},
	}
}

func TestNativeClusterOperationsMutateResources(t *testing.T) {
	writer := &testResourceWriter{
		deleteOutcome:  kube.BatchOutcome{Succeeded: []string{"web", "worker"}},
		restartOutcome: kube.BatchOutcome{Succeeded: []string{"web", "api"}},
	}
	operations := NewOperations(context.Background(), &testResourceInspector{}, &testPodReader{}, writer)
	for _, test := range resourceMutationCases(operations, writer) {
		t.Run(test.name, func(t *testing.T) {
			message := test.run()
			if message.Err != nil || message.Output != test.wantOutput || !test.recordedRequest() {
				t.Fatalf("%s = %+v", test.name, message)
			}
		})
	}
}

func TestNativeClusterOperationsReportMutationFailures(t *testing.T) {
	sentinel := errors.New("mutation failed")
	writer := &testResourceWriter{
		scaleErr:       sentinel,
		deleteErr:      sentinel,
		deleteManyErr:  sentinel,
		restartErr:     sentinel,
		restartManyErr: sentinel,
	}
	operations := NewOperations(context.Background(), &testResourceInspector{}, &testPodReader{}, writer)
	for _, test := range resourceMutationCases(operations, writer) {
		t.Run(test.name, func(t *testing.T) {
			if message := test.run(); !errors.Is(message.Err, sentinel) {
				t.Fatalf("%s = %+v, want sentinel", test.name, message)
			}
		})
	}
}

type sessionOperationsFixture struct {
	sessions   *testSessionOperations
	operations ResourceOperations
	shell      *testShellSession
	forward    *testPortForward
	info       kube.PortForwardSession
}

func newSessionOperationsFixture(t *testing.T) sessionOperationsFixture {
	t.Helper()
	shell := makeFakeShellSession(t)
	info := testModelPortForwardSession(t, "forward-1", "api-0", 18080, 8080)
	forward := &testPortForward{session: info, exit: make(chan kube.PortForwardExit, 1)}
	sessions := &testSessionOperations{
		shellSession: shell,
		portForward:  forward,
		portForwards: []kube.PortForwardSession{info},
	}
	return sessionOperationsFixture{
		sessions:   sessions,
		operations: NewOperations(context.Background(), sessions, sessions, sessions, sessions),
		shell:      shell,
		forward:    forward,
		info:       info,
	}
}

func makeFakeShellSession(t *testing.T) *testShellSession {
	t.Helper()
	return &testShellSession{
		identity: kube.ShellIdentity{
			ID:  "shell-test",
			Pod: kube.PodReference{Namespace: "ns", Name: "pod"},
		},
		output: make(chan kube.ShellOutput, 1),
		exit:   make(chan kube.ShellExit, 1),
	}
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

func sessionShellRequest() kube.ShellRequest {
	return kube.ShellRequest{Pod: kube.PodReference{Namespace: "team-a", Name: "api-0"}}
}

func sessionForwardRequest(info kube.PortForwardSession) kube.PortForwardRequest {
	return kube.PortForwardRequest{
		Pod:        kube.PodReference{Namespace: "team-a", Name: "api-0"},
		LocalPort:  info.LocalPort,
		RemotePort: info.RemotePort,
	}
}

func TestNativeClusterOperationsStartSessions(t *testing.T) {
	fixture := newSessionOperationsFixture(t)
	shellRequest := sessionShellRequest()
	startedShell, err := fixture.operations.StartShell(shellRequest)
	if err != nil || startedShell != fixture.shell || fixture.sessions.shellRequest != shellRequest {
		t.Fatalf("StartShell() = (%v, %v), request = %+v", startedShell, err, fixture.sessions.shellRequest)
	}
	forwardRequest := sessionForwardRequest(fixture.info)
	started := fixture.operations.StartPortForward(forwardRequest)().(PortForwardStartedMsg)
	if started.Session != fixture.forward || started.Err != nil || fixture.sessions.portForwardRequest != forwardRequest {
		t.Fatalf("StartPortForward() = %+v, request = %+v", started, fixture.sessions.portForwardRequest)
	}
	if active := fixture.operations.PortForwards(); !slices.Equal(active, []kube.PortForwardSession{fixture.info}) {
		t.Fatalf("PortForwards() = %+v", active)
	}
}

func TestNativeClusterOperationsStopPortForward(t *testing.T) {
	fixture := newSessionOperationsFixture(t)
	stopMessage := fixture.operations.StopPortForward(fixture.info.ID)().(PortForwardStoppedMsg)
	if stopMessage.SessionID != fixture.info.ID || stopMessage.Err != nil || fixture.sessions.stoppedForwardID != fixture.info.ID {
		t.Fatalf("StopPortForward() = %+v, stopped ID = %q", stopMessage, fixture.sessions.stoppedForwardID)
	}
}

func TestNativeClusterOperationsAwaitPortForwardExit(t *testing.T) {
	fixture := newSessionOperationsFixture(t)
	exitFailure := errors.New("forward failed")
	fixture.forward.exit <- kube.PortForwardExit{SessionID: fixture.info.ID, Err: exitFailure}
	exitMessage := fixture.operations.WaitForPortForwardExit(fixture.forward)().(PortForwardStoppedMsg)
	if exitMessage.SessionID != fixture.info.ID || !errors.Is(exitMessage.Err, exitFailure) {
		t.Fatalf("WaitForPortForwardExit() = %+v", exitMessage)
	}
	closedForward := &testPortForward{session: fixture.info, exit: make(chan kube.PortForwardExit)}
	close(closedForward.exit)
	closedMessage := fixture.operations.WaitForPortForwardExit(closedForward)().(PortForwardStoppedMsg)
	if closedMessage.SessionID != fixture.info.ID || closedMessage.Err != nil {
		t.Fatalf("closed WaitForPortForwardExit() = %+v", closedMessage)
	}
	if fixture.operations.WaitForPortForwardExit(nil) != nil || fixture.operations.WaitForPortForwardExit(&testPortForward{}) != nil {
		t.Fatal("WaitForPortForwardExit() accepted an incomplete session")
	}
}

func TestNativeClusterOperationsReportSessionFailures(t *testing.T) {
	fixture := newSessionOperationsFixture(t)
	sentinel := errors.New("session failed")
	fixture.sessions.shellErr = sentinel
	if failedShell, startErr := fixture.operations.StartShell(sessionShellRequest()); failedShell != fixture.shell || !errors.Is(startErr, sentinel) {
		t.Fatalf("StartShell() failure = (%v, %v)", failedShell, startErr)
	}
	fixture.sessions.portForwardErr = sentinel
	started := fixture.operations.StartPortForward(sessionForwardRequest(fixture.info))().(PortForwardStartedMsg)
	if started.Session != fixture.forward || !errors.Is(started.Err, sentinel) {
		t.Fatalf("StartPortForward() failure = %+v", started)
	}
	fixture.sessions.stopForwardErr = sentinel
	stopMessage := fixture.operations.StopPortForward(fixture.info.ID)().(PortForwardStoppedMsg)
	if !errors.Is(stopMessage.Err, sentinel) {
		t.Fatalf("StopPortForward() failure = %+v", stopMessage)
	}
}

func TestBatchMutationSummaryUsesSingularAndPluralSubjects(t *testing.T) {
	if got := batchMutationSummary("Deleted", "resource", kube.BatchOutcome{Succeeded: []string{"web"}}); got != "Deleted 1 resource" {
		t.Fatalf("single summary = %q", got)
	}
	if got := batchMutationSummary("Deleted", "resource", kube.BatchOutcome{}); got != "Deleted 0 resources" {
		t.Fatalf("empty summary = %q", got)
	}
}

func TestScanPodLogLinesAcceptsEmptyInput(t *testing.T) {
	lines, err := scanPodLogLines(strings.NewReader(""))
	if err != nil || lines == nil || len(lines) != 0 {
		t.Fatalf("scanPodLogLines(empty) = (%v, %v)", lines, err)
	}
}
