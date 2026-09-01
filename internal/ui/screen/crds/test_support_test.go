package crds

import (
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	clustermodel "github.com/HediAbed/opsmate/internal/cluster"
	clusterui "github.com/HediAbed/opsmate/internal/ui/cluster"
	"github.com/HediAbed/opsmate/internal/ui/component"
)

const (
	analysisOverlayMinimumPanelHeight = component.MinimumAnalysisPanelHeight
	tableHeaderChromeRows             = component.TableHeaderRows
	tableWheelStep                    = component.TableWheelStep
)

type testCommands struct {
	clusterui.Commands
}

type errStub string

func (err errStub) Error() string {
	return string(err)
}

func (testCommands) FetchCRDs() tea.Cmd {
	return func() tea.Msg { return clustermodel.CRDsMsg{} }
}

func (testCommands) FetchCRDInstances(resource clustermodel.CRD, namespace string) tea.Cmd {
	return func() tea.Msg {
		return clustermodel.CRDInstancesMsg{Resource: resource.Resource, Namespace: namespace}
	}
}

func newTestCRDsModel(namespace string) CRDsModel {
	return NewCRDsModel(namespace, testCommands{})
}

func stripAnsiForTest(value string) string {
	return ansi.Strip(value)
}
