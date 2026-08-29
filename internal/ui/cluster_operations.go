package ui

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/HediAbed/opsmate/internal/cluster"
	"github.com/HediAbed/opsmate/internal/kube"
)

const (
	clusterLogReadTimeout = time.Minute
	initialLogBufferBytes = 64 * 1024
	maximumLogLineBytes   = 1024 * 1024
)

type clusterOperations interface {
	InspectResource(kube.ResourceReference) tea.Cmd
	ResourceYAML(kube.ResourceReference) tea.Cmd
	FetchPodLogs(kube.PodLogRequest) tea.Cmd
	FetchPodContainers(kube.PodReference) tea.Cmd
	ScaleWorkload(kube.ScaleRequest) tea.Cmd
	DeleteResource(kube.ResourceReference) tea.Cmd
	DeleteResources(kube.ResourceBatch) tea.Cmd
	RestartWorkload(kube.WorkloadReference) tea.Cmd
	RestartWorkloads(kube.WorkloadBatch) tea.Cmd
	StartShell(kube.ShellRequest) (kube.ShellSession, error)
	StartPortForward(kube.PortForwardRequest) tea.Cmd
	StopPortForward(string) tea.Cmd
	PortForwards() []kube.PortForwardSession
	WaitForPortForwardExit(kube.PortForward) tea.Cmd
}

type nativeClusterOperations struct {
	parent    context.Context
	inspector kube.ResourceInspector
	pods      kube.PodReader
	writer    kube.ResourceWriter
	sheller   kube.PodSheller
	forwards  kube.PortForwardManager
}

type clusterSessionOperations interface {
	kube.PodSheller
	kube.PortForwardManager
}

func newNativeClusterOperations(
	parent context.Context,
	inspector kube.ResourceInspector,
	pods kube.PodReader,
	writer kube.ResourceWriter,
	sessions ...clusterSessionOperations,
) nativeClusterOperations {
	var sessionOperations clusterSessionOperations
	if len(sessions) > 0 {
		sessionOperations = sessions[0]
	}
	return nativeClusterOperations{
		parent:    parent,
		inspector: inspector,
		pods:      pods,
		writer:    writer,
		sheller:   sessionOperations,
		forwards:  sessionOperations,
	}
}

type portForwardStartedMsg struct {
	session kube.PortForward
	err     error
}

type portForwardStoppedMsg struct {
	sessionID string
	err       error
}

func (c nativeClusterOperations) InspectResource(reference kube.ResourceReference) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(c.parent, clusterReadTimeout)
		defer cancel()
		content, err := c.inspector.ResourceYAML(ctx, reference)
		return cluster.DescribeMsg{Output: content, Err: err}
	}
}

func (c nativeClusterOperations) ResourceYAML(reference kube.ResourceReference) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(c.parent, clusterReadTimeout)
		defer cancel()
		content, err := c.inspector.ResourceYAML(ctx, reference)
		return cluster.YAMLMsg{Output: content, Err: err}
	}
}

func (c nativeClusterOperations) FetchPodLogs(request kube.PodLogRequest) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(c.parent, clusterLogReadTimeout)
		defer cancel()
		stream, err := c.pods.OpenPodLogs(ctx, request)
		if err != nil {
			return cluster.LogsMsg{Err: err}
		}
		if stream == nil {
			return cluster.LogsMsg{Err: kube.ErrPodLogStreamUnavailable}
		}
		lines, readErr := scanPodLogLines(stream)
		closeErr := stream.Close()
		return cluster.LogsMsg{Lines: lines, Err: errors.Join(readErr, closeErr)}
	}
}

func (c nativeClusterOperations) FetchPodContainers(reference kube.PodReference) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(c.parent, clusterReadTimeout)
		defer cancel()
		containers, err := c.pods.PodContainers(ctx, reference)
		return cluster.ContainersMsg{Containers: containers, Err: err}
	}
}

func (c nativeClusterOperations) ScaleWorkload(request kube.ScaleRequest) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(c.parent, clusterActionTimeout)
		defer cancel()
		err := c.writer.Scale(ctx, request)
		return cluster.MutationResultMsg{
			Output: successfulMutation("Scaled", request.Workload.Identifier()),
			Err:    err,
		}
	}
}

func (c nativeClusterOperations) DeleteResource(reference kube.ResourceReference) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(c.parent, clusterActionTimeout)
		defer cancel()
		err := c.writer.Delete(ctx, reference)
		return cluster.MutationResultMsg{
			Output: successfulMutation("Deleted", reference.Identifier()),
			Err:    err,
		}
	}
}

func (c nativeClusterOperations) DeleteResources(batch kube.ResourceBatch) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(c.parent, clusterActionTimeout)
		defer cancel()
		outcome, err := c.writer.DeleteBatch(ctx, batch)
		return cluster.MutationResultMsg{Output: batchMutationSummary("Deleted", "resource", outcome), Err: err}
	}
}

func (c nativeClusterOperations) RestartWorkload(reference kube.WorkloadReference) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(c.parent, clusterActionTimeout)
		defer cancel()
		err := c.writer.Restart(ctx, reference)
		return cluster.MutationResultMsg{
			Output: successfulMutation("Restarted", reference.Identifier()),
			Err:    err,
		}
	}
}

func (c nativeClusterOperations) RestartWorkloads(batch kube.WorkloadBatch) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(c.parent, clusterActionTimeout)
		defer cancel()
		outcome, err := c.writer.RestartBatch(ctx, batch)
		return cluster.MutationResultMsg{Output: batchMutationSummary("Restarted", "workload", outcome), Err: err}
	}
}

func (c nativeClusterOperations) StartShell(request kube.ShellRequest) (kube.ShellSession, error) {
	return c.sheller.StartShell(c.parent, request)
}

func (c nativeClusterOperations) StartPortForward(request kube.PortForwardRequest) tea.Cmd {
	return func() tea.Msg {
		session, err := c.forwards.StartPortForward(c.parent, request)
		return portForwardStartedMsg{session: session, err: err}
	}
}

func (c nativeClusterOperations) StopPortForward(sessionID string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(c.parent, clusterActionTimeout)
		defer cancel()
		err := c.forwards.StopPortForward(ctx, sessionID)
		return portForwardStoppedMsg{sessionID: sessionID, err: err}
	}
}

func (c nativeClusterOperations) PortForwards() []kube.PortForwardSession {
	return c.forwards.PortForwards()
}

func (nativeClusterOperations) WaitForPortForwardExit(session kube.PortForward) tea.Cmd {
	if session == nil || session.Exit() == nil {
		return nil
	}
	return func() tea.Msg {
		exit, open := <-session.Exit()
		if !open {
			return portForwardStoppedMsg{sessionID: session.Session().ID}
		}
		return portForwardStoppedMsg{sessionID: exit.SessionID, err: exit.Err}
	}
}

func scanPodLogLines(reader io.Reader) ([]string, error) {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, initialLogBufferBytes), maximumLogLineBytes)
	lines := make([]string, 0)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	return lines, scanner.Err()
}

func successfulMutation(action, identifier string) string {
	return action + " " + identifier
}

func batchMutationSummary(action, subject string, outcome kube.BatchOutcome) string {
	count := len(outcome.Succeeded)
	if count == 1 {
		return fmt.Sprintf("%s 1 %s", action, subject)
	}
	return fmt.Sprintf("%s %d %ss", action, count, subject)
}

var _ clusterOperations = nativeClusterOperations{}
