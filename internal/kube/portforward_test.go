package kube

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"testing"
	"time"

	kubernetesfake "k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/transport/spdy"
	streamhttp "k8s.io/streaming/pkg/httpstream"
)

type portForwardRunnerStub struct {
	forward func() error
}

func (r portForwardRunnerStub) ForwardPorts() error {
	if r.forward == nil {
		return nil
	}
	return r.forward()
}

type streamingDialerStub struct{}

func (streamingDialerStub) Dial(...string) (streamhttp.Connection, string, error) {
	return nil, "", nil
}

type spdyUpgraderStub struct{}

func (spdyUpgraderStub) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, nil
}

func (spdyUpgraderStub) NewConnection(*http.Response) (streamhttp.Connection, error) {
	return nil, nil
}

func TestNetworkPortAndStatusTypes(t *testing.T) {
	for _, value := range []int{minimumNetworkPort, 8080, maximumNetworkPort} {
		port, err := NewNetworkPort(value)
		if err != nil || port.Int() != value || port.String() != strings.TrimSpace(port.String()) {
			t.Fatalf("NewNetworkPort(%d) = (%v, %v)", value, port, err)
		}
	}
	for _, value := range []int{-1, 0, maximumNetworkPort + 1} {
		if port, err := NewNetworkPort(value); port != (NetworkPort{}) || !errors.Is(err, ErrNetworkPortInvalid) {
			t.Fatalf("NewNetworkPort(%d) = (%v, %v)", value, port, err)
		}
	}
	if PortForwardRunning.String() != "running" || PortForwardStatus(0).String() != "unknown" {
		t.Fatalf("port-forward statuses = (%q, %q)", PortForwardRunning.String(), PortForwardStatus(0).String())
	}
}

func TestStartAndStopPortForwardLifecycle(t *testing.T) {
	manager := streamingTestManager(t)
	startedAt := time.Date(2026, time.August, 29, 8, 0, 0, 0, time.UTC)
	manager.clock = func() time.Time { return startedAt }
	request := testPortForwardRequest(t, "team-a", "api-0", 18080, 8080)
	var capturedURL *url.URL
	manager.newPortForward = func(
		_ *rest.Config,
		streamURL *url.URL,
		gotRequest PortForwardRequest,
		stop <-chan struct{},
		ready chan struct{},
		_ io.Writer,
		_ io.Writer,
	) (portForwardRunner, error) {
		capturedURL = streamURL
		if gotRequest != request {
			t.Fatalf("port-forward request = %+v, want %+v", gotRequest, request)
		}
		return portForwardRunnerStub{forward: func() error {
			close(ready)
			<-stop
			return errors.New("ignored after requested stop")
		}}, nil
	}

	handle, err := manager.StartPortForward(context.Background(), request)
	if err != nil {
		t.Fatalf("StartPortForward() error = %v", err)
	}
	session := handle.Session()
	requireRunningPortForwardSession(t, session, request, startedAt)
	if capturedURL.Path != "/api/v1/namespaces/team-a/pods/api-0/portforward" {
		t.Fatalf("port-forward path = %q", capturedURL.Path)
	}
	if active := manager.PortForwards(); !slices.Equal(active, []PortForwardSession{session}) {
		t.Fatalf("PortForwards() = %+v", active)
	}
	if err := manager.StopPortForward(context.Background(), session.ID); err != nil {
		t.Fatalf("StopPortForward() error = %v", err)
	}
	exit := receivePortForwardExit(t, handle.Exit())
	if exit.SessionID != session.ID || exit.Err != nil {
		t.Fatalf("port-forward exit = %+v", exit)
	}
	if active := manager.PortForwards(); len(active) != 0 {
		t.Fatalf("PortForwards() after stop = %+v", active)
	}
	if err := manager.StopPortForward(context.Background(), session.ID); err != nil {
		t.Fatalf("second StopPortForward() error = %v", err)
	}
}

func requireRunningPortForwardSession(t *testing.T, session PortForwardSession, request PortForwardRequest, startedAt time.Time) {
	t.Helper()
	if session.ID != "port-forward-1" || session.Pod != request.Pod || session.LocalPort != request.LocalPort || session.RemotePort != request.RemotePort || !session.StartedAt.Equal(startedAt) || session.Status != PortForwardRunning {
		t.Fatalf("port-forward session = %+v", session)
	}
}

func TestStartPortForwardReportsEarlyExitAndDiagnostics(t *testing.T) {
	sentinel := errors.New("connection refused")
	manager := streamingTestManager(t)
	manager.newPortForward = func(
		_ *rest.Config,
		_ *url.URL,
		_ PortForwardRequest,
		_ <-chan struct{},
		_ chan struct{},
		_ io.Writer,
		errorOutput io.Writer,
	) (portForwardRunner, error) {
		return portForwardRunnerStub{forward: func() error {
			_, _ = io.WriteString(errorOutput, "bind failed")
			return sentinel
		}}, nil
	}
	handle, err := manager.StartPortForward(context.Background(), testPortForwardRequest(t, "team-a", "api-0", 18080, 8080))
	if handle != nil || !errors.Is(err, sentinel) || !strings.Contains(err.Error(), "bind failed") {
		t.Fatalf("StartPortForward() = (%v, %v)", handle, err)
	}

	manager = streamingTestManager(t)
	manager.newPortForward = func(*rest.Config, *url.URL, PortForwardRequest, <-chan struct{}, chan struct{}, io.Writer, io.Writer) (portForwardRunner, error) {
		return portForwardRunnerStub{}, nil
	}
	handle, err = manager.StartPortForward(context.Background(), testPortForwardRequest(t, "team-a", "api-0", 18081, 8080))
	if handle != nil || !errors.Is(err, ErrPortForwardReadiness) {
		t.Fatalf("StartPortForward() clean early exit = (%v, %v)", handle, err)
	}
}

func TestStartPortForwardHonorsReadinessTimeout(t *testing.T) {
	request := testPortForwardRequest(t, "team-a", "api-0", 18080, 8080)
	manager := streamingTestManager(t)
	manager.forwardTimeout = time.Millisecond
	manager.newPortForward = func(
		_ *rest.Config,
		_ *url.URL,
		_ PortForwardRequest,
		stop <-chan struct{},
		_ chan struct{},
		_ io.Writer,
		_ io.Writer,
	) (portForwardRunner, error) {
		return portForwardRunnerStub{forward: func() error {
			<-stop
			return nil
		}}, nil
	}
	if handle, err := manager.StartPortForward(context.Background(), request); handle != nil || !errors.Is(err, ErrPortForwardReadiness) {
		t.Fatalf("StartPortForward() timeout = (%v, %v)", handle, err)
	}
	if err := manager.StopAllPortForwards(context.Background()); err != nil {
		t.Fatalf("StopAllPortForwards() after timeout = %v", err)
	}
}

func TestStartPortForwardHonorsCallerCancellation(t *testing.T) {
	request := testPortForwardRequest(t, "team-a", "api-0", 18080, 8080)
	manager := streamingTestManager(t)
	started := make(chan struct{})
	manager.newPortForward = func(
		_ *rest.Config,
		_ *url.URL,
		_ PortForwardRequest,
		stop <-chan struct{},
		_ chan struct{},
		_ io.Writer,
		_ io.Writer,
	) (portForwardRunner, error) {
		return portForwardRunnerStub{forward: func() error {
			close(started)
			<-stop
			return nil
		}}, nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() {
		_, startErr := manager.StartPortForward(ctx, request)
		result <- startErr
	}()
	select {
	case <-started:
	case <-time.After(streamTestTimeout):
		t.Fatal("port forward did not start")
	}
	cancel()
	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("canceled StartPortForward() error = %v", err)
		}
	case <-time.After(streamTestTimeout):
		t.Fatal("canceled StartPortForward() did not return")
	}
}

func TestStartPortForwardValidatesBoundariesAndDependencies(t *testing.T) {
	valid := testPortForwardRequest(t, "team-a", "api-0", 18080, 8080)
	connected := streamingTestManager(t)
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	tests := []struct {
		name    string
		manager *Manager
		ctx     context.Context
		request PortForwardRequest
		cause   error
	}{
		{name: "namespace", manager: connected, ctx: context.Background(), request: PortForwardRequest{Pod: PodReference{Name: "api-0"}, LocalPort: valid.LocalPort, RemotePort: valid.RemotePort}, cause: ErrNamespaceRequired},
		{name: "pod", manager: connected, ctx: context.Background(), request: PortForwardRequest{Pod: PodReference{Namespace: "team-a"}, LocalPort: valid.LocalPort, RemotePort: valid.RemotePort}, cause: ErrPodNameRequired},
		{name: "local port", manager: connected, ctx: context.Background(), request: PortForwardRequest{Pod: valid.Pod, RemotePort: valid.RemotePort}, cause: ErrNetworkPortInvalid},
		{name: "remote port", manager: connected, ctx: context.Background(), request: PortForwardRequest{Pod: valid.Pod, LocalPort: valid.LocalPort}, cause: ErrNetworkPortInvalid},
		{name: "nil context", manager: connected, request: valid, cause: ErrContextRequired},
		{name: "canceled context", manager: connected, ctx: canceled, request: valid, cause: context.Canceled},
		{name: "not connected", manager: newTestManager(t), ctx: context.Background(), request: valid, cause: ErrClientUnavailable},
		{name: "missing REST client", manager: managerWithClientsForTest(t, &Clients{}), ctx: context.Background(), request: valid, cause: ErrTypedClientUnavailable},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			handle, err := test.manager.StartPortForward(test.ctx, test.request)
			if handle != nil || !errors.Is(err, test.cause) {
				t.Fatalf("StartPortForward() = (%v, %v), want %v", handle, err, test.cause)
			}
		})
	}

	missingFactory := streamingTestManager(t)
	missingFactory.newPortForward = nil
	if handle, err := missingFactory.StartPortForward(context.Background(), valid); handle != nil || !errors.Is(err, ErrPortForwarderUnavailable) {
		t.Fatalf("StartPortForward() without factory = (%v, %v)", handle, err)
	}
	factoryFailure := errors.New("factory failed")
	failingFactory := streamingTestManager(t)
	failingFactory.newPortForward = func(*rest.Config, *url.URL, PortForwardRequest, <-chan struct{}, chan struct{}, io.Writer, io.Writer) (portForwardRunner, error) {
		return nil, factoryFailure
	}
	if handle, err := failingFactory.StartPortForward(context.Background(), valid); handle != nil || !errors.Is(err, factoryFailure) {
		t.Fatalf("StartPortForward() factory error = (%v, %v)", handle, err)
	}
	nilRunner := streamingTestManager(t)
	nilRunner.newPortForward = func(*rest.Config, *url.URL, PortForwardRequest, <-chan struct{}, chan struct{}, io.Writer, io.Writer) (portForwardRunner, error) {
		return nil, nil
	}
	if handle, err := nilRunner.StartPortForward(context.Background(), valid); handle != nil || !errors.Is(err, ErrPortForwarderUnavailable) {
		t.Fatalf("StartPortForward() nil runner = (%v, %v)", handle, err)
	}
}

func TestPortForwardStopValidationAndDeadlines(t *testing.T) {
	manager := streamingTestManager(t)
	var missingContext context.Context
	if err := manager.StopPortForward(missingContext, "session"); !errors.Is(err, ErrContextRequired) {
		t.Fatalf("StopPortForward(nil) error = %v", err)
	}
	if err := manager.StopPortForward(context.Background(), " "); !errors.Is(err, ErrPortForwardIDRequired) {
		t.Fatalf("StopPortForward(empty) error = %v", err)
	}
	if err := manager.StopAllPortForwards(missingContext); !errors.Is(err, ErrContextRequired) {
		t.Fatalf("StopAllPortForwards(nil) error = %v", err)
	}
	if err := manager.StopAllPortForwards(context.Background()); err != nil {
		t.Fatalf("StopAllPortForwards(empty) error = %v", err)
	}

	stalled := &portForwardProcess{
		info:   PortForwardSession{ID: "stalled"},
		stop:   make(chan struct{}),
		done:   make(chan struct{}),
		exit:   make(chan PortForwardExit, 1),
		cancel: func() {},
	}
	manager.portForwards.add(stalled)
	expired, cancelExpired := context.WithCancel(context.Background())
	cancelExpired()
	if err := manager.StopPortForward(expired, stalled.info.ID); !errors.Is(err, context.Canceled) {
		t.Fatalf("StopPortForward(expired) error = %v", err)
	}
	if err := manager.StopAllPortForwards(expired); !errors.Is(err, context.Canceled) {
		t.Fatalf("StopAllPortForwards(expired) error = %v", err)
	}
	manager.portForwards.finish(stalled.info.ID)
	close(stalled.done)
}

func TestAwaitPortForwardStartHandlesReadiness(t *testing.T) {
	t.Run("ready", func(t *testing.T) {
		manager := newTestManager(t)
		process := testPortForwardProcess("ready", time.Time{})
		manager.portForwards.add(process)
		close(process.ready)
		if err := manager.awaitPortForwardStart(context.Background(), context.Background(), process); err != nil {
			t.Fatalf("awaitPortForwardStart() error = %v", err)
		}
	})
	t.Run("ready after cancellation", func(t *testing.T) {
		manager := newTestManager(t)
		process := testPortForwardProcess("canceled-ready", time.Time{})
		manager.portForwards.add(process)
		close(process.ready)
		parent, cancel := context.WithCancel(context.Background())
		cancel()
		if err := manager.awaitPortForwardStart(parent, context.Background(), process); !errors.Is(err, context.Canceled) {
			t.Fatalf("awaitPortForwardStart() error = %v", err)
		}
	})
	t.Run("ready after removal", func(t *testing.T) {
		manager := newTestManager(t)
		process := testPortForwardProcess("removed", time.Time{})
		process.exitError = errors.New("ended")
		close(process.ready)
		result := awaitPortForwardStartAsync(context.Background(), context.Background(), manager, process)
		requireAwaitStartStillBlocked(t, result)
		close(process.done)
		if err := <-result; !errors.Is(err, process.exitError) {
			t.Fatalf("awaitPortForwardStart() error = %v", err)
		}
	})
}

func TestAwaitPortForwardStartHandlesProcessTermination(t *testing.T) {
	t.Run("done after cancellation", func(t *testing.T) {
		manager := newTestManager(t)
		process := testPortForwardProcess("canceled-done", time.Time{})
		close(process.done)
		parent, cancel := context.WithCancel(context.Background())
		cancel()
		if err := manager.awaitPortForwardStart(parent, context.Background(), process); !errors.Is(err, context.Canceled) {
			t.Fatalf("awaitPortForwardStart() error = %v", err)
		}
	})
	t.Run("internal context completion", func(t *testing.T) {
		manager := newTestManager(t)
		process := testPortForwardProcess("internal", time.Time{})
		session, cancel := context.WithCancel(context.Background())
		cancel()
		result := awaitPortForwardStartAsync(context.Background(), session, manager, process)
		requireAwaitStartStillBlocked(t, result)
		close(process.done)
		if err := <-result; !errors.Is(err, ErrPortForwardReadiness) {
			t.Fatalf("awaitPortForwardStart() error = %v", err)
		}
	})
	t.Run("timeout", func(t *testing.T) {
		manager := newTestManager(t)
		manager.forwardTimeout = time.Millisecond
		process := testPortForwardProcess("timeout", time.Time{})
		if err := manager.awaitPortForwardStart(context.Background(), context.Background(), process); !errors.Is(err, ErrPortForwardReadiness) {
			t.Fatalf("awaitPortForwardStart() error = %v", err)
		}
	})
}

func awaitPortForwardStartAsync(parent, session context.Context, manager *Manager, process *portForwardProcess) <-chan error {
	result := make(chan error, 1)
	go func() {
		result <- manager.awaitPortForwardStart(parent, session, process)
	}()
	return result
}

func requireAwaitStartStillBlocked(t *testing.T, result <-chan error) {
	t.Helper()
	select {
	case err := <-result:
		t.Fatalf("awaitPortForwardStart() returned before completion: %v", err)
	case <-time.After(time.Millisecond):
	}
}

func TestPortForwardCancellationClassification(t *testing.T) {
	process := testPortForwardProcess("session", time.Time{})
	if err := portForwardCancellation(context.Background(), context.Background(), process); err != nil {
		t.Fatalf("active cancellation = %v", err)
	}
	parent, cancelParent := context.WithCancel(context.Background())
	cancelParent()
	if err := portForwardCancellation(parent, context.Background(), process); !errors.Is(err, context.Canceled) {
		t.Fatalf("parent cancellation = %v", err)
	}
	process.contextCanceled.Store(true)
	session, cancelSession := context.WithCancel(context.Background())
	cancelSession()
	if err := portForwardCancellation(context.Background(), session, process); !errors.Is(err, context.Canceled) {
		t.Fatalf("session cancellation = %v", err)
	}
	if err := portForwardCancellation(context.Background(), context.Background(), process); !errors.Is(err, context.Canceled) {
		t.Fatalf("recorded cancellation = %v", err)
	}
}

func TestStopAllPortForwardsStopsEverySession(t *testing.T) {
	manager := streamingTestManager(t)
	manager.newPortForward = waitingPortForwardFactory(errors.New("ignored"))
	first, err := manager.StartPortForward(context.Background(), testPortForwardRequest(t, "team-a", "api-0", 18080, 8080))
	if err != nil {
		t.Fatalf("first StartPortForward() error = %v", err)
	}
	second, err := manager.StartPortForward(context.Background(), testPortForwardRequest(t, "team-a", "api-1", 18081, 8080))
	if err != nil {
		t.Fatalf("second StartPortForward() error = %v", err)
	}
	if err := manager.StopAllPortForwards(context.Background()); err != nil {
		t.Fatalf("StopAllPortForwards() error = %v", err)
	}
	if receivePortForwardExit(t, first.Exit()).Err != nil || receivePortForwardExit(t, second.Exit()).Err != nil {
		t.Fatal("requested stop exposed runner errors")
	}
}

func TestPodPortForwardURLValidatesClientShape(t *testing.T) {
	pod := PodReference{Namespace: "team-a", Name: "api-0"}
	if streamURL, err := podPortForwardURL(nil, pod); streamURL != nil || !errors.Is(err, ErrTypedClientUnavailable) {
		t.Fatalf("podPortForwardURL(nil) = (%v, %v)", streamURL, err)
	}
	if streamURL, err := podPortForwardURL(&Clients{}, pod); streamURL != nil || !errors.Is(err, ErrTypedClientUnavailable) {
		t.Fatalf("podPortForwardURL(empty) = (%v, %v)", streamURL, err)
	}
	clients := &Clients{kubernetes: kubernetesfake.NewSimpleClientset()}
	if streamURL, err := podPortForwardURL(clients, pod); streamURL != nil || !errors.Is(err, ErrPortForwarderUnavailable) {
		t.Fatalf("podPortForwardURL(fake client) = (%v, %v)", streamURL, err)
	}
}

func portForwardStreamURL(t *testing.T) *url.URL {
	t.Helper()
	streamURL, err := url.Parse("https://cluster.invalid/api/v1/namespaces/team-a/pods/api-0/portforward")
	if err != nil {
		t.Fatalf("url.Parse() error = %v", err)
	}
	return streamURL
}

func TestDefaultPortForwarderValidatesInputs(t *testing.T) {
	streamURL := portForwardStreamURL(t)
	request := testPortForwardRequest(t, "team-a", "api-0", 18080, 8080)
	stop := make(chan struct{})
	ready := make(chan struct{})
	if runner, buildErr := defaultPortForwarder(nil, streamURL, request, stop, ready, io.Discard, io.Discard); runner != nil || !errors.Is(buildErr, ErrPortForwarderUnavailable) {
		t.Fatalf("defaultPortForwarder(nil) = (%v, %v)", runner, buildErr)
	}
	if runner, buildErr := defaultPortForwarder(&rest.Config{}, nil, request, stop, ready, io.Discard, io.Discard); runner != nil || !errors.Is(buildErr, ErrPortForwarderUnavailable) {
		t.Fatalf("defaultPortForwarder(nil URL) = (%v, %v)", runner, buildErr)
	}
	if runner, buildErr := defaultPortForwarder(&rest.Config{Host: "https://cluster.invalid"}, streamURL, request, stop, ready, io.Discard, io.Discard); runner == nil || buildErr != nil {
		t.Fatalf("defaultPortForwarder() = (%v, %v)", runner, buildErr)
	}
}

func TestBuildPortForwarderUsesConstructors(t *testing.T) {
	streamURL := portForwardStreamURL(t)
	request := testPortForwardRequest(t, "team-a", "api-0", 18080, 8080)
	stop := make(chan struct{})
	ready := make(chan struct{})
	sentinel := errors.New("construction failed")
	base := portForwardConstructors{
		roundTripper: func(*rest.Config) (http.RoundTripper, spdy.Upgrader, error) {
			return http.DefaultTransport, spdy.NewUpgraderForStreaming(spdyUpgraderStub{}), nil
		},
		webSocket: func(*url.URL, *rest.Config) (streamhttp.Dialer, error) {
			return streamingDialerStub{}, nil
		},
		forwarder: func(_ streamhttp.Dialer, addresses, ports []string, _ <-chan struct{}, _ chan struct{}, _ io.Writer, _ io.Writer) (portForwardRunner, error) {
			if !slices.Equal(addresses, []string{portForwardLoopbackAddress}) || !slices.Equal(ports, []string{"18080:8080"}) {
				t.Fatalf("forwarder arguments = (%v, %v)", addresses, ports)
			}
			return portForwardRunnerStub{}, nil
		},
	}
	failures := []struct {
		name     string
		sabotage func(*portForwardConstructors)
	}{
		{name: "transport", sabotage: func(constructors *portForwardConstructors) {
			constructors.roundTripper = func(*rest.Config) (http.RoundTripper, spdy.Upgrader, error) { return nil, nil, sentinel }
		}},
		{name: "WebSocket", sabotage: func(constructors *portForwardConstructors) {
			constructors.webSocket = func(*url.URL, *rest.Config) (streamhttp.Dialer, error) { return nil, sentinel }
		}},
		{name: "forwarder", sabotage: func(constructors *portForwardConstructors) {
			constructors.forwarder = func(streamhttp.Dialer, []string, []string, <-chan struct{}, chan struct{}, io.Writer, io.Writer) (portForwardRunner, error) {
				return nil, sentinel
			}
		}},
	}
	for _, test := range failures {
		t.Run(test.name, func(t *testing.T) {
			constructors := base
			test.sabotage(&constructors)
			if runner, buildErr := buildPortForwarder(&rest.Config{}, streamURL, request, stop, ready, io.Discard, io.Discard, constructors); runner != nil || !errors.Is(buildErr, sentinel) {
				t.Fatalf("buildPortForwarder() = (%v, %v)", runner, buildErr)
			}
		})
	}
	if runner, buildErr := buildPortForwarder(&rest.Config{}, streamURL, request, stop, ready, io.Discard, io.Discard, base); runner == nil || buildErr != nil {
		t.Fatalf("buildPortForwarder() = (%v, %v)", runner, buildErr)
	}
}

func TestPortForwardProcessNilAccessors(t *testing.T) {
	var nilProcess *portForwardProcess
	if nilProcess.Session() != (PortForwardSession{}) || nilProcess.Exit() != nil {
		t.Fatal("nil process accessors returned state")
	}
	nilProcess.requestStop()
	nilProcess.cancelFromContext()
}

func TestPortForwardRegistryTracksSessions(t *testing.T) {
	registry := newPortForwardRegistry()
	if registry.markRunning("missing") {
		t.Fatal("markRunning() found a missing session")
	}
	startedAt := time.Date(2026, time.August, 29, 8, 0, 0, 0, time.UTC)
	second := testPortForwardProcess("second", startedAt)
	first := testPortForwardProcess("first", startedAt)
	starting := testPortForwardProcess("starting", startedAt.Add(-time.Minute))
	third := testPortForwardProcess("third", startedAt.Add(time.Minute))
	registry.add(second)
	registry.add(first)
	registry.add(starting)
	registry.add(third)
	registry.markRunning(second.info.ID)
	registry.markRunning(first.info.ID)
	registry.markRunning(third.info.ID)
	if sessions := registry.snapshot(); len(sessions) != 3 || sessions[0].ID != "first" || sessions[1].ID != "second" || sessions[2].ID != "third" {
		t.Fatalf("snapshot() = %+v", sessions)
	}
	if len(registry.active()) != 4 || registry.get("first") != first {
		t.Fatal("registry active/get mismatch")
	}
	registry.finish("first")
	registry.finish("missing")
	if registry.get("first") != nil {
		t.Fatal("finish() did not remove session")
	}
}

func TestPortForwardProcessCompletionAndStop(t *testing.T) {
	startedAt := time.Date(2026, time.August, 29, 8, 0, 0, 0, time.UTC)
	process := testPortForwardProcess("complete", startedAt)
	watchStopped := false
	process.stopContextWatch = func() bool {
		watchStopped = true
		return true
	}
	sentinel := errors.New("finished")
	process.complete(sentinel)
	if !watchStopped || !errors.Is(process.result(), sentinel) {
		t.Fatalf("complete() result = %v, watch stopped = %t", process.result(), watchStopped)
	}
	if exit := receivePortForwardExit(t, process.Exit()); !errors.Is(exit.Err, sentinel) {
		t.Fatalf("process exit = %+v", exit)
	}
	process.requestStop()
	process.requestStop()
	if !process.stopRequested.Load() {
		t.Fatal("requestStop() did not record the stop")
	}
}

func TestPortForwardRunErrorWithoutDiagnostic(t *testing.T) {
	sentinel := errors.New("stream failed")
	err := portForwardRunError(PodReference{Namespace: "team-a", Name: "api-0"}, sentinel, " ")
	if !errors.Is(err, sentinel) || strings.Contains(err.Error(), "  ") {
		t.Fatalf("portForwardRunError() = %v", err)
	}
}

func TestPortForwardDiagnosticsKeepBoundedTail(t *testing.T) {
	disabled := newPortForwardDiagnostics(0)
	if written, err := disabled.Write([]byte("ignored")); written != len("ignored") || err != nil || disabled.String() != "" {
		t.Fatalf("disabled diagnostics = (%d, %v, %q)", written, err, disabled.String())
	}
	diagnostics := newPortForwardDiagnostics(5)
	_, _ = diagnostics.Write([]byte("ab"))
	_, _ = diagnostics.Write([]byte("cde"))
	if diagnostics.String() != "abcde" {
		t.Fatalf("diagnostics = %q", diagnostics.String())
	}
	_, _ = diagnostics.Write([]byte("fg"))
	if diagnostics.String() != "cdefg" {
		t.Fatalf("diagnostic tail = %q", diagnostics.String())
	}
	_, _ = diagnostics.Write([]byte("1234567"))
	if diagnostics.String() != "34567" {
		t.Fatalf("large diagnostic tail = %q", diagnostics.String())
	}
}

func TestPortForwardsOnNilManagerIsEmpty(t *testing.T) {
	var manager *Manager
	if sessions := manager.PortForwards(); sessions == nil || len(sessions) != 0 {
		t.Fatalf("nil manager PortForwards() = %+v", sessions)
	}
	manager = &Manager{}
	if sessions := manager.PortForwards(); sessions == nil || len(sessions) != 0 {
		t.Fatalf("empty manager PortForwards() = %+v", sessions)
	}
}

func waitingPortForwardFactory(runErr error) portForwardFactory {
	return func(
		_ *rest.Config,
		_ *url.URL,
		_ PortForwardRequest,
		stop <-chan struct{},
		ready chan struct{},
		_ io.Writer,
		_ io.Writer,
	) (portForwardRunner, error) {
		return portForwardRunnerStub{forward: func() error {
			close(ready)
			<-stop
			return runErr
		}}, nil
	}
}

func testPortForwardRequest(t *testing.T, namespace, pod string, local, remote int) PortForwardRequest {
	t.Helper()
	localPort, err := NewNetworkPort(local)
	if err != nil {
		t.Fatalf("NewNetworkPort(%d) error = %v", local, err)
	}
	remotePort, err := NewNetworkPort(remote)
	if err != nil {
		t.Fatalf("NewNetworkPort(%d) error = %v", remote, err)
	}
	return PortForwardRequest{
		Pod:        PodReference{Namespace: namespace, Name: pod},
		LocalPort:  localPort,
		RemotePort: remotePort,
	}
}

func testPortForwardProcess(sessionID string, startedAt time.Time) *portForwardProcess {
	return &portForwardProcess{
		info:   PortForwardSession{ID: sessionID, StartedAt: startedAt},
		stop:   make(chan struct{}),
		ready:  make(chan struct{}),
		done:   make(chan struct{}),
		exit:   make(chan PortForwardExit, 1),
		cancel: func() {},
	}
}

func receivePortForwardExit(t *testing.T, exits <-chan PortForwardExit) PortForwardExit {
	t.Helper()
	select {
	case exit := <-exits:
		return exit
	case <-time.After(streamTestTimeout):
		t.Fatal("timed out waiting for port-forward exit")
		return PortForwardExit{}
	}
}
