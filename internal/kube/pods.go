package kube

import (
	"context"
	"io"
	"sync"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type sessionReadCloser struct {
	stream   io.ReadCloser
	cancel   context.CancelFunc
	close    sync.Once
	closeErr error
}

func (c *sessionReadCloser) Read(buffer []byte) (int, error) {
	return c.stream.Read(buffer)
}

func (m *Manager) PodContainers(parent context.Context, reference PodReference) ([]string, error) {
	identifier := reference.Identifier()
	if parent == nil {
		return nil, newResourceError(OperationGet, SubjectPod, identifier, ErrContextRequired)
	}
	if err := validatePodReference(reference); err != nil {
		return nil, newResourceError(OperationGet, SubjectPod, identifier, err)
	}
	clients, ctx, cancel, err := m.clientSession(parent)
	if err != nil {
		return nil, newResourceError(OperationGet, SubjectPod, identifier, err)
	}
	defer cancel()
	typedClient := clients.Kubernetes()
	if typedClient == nil {
		return nil, newResourceError(OperationGet, SubjectPod, identifier, ErrTypedClientUnavailable)
	}
	pod, err := typedClient.CoreV1().Pods(reference.Namespace).Get(ctx, reference.Name, metav1.GetOptions{})
	if err != nil {
		return nil, newResourceError(OperationGet, SubjectPod, identifier, err)
	}
	return podContainerNames(pod), nil
}

func podContainerNames(pod *corev1.Pod) []string {
	count := len(pod.Spec.InitContainers) + len(pod.Spec.Containers) + len(pod.Spec.EphemeralContainers)
	names := make([]string, 0, count)
	for _, container := range pod.Spec.InitContainers {
		names = append(names, container.Name)
	}
	for _, container := range pod.Spec.Containers {
		names = append(names, container.Name)
	}
	for _, container := range pod.Spec.EphemeralContainers {
		names = append(names, container.Name)
	}
	return names
}

func (m *Manager) OpenPodLogs(parent context.Context, request PodLogRequest) (io.ReadCloser, error) {
	identifier := request.Pod.Identifier()
	if parent == nil {
		return nil, newResourceError(OperationStream, SubjectPodLogs, identifier, ErrContextRequired)
	}
	if err := validatePodReference(request.Pod); err != nil {
		return nil, newResourceError(OperationStream, SubjectPodLogs, identifier, err)
	}
	if request.TailLines <= 0 {
		return nil, newResourceError(OperationStream, SubjectPodLogs, identifier, ErrLogTailLinesInvalid)
	}
	clients, ctx, cancel, err := m.clientSession(parent)
	if err != nil {
		return nil, newResourceError(OperationStream, SubjectPodLogs, identifier, err)
	}
	options := &corev1.PodLogOptions{Container: request.Container, TailLines: &request.TailLines}
	stream, err := clients.OpenPodLogs(ctx, request.Pod.Namespace, request.Pod.Name, options)
	if err != nil {
		cancel()
		return nil, newResourceError(OperationStream, SubjectPodLogs, identifier, err)
	}
	if stream == nil {
		cancel()
		return nil, newResourceError(OperationStream, SubjectPodLogs, identifier, ErrPodLogStreamUnavailable)
	}
	return &sessionReadCloser{stream: stream, cancel: cancel}, nil
}

func (c *sessionReadCloser) Close() error {
	if c == nil {
		return nil
	}
	c.close.Do(func() {
		c.cancel()
		c.closeErr = c.stream.Close()
	})
	return c.closeErr
}

var _ PodReader = (*Manager)(nil)
