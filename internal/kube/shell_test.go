package kube

import (
	"context"
	"errors"
	"io"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	kubernetesfake "k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/remotecommand"
	"k8s.io/streaming/pkg/httpstream"
)

const streamTestTimeout = time.Second

type shellExecutorStub struct {
	stream func(context.Context, remotecommand.StreamOptions) error
}

type shellStreamStub struct {
	io.Reader
	writeErr error
	closeErr error
}

func (s *shellStreamStub) Write([]byte) (int, error) {
	return 0, s.writeErr
}

func (s *shellStreamStub) Close() error {
	return s.closeErr
}

func (s shellExecutorStub) Stream(options remotecommand.StreamOptions) error {
	return s.StreamWithContext(context.Background(), options)
}

func (s shellExecutorStub) StreamWithContext(ctx context.Context, options remotecommand.StreamOptions) error {
	if s.stream == nil {
		return nil
	}
	return s.stream(ctx, options)
}

func awaitShellSignal(t *testing.T, signal <-chan struct{}, message string) {
	t.Helper()
	select {
	case <-signal:
	case <-time.After(streamTestTimeout):
		t.Fatal(message)
	}
}

func requireShellInput(t *testing.T, receivedInput <-chan string, want string) {
	t.Helper()
	select {
	case input := <-receivedInput:
		if input != want {
			t.Fatalf("executor input = %q, want %q", input, want)
		}
	case <-time.After(streamTestTimeout):
		t.Fatal("executor did not receive input")
	}
}

func requireShellOutputLines(t *testing.T, outputs []ShellOutput, sessionID, stdout, stderr string) {
	t.Helper()
	if len(outputs) != 2 {
		t.Fatalf("output count = %d, want 2", len(outputs))
	}
	byStream := map[bool]string{}
	for _, output := range outputs {
		if output.SessionID != sessionID {
			t.Fatalf("output session ID = %q", output.SessionID)
		}
		byStream[output.Stderr] = output.Line
	}
	if byStream[false] != stdout || byStream[true] != stderr {
		t.Fatalf("outputs = %+v", outputs)
	}
}

func TestStartShellStreamsThroughNativeExecutor(t *testing.T) {
	manager := streamingTestManager(t)
	request := ShellRequest{
		Pod:       PodReference{Namespace: "team-a", Name: "api-0"},
		Container: "sidecar",
	}
	started := make(chan struct{})
	receivedInput := make(chan string, 1)
	manager.newShellStream = func(_ *rest.Config, _ *url.URL) (remotecommand.Executor, error) {
		return shellExecutorStub{stream: func(_ context.Context, options remotecommand.StreamOptions) error {
			close(started)
			input, err := io.ReadAll(io.LimitReader(options.Stdin, 4))
			if err != nil {
				return err
			}
			receivedInput <- string(input)
			if _, err := io.WriteString(options.Stdout, "result\n"); err != nil {
				return err
			}
			_, err = io.WriteString(options.Stderr, "warning\n")
			return err
		}}, nil
	}

	session, err := manager.StartShell(context.Background(), request)
	if err != nil {
		t.Fatalf("StartShell() error = %v", err)
	}
	awaitShellSignal(t, started, "executor did not start")
	if err := session.Send("pwd"); err != nil {
		t.Fatalf("Send() error = %v", err)
	}
	requireShellInput(t, receivedInput, "pwd\n")
	requireShellOutputLines(t, receiveShellOutputs(t, session.Output()), "shell-1", "result", "warning")
	exit := receiveShellExit(t, session.Exit())
	if exit.SessionID != "shell-1" || exit.Err != nil {
		t.Fatalf("exit = %+v", exit)
	}
	if err := session.Send("after exit"); !errors.Is(err, ErrShellSessionClosed) {
		t.Fatalf("Send() after exit error = %v", err)
	}
}

func TestStartShellBuildsExecRequestAndIdentity(t *testing.T) {
	manager := streamingTestManager(t)
	request := ShellRequest{
		Pod:       PodReference{Namespace: "team-a", Name: "api-0"},
		Container: "sidecar",
	}
	var capturedConfig *rest.Config
	var capturedURL *url.URL
	manager.newShellStream = func(config *rest.Config, streamURL *url.URL) (remotecommand.Executor, error) {
		capturedConfig = config
		capturedURL = streamURL
		return shellExecutorStub{}, nil
	}
	session, err := manager.StartShell(context.Background(), request)
	if err != nil {
		t.Fatalf("StartShell() error = %v", err)
	}
	if identity := session.Identity(); identity.Pod != request.Pod || identity.Container != request.Container || identity.ID != "shell-1" {
		t.Fatalf("Identity() = %+v", identity)
	}
	if capturedConfig == nil || capturedConfig.Host != "https://cluster.invalid" {
		t.Fatalf("executor config = %#v", capturedConfig)
	}
	if capturedURL.Path != "/api/v1/namespaces/team-a/pods/api-0/exec" {
		t.Fatalf("exec path = %q", capturedURL.Path)
	}
	requireShellExecQuery(t, capturedURL.Query(), request.Container)
	receiveShellExit(t, session.Exit())
	receiveShellOutputs(t, session.Output())
}

func requireShellExecQuery(t *testing.T, query url.Values, container string) {
	t.Helper()
	if query.Get("container") != container || query.Get("command") != shellCommand || query.Get("stdin") != "true" || query.Get("stdout") != "true" || query.Get("stderr") != "true" {
		t.Fatalf("exec query = %v", query)
	}
}

func TestStartShellValidatesBoundariesAndDependencies(t *testing.T) {
	validRequest := ShellRequest{Pod: PodReference{Namespace: "team-a", Name: "api-0"}}
	connected := streamingTestManager(t)
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	tests := []struct {
		name    string
		manager *Manager
		ctx     context.Context
		request ShellRequest
		cause   error
	}{
		{name: "namespace", manager: connected, ctx: context.Background(), request: ShellRequest{Pod: PodReference{Name: "api-0"}}, cause: ErrNamespaceRequired},
		{name: "pod", manager: connected, ctx: context.Background(), request: ShellRequest{Pod: PodReference{Namespace: "team-a"}}, cause: ErrPodNameRequired},
		{name: "nil context", manager: connected, request: validRequest, cause: ErrContextRequired},
		{name: "canceled context", manager: connected, ctx: canceled, request: validRequest, cause: context.Canceled},
		{name: "not connected", manager: newTestManager(t), ctx: context.Background(), request: validRequest, cause: ErrClientUnavailable},
		{name: "missing REST client", manager: managerWithClientsForTest(t, &Clients{}), ctx: context.Background(), request: validRequest, cause: ErrTypedClientUnavailable},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			session, err := test.manager.StartShell(test.ctx, test.request)
			if session != nil || !errors.Is(err, test.cause) {
				t.Fatalf("StartShell() = (%v, %v), want cause %v", session, err, test.cause)
			}
		})
	}

	missingExecutor := streamingTestManager(t)
	missingExecutor.newShellStream = nil
	if session, err := missingExecutor.StartShell(context.Background(), validRequest); session != nil || !errors.Is(err, ErrPodExecutorUnavailable) {
		t.Fatalf("StartShell() without factory = (%v, %v)", session, err)
	}
	factoryFailure := errors.New("executor factory failed")
	failingExecutor := streamingTestManager(t)
	failingExecutor.newShellStream = func(*rest.Config, *url.URL) (remotecommand.Executor, error) {
		return nil, factoryFailure
	}
	if session, err := failingExecutor.StartShell(context.Background(), validRequest); session != nil || !errors.Is(err, factoryFailure) {
		t.Fatalf("StartShell() factory failure = (%v, %v)", session, err)
	}
	nilExecutor := streamingTestManager(t)
	nilExecutor.newShellStream = func(*rest.Config, *url.URL) (remotecommand.Executor, error) { return nil, nil }
	if session, err := nilExecutor.StartShell(context.Background(), validRequest); session != nil || !errors.Is(err, ErrPodExecutorUnavailable) {
		t.Fatalf("StartShell() nil executor = (%v, %v)", session, err)
	}
}

func TestPodExecURLValidatesClientShape(t *testing.T) {
	request := ShellRequest{Pod: PodReference{Namespace: "team-a", Name: "api-0"}}
	if streamURL, err := podExecURL(nil, request); streamURL != nil || !errors.Is(err, ErrTypedClientUnavailable) {
		t.Fatalf("podExecURL(nil) = (%v, %v)", streamURL, err)
	}
	if streamURL, err := podExecURL(&Clients{}, request); streamURL != nil || !errors.Is(err, ErrTypedClientUnavailable) {
		t.Fatalf("podExecURL(empty) = (%v, %v)", streamURL, err)
	}
	clients := &Clients{kubernetes: kubernetesfake.NewSimpleClientset()}
	if streamURL, err := podExecURL(clients, request); streamURL != nil || !errors.Is(err, ErrPodExecutorUnavailable) {
		t.Fatalf("podExecURL(fake client) = (%v, %v)", streamURL, err)
	}
}

func shellExecStreamURL(t *testing.T) *url.URL {
	t.Helper()
	streamURL, err := url.Parse("https://cluster.invalid/api/v1/namespaces/team-a/pods/api-0/exec")
	if err != nil {
		t.Fatalf("url.Parse() error = %v", err)
	}
	return streamURL
}

func TestDefaultShellExecutorValidatesInputs(t *testing.T) {
	streamURL := shellExecStreamURL(t)
	if executor, buildErr := defaultShellExecutor(nil, streamURL); executor != nil || !errors.Is(buildErr, ErrPodExecutorUnavailable) {
		t.Fatalf("defaultShellExecutor(nil) = (%v, %v)", executor, buildErr)
	}
	if executor, buildErr := defaultShellExecutor(&rest.Config{}, nil); executor != nil || !errors.Is(buildErr, ErrPodExecutorUnavailable) {
		t.Fatalf("defaultShellExecutor(nil URL) = (%v, %v)", executor, buildErr)
	}
	if executor, buildErr := defaultShellExecutor(&rest.Config{Host: "https://cluster.invalid"}, streamURL); executor == nil || buildErr != nil {
		t.Fatalf("defaultShellExecutor() = (%v, %v)", executor, buildErr)
	}
}

func TestBuildShellExecutorUsesConstructors(t *testing.T) {
	streamURL := shellExecStreamURL(t)
	sentinel := errors.New("construction failed")
	stub := shellExecutorStub{}
	base := shellExecutorConstructors{
		spdy:      func(*rest.Config, string, *url.URL) (remotecommand.Executor, error) { return stub, nil },
		webSocket: func(*rest.Config, string, string) (remotecommand.Executor, error) { return stub, nil },
		fallback: func(primary, secondary remotecommand.Executor, predicate func(error) bool) (remotecommand.Executor, error) {
			if primary == nil || secondary == nil || predicate(errors.New("plain")) {
				t.Fatal("fallback inputs are invalid")
			}
			return stub, nil
		},
	}
	failures := []struct {
		name     string
		sabotage func(*shellExecutorConstructors)
	}{
		{name: "SPDY failure", sabotage: func(constructors *shellExecutorConstructors) {
			constructors.spdy = func(*rest.Config, string, *url.URL) (remotecommand.Executor, error) { return nil, sentinel }
		}},
		{name: "WebSocket failure", sabotage: func(constructors *shellExecutorConstructors) {
			constructors.webSocket = func(*rest.Config, string, string) (remotecommand.Executor, error) { return nil, sentinel }
		}},
		{name: "fallback result", sabotage: func(constructors *shellExecutorConstructors) {
			constructors.fallback = func(remotecommand.Executor, remotecommand.Executor, func(error) bool) (remotecommand.Executor, error) {
				return nil, sentinel
			}
		}},
	}
	for _, test := range failures {
		t.Run(test.name, func(t *testing.T) {
			constructors := base
			test.sabotage(&constructors)
			if executor, buildErr := buildShellExecutor(&rest.Config{}, streamURL, constructors); executor != nil || !errors.Is(buildErr, sentinel) {
				t.Fatalf("buildShellExecutor() = (%v, %v)", executor, buildErr)
			}
		})
	}
	if executor, buildErr := buildShellExecutor(&rest.Config{}, streamURL, base); executor == nil || buildErr != nil {
		t.Fatalf("buildShellExecutor() = (%v, %v)", executor, buildErr)
	}
}

func TestShellFallbackClassification(t *testing.T) {
	if shouldFallbackStream(nil) || shouldFallbackStream(errors.New("plain")) {
		t.Fatal("plain errors must not trigger fallback")
	}
	if !shouldFallbackStream(&httpstream.UpgradeFailureError{Cause: errors.New("upgrade")}) {
		t.Fatal("upgrade failure must trigger fallback")
	}
	if !shouldFallbackStream(errors.New("proxy: unknown scheme: https")) {
		t.Fatal("HTTPS proxy failure must trigger fallback")
	}
}

func TestShellSessionNilControls(t *testing.T) {
	var nilSession *shellSession
	if nilSession.Identity() != (ShellIdentity{}) || nilSession.Output() != nil || nilSession.Exit() != nil {
		t.Fatal("nil session accessors returned state")
	}
	if err := nilSession.Send("line"); !errors.Is(err, ErrShellSessionClosed) {
		t.Fatalf("nil Send() error = %v", err)
	}
	if err := nilSession.Interrupt(); !errors.Is(err, ErrShellSessionClosed) {
		t.Fatalf("nil Interrupt() error = %v", err)
	}
	nilSession.Close()
}

func TestShellSessionControls(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	session := &shellSession{
		identity: ShellIdentity{ID: "shell-test", Pod: PodReference{Namespace: "team-a", Name: "api-0"}},
		input:    make(chan string, 1),
		ctx:      ctx,
		cancel:   cancel,
	}
	if err := session.Send("first\n"); err != nil {
		t.Fatalf("Send(first) error = %v", err)
	}
	if err := session.Send("second"); !errors.Is(err, ErrShellInputBackpressure) {
		t.Fatalf("Send(second) error = %v", err)
	}
	if err := session.Interrupt(); err != nil {
		t.Fatalf("Interrupt() error = %v", err)
	}
	session.Close()
	if err := session.Interrupt(); !errors.Is(err, ErrShellSessionClosed) {
		t.Fatalf("Interrupt() after close error = %v", err)
	}
	if err := session.Send("after close"); !errors.Is(err, ErrShellSessionClosed) {
		t.Fatalf("Send() after close error = %v", err)
	}
	externallyCanceled, cancelExternally := context.WithCancel(context.Background())
	cancelExternally()
	canceledSession := &shellSession{
		identity: ShellIdentity{Pod: PodReference{Namespace: "team-a", Name: "api-0"}},
		input:    make(chan string, 1),
		ctx:      externallyCanceled,
	}
	if err := canceledSession.Send("line"); !errors.Is(err, ErrShellSessionClosed) {
		t.Fatalf("externally canceled Send() error = %v", err)
	}
}

func TestShellSessionReportsStreamFailures(t *testing.T) {
	sentinel := errors.New("remote stream failed")
	ctx, cancel := context.WithCancel(context.Background())
	session := startShellSession(ctx, cancel, ShellIdentity{ID: "shell-error", Pod: PodReference{Namespace: "team-a", Name: "api-0"}}, shellExecutorStub{
		stream: func(context.Context, remotecommand.StreamOptions) error { return sentinel },
	})
	exit := receiveShellExit(t, session.Exit())
	if !errors.Is(exit.Err, sentinel) {
		t.Fatalf("shell exit error = %v, want sentinel", exit.Err)
	}
	receiveShellOutputs(t, session.Output())

	ctx, cancel = context.WithCancel(context.Background())
	release := make(chan struct{})
	session = startShellSession(ctx, cancel, ShellIdentity{ID: "shell-async", Pod: PodReference{Namespace: "team-a", Name: "api-0"}}, shellExecutorStub{
		stream: func(context.Context, remotecommand.StreamOptions) error {
			<-release
			return nil
		},
	})
	concrete := session.(*shellSession)
	asyncFailure := errors.New("input failed")
	concrete.setAsyncError(asyncFailure)
	concrete.setAsyncError(errors.New("ignored"))
	close(release)
	exit = receiveShellExit(t, session.Exit())
	if !errors.Is(exit.Err, asyncFailure) || !errors.Is(concrete.getAsyncError(), asyncFailure) {
		t.Fatalf("shell async error = %v", exit.Err)
	}
	receiveShellOutputs(t, session.Output())
}

func TestShellSessionReportsOversizedAndDroppedOutput(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	oversized := strings.Repeat("x", shellMaximumLineBytes+1) + "\n"
	session := startShellSession(ctx, cancel, ShellIdentity{ID: "shell-large", Pod: PodReference{Namespace: "team-a", Name: "api-0"}}, shellExecutorStub{
		stream: func(_ context.Context, options remotecommand.StreamOptions) error {
			_, _ = io.WriteString(options.Stdout, oversized)
			return nil
		},
	})
	exit := receiveShellExit(t, session.Exit())
	if !errors.Is(exit.Err, ErrShellOutputLineTooLong) {
		t.Fatalf("oversized output error = %v", exit.Err)
	}
	receiveShellOutputs(t, session.Output())

	ctx, cancel = context.WithCancel(context.Background())
	session = startShellSession(ctx, cancel, ShellIdentity{ID: "shell-drop", Pod: PodReference{Namespace: "team-a", Name: "api-0"}}, shellExecutorStub{
		stream: func(_ context.Context, options remotecommand.StreamOptions) error {
			for range shellOutputCapacity + 2 {
				if _, err := io.WriteString(options.Stdout, "line\n"); err != nil {
					return err
				}
			}
			return nil
		},
	})
	exit = receiveShellExit(t, session.Exit())
	var dropped *ShellOutputDroppedError
	if !errors.As(exit.Err, &dropped) || dropped.Count != 2 {
		t.Fatalf("dropped output error = %v", exit.Err)
	}
	receiveShellOutputs(t, session.Output())
}

func TestShellInputAndOutputWorkersHandleCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	reader, writer := io.Pipe()
	writeDone := make(chan struct{})
	session := &shellSession{
		identity:  ShellIdentity{Pod: PodReference{Namespace: "team-a", Name: "api-0"}},
		stdin:     writer,
		input:     make(chan string, 1),
		inputDone: writeDone,
		ctx:       ctx,
		cancel:    cancel,
	}
	sentinel := errors.New("reader failed")
	if err := reader.CloseWithError(sentinel); err != nil {
		t.Fatalf("CloseWithError() = %v", err)
	}
	session.input <- "line\n"
	session.writeInput()
	if !errors.Is(session.getAsyncError(), sentinel) {
		t.Fatalf("input worker error = %v", session.getAsyncError())
	}

	canceledContext, cancelOutput := context.WithCancel(context.Background())
	outputSession := &shellSession{ctx: canceledContext, output: make(chan ShellOutput)}
	if !outputSession.emitOutput(ShellOutput{}) || outputSession.droppedOutput.Load() != 1 {
		t.Fatal("full output channel did not count a dropped line")
	}
	cancelOutput()
	if outputSession.emitOutput(ShellOutput{}) {
		t.Fatal("canceled output session accepted output")
	}
	reader, writer = io.Pipe()
	streamErrors := make(chan error, 1)
	var readers sync.WaitGroup
	readers.Add(1)
	go outputSession.readOutput(reader, false, &readers, streamErrors)
	if _, err := io.WriteString(writer, "ignored\n"); err != nil {
		t.Fatalf("write canceled output = %v", err)
	}
	_ = writer.Close()
	readers.Wait()
	if err := <-streamErrors; err != nil {
		t.Fatalf("canceled output reader error = %v", err)
	}
}

func TestShellWorkersReportCloseFailures(t *testing.T) {
	closeFailure := errors.New("close failed")
	canceledContext, cancel := context.WithCancel(context.Background())
	cancel()
	inputSession := &shellSession{
		identity:  ShellIdentity{ID: "shell-input"},
		stdin:     &shellStreamStub{closeErr: closeFailure},
		input:     make(chan string),
		inputDone: make(chan struct{}),
		ctx:       canceledContext,
		cancel:    func() {},
	}
	inputSession.writeInput()
	if !errors.Is(inputSession.getAsyncError(), closeFailure) {
		t.Fatalf("input close error = %v, want close failure", inputSession.getAsyncError())
	}
	closedPipeSession := &shellSession{}
	closedPipeSession.recordInputFailure("write input", io.ErrClosedPipe)
	if closedPipeSession.getAsyncError() != nil {
		t.Fatalf("closed pipe was reported as an asynchronous failure: %v", closedPipeSession.getAsyncError())
	}

	outputSession := &shellSession{ctx: context.Background(), output: make(chan ShellOutput, 1)}
	streamErrors := make(chan error, 1)
	var readers sync.WaitGroup
	readers.Add(1)
	outputSession.readOutput(
		&shellStreamStub{Reader: strings.NewReader("line\n"), closeErr: closeFailure},
		false,
		&readers,
		streamErrors,
	)
	readers.Wait()
	if err := <-streamErrors; !errors.Is(err, closeFailure) {
		t.Fatalf("output close error = %v, want close failure", err)
	}
}

func TestCoreRESTClientHandlesNilBundle(t *testing.T) {
	if coreRESTClient(nil) != nil || coreRESTClient(&Clients{}) != nil {
		t.Fatal("coreRESTClient() returned a client for an empty bundle")
	}
}

func receiveShellOutputs(t *testing.T, output <-chan ShellOutput) []ShellOutput {
	t.Helper()
	outputs := []ShellOutput{}
	for {
		select {
		case item, open := <-output:
			if !open {
				return outputs
			}
			outputs = append(outputs, item)
		case <-time.After(streamTestTimeout):
			t.Fatal("timed out waiting for shell output closure")
		}
	}
}

func receiveShellExit(t *testing.T, exits <-chan ShellExit) ShellExit {
	t.Helper()
	select {
	case exit := <-exits:
		return exit
	case <-time.After(streamTestTimeout):
		t.Fatal("timed out waiting for shell exit")
		return ShellExit{}
	}
}

func streamingTestManager(t *testing.T) *Manager {
	t.Helper()
	clients, err := NewClients(&rest.Config{Host: "https://cluster.invalid"})
	if err != nil {
		t.Fatalf("NewClients() error = %v", err)
	}
	return managerWithClientsForTest(t, clients)
}

func newTestManager(t *testing.T) *Manager {
	t.Helper()
	manager, err := NewManager(&fakeConfigSource{}, &fakeClientBuilder{})
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}
	return manager
}

func TestShellOutputDroppedErrorIsStable(t *testing.T) {
	errorText := (&ShellOutputDroppedError{Count: 3}).Error()
	if errorText != "kubernetes shell dropped 3 output lines because the consumer was too slow" {
		t.Fatalf("Error() = %q", errorText)
	}
}

func TestShellSessionConcurrentCloseIsSafe(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	session := &shellSession{ctx: ctx, cancel: cancel}
	var group sync.WaitGroup
	group.Add(2)
	go func() {
		defer group.Done()
		session.Close()
	}()
	go func() {
		defer group.Done()
		session.Close()
	}()
	group.Wait()
	if ctx.Err() == nil {
		t.Fatal("concurrent Close() did not cancel the session")
	}
}
