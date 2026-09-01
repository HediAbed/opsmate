package logs

import (
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	clustermodel "github.com/HediAbed/opsmate/internal/cluster"
	"github.com/HediAbed/opsmate/internal/kube"
	clusterui "github.com/HediAbed/opsmate/internal/ui/cluster"
)

type errStub string

func (err errStub) Error() string {
	return string(err)
}

type testCommands struct {
	clusterui.Commands
}

func (testCommands) FetchPods(string) tea.Cmd {
	return func() tea.Msg { return clustermodel.PodsMsg{} }
}

type testOperations struct {
	clusterui.Operations
	logLines     []string
	logErr       error
	containers   []string
	containerErr error
}

func (operations *testOperations) FetchPodLogs(kube.PodLogRequest) tea.Cmd {
	return func() tea.Msg {
		return clustermodel.LogsMsg{Lines: operations.logLines, Err: operations.logErr}
	}
}

func (operations *testOperations) FetchPodContainers(kube.PodReference) tea.Cmd {
	return func() tea.Msg {
		return clustermodel.ContainersMsg{Containers: operations.containers, Err: operations.containerErr}
	}
}

func newTestLogsModel(namespace string) LogsModel {
	return NewLogsModel(namespace, testCommands{}, &testOperations{})
}

func stripAnsiForTest(value string) string {
	return ansi.Strip(value)
}
