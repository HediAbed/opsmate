package service

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	tea "charm.land/bubbletea/v2"
)

const (
	maxNetworkPort             = 65535
	portForwardReadyTimeout    = 10 * time.Second
	portForwardStopTimeout     = 2 * time.Second
	portForwardDiagnosticLimit = 32 * 1024
	portForwardReadyMarker     = "Forwarding from "
)

type PortForwardStatus string

const (
	PortForwardRunning PortForwardStatus = "running"
)

var ErrPortForwardReadinessTimeout = errors.New("port-forward readiness timed out")

type PortForwardError struct {
	Stage     string
	Namespace string
	Pod       string
	Err       error
}

func (e *PortForwardError) Error() string {
	prefix := "port-forward"
	if e.Namespace != "" && e.Pod != "" {
		prefix += " " + e.Namespace + "/" + e.Pod
	}
	if e.Stage != "" {
		prefix += " (" + e.Stage + ")"
	}
	if e.Err == nil {
		return prefix + ": unknown error"
	}
	return prefix + ": " + e.Err.Error()
}

func (e *PortForwardError) Unwrap() error {
	return e.Err
}

type PortForwardSession struct {
	ID         string
	Namespace  string
	Pod        string
	LocalPort  int
	RemotePort int
	Started    time.Time
	Status     PortForwardStatus
}

type PortForwardHandle struct {
	PortForwardSession
	process *portForwardProcess
}

type portForwardProcess struct {
	info PortForwardSession

	cmd           *exec.Cmd
	done          chan struct{}
	stopRequested chan struct{}
	stopOnce      sync.Once
	exitErr       error
	kill          func() error
}

type portForwardRegistry struct {
	mu       sync.Mutex
	sessions map[string]*portForwardProcess
}

var pfRegistry = &portForwardRegistry{sessions: map[string]*portForwardProcess{}}

var pfIDCounter uint64

type PortForwardStartedMsg struct {
	Session *PortForwardHandle
	Err     error
}

type PortForwardStoppedMsg struct {
	ID  string
	Err error
}

func StartPortForward(namespace, pod string, localPort, remotePort int) tea.Cmd {
	return startPortForwardWithTimeout(namespace, pod, localPort, remotePort, portForwardReadyTimeout)
}

func startPortForwardWithTimeout(namespace, pod string, localPort, remotePort int, readyTimeout time.Duration) tea.Cmd {
	return func() tea.Msg {
		if err := validatePortForward(namespace, pod, localPort, remotePort); err != nil {
			return PortForwardStartedMsg{Err: err}
		}

		portArgument := strconv.Itoa(localPort) + ":" + strconv.Itoa(remotePort)
		command := newExternalCommand(kubectlBinary, "port-forward", pod, "-n", namespace, portArgument)
		diagnostics := newPortForwardOutput()
		command.Stdout = diagnostics
		command.Stderr = diagnostics
		if err := command.Start(); err != nil {
			return PortForwardStartedMsg{Err: newPortForwardError("start", namespace, pod, err)}
		}

		waitResult := make(chan error, 1)
		go func() {
			waitResult <- command.Wait()
			close(waitResult)
		}()

		timer := time.NewTimer(readyTimeout)
		defer timer.Stop()
		select {
		case <-diagnostics.Ready():
			process := newPortForwardProcess(command, namespace, pod, localPort, remotePort)
			pfRegistry.add(process)
			go monitorPortForward(process, waitResult, diagnostics)
			return PortForwardStartedMsg{Session: &PortForwardHandle{
				PortForwardSession: process.info,
				process:            process,
			}}
		case waitErr := <-waitResult:
			return PortForwardStartedMsg{
				Err: portForwardProcessError("readiness", namespace, pod, waitErr, diagnostics.String()),
			}
		case <-timer.C:
			killErr := command.Process.Kill()
			waitErr := <-waitResult
			waitErr = mergePortForwardKillError(waitErr, killErr)
			return PortForwardStartedMsg{Err: newPortForwardError(
				"readiness",
				namespace,
				pod,
				errors.Join(ErrPortForwardReadinessTimeout, waitErr),
			)}
		}
	}
}

func mergePortForwardKillError(waitErr, killErr error) error {
	if killErr == nil || errors.Is(killErr, os.ErrProcessDone) {
		return waitErr
	}
	return errors.Join(waitErr, killErr)
}

func validatePortForward(namespace, pod string, localPort, remotePort int) error {
	if err := requireNamespace("port-forward", namespace); err != nil {
		return err
	}
	if strings.TrimSpace(pod) == "" {
		return newPortForwardError("validate", namespace, pod, errors.New("pod is required"))
	}
	if localPort < 1 || localPort > maxNetworkPort {
		return newPortForwardError("validate", namespace, pod, fmt.Errorf("local port must be between 1 and %d", maxNetworkPort))
	}
	if remotePort < 1 || remotePort > maxNetworkPort {
		return newPortForwardError("validate", namespace, pod, fmt.Errorf("remote port must be between 1 and %d", maxNetworkPort))
	}
	return nil
}

func newPortForwardProcess(
	command *exec.Cmd,
	namespace string,
	pod string,
	localPort int,
	remotePort int,
) *portForwardProcess {
	return &portForwardProcess{
		info: PortForwardSession{
			ID:         fmt.Sprintf("pf-%d", atomic.AddUint64(&pfIDCounter, 1)),
			Namespace:  namespace,
			Pod:        pod,
			LocalPort:  localPort,
			RemotePort: remotePort,
			Started:    time.Now(),
			Status:     PortForwardRunning,
		},
		cmd:           command,
		done:          make(chan struct{}),
		stopRequested: make(chan struct{}),
		kill:          command.Process.Kill,
	}
}

func monitorPortForward(process *portForwardProcess, waitResult <-chan error, diagnostics *portForwardOutput) {
	waitErr := <-waitResult
	if process.wasStopRequested() {
		waitErr = nil
	} else if waitErr != nil {
		waitErr = portForwardProcessError(
			"run",
			process.info.Namespace,
			process.info.Pod,
			waitErr,
			diagnostics.String(),
		)
	}
	pfRegistry.finish(process.info.ID, waitErr)
	close(process.done)
}

func StopPortForward(id string) tea.Cmd {
	return stopPortForwardWithTimeout(id, portForwardStopTimeout)
}

func stopPortForwardWithTimeout(id string, timeout time.Duration) tea.Cmd {
	return func() tea.Msg {
		process := pfRegistry.get(id)
		if process == nil {
			return PortForwardStoppedMsg{ID: id}
		}
		killErr := process.stop()
		select {
		case <-process.done:
			if killErr != nil && !errors.Is(killErr, os.ErrProcessDone) {
				return PortForwardStoppedMsg{ID: id, Err: newPortForwardError(
					"stop", process.info.Namespace, process.info.Pod, killErr,
				)}
			}
			return PortForwardStoppedMsg{ID: id}
		case <-time.After(timeout):
			return PortForwardStoppedMsg{ID: id, Err: newPortForwardError(
				"stop", process.info.Namespace, process.info.Pod, errors.New("process did not exit before timeout"),
			)}
		}
	}
}

func WaitForPortForwardExit(handle *PortForwardHandle) tea.Cmd {
	if handle == nil || handle.process == nil || handle.process.done == nil {
		return nil
	}
	return func() tea.Msg {
		<-handle.process.done
		return PortForwardStoppedMsg{ID: handle.process.info.ID, Err: handle.process.exitErr}
	}
}

func StopAllPortForwards() {
	stopAllPortForwardsWithTimeout(portForwardStopTimeout)
}

func stopAllPortForwardsWithTimeout(timeout time.Duration) {
	sessions := pfRegistry.active()
	for _, process := range sessions {
		_ = process.stop()
	}
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	for _, process := range sessions {
		select {
		case <-process.done:
		case <-deadline.C:
			return
		}
	}
}

func ListPortForwards() []PortForwardSession {
	return pfRegistry.snapshot()
}

func (s *portForwardProcess) stop() error {
	var killErr error
	s.stopOnce.Do(func() {
		close(s.stopRequested)
		if s.kill != nil {
			killErr = s.kill()
		}
	})
	return killErr
}

func (s *portForwardProcess) wasStopRequested() bool {
	select {
	case <-s.stopRequested:
		return true
	default:
		return false
	}
}

func (r *portForwardRegistry) add(process *portForwardProcess) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.sessions[process.info.ID] = process
}

func (r *portForwardRegistry) get(id string) *portForwardProcess {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.sessions[id]
}

func (r *portForwardRegistry) finish(id string, err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	process := r.sessions[id]
	if process == nil {
		return
	}
	process.exitErr = err
	delete(r.sessions, id)
}

func (r *portForwardRegistry) active() []*portForwardProcess {
	r.mu.Lock()
	defer r.mu.Unlock()
	sessions := make([]*portForwardProcess, 0, len(r.sessions))
	for _, process := range r.sessions {
		sessions = append(sessions, process)
	}
	return sessions
}

func (r *portForwardRegistry) snapshot() []PortForwardSession {
	r.mu.Lock()
	defer r.mu.Unlock()
	sessions := make([]PortForwardSession, 0, len(r.sessions))
	for _, process := range r.sessions {
		sessions = append(sessions, process.info)
	}
	return sessions
}

func newPortForwardError(stage, namespace, pod string, err error) error {
	return &PortForwardError{Stage: stage, Namespace: namespace, Pod: pod, Err: err}
}

func portForwardProcessError(stage, namespace, pod string, processErr error, diagnostic string) error {
	diagnostic = strings.TrimSpace(stripANSI(diagnostic))
	if processErr == nil {
		processErr = errors.New("process exited before reporting readiness")
	}
	if diagnostic != "" {
		processErr = fmt.Errorf("%s: %w", diagnostic, processErr)
	}
	return newPortForwardError(stage, namespace, pod, processErr)
}

type portForwardOutput struct {
	buffer *limitedBuffer
	ready  chan struct{}
	once   sync.Once
	mu     sync.Mutex
	tail   string
}

func newPortForwardOutput() *portForwardOutput {
	return &portForwardOutput{
		buffer: newLimitedBuffer(portForwardDiagnosticLimit),
		ready:  make(chan struct{}),
	}
}

func (o *portForwardOutput) Write(data []byte) (int, error) {
	written, _ := o.buffer.Write(data)
	o.mu.Lock()
	candidate := o.tail + string(data)
	if strings.Contains(candidate, portForwardReadyMarker) {
		o.once.Do(func() { close(o.ready) })
	}
	markerTailLength := len(portForwardReadyMarker) - 1
	if len(candidate) > markerTailLength {
		o.tail = candidate[len(candidate)-markerTailLength:]
	} else {
		o.tail = candidate
	}
	o.mu.Unlock()
	return written, nil
}

func (o *portForwardOutput) Ready() <-chan struct{} {
	return o.ready
}

func (o *portForwardOutput) String() string {
	return o.buffer.String()
}
