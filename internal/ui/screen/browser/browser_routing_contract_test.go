package browser

import (
	"errors"
	"strings"
	"testing"

	"github.com/HediAbed/opsmate/internal/analysis"
	"github.com/HediAbed/opsmate/internal/cluster"
	"github.com/HediAbed/opsmate/internal/kube"
	"github.com/HediAbed/opsmate/internal/ui/screen"
)

func TestBrowserUpdateRejectsStaleFetchResult(t *testing.T) {
	model := newTestBrowserModel("team-a")
	message := model.fetchCurrentResources()().(browserResultMsg)
	_ = model.fetchCurrentResources()
	message.payload = cluster.PodsMsg{Pods: []cluster.Pod{{Name: "stale", Namespace: "team-a"}}}
	if model.Accepts(message) {
		t.Fatal("stale fetch result was accepted")
	}

	updated, command := model.Update(message)
	if command != nil || len(updated.pods) != 0 {
		t.Fatalf("stale fetch changed browser: command=%v pods=%v", command, updated.pods)
	}
}

func TestBrowserUpdateAcceptsOnlyCurrentDetailSummary(t *testing.T) {
	model := newTestBrowserModel("team-a")
	model.pods = []cluster.Pod{{Name: "web", Namespace: "team-a"}}
	model.rebuildTable()
	model.detailKind = "describe"
	model.detailContent = "pod details"
	model.detailRequestID++
	identity, selected := model.selectedIdentity()
	if !selected {
		t.Fatal("browser fixture has no selected resource")
	}
	message := browserDetailSummaryResultMsg{
		requestID: model.detailRequestID,
		identity:  identity,
		content:   model.detailContent,
		payload:   analysis.DescribeSummaryMsg{Summary: "healthy"},
	}
	if !model.Accepts(message) {
		t.Fatal("current detail summary was rejected")
	}
	updated, command := model.Update(message)
	if command != nil || updated.analysisSummary != "healthy" {
		t.Fatalf("current detail summary = %q, command=%v", updated.analysisSummary, command)
	}

	staleMutations := []struct {
		name   string
		mutate func(*browserDetailSummaryResultMsg)
	}{
		{name: "request id", mutate: func(msg *browserDetailSummaryResultMsg) { msg.requestID-- }},
		{name: "identity", mutate: func(msg *browserDetailSummaryResultMsg) {
			msg.identity = resourceIdentity{Kind: identity.Kind, Name: "other", Namespace: identity.Namespace}
		}},
		{name: "content", mutate: func(msg *browserDetailSummaryResultMsg) { msg.content = "stale details" }},
	}
	for _, mutation := range staleMutations {
		t.Run(mutation.name, func(t *testing.T) {
			stale := message
			mutation.mutate(&stale)
			stale.payload = analysis.DescribeSummaryMsg{Summary: "stale sentinel"}
			if updated.Accepts(stale) {
				t.Fatal("stale detail summary was accepted")
			}
			unchanged, command := updated.Update(stale)
			if command != nil || unchanged.analysisSummary != "healthy" {
				t.Fatalf("stale detail summary changed state: summary=%q command=%v", unchanged.analysisSummary, command)
			}
		})
	}
}

func TestBrowserMessageContractsClassifyShellAndContextEvents(t *testing.T) {
	model := newTestBrowserModel("team-a")
	if !model.Accepts(shellOutputMsg{}) {
		t.Fatal("browser did not accept its shell output message")
	}
	if !model.ContextChangedBy(model.fetchCurrentResources()()) {
		t.Fatal("browser ignored its current fetch result")
	}
	model.active = true
	liveCommand := model.startResourceLiveSet()
	if liveCommand == nil {
		t.Fatal("browser did not start its live resource set")
	}
	liveMessage, ok := liveCommand().(screen.LiveMessage)
	if !ok {
		t.Fatalf("live command returned %#v", liveMessage)
	}
	if !model.ContextChangedBy(liveMessage) {
		t.Fatal("browser ignored its owned live message")
	}
	model.stopAllLiveSets()
	if model.ContextChangedBy(liveMessage) {
		t.Fatal("browser still claims a live message after stopping its sets")
	}
	if model.ContextChangedBy(screen.LiveMessage{}) {
		t.Fatal("browser claimed a live message without an owned generation")
	}
	if model.ContextChangedBy(struct{}{}) {
		t.Fatal("browser claimed an unrelated message changed its context")
	}
}

func TestBrowserShellMessagesWithoutSessionAreIgnored(t *testing.T) {
	model := newTestBrowserModel("team-a")
	model.state = stateShell
	withoutSession, command := model.Update(shellOutputMsg{SessionID: "missing", Line: "discarded"})
	if command != nil || len(withoutSession.shellLines) != 0 {
		t.Fatalf("output without a session changed shell: command=%v lines=%v", command, withoutSession.shellLines)
	}
	interrupted, command := model.interruptShell()
	if command != nil || interrupted.state != stateBrowsing || !strings.Contains(interrupted.statusMsg, "interrupted") {
		t.Fatalf("missing-session interrupt = state:%d status:%q command:%v", interrupted.state, interrupted.statusMsg, command)
	}
}

func TestBrowserShellLifecycleRoutesThroughUpdate(t *testing.T) {
	model := newTestBrowserModel("team-a")
	model.state = stateShell
	session := &testShellSession{
		identity: kube.ShellIdentity{ID: "active"},
		output:   make(chan kube.ShellOutput),
		exit:     make(chan kube.ShellExit),
	}
	model.shellSession = session
	updated, command := model.Update(shellOutputMsg{SessionID: session.identity.ID, Line: "ready"})
	if command == nil || len(updated.shellLines) != 1 || updated.shellLines[0] != "ready" {
		t.Fatalf("shell output = lines:%v command:%v", updated.shellLines, command)
	}
	updated, command = updated.Update(shellExitMsg{SessionID: session.identity.ID})
	if command != nil || updated.state != stateBrowsing || updated.shellSession != nil || !session.closed {
		t.Fatalf("shell exit = state:%d session:%v closed:%v command:%v", updated.state, updated.shellSession, session.closed, command)
	}
}

func TestBrowserInspectResourceReportsUnsupportedKind(t *testing.T) {
	message := newTestBrowserModel("team-a").InspectResource("unsupported", "name", "team-a")()
	describe, ok := message.(cluster.DescribeMsg)
	if !ok {
		t.Fatalf("unsupported inspection returned %#v", message)
	}
	if !errors.Is(describe.Err, ErrUnsupportedResourceKind) {
		t.Fatalf("unsupported inspection error = %v, want ErrUnsupportedResourceKind", describe.Err)
	}
	var kindErr *ResourceKindError
	if !errors.As(describe.Err, &kindErr) || kindErr.Kind != "unsupported" {
		t.Fatalf("unsupported inspection error = %#v, want resource kind error for unsupported", describe.Err)
	}
}

func TestBrowserStatefulSetReplicaInfoRequiresMatchingIdentity(t *testing.T) {
	model := newTestBrowserModel("team-a")
	model.statefulsets = []cluster.StatefulSet{{Name: "database", Namespace: "team-a", Ready: "1/1", Replicas: 1}}
	if got := model.statefulSetReplicaInfo(resourceIdentity{Name: "database", Namespace: "team-a"}); got != "currently 1/1 ready, 1 replicas" {
		t.Fatalf("matching stateful set replica info = %q", got)
	}
	if got := model.statefulSetReplicaInfo(resourceIdentity{Name: "other", Namespace: "team-a"}); got != "" {
		t.Fatalf("unmatched stateful set replica info = %q", got)
	}
	if got := model.statefulSetReplicaInfo(resourceIdentity{Name: "database", Namespace: "other"}); got != "" {
		t.Fatalf("foreign namespace replica info = %q", got)
	}
}
