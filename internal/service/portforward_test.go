package service

import (
	"errors"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestRegistry_AddAndGet(t *testing.T) {
	r := &portForwardRegistry{sessions: map[string]*portForwardProcess{}}
	sess := testPortForwardProcess(PortForwardSession{ID: "pf-1", Pod: "foo"})
	r.add(sess)

	if got := r.get("pf-1"); got != sess {
		t.Errorf("get returned %v; want %v", got, sess)
	}

}

func TestRegistry_FinishRemovesSessionAndRecordsCleanExit(t *testing.T) {
	r := &portForwardRegistry{sessions: map[string]*portForwardProcess{}}
	sess := testPortForwardProcess(PortForwardSession{ID: "pf-1", Status: PortForwardRunning})
	r.add(sess)

	r.finish("pf-1", nil)
	if r.get("pf-1") != nil {
		t.Fatal("finished session must be removed")
	}
}

func TestRegistry_FinishRecordsFailure(t *testing.T) {
	r := &portForwardRegistry{sessions: map[string]*portForwardProcess{}}
	sess := testPortForwardProcess(PortForwardSession{ID: "pf-2", Status: PortForwardRunning})
	r.add(sess)
	wantErr := testError("bind: address already in use")

	r.finish("pf-2", wantErr)
	if !errors.Is(sess.exitErr, wantErr) {
		t.Fatalf("session exit error = %v, want original error", sess.exitErr)
	}
}

func TestRegistry_Snapshot_IsValueCopy(t *testing.T) {
	r := &portForwardRegistry{sessions: map[string]*portForwardProcess{}}
	r.add(testPortForwardProcess(PortForwardSession{ID: "pf-1", Pod: "orig", Started: time.Now()}))

	snap := r.snapshot()
	if len(snap) != 1 {
		t.Fatalf("snapshot len = %d; want 1", len(snap))
	}
	snap[0].Pod = "modified"
	if got := r.get("pf-1"); got.info.Pod != "orig" {
		t.Errorf("registry mutated through snapshot: pod = %q", got.info.Pod)
	}
}

func TestRegistry_ConcurrentAccess_NoDataRace(_ *testing.T) {
	const (
		writerCount    = 8
		readerCount    = 16
		operationCount = 100
	)
	r := &portForwardRegistry{sessions: map[string]*portForwardProcess{}}
	var wg sync.WaitGroup

	for i := 0; i < writerCount; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			for j := 0; j < operationCount; j++ {
				id := "pf-" + strconv.Itoa(n*operationCount+j)
				r.add(testPortForwardProcess(PortForwardSession{ID: id}))
				r.finish(id, nil)
			}
		}(i)
	}
	for i := 0; i < readerCount; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < operationCount; j++ {
				_ = r.snapshot()
			}
		}()
	}
	wg.Wait()
}

func testPortForwardProcess(info PortForwardSession) *portForwardProcess {
	return &portForwardProcess{
		info:          info,
		done:          make(chan struct{}),
		stopRequested: make(chan struct{}),
	}
}

func TestPortForwardOutput_DetectsSplitReadinessMarker(t *testing.T) {
	output := newPortForwardOutput()
	if _, err := output.Write([]byte("Forward")); err != nil {
		t.Fatalf("first write: %v", err)
	}
	if _, err := output.Write([]byte("ing from 127.0.0.1:8080 -> 80\n")); err != nil {
		t.Fatalf("second write: %v", err)
	}
	select {
	case <-output.Ready():
	default:
		t.Fatal("split readiness marker was not detected")
	}
	if !strings.Contains(output.String(), "127.0.0.1:8080") {
		t.Fatalf("diagnostics = %q, want forwarded address", output.String())
	}
}

type testError string

func (e testError) Error() string { return string(e) }

func TestStopPortForward_NonexistentIDReturnsStoppedMsg(t *testing.T) {
	msg := StopPortForward("does-not-exist")().(PortForwardStoppedMsg)
	if msg.ID != "does-not-exist" {
		t.Errorf("ID = %q, want does-not-exist", msg.ID)
	}
}

func TestStopAllPortForwards_NoActiveIsSafe(_ *testing.T) {
	StopAllPortForwards()
}

func TestListPortForwards_EmptyByDefault(t *testing.T) {
	if ListPortForwards() == nil {
		t.Error("ListPortForwards must return non-nil slice")
	}
}
