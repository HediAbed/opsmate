package cluster

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	tea "charm.land/bubbletea/v2"

	model "github.com/HediAbed/opsmate/internal/cluster"
	"github.com/HediAbed/opsmate/internal/kube"
	"github.com/HediAbed/opsmate/internal/ui/component"
)

const (
	clusterLogReadTimeout = time.Minute
	initialLogBufferBytes = 64 * 1024
	maximumLogLineBytes   = 1024 * 1024
)

type Operations interface {
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

type ResourceOperations struct {
	parent    context.Context
	inspector kube.ResourceInspector
	pods      kube.PodReader
	writer    kube.ResourceWriter
	sheller   kube.PodSheller
	forwards  kube.PortForwardManager
}

type SessionOperations interface {
	kube.PodSheller
	kube.PortForwardManager
}

func NewOperations(
	parent context.Context,
	inspector kube.ResourceInspector,
	pods kube.PodReader,
	writer kube.ResourceWriter,
	sessions ...SessionOperations,
) ResourceOperations {
	var sessionOperations SessionOperations
	if len(sessions) > 0 {
		sessionOperations = sessions[0]
	}
	return ResourceOperations{
		parent:    parent,
		inspector: inspector,
		pods:      pods,
		writer:    writer,
		sheller:   sessionOperations,
		forwards:  sessionOperations,
	}
}

type PortForwardStartedMsg struct {
	Session kube.PortForward
	Err     error
}

type PortForwardStoppedMsg struct {
	SessionID string
	Err       error
}

func (c ResourceOperations) InspectResource(reference kube.ResourceReference) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(c.parent, ReadTimeout)
		defer cancel()
		content, err := c.inspector.ResourceYAML(ctx, reference)
		return model.DescribeMsg{Output: content, Err: err}
	}
}

func (c ResourceOperations) ResourceYAML(reference kube.ResourceReference) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(c.parent, ReadTimeout)
		defer cancel()
		content, err := c.inspector.ResourceYAML(ctx, reference)
		return model.YAMLMsg{Output: content, Err: err}
	}
}

func (c ResourceOperations) FetchPodLogs(request kube.PodLogRequest) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(c.parent, clusterLogReadTimeout)
		defer cancel()
		stream, err := c.pods.OpenPodLogs(ctx, request)
		if err != nil {
			return model.LogsMsg{Err: err}
		}
		if stream == nil {
			return model.LogsMsg{Err: kube.ErrPodLogStreamUnavailable}
		}
		lines, readErr := scanPodLogLines(stream)
		closeErr := stream.Close()
		return model.LogsMsg{Lines: lines, Err: errors.Join(readErr, closeErr)}
	}
}

func (c ResourceOperations) FetchPodContainers(reference kube.PodReference) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(c.parent, ReadTimeout)
		defer cancel()
		containers, err := c.pods.PodContainers(ctx, reference)
		return model.ContainersMsg{Containers: containers, Err: err}
	}
}

func (c ResourceOperations) ScaleWorkload(request kube.ScaleRequest) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(c.parent, ActionTimeout)
		defer cancel()
		err := c.writer.Scale(ctx, request)
		return model.MutationResultMsg{
			Output: successfulMutation("Scaled", request.Workload.Identifier()),
			Err:    err,
		}
	}
}

func (c ResourceOperations) DeleteResource(reference kube.ResourceReference) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(c.parent, ActionTimeout)
		defer cancel()
		err := c.writer.Delete(ctx, reference)
		return model.MutationResultMsg{
			Output: successfulMutation("Deleted", reference.Identifier()),
			Err:    err,
		}
	}
}

func (c ResourceOperations) DeleteResources(batch kube.ResourceBatch) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(c.parent, ActionTimeout)
		defer cancel()
		outcome, err := c.writer.DeleteBatch(ctx, batch)
		return model.MutationResultMsg{Output: batchMutationSummary("Deleted", "resource", outcome), Err: err}
	}
}

func (c ResourceOperations) RestartWorkload(reference kube.WorkloadReference) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(c.parent, ActionTimeout)
		defer cancel()
		err := c.writer.Restart(ctx, reference)
		return model.MutationResultMsg{
			Output: successfulMutation("Restarted", reference.Identifier()),
			Err:    err,
		}
	}
}

func (c ResourceOperations) RestartWorkloads(batch kube.WorkloadBatch) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(c.parent, ActionTimeout)
		defer cancel()
		outcome, err := c.writer.RestartBatch(ctx, batch)
		return model.MutationResultMsg{Output: batchMutationSummary("Restarted", "workload", outcome), Err: err}
	}
}

func (c ResourceOperations) StartShell(request kube.ShellRequest) (kube.ShellSession, error) {
	return c.sheller.StartShell(c.parent, request)
}

func (c ResourceOperations) StartPortForward(request kube.PortForwardRequest) tea.Cmd {
	return func() tea.Msg {
		session, err := c.forwards.StartPortForward(c.parent, request)
		return PortForwardStartedMsg{Session: session, Err: err}
	}
}

func (c ResourceOperations) StopPortForward(sessionID string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(c.parent, ActionTimeout)
		defer cancel()
		err := c.forwards.StopPortForward(ctx, sessionID)
		return PortForwardStoppedMsg{SessionID: sessionID, Err: err}
	}
}

func (c ResourceOperations) PortForwards() []kube.PortForwardSession {
	return c.forwards.PortForwards()
}

func (ResourceOperations) WaitForPortForwardExit(session kube.PortForward) tea.Cmd {
	if session == nil || session.Exit() == nil {
		return nil
	}
	return func() tea.Msg {
		exit, open := <-session.Exit()
		if !open {
			return PortForwardStoppedMsg{SessionID: session.Session().ID}
		}
		return PortForwardStoppedMsg{SessionID: exit.SessionID, Err: exit.Err}
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
	noun := component.NounForCount(subject, subject+"s", count)
	return fmt.Sprintf("%s %d %s", action, count, noun)
}

var _ Operations = ResourceOperations{}
