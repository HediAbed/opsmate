package kube

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/remotecommand"
	"k8s.io/streaming/pkg/httpstream"
)

const (
	shellCommand           = "/bin/sh"
	shellInputCapacity     = 64
	shellOutputCapacity    = 256
	shellInitialScanBuffer = 64 * 1024
	shellMaximumLineBytes  = 4 * 1024 * 1024
	shellStreamCount       = 2
	shellFailureCapacity   = shellStreamCount + 3
)

type shellExecutorFactory func(*rest.Config, *url.URL) (remotecommand.Executor, error)

type shellExecutorConstructors struct {
	spdy      func(*rest.Config, string, *url.URL) (remotecommand.Executor, error)
	webSocket func(*rest.Config, string, string) (remotecommand.Executor, error)
	fallback  func(remotecommand.Executor, remotecommand.Executor, func(error) bool) (remotecommand.Executor, error)
}

var defaultShellExecutorConstructors = shellExecutorConstructors{
	spdy:      remotecommand.NewSPDYExecutor,
	webSocket: remotecommand.NewWebSocketExecutor,
	fallback:  remotecommand.NewFallbackExecutor,
}

type shellSession struct {
	identity      ShellIdentity
	stdin         io.WriteCloser
	input         chan string
	inputDone     chan struct{}
	output        chan ShellOutput
	exit          chan ShellExit
	ctx           context.Context
	cancel        context.CancelFunc
	closeOnce     sync.Once
	stateMu       sync.RWMutex
	closed        bool
	droppedOutput atomic.Uint64
	asyncErrorMu  sync.Mutex
	asyncError    error
}

type ShellOutputDroppedError struct {
	Count uint64
}

func (e *ShellOutputDroppedError) Error() string {
	return fmt.Sprintf("kubernetes shell dropped %d output lines because the consumer was too slow", e.Count)
}

func (m *Manager) StartShell(parent context.Context, request ShellRequest) (ShellSession, error) {
	if err := validatePodReference(request.Pod); err != nil {
		return nil, newResourceError(OperationStart, SubjectPodShell, request.Pod.Identifier(), err)
	}
	if parent == nil {
		return nil, newResourceError(OperationStart, SubjectPodShell, request.Pod.Identifier(), ErrContextRequired)
	}
	if err := parent.Err(); err != nil {
		return nil, newResourceError(OperationStart, SubjectPodShell, request.Pod.Identifier(), err)
	}
	clients, ctx, cancel, err := m.clientSession(parent)
	if err != nil {
		return nil, newResourceError(OperationStart, SubjectPodShell, request.Pod.Identifier(), err)
	}
	streamURL, err := podExecURL(clients, request)
	if err != nil {
		cancel()
		return nil, newResourceError(OperationBuild, SubjectPodShell, request.Pod.Identifier(), err)
	}
	if m.newShellStream == nil {
		cancel()
		return nil, newResourceError(OperationBuild, SubjectPodShell, request.Pod.Identifier(), ErrPodExecutorUnavailable)
	}
	executor, err := m.newShellStream(clients.RESTConfig(), streamURL)
	if err != nil {
		cancel()
		return nil, newResourceError(OperationBuild, SubjectPodShell, request.Pod.Identifier(), err)
	}
	if executor == nil {
		cancel()
		return nil, newResourceError(OperationBuild, SubjectPodShell, request.Pod.Identifier(), ErrPodExecutorUnavailable)
	}
	identity := ShellIdentity{
		ID:        fmt.Sprintf("shell-%d", m.shellSequence.Add(1)),
		Pod:       request.Pod,
		Container: request.Container,
	}
	return startShellSession(ctx, cancel, identity, executor), nil
}

func podExecURL(clients *Clients, request ShellRequest) (*url.URL, error) {
	if clients == nil || clients.Kubernetes() == nil {
		return nil, ErrTypedClientUnavailable
	}
	restClient := coreRESTClient(clients)
	if restClient == nil {
		return nil, ErrPodExecutorUnavailable
	}
	options := &corev1.PodExecOptions{
		Container: request.Container,
		Command:   []string{shellCommand},
		Stdin:     true,
		Stdout:    true,
		Stderr:    true,
	}
	return restClient.Post().
		Resource("pods").
		Namespace(request.Pod.Namespace).
		Name(request.Pod.Name).
		SubResource("exec").
		VersionedParams(options, scheme.ParameterCodec).
		URL(), nil
}

func defaultShellExecutor(config *rest.Config, streamURL *url.URL) (remotecommand.Executor, error) {
	return buildShellExecutor(config, streamURL, defaultShellExecutorConstructors)
}

func buildShellExecutor(config *rest.Config, streamURL *url.URL, constructors shellExecutorConstructors) (remotecommand.Executor, error) {
	if config == nil || streamURL == nil {
		return nil, ErrPodExecutorUnavailable
	}
	spdyExecutor, err := constructors.spdy(config, "POST", streamURL)
	if err != nil {
		return nil, err
	}
	webSocketExecutor, err := constructors.webSocket(config, "GET", streamURL.String())
	if err != nil {
		return nil, err
	}
	return constructors.fallback(webSocketExecutor, spdyExecutor, shouldFallbackStream)
}

func shouldFallbackStream(err error) bool {
	return httpstream.IsUpgradeFailure(err) || httpstream.IsHTTPSProxyError(err)
}

func startShellSession(
	ctx context.Context,
	cancel context.CancelFunc,
	identity ShellIdentity,
	executor remotecommand.Executor,
) ShellSession {
	stdinReader, stdinWriter := io.Pipe()
	stdoutReader, stdoutWriter := io.Pipe()
	stderrReader, stderrWriter := io.Pipe()
	session := &shellSession{
		identity:  identity,
		stdin:     stdinWriter,
		input:     make(chan string, shellInputCapacity),
		inputDone: make(chan struct{}),
		output:    make(chan ShellOutput, shellOutputCapacity),
		exit:      make(chan ShellExit, 1),
		ctx:       ctx,
		cancel:    cancel,
	}
	go session.writeInput()
	go session.run(executor, stdinReader, stdoutReader, stdoutWriter, stderrReader, stderrWriter)
	return session
}

func (s *shellSession) Identity() ShellIdentity {
	if s == nil {
		return ShellIdentity{}
	}
	return s.identity
}

func (s *shellSession) Send(line string) error {
	if s == nil {
		return newResourceError(OperationSend, SubjectPodShell, "", ErrShellSessionClosed)
	}
	if !strings.HasSuffix(line, "\n") {
		line += "\n"
	}
	s.stateMu.RLock()
	defer s.stateMu.RUnlock()
	if s.closed {
		return s.sessionError(OperationSend, ErrShellSessionClosed)
	}
	if s.ctx.Err() != nil {
		return s.sessionError(OperationSend, ErrShellSessionClosed)
	}
	select {
	case s.input <- line:
		return nil
	default:
		return s.sessionError(OperationSend, ErrShellInputBackpressure)
	}
}

func (s *shellSession) Output() <-chan ShellOutput {
	if s == nil {
		return nil
	}
	return s.output
}

func (s *shellSession) Exit() <-chan ShellExit {
	if s == nil {
		return nil
	}
	return s.exit
}

func (s *shellSession) Interrupt() error {
	if s == nil {
		return newResourceError(OperationStop, SubjectPodShell, "", ErrShellSessionClosed)
	}
	s.stateMu.RLock()
	closed := s.closed
	s.stateMu.RUnlock()
	if closed || s.ctx.Err() != nil {
		return s.sessionError(OperationStop, ErrShellSessionClosed)
	}
	s.Close()
	return nil
}

func (s *shellSession) Close() {
	if s == nil {
		return
	}
	s.closeOnce.Do(func() {
		s.stateMu.Lock()
		s.closed = true
		s.stateMu.Unlock()
		s.cancel()
	})
}

func (s *shellSession) writeInput() {
	defer close(s.inputDone)
	defer s.closeInput()
	for {
		select {
		case <-s.ctx.Done():
			return
		case line := <-s.input:
			if _, err := io.WriteString(s.stdin, line); err != nil {
				s.recordInputFailure("write input", err)
				s.Close()
				return
			}
		}
	}
}

func (s *shellSession) closeInput() {
	if err := s.stdin.Close(); err != nil {
		s.recordInputFailure("close input", err)
	}
}

func (s *shellSession) recordInputFailure(operation string, err error) {
	if errors.Is(err, io.ErrClosedPipe) {
		return
	}
	s.setAsyncError(s.sessionError(OperationSend, fmt.Errorf("%s: %w", operation, err)))
}

func (s *shellSession) run(
	executor remotecommand.Executor,
	stdinReader *io.PipeReader,
	stdoutReader *io.PipeReader,
	stdoutWriter *io.PipeWriter,
	stderrReader *io.PipeReader,
	stderrWriter *io.PipeWriter,
) {
	streamErrors := make(chan error, shellStreamCount)
	var streams sync.WaitGroup
	streams.Add(shellStreamCount)
	go s.readOutput(stdoutReader, false, &streams, streamErrors)
	go s.readOutput(stderrReader, true, &streams, streamErrors)

	streamErr := executor.StreamWithContext(s.ctx, remotecommand.StreamOptions{
		Stdin:  stdinReader,
		Stdout: stdoutWriter,
		Stderr: stderrWriter,
	})
	wasCanceled := s.ctx.Err() != nil
	_ = stdoutWriter.Close()
	_ = stderrWriter.Close()
	streams.Wait()
	s.Close()
	_ = stdinReader.Close()
	<-s.inputDone

	failures := make([]error, 0, shellFailureCapacity)
	if streamErr != nil && !wasCanceled {
		failures = append(failures, streamErr)
	}
	for range shellStreamCount {
		if err := <-streamErrors; err != nil {
			failures = append(failures, err)
		}
	}
	if err := s.getAsyncError(); err != nil {
		failures = append(failures, err)
	}
	if dropped := s.droppedOutput.Load(); dropped > 0 {
		failures = append(failures, &ShellOutputDroppedError{Count: dropped})
	}
	close(s.output)
	exitErr := errors.Join(failures...)
	if exitErr != nil {
		exitErr = s.sessionError(OperationStream, exitErr)
	}
	s.exit <- ShellExit{SessionID: s.identity.ID, Err: exitErr}
	close(s.exit)
}

func (s *shellSession) readOutput(reader io.ReadCloser, stderr bool, done *sync.WaitGroup, errorsOut chan<- error) {
	defer done.Done()
	errorsOut <- s.scanOutput(reader, stderr)
}

func (s *shellSession) scanOutput(reader io.ReadCloser, stderr bool) (returnErr error) {
	defer func() {
		if closeErr := reader.Close(); closeErr != nil {
			returnErr = errors.Join(returnErr, fmt.Errorf("close output: %w", closeErr))
		}
	}()
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, shellInitialScanBuffer), shellMaximumLineBytes)
	for scanner.Scan() {
		if !s.emitOutput(ShellOutput{SessionID: s.identity.ID, Line: scanner.Text(), Stderr: stderr}) {
			break
		}
	}
	if err := scanner.Err(); err != nil {
		return errors.Join(ErrShellOutputLineTooLong, err)
	}
	return nil
}

func (s *shellSession) emitOutput(output ShellOutput) bool {
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

func (s *shellSession) sessionError(operation Operation, err error) *Error {
	return newResourceError(operation, SubjectPodShell, s.identity.Pod.Identifier(), err)
}

func (s *shellSession) setAsyncError(err error) {
	s.asyncErrorMu.Lock()
	defer s.asyncErrorMu.Unlock()
	if s.asyncError == nil {
		s.asyncError = err
	}
}

func (s *shellSession) getAsyncError() error {
	s.asyncErrorMu.Lock()
	defer s.asyncErrorMu.Unlock()
	return s.asyncError
}
