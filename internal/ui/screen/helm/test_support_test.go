package helm

import (
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/HediAbed/opsmate/internal/kube"
	clusterui "github.com/HediAbed/opsmate/internal/ui/cluster"
)

type helmReleasesMsg = clusterui.HelmReleasesMsg
type helmValuesMsg = clusterui.HelmValuesMsg

type testHelmCommands struct {
	listReleases func(string) tea.Cmd
	getValues    func(kube.HelmReleaseReference) tea.Cmd
}

func (commands *testHelmCommands) ListReleases(namespace string) tea.Cmd {
	if commands != nil && commands.listReleases != nil {
		return commands.listReleases(namespace)
	}
	return func() tea.Msg { return helmReleasesMsg{} }
}

func (commands *testHelmCommands) GetValues(reference kube.HelmReleaseReference) tea.Cmd {
	if commands != nil && commands.getValues != nil {
		return commands.getValues(reference)
	}
	return func() tea.Msg {
		return helmValuesMsg{Release: reference.Name, Namespace: reference.Namespace}
	}
}

func newTestHelmModel(namespace string) HelmModel {
	return NewHelmModel(namespace, &testHelmCommands{})
}

func stripAnsiForTest(value string) string {
	return ansi.Strip(value)
}
