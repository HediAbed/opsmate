//go:build !windows

package service

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
)

func drain[T WatchResource](t *testing.T, w Watcher[T], timeout time.Duration) []WatchEvent[T] {
	t.Helper()
	deadline := time.After(timeout)
	var got []WatchEvent[T]
	for {
		select {
		case ev, ok := <-w.Events():
			if !ok {
				return got
			}
			got = append(got, ev)
		case <-deadline:
			t.Fatalf("drain timed out after %s; collected %d events", timeout, len(got))
		}
	}
}

func recvOne[T WatchResource](t *testing.T, w Watcher[T], timeout time.Duration) (WatchEvent[T], bool) {
	t.Helper()
	select {
	case ev, ok := <-w.Events():
		return ev, ok
	case <-time.After(timeout):
		t.Fatalf("recvOne timed out after %s", timeout)
		return WatchEvent[T]{}, false
	}
}

func TestParseWatchLine_AddedPod(t *testing.T) {
	line := []byte(`{"type":"ADDED","object":{"metadata":{"name":"foo","namespace":"ns","creationTimestamp":"2024-01-01T00:00:00Z"},"status":{"phase":"Running"},"spec":{"nodeName":"node-a"}}}`)
	ev, err := parseWatchLine(line, "pods", decodePodObject)
	if err != nil {
		t.Fatalf("parseWatchLine returned error: %v", err)
	}
	if ev.Kind != WatchAdded {
		t.Errorf("Kind = %q; want %q", ev.Kind, WatchAdded)
	}
	if ev.Item.Name != "foo" || ev.Item.Namespace != "ns" || ev.Item.Status != "Running" {
		t.Errorf("Pod fields wrong: %+v", ev.Item)
	}
}

func TestParseWatchLine_ModifiedDeployment(t *testing.T) {
	line := []byte(`{"type":"MODIFIED","object":{"metadata":{"name":"web","namespace":"prod","creationTimestamp":"2024-01-01T00:00:00Z"},"status":{"readyReplicas":3,"replicas":3,"updatedReplicas":3,"availableReplicas":3}}}`)
	ev, err := parseWatchLine(line, "deployments", decodeDeploymentObject)
	if err != nil {
		t.Fatalf("parseWatchLine returned error: %v", err)
	}
	if ev.Kind != WatchModified {
		t.Errorf("Kind = %q; want %q", ev.Kind, WatchModified)
	}
	if ev.Item.Ready != "3/3" {
		t.Errorf("Ready = %q; want 3/3", ev.Item.Ready)
	}
}

func TestParseWatchLine_DeletedEvent(t *testing.T) {
	line := []byte(`{"type":"DELETED","object":{"metadata":{"name":"retry-event","namespace":"ops","uid":"event-uid"},"type":"Warning","reason":"BackOff","message":"x","count":1,"lastTimestamp":"2024-01-01T00:00:00Z","involvedObject":{"kind":"Pod","name":"p"}}}`)
	ev, err := parseWatchLine(line, "events", decodeEventObject)
	if err != nil {
		t.Fatalf("parseWatchLine returned error: %v", err)
	}
	if ev.Kind != WatchDeleted {
		t.Errorf("Kind = %q; want %q", ev.Kind, WatchDeleted)
	}
	if ev.Item.Object != "Pod/p" {
		t.Errorf("Object = %q; want Pod/p", ev.Item.Object)
	}
	if ev.Item.Namespace != "ops" {
		t.Errorf("Namespace = %q; want ops", ev.Item.Namespace)
	}
	if ev.Item.Name != "retry-event" || ev.Item.UID != "event-uid" {
		t.Errorf("event identity = %q/%q; want retry-event/event-uid", ev.Item.Name, ev.Item.UID)
	}
}

func TestParseWatchLine_Bookmark(t *testing.T) {
	line := []byte(`{"type":"BOOKMARK","object":{"metadata":{"resourceVersion":"42"}}}`)
	ev, err := parseWatchLine(line, "pods", decodePodObject)
	if err != nil {
		t.Fatalf("parseWatchLine returned error: %v", err)
	}
	if ev.Kind != WatchBookmark {
		t.Errorf("Kind = %q; want %q", ev.Kind, WatchBookmark)
	}
}

func TestParseWatchLine_MalformedEnvelope(t *testing.T) {
	line := []byte(`{not-json`)
	_, err := parseWatchLine(line, "pods", decodePodObject)
	if err == nil {
		t.Fatal("expected error for malformed JSON")
	}
	var watchErr *WatchStreamError
	if !errors.As(err, &watchErr) {
		t.Fatalf("expected *WatchStreamError, got %T", err)
	}
	if watchErr.Stage != "decode envelope" {
		t.Errorf("Stage = %q; want decode envelope", watchErr.Stage)
	}
	if watchErr.Resource != "pods" {
		t.Errorf("Resource = %q; want pods", watchErr.Resource)
	}
}

func TestParseWatchLine_UnknownEventType(t *testing.T) {
	line := []byte(`{"type":"WTF","object":{}}`)
	_, err := parseWatchLine(line, "pods", decodePodObject)
	if err == nil {
		t.Fatal("expected error for unknown type")
	}
	if !strings.Contains(err.Error(), "WTF") {
		t.Errorf("error should mention the unknown type, got: %v", err)
	}
}

func TestParseWatchLine_ObjectDecodeError(t *testing.T) {
	line := []byte(`{"type":"ADDED","object":{"metadata":{"name":"x"},"status":42}}`)
	_, err := parseWatchLine(line, "pods", decodePodObject)
	if err == nil {
		t.Fatal("expected decode error")
	}
	var watchErr *WatchStreamError
	if !errors.As(err, &watchErr) {
		t.Fatalf("expected *WatchStreamError, got %T", err)
	}
	if watchErr.Stage != "decode object" {
		t.Errorf("Stage = %q; want decode object", watchErr.Stage)
	}
}

func TestBuildWatchArgs_NamespaceScoped(t *testing.T) {
	got := buildWatchArgs("pods", "kube-system")
	want := []string{"get", "pods", "-n", "kube-system", "-o", "json", "--watch", "--output-watch-events=true"}
	if !equalSlices(got, want) {
		t.Errorf("buildWatchArgs(pods, kube-system) = %v; want %v", got, want)
	}
}

func TestBuildWatchArgs_AllNamespaces(t *testing.T) {
	got := buildWatchArgs("deployments", "")
	want := []string{"get", "deployments", "--all-namespaces", "-o", "json", "--watch", "--output-watch-events=true"}
	if !equalSlices(got, want) {
		t.Errorf("buildWatchArgs(deployments, \"\") = %v; want %v", got, want)
	}
}

func equalSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestWatchStreamError_FormatsContext(t *testing.T) {
	wrapped := errors.New("connection refused")
	err := &WatchStreamError{Resource: "pods", Stage: "start", Err: wrapped}
	got := err.Error()
	for _, want := range []string{"watch", "pods", "start", "connection refused"} {
		if !strings.Contains(got, want) {
			t.Errorf("Error() = %q; missing %q", got, want)
		}
	}
}

func TestWatchStreamError_Unwrap(t *testing.T) {
	wrapped := errors.New("boom")
	err := &WatchStreamError{Err: wrapped}
	if !errors.Is(err, wrapped) {
		t.Error("errors.Is should match the wrapped error")
	}
}

func TestWatchStreamError_NoResourceNoStageNoErr(t *testing.T) {
	err := &WatchStreamError{}
	got := err.Error()
	if !strings.Contains(got, "watch") {
		t.Errorf("Error() with empty fields = %q; want it to mention 'watch'", got)
	}
	if !strings.Contains(got, "unknown error") {
		t.Errorf("Error() with no inner error = %q; expected 'unknown error' fallback", got)
	}
}

func TestDecodePodObject_RejectsMalformedObject(t *testing.T) {
	if _, err := decodePodObject([]byte(`{"metadata": "not-an-object"}`)); err == nil {
		t.Error("decodePodObject should fail on shape mismatch")
	}
}

func TestDecodePodObject_TalliesReadyAndRestartCount(t *testing.T) {
	raw := []byte(`{
		"metadata":{"name":"p","namespace":"ns","creationTimestamp":"2024-01-01T00:00:00Z"},
		"status":{
			"phase":"Running",
			"containerStatuses":[
				{"ready":true,"restartCount":2},
				{"ready":false,"restartCount":3},
				{"ready":true,"restartCount":0}
			]
		},
		"spec":{"nodeName":"node-a"}
	}`)
	pod, err := decodePodObject(raw)
	if err != nil {
		t.Fatalf("decodePodObject returned error: %v", err)
	}
	if pod.Ready != "2/3" {
		t.Errorf("Ready = %q; want 2/3 (two of three containers Ready)", pod.Ready)
	}
	if pod.Restarts != 5 {
		t.Errorf("Restarts = %d; want 5 (sum of restart counts)", pod.Restarts)
	}
}

func TestDecodeDeploymentObject_RejectsMalformedObject(t *testing.T) {
	if _, err := decodeDeploymentObject([]byte(`{"status": "not-an-object"}`)); err == nil {
		t.Error("decodeDeploymentObject should fail on shape mismatch")
	}
}

func TestDecodeEventObject_RejectsMalformedObject(t *testing.T) {
	if _, err := decodeEventObject([]byte(`{"count": "should-be-int"}`)); err == nil {
		t.Error("decodeEventObject should fail on type mismatch")
	}
}

func TestStartWatch_StreamsAddedAndModified(t *testing.T) {
	installFakeKubectl(t, `#!/bin/sh
echo '{"type":"ADDED","object":{"metadata":{"name":"a","namespace":"ns","creationTimestamp":"2024-01-01T00:00:00Z"},"status":{"phase":"Running"},"spec":{"nodeName":""}}}'
sleep 0.05
echo '{"type":"MODIFIED","object":{"metadata":{"name":"a","namespace":"ns","creationTimestamp":"2024-01-01T00:00:00Z"},"status":{"phase":"Succeeded"},"spec":{"nodeName":""}}}'
`)

	w := WatchPods(context.Background(), "ns")
	got := drain(t, w, 5*time.Second)

	if len(got) < 3 {
		t.Fatalf("expected at least 3 events (ADDED, MODIFIED, CLOSED), got %d: %+v", len(got), got)
	}
	if got[0].Kind != WatchAdded {
		t.Errorf("event 0: kind = %q; want %q", got[0].Kind, WatchAdded)
	}
	if got[0].Item.Name != "a" || got[0].Item.Status != "Running" {
		t.Errorf("event 0: Pod = %+v", got[0].Item)
	}
	if got[1].Kind != WatchModified {
		t.Errorf("event 1: kind = %q; want %q", got[1].Kind, WatchModified)
	}
	if got[1].Item.Status != "Succeeded" {
		t.Errorf("event 1: Pod = %+v", got[1].Item)
	}
	if got[len(got)-1].Kind != WatchClosed {
		t.Errorf("last event: kind = %q; want %q", got[len(got)-1].Kind, WatchClosed)
	}
}

func TestStartWatch_DeletedDelivered(t *testing.T) {
	installFakeKubectl(t, `#!/bin/sh
echo '{"type":"DELETED","object":{"metadata":{"name":"gone","namespace":"ns","creationTimestamp":"2024-01-01T00:00:00Z"},"status":{"phase":"Failed"},"spec":{"nodeName":""}}}'
`)
	w := WatchPods(context.Background(), "ns")
	got := drain(t, w, 5*time.Second)

	if len(got) < 2 {
		t.Fatalf("expected DELETED+CLOSED, got %+v", got)
	}
	if got[0].Kind != WatchDeleted || got[0].Item.Name != "gone" {
		t.Errorf("DELETED frame wrong: %+v", got[0])
	}
}

func TestStartWatch_LargeObjectAboveDefaultBuffer(t *testing.T) {
	const annotationSize = 8 * 1024 * 1024
	bigAnnotation := strings.Repeat("x", annotationSize)
	line := fmt.Sprintf(
		`{"type":"ADDED","object":{"metadata":{"name":"big","namespace":"ns","creationTimestamp":"2024-01-01T00:00:00Z","annotations":{"k":"%s"}},"status":{"phase":"Running"},"spec":{"nodeName":""}}}`,
		bigAnnotation,
	)
	script := "#!/bin/sh\nprintf '%s\\n' '" + line + "'\n"
	installFakeKubectl(t, script)

	w := WatchPods(context.Background(), "ns")
	got := drain(t, w, 10*time.Second)

	if len(got) < 1 || got[0].Kind != WatchAdded {
		t.Fatalf("expected ADDED first, got %+v", got)
	}
	if got[0].Item.Name != "big" {
		t.Errorf("expected pod name 'big', got %q", got[0].Item.Name)
	}
}

func TestStartWatch_MalformedLineBecomesErrorEvent(t *testing.T) {
	installFakeKubectl(t, `#!/bin/sh
echo '{not-json'
echo '{"type":"ADDED","object":{"metadata":{"name":"ok","namespace":"ns","creationTimestamp":"2024-01-01T00:00:00Z"},"status":{"phase":"Running"},"spec":{"nodeName":""}}}'
`)
	w := WatchPods(context.Background(), "ns")
	got := drain(t, w, 5*time.Second)

	if len(got) < 3 {
		t.Fatalf("expected ERROR+ADDED+CLOSED, got %d events: %+v", len(got), got)
	}
	if got[0].Kind != WatchErrored {
		t.Errorf("first frame kind = %q; want %q", got[0].Kind, WatchErrored)
	}
	if got[0].Err == nil {
		t.Error("error frame should carry an Err")
	}
	if got[1].Kind != WatchAdded || got[1].Item.Name != "ok" {
		t.Errorf("second frame should be ADDED ok, got %+v", got[1])
	}
}

func TestStartWatch_ContextCancelKillsSubprocess(t *testing.T) {
	installFakeKubectl(t, `#!/bin/sh
trap "exit 0" TERM
while :; do sleep 0.05; done
`)
	w := WatchPods(context.Background(), "ns")

	time.Sleep(50 * time.Millisecond)

	w.Cancel()
	drain(t, w, 3*time.Second)
}

func TestStartWatch_CancelTwiceIsSafe(t *testing.T) {
	installFakeKubectl(t, `#!/bin/sh
sleep 5
`)
	w := WatchPods(context.Background(), "ns")
	w.Cancel()
	w.Cancel()
	drain(t, w, 3*time.Second)
}

func TestStartWatch_ExternalContextCancelStops(t *testing.T) {
	installFakeKubectl(t, `#!/bin/sh
trap "exit 0" TERM
while :; do sleep 0.05; done
`)
	ctx, cancel := context.WithCancel(context.Background())
	w := WatchPods(ctx, "ns")
	time.Sleep(50 * time.Millisecond)
	cancel()
	drain(t, w, 3*time.Second)
}

func TestStartWatch_EmptyOutputClosesCleanly(t *testing.T) {
	installFakeKubectl(t, `#!/bin/sh
exit 0
`)
	w := WatchPods(context.Background(), "ns")
	got := drain(t, w, 3*time.Second)
	if len(got) == 0 || got[len(got)-1].Kind != WatchClosed {
		t.Errorf("empty stream should still close gracefully, got %+v", got)
	}
}

func TestStartWatch_NonZeroExitIncludesStderr(t *testing.T) {
	installFakeKubectl(t, `#!/bin/sh
printf 'authorization denied' >&2
exit 2
`)
	w := WatchPods(context.Background(), "ns")
	events := drain(t, w, 3*time.Second)
	if len(events) != 2 {
		t.Fatalf("events = %+v, want error then closed", events)
	}
	if events[0].Kind != WatchErrored || events[0].Err == nil {
		t.Fatalf("first event = %+v, want error", events[0])
	}
	if !strings.Contains(events[0].Err.Error(), "authorization denied") {
		t.Fatalf("error = %q, want captured stderr", events[0].Err)
	}
	var streamErr *WatchStreamError
	if !errors.As(events[0].Err, &streamErr) || streamErr.Stage != "exit" {
		t.Fatalf("error = %#v, want exit-stage WatchStreamError", events[0].Err)
	}
	if events[1].Kind != WatchClosed {
		t.Fatalf("last event = %q, want CLOSED", events[1].Kind)
	}
}

func TestStartWatch_DeploymentsRouteToCorrectDecoder(t *testing.T) {
	installFakeKubectl(t, `#!/bin/sh
echo '{"type":"ADDED","object":{"metadata":{"name":"web","namespace":"prod","creationTimestamp":"2024-01-01T00:00:00Z"},"status":{"readyReplicas":2,"replicas":3,"updatedReplicas":3,"availableReplicas":2}}}'
`)
	w := WatchDeployments(context.Background(), "prod")
	ev, ok := recvOne(t, w, 5*time.Second)
	if !ok {
		t.Fatal("channel closed before delivering event")
	}
	if ev.Kind != WatchAdded {
		t.Errorf("Kind = %q; want ADDED", ev.Kind)
	}
	if ev.Item.Ready != "2/3" {
		t.Errorf("Ready = %q; want 2/3", ev.Item.Ready)
	}
	w.Cancel()
	drain(t, w, 3*time.Second)
}

func TestStartWatch_EventsRouteToCorrectDecoder(t *testing.T) {
	installFakeKubectl(t, `#!/bin/sh
echo '{"type":"ADDED","object":{"type":"Warning","reason":"FailedScheduling","message":"no nodes","count":3,"lastTimestamp":"2024-01-01T00:00:00Z","involvedObject":{"kind":"Pod","name":"p"}}}'
`)
	w := WatchEvents(context.Background(), "ns")
	ev, ok := recvOne(t, w, 5*time.Second)
	if !ok {
		t.Fatal("channel closed before delivering event")
	}
	if ev.Item.Reason != "FailedScheduling" || ev.Item.Object != "Pod/p" {
		t.Errorf("Event = %+v", ev.Item)
	}
	w.Cancel()
	drain(t, w, 3*time.Second)
}

func TestNextWatchEvent_DeliversFrame(t *testing.T) {
	installFakeKubectl(t, `#!/bin/sh
echo '{"type":"ADDED","object":{"metadata":{"name":"a","namespace":"ns","creationTimestamp":"2024-01-01T00:00:00Z"},"status":{"phase":"Running"},"spec":{"nodeName":""}}}'
sleep 0.5
`)
	w := WatchPods(context.Background(), "ns")
	defer w.Cancel()

	cmd := NextWatchEvent(w)
	msg := cmd()
	wrapped, ok := msg.(WatchEventMsg[Pod])
	if !ok {
		t.Fatalf("expected WatchEventMsg[Pod], got %T", msg)
	}
	if wrapped.Event.Kind != WatchAdded {
		t.Errorf("Kind = %q; want ADDED", wrapped.Event.Kind)
	}
}

func TestNextWatchEvent_ReturnsClosedMsgOnChannelClose(t *testing.T) {
	installFakeKubectl(t, `#!/bin/sh
exit 0
`)
	w := WatchPods(context.Background(), "ns")

	drainEvent := <-w.Events()
	if drainEvent.Kind != WatchClosed {
		t.Fatalf("first event = %q; want CLOSED", drainEvent.Kind)
	}

	cmd := NextWatchEvent(w)
	msg := cmd()
	if _, ok := msg.(WatchClosedMsg); !ok {
		t.Errorf("expected WatchClosedMsg after channel close, got %T", msg)
	}
}

func TestStartWatch_BinaryMissingReportsErrorThenCloses(t *testing.T) {
	tmp := t.TempDir()
	prev := os.Getenv("PATH")
	t.Cleanup(func() { _ = os.Setenv("PATH", prev) })
	if err := os.Setenv("PATH", tmp); err != nil {
		t.Fatalf("set PATH: %v", err)
	}

	w := WatchPods(context.Background(), "ns")
	got := drain(t, w, 3*time.Second)

	if len(got) != 2 {
		t.Fatalf("expected ERROR + CLOSED frames, got %d: %+v", len(got), got)
	}
	if got[0].Kind != WatchErrored {
		t.Errorf("first frame kind = %q; want %q", got[0].Kind, WatchErrored)
	}
	if got[0].Err == nil {
		t.Error("first frame should carry an Err")
	}
	if got[1].Kind != WatchClosed {
		t.Errorf("second frame kind = %q; want %q", got[1].Kind, WatchClosed)
	}
}

func TestStartWatch_BinaryMissingErrorClassifiedAsStart(t *testing.T) {
	tmp := t.TempDir()
	prev := os.Getenv("PATH")
	t.Cleanup(func() { _ = os.Setenv("PATH", prev) })
	if err := os.Setenv("PATH", tmp); err != nil {
		t.Fatalf("set PATH: %v", err)
	}

	w := WatchPods(context.Background(), "ns")
	got := drain(t, w, 3*time.Second)

	if len(got) == 0 || got[0].Err == nil {
		t.Fatalf("expected an ERROR frame with Err, got %+v", got)
	}
	var streamErr *WatchStreamError
	if !errors.As(got[0].Err, &streamErr) {
		t.Fatalf("expected *WatchStreamError, got %T", got[0].Err)
	}
	if streamErr.Stage != "start" {
		t.Errorf("Stage = %q; want start", streamErr.Stage)
	}
	if streamErr.Resource != "pods" {
		t.Errorf("Resource = %q; want pods", streamErr.Resource)
	}
}
