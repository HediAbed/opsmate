package service

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"sync"
	"time"

	tea "charm.land/bubbletea/v2"
)

const kubectlBinary = "kubectl"

// WatchEventKind identifies a Kubernetes watch frame or local lifecycle event.
type WatchEventKind string

const (
	WatchAdded              WatchEventKind = "ADDED"
	WatchModified           WatchEventKind = "MODIFIED"
	WatchDeleted            WatchEventKind = "DELETED"
	WatchBookmark           WatchEventKind = "BOOKMARK"
	WatchClosed             WatchEventKind = "CLOSED"
	WatchErrored            WatchEventKind = "ERROR"
	maxWatchEventBytes                     = 16 * 1024 * 1024
	initialWatchBufferBytes                = 64 * 1024
	watchProcessWaitDelay                  = 250 * time.Millisecond
)

// WatchStreamError identifies the resource and stage of a watch failure.
type WatchStreamError struct {
	Resource string
	Stage    string
	Err      error
}

func (e *WatchStreamError) Error() string {
	prefix := "watch"
	if e.Resource != "" {
		prefix = "watch " + e.Resource
	}
	if e.Stage != "" {
		prefix = prefix + " (" + e.Stage + ")"
	}
	if e.Err != nil {
		return prefix + ": " + e.Err.Error()
	}
	return prefix + ": unknown error"
}

func (e *WatchStreamError) Unwrap() error { return e.Err }

type WatchResource interface {
	Pod | Deployment | Event | Ingress | NetworkPolicy | PersistentVolumeClaim |
		CronJob | HPA | Secret | ReplicaSet
}

// WatchEvent carries a resource update or stream lifecycle event.
type WatchEvent[T WatchResource] struct {
	Kind WatchEventKind
	Item T
	Err  error
}

// Watcher exposes a cancellable stream of resource events.
type Watcher[T WatchResource] interface {
	Events() <-chan WatchEvent[T]
	Cancel()
}

// WatchClosedMsg indicates that a watcher channel closed.
type WatchClosedMsg struct{}

// WatchEventMsg carries one watch event through the UI event loop.
type WatchEventMsg[T WatchResource] struct {
	Event WatchEvent[T]
}

// NextWatchEvent reads one event from watcher.
func NextWatchEvent[T WatchResource](w Watcher[T]) tea.Cmd {
	return func() tea.Msg {
		ev, ok := <-w.Events()
		if !ok {
			return WatchClosedMsg{}
		}
		return WatchEventMsg[T]{Event: ev}
	}
}

type watchEventEnvelope struct {
	Type   string          `json:"type"`
	Object json.RawMessage `json:"object"`
}

type decodeFunc[T WatchResource] func(raw json.RawMessage) (T, error)

type watcher[T WatchResource] struct {
	events     chan WatchEvent[T]
	stop       chan struct{}
	done       chan struct{}
	cancelOnce sync.Once
}

func (w *watcher[T]) Events() <-chan WatchEvent[T] { return w.events }

func (w *watcher[T]) Cancel() {
	w.cancelOnce.Do(func() {
		close(w.stop)
	})
	<-w.done
}

// WatchPods streams pod changes. An empty namespace selects all namespaces.
func WatchPods(ctx context.Context, namespace string) Watcher[Pod] {
	return startWatch(ctx, "pods", namespace, decodePodObject)
}

// WatchDeployments streams deployment changes.
func WatchDeployments(ctx context.Context, namespace string) Watcher[Deployment] {
	return startWatch(ctx, "deployments", namespace, decodeDeploymentObject)
}

// WatchEvents streams Kubernetes events.
func WatchEvents(ctx context.Context, namespace string) Watcher[Event] {
	return startWatch(ctx, "events", namespace, decodeEventObject)
}

func WatchIngresses(ctx context.Context, namespace string) Watcher[Ingress] {
	return startWatch(ctx, "ingresses", namespace, decodeIngressObject)
}

func WatchNetworkPolicies(ctx context.Context, namespace string) Watcher[NetworkPolicy] {
	return startWatch(ctx, "networkpolicies", namespace, decodeNetworkPolicyObject)
}

func WatchPVCs(ctx context.Context, namespace string) Watcher[PersistentVolumeClaim] {
	return startWatch(ctx, "pvc", namespace, decodePVCObject)
}

func WatchCronJobs(ctx context.Context, namespace string) Watcher[CronJob] {
	return startWatch(ctx, "cronjobs", namespace, decodeCronJobObject)
}

func WatchHPAs(ctx context.Context, namespace string) Watcher[HPA] {
	return startWatch(ctx, "hpa", namespace, decodeHPAObject)
}

func WatchSecrets(ctx context.Context, namespace string) Watcher[Secret] {
	return startWatch(ctx, "secrets", namespace, decodeSecretObject)
}

func WatchReplicaSets(ctx context.Context, namespace string) Watcher[ReplicaSet] {
	return startWatch(ctx, "replicasets", namespace, decodeReplicaSetObject)
}

func startWatch[T WatchResource](ctx context.Context, resource, namespace string, decode decodeFunc[T]) Watcher[T] {
	w := &watcher[T]{
		events: make(chan WatchEvent[T], 1),
		stop:   make(chan struct{}),
		done:   make(chan struct{}),
	}
	go w.start(ctx, resource, namespace, decode)
	return w
}

func (w *watcher[T]) start(
	parent context.Context,
	resource string,
	namespace string,
	decode decodeFunc[T],
) {
	ctx, cancel := context.WithCancel(parent)
	defer cancel()
	defer close(w.events)
	defer close(w.done)
	go w.cancelWhenStopped(ctx, cancel)

	args := buildWatchArgs(resource, namespace)
	cmd := newExternalCommandContext(ctx, kubectlBinary, args...)
	cmd.WaitDelay = watchProcessWaitDelay
	w.runCommand(ctx, cmd, resource, decode)
}

func (w *watcher[T]) runCommand(
	ctx context.Context,
	cmd *exec.Cmd,
	resource string,
	decode decodeFunc[T],
) {
	stderr := newLimitedBuffer(maxKubectlErrorOutput)
	cmd.Stderr = stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		w.deliverStartError(ctx, resource, "stdout pipe", err)
		return
	}
	if err := cmd.Start(); err != nil {
		w.deliverStartError(ctx, resource, "start", err)
		return
	}

	w.run(ctx, cmd, stdout, stderr, resource, decode)
}

func (w *watcher[T]) cancelWhenStopped(ctx context.Context, cancel context.CancelFunc) {
	select {
	case <-w.stop:
		cancel()
	case <-ctx.Done():
	}
}

func (w *watcher[T]) deliverStartError(ctx context.Context, resource, stage string, err error) {
	wrapped := &WatchStreamError{Resource: resource, Stage: stage, Err: err}
	if !w.send(ctx, WatchEvent[T]{Kind: WatchErrored, Err: wrapped}) {
		return
	}
	w.send(ctx, WatchEvent[T]{Kind: WatchClosed})
}

func (w *watcher[T]) run(
	ctx context.Context,
	cmd *exec.Cmd,
	stdout io.ReadCloser,
	stderr *limitedBuffer,
	resource string,
	decode decodeFunc[T],
) {
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, initialWatchBufferBytes), maxWatchEventBytes)
	for scanner.Scan() {
		if !w.dispatchLine(ctx, scanner.Bytes(), resource, decode) {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
			return
		}
	}
	readErr := scanner.Err()
	if readErr != nil && cmd.Process != nil {
		_ = cmd.Process.Kill()
	}
	waitErr := cmd.Wait()

	if ctx.Err() != nil {
		return
	}
	w.reportTermination(ctx, resource, stderr.String(), waitErr, readErr)
}

func (w *watcher[T]) reportTermination(
	ctx context.Context,
	resource string,
	stderr string,
	waitErr error,
	readErr error,
) {
	if !w.reportReadError(ctx, resource, readErr) {
		return
	}
	if !w.reportExitError(ctx, resource, stderr, waitErr, readErr) {
		return
	}
	w.send(ctx, WatchEvent[T]{Kind: WatchClosed})
}

func (w *watcher[T]) reportReadError(ctx context.Context, resource string, readErr error) bool {
	if readErr == nil {
		return true
	}
	return w.send(ctx, WatchEvent[T]{
		Kind: WatchErrored,
		Err:  &WatchStreamError{Resource: resource, Stage: "read", Err: readErr},
	})
}

func (w *watcher[T]) reportExitError(
	ctx context.Context,
	resource string,
	stderr string,
	waitErr error,
	readErr error,
) bool {
	if waitErr == nil || readErr != nil {
		return true
	}
	exitErr := waitErr
	if diagnostic := strings.TrimSpace(stderr); diagnostic != "" {
		exitErr = fmt.Errorf("%s: %w", diagnostic, waitErr)
	}
	return w.send(ctx, WatchEvent[T]{
		Kind: WatchErrored,
		Err: &WatchStreamError{
			Resource: resource,
			Stage:    "exit",
			Err:      exitErr,
		},
	})
}

func (w *watcher[T]) dispatchLine(ctx context.Context, line []byte, resource string, decode decodeFunc[T]) bool {
	if ctx.Err() != nil {
		return false
	}
	if len(line) == 0 {
		return true
	}
	event, err := parseWatchLine(line, resource, decode)
	if err != nil {
		return w.send(ctx, WatchEvent[T]{Kind: WatchErrored, Err: err})
	}
	return w.send(ctx, event)
}

func (w *watcher[T]) send(ctx context.Context, ev WatchEvent[T]) bool {
	select {
	case <-ctx.Done():
		return false
	case w.events <- ev:
		return true
	}
}

func parseWatchLine[T WatchResource](line []byte, resource string, decode decodeFunc[T]) (WatchEvent[T], error) {
	var envelope watchEventEnvelope
	if err := json.Unmarshal(line, &envelope); err != nil {
		return WatchEvent[T]{}, &WatchStreamError{Resource: resource, Stage: "decode envelope", Err: err}
	}
	kind := WatchEventKind(envelope.Type)
	switch kind {
	case WatchBookmark:
		return WatchEvent[T]{Kind: WatchBookmark}, nil
	case WatchAdded, WatchModified, WatchDeleted:
	case WatchClosed, WatchErrored:
		return WatchEvent[T]{}, &WatchStreamError{
			Resource: resource,
			Stage:    "decode envelope",
			Err:      fmt.Errorf("unexpected event type %q", envelope.Type),
		}
	default:
		return WatchEvent[T]{}, &WatchStreamError{
			Resource: resource,
			Stage:    "decode envelope",
			Err:      fmt.Errorf("unknown event type %q", envelope.Type),
		}
	}
	item, err := decode(envelope.Object)
	if err != nil {
		return WatchEvent[T]{}, &WatchStreamError{Resource: resource, Stage: "decode object", Err: err}
	}
	return WatchEvent[T]{Kind: kind, Item: item}, nil
}

func buildWatchArgs(resource, namespace string) []string {
	args := []string{"get", resource}
	args = append(args, namespaceArgs(namespace)...)
	return append(args, "-o", "json", "--watch", "--output-watch-events=true")
}

func decodePodObject(raw json.RawMessage) (Pod, error) {
	return decodeWatchObject(raw, podFromRaw)
}

type rawDeployment struct {
	Metadata struct {
		Name              string    `json:"name"`
		Namespace         string    `json:"namespace"`
		CreationTimestamp time.Time `json:"creationTimestamp"`
	} `json:"metadata"`
	Spec struct {
		Selector struct {
			MatchLabels map[string]string `json:"matchLabels"`
		} `json:"selector"`
		Template struct {
			Spec struct {
				Containers []struct {
					Name  string `json:"name"`
					Image string `json:"image"`
				} `json:"containers"`
			} `json:"spec"`
		} `json:"template"`
	} `json:"spec"`
	Status struct {
		ReadyReplicas     int `json:"readyReplicas"`
		Replicas          int `json:"replicas"`
		UpdatedReplicas   int `json:"updatedReplicas"`
		AvailableReplicas int `json:"availableReplicas"`
	} `json:"status"`
}

func decodeDeploymentObject(raw json.RawMessage) (Deployment, error) {
	var item rawDeployment
	if err := json.Unmarshal(raw, &item); err != nil {
		return Deployment{}, err
	}
	containers := make([]string, 0, len(item.Spec.Template.Spec.Containers))
	images := make([]string, 0, len(item.Spec.Template.Spec.Containers))
	for _, c := range item.Spec.Template.Spec.Containers {
		containers = append(containers, c.Name)
		images = append(images, c.Image)
	}
	return Deployment{
		Name:       item.Metadata.Name,
		Namespace:  item.Metadata.Namespace,
		Ready:      fmt.Sprintf("%d/%d", item.Status.ReadyReplicas, item.Status.Replicas),
		UpToDate:   item.Status.UpdatedReplicas,
		Available:  item.Status.AvailableReplicas,
		Age:        formatAge(time.Since(item.Metadata.CreationTimestamp)),
		Containers: containers,
		Images:     images,
		Selector:   formatLabelMap(item.Spec.Selector.MatchLabels),
	}, nil
}

type rawEvent struct {
	Metadata struct {
		Namespace string `json:"namespace"`
		Name      string `json:"name"`
		UID       string `json:"uid"`
	} `json:"metadata"`
	Type           string    `json:"type"`
	Reason         string    `json:"reason"`
	Message        string    `json:"message"`
	Count          int       `json:"count"`
	LastTimestamp  time.Time `json:"lastTimestamp"`
	InvolvedObject struct {
		Kind string `json:"kind"`
		Name string `json:"name"`
	} `json:"involvedObject"`
}

func decodeEventObject(raw json.RawMessage) (Event, error) {
	var item rawEvent
	if err := json.Unmarshal(raw, &item); err != nil {
		return Event{}, err
	}
	return Event{
		Name:          item.Metadata.Name,
		UID:           item.Metadata.UID,
		Namespace:     item.Metadata.Namespace,
		Type:          item.Type,
		Reason:        item.Reason,
		Object:        strings.TrimPrefix(item.InvolvedObject.Kind+"/"+item.InvolvedObject.Name, "/"),
		Message:       item.Message,
		Age:           formatAge(time.Since(item.LastTimestamp)),
		Count:         item.Count,
		LastTimestamp: item.LastTimestamp,
	}, nil
}

func decodeIngressObject(raw json.RawMessage) (Ingress, error) {
	return decodeWatchObject(raw, ingressFromRaw)
}

func decodeNetworkPolicyObject(raw json.RawMessage) (NetworkPolicy, error) {
	return decodeWatchObject(raw, networkPolicyFromRaw)
}

func decodePVCObject(raw json.RawMessage) (PersistentVolumeClaim, error) {
	return decodeWatchObject(raw, pvcFromRaw)
}

func decodeCronJobObject(raw json.RawMessage) (CronJob, error) {
	return decodeWatchObject(raw, cronJobFromRaw)
}

func decodeHPAObject(raw json.RawMessage) (HPA, error) {
	return decodeWatchObject(raw, hpaFromRaw)
}

func decodeSecretObject(raw json.RawMessage) (Secret, error) {
	return decodeWatchObject(raw, secretFromRaw)
}

func decodeReplicaSetObject(raw json.RawMessage) (ReplicaSet, error) {
	return decodeWatchObject(raw, replicaSetFromRaw)
}
