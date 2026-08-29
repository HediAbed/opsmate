package ui

import (
	"context"
	"errors"
	"io"
	"slices"
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/HediAbed/opsmate/internal/cluster"
	"github.com/HediAbed/opsmate/internal/kube"
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
			operations := newNativeClusterOperations(context.Background(), inspector, &testPodReader{}, &testResourceWriter{})
			if test.inspect {
				message := operations.InspectResource(reference)().(cluster.DescribeMsg)
				if message.Output != test.content || !errors.Is(message.Err, test.err) {
					t.Fatalf("InspectResource() = %+v", message)
				}
			} else {
				message := operations.ResourceYAML(reference)().(cluster.YAMLMsg)
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
	operations := newNativeClusterOperations(context.Background(), &testResourceInspector{}, reader, &testResourceWriter{})
	message := operations.FetchPodLogs(request)().(cluster.LogsMsg)
	if message.Lines != nil || !errors.Is(message.Err, sentinel) || reader.logRequest != request {
		t.Fatalf("FetchPodLogs(open failure) = %+v, request = %+v", message, reader.logRequest)
	}

	reader = &testPodReader{}
	operations = newNativeClusterOperations(context.Background(), &testResourceInspector{}, reader, &testResourceWriter{})
	message = operations.FetchPodLogs(request)().(cluster.LogsMsg)
	if message.Lines != nil || !errors.Is(message.Err, kube.ErrPodLogStreamUnavailable) {
		t.Fatalf("FetchPodLogs(empty stream) = %+v", message)
	}

	stream := &testLogReadCloser{reader: strings.NewReader("first\nsecond\n")}
	reader = &testPodReader{logStream: stream}
	operations = newNativeClusterOperations(context.Background(), &testResourceInspector{}, reader, &testResourceWriter{})
	message = operations.FetchPodLogs(request)().(cluster.LogsMsg)
	if message.Err != nil || !slices.Equal(message.Lines, []string{"first", "second"}) || stream.closeCalls != 1 {
		t.Fatalf("FetchPodLogs(success) = %+v, close calls = %d", message, stream.closeCalls)
	}
}

func TestNativeClusterOperationsReportsLogReadAndCloseFailures(t *testing.T) {
	closeErr := errors.New("close failed")
	oversizedLine := strings.Repeat("x", maximumLogLineBytes+1)
	stream := &testLogReadCloser{reader: strings.NewReader(oversizedLine), closeErr: closeErr}
	reader := &testPodReader{logStream: stream}
	operations := newNativeClusterOperations(context.Background(), &testResourceInspector{}, reader, &testResourceWriter{})
	message := operations.FetchPodLogs(kube.PodLogRequest{Pod: kube.PodReference{Namespace: "team-a", Name: "web"}, TailLines: 1})().(cluster.LogsMsg)
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
			operations := newNativeClusterOperations(context.Background(), &testResourceInspector{}, reader, &testResourceWriter{})
			message := operations.FetchPodContainers(reference)().(cluster.ContainersMsg)
			if !slices.Equal(message.Containers, test.containers) || !errors.Is(message.Err, test.err) || reader.containerReference != reference {
				t.Fatalf("FetchPodContainers() = %+v, reference = %+v", message, reader.containerReference)
			}
		})
	}
}

type resourceMutationCase struct {
	name            string
	run             func() cluster.MutationResultMsg
	wantOutput      string
	recordedRequest func() bool
}

func resourceMutationCases(operations nativeClusterOperations, writer *testResourceWriter) []resourceMutationCase {
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
			run: func() cluster.MutationResultMsg {
				return operations.ScaleWorkload(scaleRequest)().(cluster.MutationResultMsg)
			},
			wantOutput:      "Scaled team-a/deployment/web",
			recordedRequest: func() bool { return writer.scaleRequest == scaleRequest },
		},
		{
			name: "delete",
			run: func() cluster.MutationResultMsg {
				return operations.DeleteResource(deleteReference)().(cluster.MutationResultMsg)
			},
			wantOutput:      "Deleted team-a/pods/web",
			recordedRequest: func() bool { return writer.deleted == deleteReference },
		},
		{
			name: "delete batch",
			run: func() cluster.MutationResultMsg {
				return operations.DeleteResources(deleteBatch)().(cluster.MutationResultMsg)
			},
			wantOutput:      "Deleted 2 resources",
			recordedRequest: func() bool { return slices.Equal(writer.deleteBatch.Names, deleteBatch.Names) },
		},
		{
			name: "restart",
			run: func() cluster.MutationResultMsg {
				return operations.RestartWorkload(restartReference)().(cluster.MutationResultMsg)
			},
			wantOutput:      "Restarted team-a/statefulset/database",
			recordedRequest: func() bool { return writer.restarted == restartReference },
		},
		{
			name: "restart batch",
			run: func() cluster.MutationResultMsg {
				return operations.RestartWorkloads(restartBatch)().(cluster.MutationResultMsg)
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
	operations := newNativeClusterOperations(context.Background(), &testResourceInspector{}, &testPodReader{}, writer)
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
	operations := newNativeClusterOperations(context.Background(), &testResourceInspector{}, &testPodReader{}, writer)
	for _, test := range resourceMutationCases(operations, writer) {
		t.Run(test.name, func(t *testing.T) {
			if message := test.run(); !errors.Is(message.Err, sentinel) {
				t.Fatalf("%s = %+v, want sentinel", test.name, message)
			}
		})
	}
}

type sessionOperationsFixture struct {
	sessions   *testClusterOperations
	operations nativeClusterOperations
	shell      *testShellSession
	forward    *testPortForward
	info       kube.PortForwardSession
}

func newSessionOperationsFixture(t *testing.T) sessionOperationsFixture {
	t.Helper()
	shell := makeFakeShellSession(t)
	info := testModelPortForwardSession(t, "forward-1", "api-0", 18080, 8080)
	forward := &testPortForward{session: info, exit: make(chan kube.PortForwardExit, 1)}
	sessions := &testClusterOperations{
		shellSession: shell,
		portForward:  forward,
		portForwards: []kube.PortForwardSession{info},
	}
	return sessionOperationsFixture{
		sessions:   sessions,
		operations: newNativeClusterOperations(context.Background(), sessions, sessions, sessions, sessions),
		shell:      shell,
		forward:    forward,
		info:       info,
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
	started := fixture.operations.StartPortForward(forwardRequest)().(portForwardStartedMsg)
	if started.session != fixture.forward || started.err != nil || fixture.sessions.portForwardRequest != forwardRequest {
		t.Fatalf("StartPortForward() = %+v, request = %+v", started, fixture.sessions.portForwardRequest)
	}
	if active := fixture.operations.PortForwards(); !slices.Equal(active, []kube.PortForwardSession{fixture.info}) {
		t.Fatalf("PortForwards() = %+v", active)
	}
}

func TestNativeClusterOperationsStopPortForward(t *testing.T) {
	fixture := newSessionOperationsFixture(t)
	stopMessage := fixture.operations.StopPortForward(fixture.info.ID)().(portForwardStoppedMsg)
	if stopMessage.sessionID != fixture.info.ID || stopMessage.err != nil || fixture.sessions.stoppedForwardID != fixture.info.ID {
		t.Fatalf("StopPortForward() = %+v, stopped ID = %q", stopMessage, fixture.sessions.stoppedForwardID)
	}
}

func TestNativeClusterOperationsAwaitPortForwardExit(t *testing.T) {
	fixture := newSessionOperationsFixture(t)
	exitFailure := errors.New("forward failed")
	fixture.forward.exit <- kube.PortForwardExit{SessionID: fixture.info.ID, Err: exitFailure}
	exitMessage := fixture.operations.WaitForPortForwardExit(fixture.forward)().(portForwardStoppedMsg)
	if exitMessage.sessionID != fixture.info.ID || !errors.Is(exitMessage.err, exitFailure) {
		t.Fatalf("WaitForPortForwardExit() = %+v", exitMessage)
	}
	closedForward := &testPortForward{session: fixture.info, exit: make(chan kube.PortForwardExit)}
	close(closedForward.exit)
	closedMessage := fixture.operations.WaitForPortForwardExit(closedForward)().(portForwardStoppedMsg)
	if closedMessage.sessionID != fixture.info.ID || closedMessage.err != nil {
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
	started := fixture.operations.StartPortForward(sessionForwardRequest(fixture.info))().(portForwardStartedMsg)
	if started.session != fixture.forward || !errors.Is(started.err, sentinel) {
		t.Fatalf("StartPortForward() failure = %+v", started)
	}
	fixture.sessions.stopForwardErr = sentinel
	stopMessage := fixture.operations.StopPortForward(fixture.info.ID)().(portForwardStoppedMsg)
	if !errors.Is(stopMessage.err, sentinel) {
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
