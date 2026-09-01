package kube

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"k8s.io/client-go/rest"
	clientportforward "k8s.io/client-go/tools/portforward"
	"k8s.io/client-go/transport/spdy"
	streamhttp "k8s.io/streaming/pkg/httpstream"
)

const (
	defaultPortForwardReadinessTimeout = 10 * time.Second
	portForwardDiagnosticLimit         = 32 * 1024
	portForwardLoopbackAddress         = "127.0.0.1"
)

type portForwardRunner interface {
	ForwardPorts() error
}

type portForwardFactory func(
	*rest.Config,
	*url.URL,
	PortForwardRequest,
	<-chan struct{},
	chan struct{},
	io.Writer,
	io.Writer,
) (portForwardRunner, error)

type portForwardConstructors struct {
	roundTripper func(*rest.Config) (http.RoundTripper, spdy.Upgrader, error)
	webSocket    func(*url.URL, *rest.Config) (streamhttp.Dialer, error)
	forwarder    func(streamhttp.Dialer, []string, []string, <-chan struct{}, chan struct{}, io.Writer, io.Writer) (portForwardRunner, error)
}

var defaultPortForwardConstructors = portForwardConstructors{
	roundTripper: spdy.RoundTripperFor,
	webSocket:    clientportforward.NewSPDYOverWebsocketDialerForStreaming,
	forwarder: func(
		dialer streamhttp.Dialer,
		addresses []string,
		ports []string,
		stop <-chan struct{},
		ready chan struct{},
		output io.Writer,
		errorOutput io.Writer,
	) (portForwardRunner, error) {
		return clientportforward.NewOnAddressesForStreaming(dialer, addresses, ports, stop, ready, output, errorOutput)
	},
}

type portForwardProcess struct {
	info             PortForwardSession
	stop             chan struct{}
	ready            chan struct{}
	done             chan struct{}
	exit             chan PortForwardExit
	cancel           context.CancelFunc
	stopContextWatch func() bool
	stopOnce         sync.Once
	stopRequested    atomic.Bool
	contextCanceled  atomic.Bool
	exitErrorMu      sync.RWMutex
	exitError        error
}

type portForwardRegistry struct {
	mu       sync.RWMutex
	sessions map[string]*portForwardProcess
}

type portForwardDiagnostics struct {
	mu      sync.Mutex
	content []byte
	limit   int
}

type preparedPortForward struct {
	process     *portForwardProcess
	runner      portForwardRunner
	diagnostics *portForwardDiagnostics
}

func (m *Manager) StartPortForward(parent context.Context, request PortForwardRequest) (PortForward, error) {
	if err := validatePortForwardRequest(request); err != nil {
		return nil, newResourceError(OperationStart, SubjectPortForward, request.Pod.Identifier(), err)
	}
	if parent == nil {
		return nil, newResourceError(OperationStart, SubjectPortForward, request.Pod.Identifier(), ErrContextRequired)
	}
	if err := parent.Err(); err != nil {
		return nil, newResourceError(OperationStart, SubjectPortForward, request.Pod.Identifier(), err)
	}
	clients, ctx, cancel, err := m.clientSession(parent)
	if err != nil {
		return nil, newResourceError(OperationStart, SubjectPortForward, request.Pod.Identifier(), err)
	}
	prepared, err := m.preparePortForward(clients, cancel, request)
	if err != nil {
		cancel()
		return nil, newResourceError(OperationBuild, SubjectPortForward, request.Pod.Identifier(), err)
	}
	prepared.process.stopContextWatch = context.AfterFunc(ctx, prepared.process.cancelFromContext)
	m.portForwards.add(prepared.process)
	go m.runPortForward(prepared.process, prepared.runner, prepared.diagnostics)
	if err := m.awaitPortForwardStart(parent, ctx, prepared.process); err != nil {
		return nil, newResourceError(OperationStart, SubjectPortForward, request.Pod.Identifier(), err)
	}
	return prepared.process, nil
}

func (m *Manager) preparePortForward(clients *Clients, cancel context.CancelFunc, request PortForwardRequest) (preparedPortForward, error) {
	streamURL, err := podPortForwardURL(clients, request.Pod)
	if err != nil {
		return preparedPortForward{}, err
	}
	if m.newPortForward == nil {
		return preparedPortForward{}, ErrPortForwarderUnavailable
	}
	diagnostics := newPortForwardDiagnostics(portForwardDiagnosticLimit)
	process := m.newPortForwardProcess(cancel, request)
	runner, err := m.newPortForward(
		clients.RESTConfig(), streamURL, request, process.stop, process.ready, io.Discard, diagnostics,
	)
	if err != nil {
		return preparedPortForward{}, err
	}
	if runner == nil {
		return preparedPortForward{}, ErrPortForwarderUnavailable
	}
	return preparedPortForward{process: process, runner: runner, diagnostics: diagnostics}, nil
}

func (m *Manager) awaitPortForwardStart(parent, session context.Context, process *portForwardProcess) error {
	timer := time.NewTimer(m.forwardTimeout)
	defer timer.Stop()
	var cause error
	select {
	case <-process.ready:
		if cause = portForwardCancellation(parent, session, process); cause == nil {
			if m.portForwards.markRunning(process.info.ID) {
				return nil
			}
			cause = portForwardStartCause(process, ErrPortForwardReadiness)
		}
	case <-process.done:
		cause = portForwardCancellation(parent, session, process)
		if cause == nil {
			cause = portForwardStartCause(process, ErrPortForwardReadiness)
		}
	case <-session.Done():
		cause = portForwardCancellation(parent, session, process)
		if cause == nil {
			cause = portForwardStartCause(process, ErrPortForwardReadiness)
		}
	case <-timer.C:
		cause = ErrPortForwardReadiness
	}
	return m.abandonPortForwardStart(process, cause)
}

func (m *Manager) abandonPortForwardStart(process *portForwardProcess, cause error) error {
	process.requestStop()
	m.portForwards.finish(process.info.ID)
	return cause
}

func portForwardCancellation(parent, session context.Context, process *portForwardProcess) error {
	if err := parent.Err(); err != nil {
		return err
	}
	if process.contextCanceled.Load() {
		if err := session.Err(); err != nil {
			return err
		}
		return context.Canceled
	}
	return nil
}

func (m *Manager) StopPortForward(ctx context.Context, sessionID string) error {
	if ctx == nil {
		return newResourceError(OperationStop, SubjectPortForward, sessionID, ErrContextRequired)
	}
	if strings.TrimSpace(sessionID) == "" {
		return newResourceError(OperationStop, SubjectPortForward, sessionID, ErrPortForwardIDRequired)
	}
	process := m.portForwards.get(sessionID)
	if process == nil {
		return nil
	}
	process.requestStop()
	select {
	case <-process.done:
		return process.result()
	case <-ctx.Done():
		return newResourceError(OperationStop, SubjectPortForward, sessionID, ctx.Err())
	}
}

func (m *Manager) StopAllPortForwards(ctx context.Context) error {
	if ctx == nil {
		return newError(OperationStop, SubjectPortForward, "", ErrContextRequired)
	}
	processes := m.portForwards.active()
	for _, process := range processes {
		process.requestStop()
	}
	for _, process := range processes {
		select {
		case <-process.done:
		case <-ctx.Done():
			return newError(OperationStop, SubjectPortForward, "", ctx.Err())
		}
	}
	return nil
}

func (m *Manager) PortForwards() []PortForwardSession {
	if m == nil || m.portForwards == nil {
		return []PortForwardSession{}
	}
	return m.portForwards.snapshot()
}

func podPortForwardURL(clients *Clients, pod PodReference) (*url.URL, error) {
	if clients == nil || clients.Kubernetes() == nil {
		return nil, ErrTypedClientUnavailable
	}
	restClient := coreRESTClient(clients)
	if restClient == nil {
		return nil, ErrPortForwarderUnavailable
	}
	return restClient.Post().
		Resource("pods").
		Namespace(pod.Namespace).
		Name(pod.Name).
		SubResource("portforward").
		URL(), nil
}

func defaultPortForwarder(
	config *rest.Config,
	streamURL *url.URL,
	request PortForwardRequest,
	stop <-chan struct{},
	ready chan struct{},
	output io.Writer,
	errorOutput io.Writer,
) (portForwardRunner, error) {
	return buildPortForwarder(config, streamURL, request, stop, ready, output, errorOutput, defaultPortForwardConstructors)
}

func buildPortForwarder(
	config *rest.Config,
	streamURL *url.URL,
	request PortForwardRequest,
	stop <-chan struct{},
	ready chan struct{},
	output io.Writer,
	errorOutput io.Writer,
	constructors portForwardConstructors,
) (portForwardRunner, error) {
	if config == nil || streamURL == nil {
		return nil, ErrPortForwarderUnavailable
	}
	transport, upgrader, err := constructors.roundTripper(config)
	if err != nil {
		return nil, err
	}
	spdyDialer := spdy.NewDialerForStreaming(upgrader, &http.Client{Transport: transport}, "POST", streamURL)
	webSocketDialer, err := constructors.webSocket(streamURL, config)
	if err != nil {
		return nil, err
	}
	dialer := clientportforward.NewFallbackDialerForStreaming(webSocketDialer, spdyDialer, shouldFallbackStream)
	ports := []string{request.LocalPort.String() + ":" + request.RemotePort.String()}
	return constructors.forwarder(
		dialer,
		[]string{portForwardLoopbackAddress},
		ports,
		stop,
		ready,
		output,
		errorOutput,
	)
}

func (m *Manager) newPortForwardProcess(cancel context.CancelFunc, request PortForwardRequest) *portForwardProcess {
	return &portForwardProcess{
		info: PortForwardSession{
			ID:         fmt.Sprintf("port-forward-%d", m.forwardSequence.Add(1)),
			Pod:        request.Pod,
			LocalPort:  request.LocalPort,
			RemotePort: request.RemotePort,
			StartedAt:  m.clock(),
		},
		stop:   make(chan struct{}),
		ready:  make(chan struct{}),
		done:   make(chan struct{}),
		exit:   make(chan PortForwardExit, 1),
		cancel: cancel,
	}
}

func (m *Manager) runPortForward(process *portForwardProcess, runner portForwardRunner, diagnostics *portForwardDiagnostics) {
	runErr := runner.ForwardPorts()
	if process.stopRequested.Load() {
		runErr = nil
	} else if runErr != nil {
		runErr = portForwardRunError(process.info.Pod, runErr, diagnostics.String())
	}
	m.portForwards.finish(process.info.ID)
	process.complete(runErr)
}

func portForwardStartCause(process *portForwardProcess, fallback error) error {
	err := process.result()
	if err == nil {
		err = fallback
	}
	return err
}

func portForwardRunError(pod PodReference, runErr error, diagnostic string) error {
	diagnostic = strings.TrimSpace(diagnostic)
	if diagnostic != "" {
		runErr = fmt.Errorf("%s: %w", diagnostic, runErr)
	}
	return newResourceError(OperationStream, SubjectPortForward, pod.Identifier(), runErr)
}

func (p *portForwardProcess) Session() PortForwardSession {
	if p == nil {
		return PortForwardSession{}
	}
	return p.info
}

func (p *portForwardProcess) Exit() <-chan PortForwardExit {
	if p == nil {
		return nil
	}
	return p.exit
}

func (p *portForwardProcess) requestStop() {
	if p == nil {
		return
	}
	p.stopOnce.Do(func() {
		p.stopRequested.Store(true)
		close(p.stop)
		p.cancel()
	})
}

func (p *portForwardProcess) cancelFromContext() {
	if p == nil {
		return
	}
	p.contextCanceled.Store(true)
	p.requestStop()
}

func (p *portForwardProcess) complete(err error) {
	if p.stopContextWatch != nil {
		p.stopContextWatch()
	}
	p.exitErrorMu.Lock()
	p.exitError = err
	p.exitErrorMu.Unlock()
	close(p.done)
	p.exit <- PortForwardExit{SessionID: p.info.ID, Err: err}
	close(p.exit)
	p.cancel()
}

func (p *portForwardProcess) result() error {
	p.exitErrorMu.RLock()
	defer p.exitErrorMu.RUnlock()
	return p.exitError
}

func newPortForwardRegistry() *portForwardRegistry {
	return &portForwardRegistry{sessions: make(map[string]*portForwardProcess)}
}

func (r *portForwardRegistry) add(process *portForwardProcess) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.sessions[process.info.ID] = process
}

func (r *portForwardRegistry) markRunning(sessionID string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	process := r.sessions[sessionID]
	if process == nil {
		return false
	}
	process.info.Status = PortForwardRunning
	return true
}

func (r *portForwardRegistry) get(sessionID string) *portForwardProcess {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.sessions[sessionID]
}

func (r *portForwardRegistry) finish(sessionID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.sessions, sessionID)
}

func (r *portForwardRegistry) active() []*portForwardProcess {
	r.mu.RLock()
	defer r.mu.RUnlock()
	processes := make([]*portForwardProcess, 0, len(r.sessions))
	for _, process := range r.sessions {
		processes = append(processes, process)
	}
	return processes
}

func (r *portForwardRegistry) snapshot() []PortForwardSession {
	r.mu.RLock()
	sessions := make([]PortForwardSession, 0, len(r.sessions))
	for _, process := range r.sessions {
		if process.info.Status == PortForwardRunning {
			sessions = append(sessions, process.info)
		}
	}
	r.mu.RUnlock()
	slices.SortFunc(sessions, func(left, right PortForwardSession) int {
		if order := left.StartedAt.Compare(right.StartedAt); order != 0 {
			return order
		}
		return strings.Compare(left.ID, right.ID)
	})
	return sessions
}

func newPortForwardDiagnostics(limit int) *portForwardDiagnostics {
	return &portForwardDiagnostics{limit: limit}
}

func (d *portForwardDiagnostics) Write(data []byte) (int, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.limit <= 0 {
		return len(data), nil
	}
	combinedLength := len(d.content) + len(data)
	if combinedLength <= d.limit {
		d.content = append(d.content, data...)
		return len(data), nil
	}
	keepFromData := min(len(data), d.limit)
	keepFromContent := d.limit - keepFromData
	if keepFromContent > 0 {
		copy(d.content[:keepFromContent], d.content[len(d.content)-keepFromContent:])
	}
	d.content = append(d.content[:keepFromContent], data[len(data)-keepFromData:]...)
	return len(data), nil
}

func (d *portForwardDiagnostics) String() string {
	d.mu.Lock()
	defer d.mu.Unlock()
	return string(d.content)
}

var _ PortForward = (*portForwardProcess)(nil)
