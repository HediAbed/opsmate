package dashboard

import (
	"sync"
	"sync/atomic"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	clustermodel "github.com/HediAbed/opsmate/internal/cluster"
	clusterui "github.com/HediAbed/opsmate/internal/ui/cluster"
)

type errStub string

func (err errStub) Error() string {
	return string(err)
}

type testCommands struct {
	clusterui.Commands
	observeErr error
	calls      []string
}

func (*testCommands) FetchPodMetrics(string) tea.Cmd {
	return func() tea.Msg { return clustermodel.MetricsMsg{} }
}

func (commands *testCommands) ObservePods(string) (clusterui.LiveSet[clustermodel.Pod], error) {
	commands.calls = append(commands.calls, podResourceType)
	if commands.observeErr != nil {
		return nil, commands.observeErr
	}
	return newTestResourceLiveSet(clusterui.LiveState[clustermodel.Pod]{}), nil
}

func (commands *testCommands) ObserveDeployments(string) (clusterui.LiveSet[clustermodel.Deployment], error) {
	commands.calls = append(commands.calls, "deployments")
	if commands.observeErr != nil {
		return nil, commands.observeErr
	}
	return newTestResourceLiveSet(clusterui.LiveState[clustermodel.Deployment]{}), nil
}

func (commands *testCommands) ObserveEvents(string) (clusterui.LiveSet[clustermodel.Event], error) {
	commands.calls = append(commands.calls, "events")
	if commands.observeErr != nil {
		return nil, commands.observeErr
	}
	return newTestResourceLiveSet(clusterui.LiveState[clustermodel.Event]{}), nil
}

type testResourceLiveSet[T interface{}] struct {
	changes  chan struct{}
	state    clusterui.LiveState[T]
	stopOnce sync.Once
	stops    atomic.Int32
}

func newTestResourceLiveSet[T interface{}](state clusterui.LiveState[T]) *testResourceLiveSet[T] {
	return &testResourceLiveSet[T]{changes: make(chan struct{}, 1), state: state}
}

func (set *testResourceLiveSet[T]) Changes() <-chan struct{} {
	return set.changes
}

func (set *testResourceLiveSet[T]) State() clusterui.LiveState[T] {
	return set.state
}

func (set *testResourceLiveSet[T]) Stop() {
	set.stopOnce.Do(func() {
		set.stops.Add(1)
		close(set.changes)
	})
}

func newTestDashboardModel(namespace string) DashboardModel {
	return NewDashboardModel(namespace, &testCommands{})
}

func stripAnsiForTest(value string) string {
	return ansi.Strip(value)
}
