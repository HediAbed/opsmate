package service

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"sync/atomic"
	"time"
)

const (
	shellScanBufferStart      = 64 * 1024
	shellScanBufferMax        = 4 * 1024 * 1024
	shellOutputCapacity       = 256
	shellInputCapacity        = 64
	shellProcessWaitDelay     = 2 * time.Second
	shellStreamCount          = 2
	shellExitCapacity         = 1
	initialShellErrorCapacity = 4
)

var (
	ErrShellPodRequired          = errors.New("shell: pod required")
	ErrShellNamespaceRequired    = errors.New("shell: namespace required")
	ErrShellNilSession           = errors.New("shell: nil session")
	ErrShellSessionClosed        = errors.New("shell: session closed")
	ErrShellNoProcess            = errors.New("shell: no running process")
	ErrShellInputBackpressure    = errors.New("shell: input queue is full")
	ErrShellInterruptUnsupported = errors.New("shell: interrupt is unsupported on this platform")
)

// ShellOutput carries one stdout or stderr line from a shell process.
type ShellOutput struct {
	SessionID string
	Line      string
	Stderr    bool
}

// ShellExitError is delivered once through Exit when the shell process ends.
type ShellExitError struct {
	SessionID string
	Err       error
}

type ShellSessionIdentity struct {
	ID        string
	Namespace string
	Pod       string
	Container string
}

type ShellStreamError struct {
	Stream string
	Err    error
}

func (e *ShellStreamError) Error() string {
	if e.Err == nil {
		return "shell: " + e.Stream + " stream failed"
	}
	return "shell: " + e.Stream + " stream: " + e.Err.Error()
}

func (e *ShellStreamError) Unwrap() error {
	return e.Err
}

type ShellOutputDroppedError struct {
	Count uint64
}

func (e *ShellOutputDroppedError) Error() string {
	return fmt.Sprintf("shell: %d output lines were dropped because the consumer was too slow", e.Count)
}

// ShellSession owns one kubectl exec process and its input, output, and exit channels.
type ShellSession struct {
	identity      ShellSessionIdentity
	cmd           *exec.Cmd
	stdin         io.WriteCloser
	input         chan string
	inputDone     chan struct{}
	output        chan ShellOutput
	exit          chan ShellExitError
	ctx           context.Context
	cancel        context.CancelFunc
	closeMu       sync.Mutex
	closed        bool
	droppedOutput atomic.Uint64
	asyncErrMu    sync.Mutex
	asyncErr      error
}

var shellSessionCounter atomic.Uint64

type shellProcessPipes struct {
	stdin  io.WriteCloser
	stdout io.ReadCloser
	stderr io.ReadCloser
}

// StartShellSession launches /bin/sh through kubectl exec. An empty container
// selects the pod's default container.
func StartShellSession(namespace, pod, container string) (*ShellSession, error) {
	return StartShellSessionContext(context.Background(), namespace, pod, container)
}

func StartShellSessionContext(parent context.Context, namespace, pod, container string) (*ShellSession, error) {
	if err := validateShellTarget(namespace, pod); err != nil {
		return nil, err
	}

	ctx, cancel := context.WithCancel(parent)
	cmd := newExternalCommandContext(ctx, kubectlBinary, shellCommandArgs(namespace, pod, container)...)
	configureShellCommand(cmd)
	cmd.WaitDelay = shellProcessWaitDelay
	return startShellProcess(ctx, cancel, cmd, namespace, pod, container)
}

func startShellProcess(
	ctx context.Context,
	cancel context.CancelFunc,
	cmd *exec.Cmd,
	namespace string,
	pod string,
	container string,
) (*ShellSession, error) {
	pipes, err := openShellProcessPipes(cmd)
	if err != nil {
		cancel()
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		cancel()
		return nil, errors.Join(fmt.Errorf("shell: start: %w", err), pipes.close())
	}

	session := &ShellSession{
		identity: ShellSessionIdentity{
			ID:        fmt.Sprintf("shell-%d", shellSessionCounter.Add(1)),
			Namespace: namespace,
			Pod:       pod,
			Container: container,
		},
		cmd:       cmd,
		stdin:     pipes.stdin,
		input:     make(chan string, shellInputCapacity),
		inputDone: make(chan struct{}),
		output:    make(chan ShellOutput, shellOutputCapacity),
		exit:      make(chan ShellExitError, shellExitCapacity),
		ctx:       ctx,
		cancel:    cancel,
	}
	session.startWorkers(pipes.stdout, pipes.stderr)
	return session, nil
}

func validateShellTarget(namespace, pod string) error {
	if pod == "" {
		return ErrShellPodRequired
	}
	if namespace == "" {
		return ErrShellNamespaceRequired
	}
	return nil
}

func shellCommandArgs(namespace, pod, container string) []string {
	args := []string{"exec", "-i", "-n", namespace, pod}
	if container != "" {
		args = append(args, "-c", container)
	}
	return append(args, "--", "/bin/sh")
}

func openShellProcessPipes(cmd *exec.Cmd) (shellProcessPipes, error) {
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return shellProcessPipes{}, fmt.Errorf("shell: stdin pipe: %w", err)
	}
	pipes := shellProcessPipes{stdin: stdin}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return shellProcessPipes{}, errors.Join(fmt.Errorf("shell: stdout pipe: %w", err), pipes.close())
	}
	pipes.stdout = stdout
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return shellProcessPipes{}, errors.Join(fmt.Errorf("shell: stderr pipe: %w", err), pipes.close())
	}
	pipes.stderr = stderr
	return pipes, nil
}

func (p shellProcessPipes) close() error {
	var closeErrors []error
	streams := []struct {
		name   string
		closer io.Closer
	}{
		{name: "stdin", closer: p.stdin},
		{name: "stdout", closer: p.stdout},
		{name: "stderr", closer: p.stderr},
	}
	for _, stream := range streams {
		if stream.closer == nil {
			continue
		}
		if err := stream.closer.Close(); err != nil {
			closeErrors = append(closeErrors, fmt.Errorf("shell: close %s: %w", stream.name, err))
		}
	}
	return errors.Join(closeErrors...)
}

func (s *ShellSession) startWorkers(stdout, stderr io.ReadCloser) {
	var pipesDone sync.WaitGroup
	pipesDone.Add(shellStreamCount)
	pipeErrors := make(chan error, shellStreamCount)
	go s.writeInput()
	go s.streamPipe(stdout, "stdout", false, &pipesDone, pipeErrors)
	go s.streamPipe(stderr, "stderr", true, &pipesDone, pipeErrors)
	go s.waitForExit(&pipesDone, pipeErrors)
}

// Send writes a line to the shell's stdin. The newline terminator is
// appended if missing. Returns an error if the session has been closed.
func (s *ShellSession) Send(line string) error {
	if s == nil {
		return ErrShellNilSession
	}
	s.closeMu.Lock()
	defer s.closeMu.Unlock()
	if s.closed {
		return ErrShellSessionClosed
	}
	if len(line) == 0 || line[len(line)-1] != '\n' {
		line += "\n"
	}
	select {
	case s.input <- line:
		return nil
	case <-s.ctx.Done():
		return ErrShellSessionClosed
	default:
		return ErrShellInputBackpressure
	}
}

func (s *ShellSession) Identity() ShellSessionIdentity {
	if s == nil {
		return ShellSessionIdentity{}
	}
	return s.identity
}

// Output returns the receive channel for shell stdout/stderr lines. The
// channel is closed when the process exits.
func (s *ShellSession) Output() <-chan ShellOutput {
	return s.output
}

// Exit returns the receive channel that delivers exactly one
// ShellExitError when the process exits. Use this to drive UI cleanup.
func (s *ShellSession) Exit() <-chan ShellExitError {
	return s.exit
}

// Close requests process termination. It is safe to call more than once.
func (s *ShellSession) Close() {
	if s == nil {
		return
	}
	s.closeMu.Lock()
	if s.closed {
		s.closeMu.Unlock()
		return
	}
	s.closed = true
	s.closeMu.Unlock()

	s.cancel()
}

// Interrupt sends SIGINT to the shell process so the currently-running
// command (not the shell itself) is interrupted, mimicking Ctrl+C.
func (s *ShellSession) Interrupt() error {
	if s == nil || s.cmd == nil || s.cmd.Process == nil {
		return ErrShellNoProcess
	}
	return interruptShellProcess(s.cmd.Process)
}

func (s *ShellSession) writeInput() {
	defer close(s.inputDone)
	defer func() {
		if err := s.stdin.Close(); err != nil &&
			!errors.Is(err, io.ErrClosedPipe) &&
			!errors.Is(err, os.ErrClosed) {
			s.setAsyncError(fmt.Errorf("shell: close stdin: %w", err))
		}
	}()
	for {
		select {
		case <-s.ctx.Done():
			return
		case line := <-s.input:
			if _, err := io.WriteString(s.stdin, line); err != nil {
				s.setAsyncError(fmt.Errorf("shell: write stdin: %w", err))
				s.cancel()
				return
			}
		}
	}
}

func (s *ShellSession) streamPipe(
	reader io.Reader,
	stream string,
	stderr bool,
	done *sync.WaitGroup,
	errorsOut chan<- error,
) {
	defer done.Done()
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, shellScanBufferStart), shellScanBufferMax)
	for scanner.Scan() {
		if !s.emitOutput(ShellOutput{
			SessionID: s.identity.ID,
			Line:      stripANSI(scanner.Text()),
			Stderr:    stderr,
		}) {
			break
		}
	}
	streamErr := scanner.Err()
	if streamErr != nil {
		streamErr = &ShellStreamError{Stream: stream, Err: streamErr}
		s.cancel()
	}
	errorsOut <- streamErr
}

func (s *ShellSession) emitOutput(output ShellOutput) bool {
	select {
	case <-s.ctx.Done():
		return false
	case s.output <- output:
		return true
	default:
		s.droppedOutput.Add(1)
		return true
	}
}

func (s *ShellSession) waitForExit(pipes *sync.WaitGroup, pipeErrors <-chan error) {
	pipes.Wait()
	errorsFound := make([]error, 0, initialShellErrorCapacity)
	for range shellStreamCount {
		if err := <-pipeErrors; err != nil {
			errorsFound = append(errorsFound, err)
		}
	}
	if err := s.cmd.Wait(); err != nil && s.ctx.Err() == nil {
		errorsFound = append(errorsFound, err)
	}
	s.cancel()
	<-s.inputDone
	if err := s.getAsyncError(); err != nil {
		errorsFound = append(errorsFound, err)
	}
	if dropped := s.droppedOutput.Load(); dropped > 0 {
		errorsFound = append(errorsFound, &ShellOutputDroppedError{Count: dropped})
	}
	s.closeMu.Lock()
	s.closed = true
	s.closeMu.Unlock()
	close(s.output)
	s.exit <- ShellExitError{SessionID: s.identity.ID, Err: errors.Join(errorsFound...)}
	close(s.exit)
}

func (s *ShellSession) setAsyncError(err error) {
	s.asyncErrMu.Lock()
	defer s.asyncErrMu.Unlock()
	if s.asyncErr == nil {
		s.asyncErr = err
	}
}

func (s *ShellSession) getAsyncError() error {
	s.asyncErrMu.Lock()
	defer s.asyncErrMu.Unlock()
	return s.asyncErr
}
