package ui

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/HediAbed/opsmate/internal/cluster"
	"github.com/HediAbed/opsmate/internal/failure"
	"github.com/HediAbed/opsmate/internal/kube"
)

var (
	ErrPortForwardArgumentsRequired = errors.New("expected <pod> <local>:<remote>")
	ErrPortForwardNamespaceRequired = errors.New("select a namespace before starting a port forward")
	ErrPortMappingInvalid           = errors.New("expected <local>:<remote>")
	ErrLocalPortInvalid             = errors.New("local port is invalid")
	ErrRemotePortInvalid            = errors.New("remote port is invalid")
)

type PortForwardInputError struct {
	Input string
	Err   error
}

func (e PortForwardInputError) Error() string {
	prefix := "port-forward input"
	if e.Input != "" {
		prefix += fmt.Sprintf(" %q", e.Input)
	}
	if e.Err == nil {
		return prefix + " is invalid"
	}
	return prefix + ": " + e.Err.Error()
}

func (e PortForwardInputError) Unwrap() error {
	return e.Err
}

func (PortForwardInputError) FailureCode() failure.Code {
	return failure.CodeInvalidArgument
}

func (m RootModel) handleCmdPalette(key string, msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch key {
	case "esc":
		m.showCmdPalette = false
		m.cmdInput.Blur()
		return m, nil
	case "enter":
		cmd := strings.TrimSpace(m.cmdInput.Value())
		m.showCmdPalette = false
		m.cmdInput.Blur()
		return m.executePaletteCommand(cmd)
	default:
		var cmd tea.Cmd
		m.cmdInput, cmd = m.cmdInput.Update(msg)
		return m, cmd
	}
}

func (m RootModel) executePaletteCommand(input string) (tea.Model, tea.Cmd) {
	parts := strings.Fields(input)
	if len(parts) == 0 {
		return m, nil
	}
	command := strings.ToLower(parts[0])
	if resourceType, resourceCommand := paletteResourceType(command); resourceCommand {
		return m.openPaletteResource(resourceType)
	}

	switch command {
	case "q", "quit":
		return m, tea.Quit
	case "pf", "port-forward":
		return m.startPortForwardFromPalette(parts[1:])
	case "ns":
		return m.executeNamespacePaletteCommand(parts[1:])
	case "logs", "log":
		return m.openPaletteLogs(parts[1:])
	default:
		return m, nil
	}
}

func paletteResourceType(command string) (string, bool) {
	switch command {
	case resourceKindPod, resourceTypePods:
		return resourceTypePods, true
	case "deploy", "dep":
		return resourceTypeDeployments, true
	case "svc":
		return resourceTypeServices, true
	case "sts":
		return resourceTypeStatefulSets, true
	case "ds":
		return resourceTypeDaemonSets, true
	case "cm":
		return resourceTypeConfigMaps, true
	case resourceKindNode, resourceTypeNodes:
		return resourceTypeNodes, true
	case resourceKindJob, resourceTypeJobs:
		return resourceTypeJobs, true
	default:
		return "", false
	}
}

func (m RootModel) openPaletteResource(resourceType string) (tea.Model, tea.Cmd) {
	m.browser.SetResourceType(resourceType)
	m.browser.loading = true
	var command tea.Cmd
	if m.screen == ScreenBrowser {
		command = m.browser.Activate()
	} else {
		command = m.transitionTo(ScreenBrowser, true)
	}
	m.persistSession()
	return m, command
}

func (m RootModel) executeNamespacePaletteCommand(args []string) (tea.Model, tea.Cmd) {
	if len(args) == 0 {
		return m, m.openNamespacePicker()
	}
	m.namespace = args[0]
	return m, m.switchNamespace()
}

func (m RootModel) openPaletteLogs(args []string) (tea.Model, tea.Cmd) {
	podName := ""
	if len(args) > 0 {
		podName = args[0]
	}
	return m, func() tea.Msg {
		return DrillDownMsg{Screen: ScreenLogs, ResourceName: podName}
	}
}

func (m RootModel) startPortForwardFromPalette(args []string) (tea.Model, tea.Cmd) {
	if len(args) < portForwardMinimumArgumentCount {
		return m, func() tea.Msg {
			return PortForwardFeedbackMsg{
				Err: PortForwardInputError{Err: ErrPortForwardArgumentsRequired},
			}
		}
	}
	pod := args[0]
	localPort, remotePort, err := parsePortSpec(args[1])
	if err != nil {
		return m, func() tea.Msg {
			return PortForwardFeedbackMsg{Err: err}
		}
	}
	ns := m.namespace
	if ns == "" {
		return m, func() tea.Msg {
			return PortForwardFeedbackMsg{
				Err: PortForwardInputError{Err: ErrPortForwardNamespaceRequired},
			}
		}
	}
	return m, m.operations.StartPortForward(kube.PortForwardRequest{
		Pod:        kube.PodReference{Namespace: ns, Name: pod},
		LocalPort:  localPort,
		RemotePort: remotePort,
	})
}

func parsePortSpec(spec string) (kube.NetworkPort, kube.NetworkPort, error) {
	parts := strings.SplitN(spec, ":", portSpecPartCount)
	if len(parts) != portSpecPartCount {
		return kube.NetworkPort{}, kube.NetworkPort{}, PortForwardInputError{Input: spec, Err: ErrPortMappingInvalid}
	}
	localValue, err := strconv.Atoi(parts[0])
	if err != nil {
		return kube.NetworkPort{}, kube.NetworkPort{}, PortForwardInputError{Input: spec, Err: ErrLocalPortInvalid}
	}
	localPort, err := kube.NewNetworkPort(localValue)
	if err != nil {
		return kube.NetworkPort{}, kube.NetworkPort{}, PortForwardInputError{Input: spec, Err: fmt.Errorf("%w: %w", ErrLocalPortInvalid, err)}
	}
	remoteValue, err := strconv.Atoi(parts[1])
	if err != nil {
		return kube.NetworkPort{}, kube.NetworkPort{}, PortForwardInputError{Input: spec, Err: ErrRemotePortInvalid}
	}
	remotePort, err := kube.NewNetworkPort(remoteValue)
	if err != nil {
		return kube.NetworkPort{}, kube.NetworkPort{}, PortForwardInputError{Input: spec, Err: fmt.Errorf("%w: %w", ErrRemotePortInvalid, err)}
	}
	return localPort, remotePort, nil
}

type PortForwardFeedbackMsg struct {
	Err error
}

func (m RootModel) handleDrillDown(msg DrillDownMsg) (tea.Model, tea.Cmd) {
	m.prepareDrillDown(msg)
	commands := make([]tea.Cmd, 0, rootActivationCommandCapacity)
	commands = appendCommand(commands, m.transitionTo(msg.Screen, true))
	commands = appendCommand(commands, m.drillDownCommand(msg))
	m.persistSession()
	return m, tea.Batch(commands...)
}

func (m *RootModel) prepareDrillDown(msg DrillDownMsg) {
	switch msg.Screen {
	case ScreenBrowser:
		if resourceType := browserResourceType(msg.ResourceType); resourceType != "" {
			m.browser.SetResourceType(resourceType)
		}
	case ScreenLogs:
		m.prepareLogDrillDown(msg)
	case ScreenDashboard, ScreenAnalysis, ScreenHelm, ScreenCRDs:
	}
}

func (m *RootModel) prepareLogDrillDown(msg DrillDownMsg) {
	if msg.ResourceName == "" {
		return
	}
	m.logs.SetPodInNamespace(msg.ResourceName, defaultNamespace(msg.ResourceNS, m.namespace))
}

func (m RootModel) drillDownCommand(msg DrillDownMsg) tea.Cmd {
	if msg.ResourceName == "" {
		return nil
	}
	switch msg.Screen {
	case ScreenBrowser:
		resource, err := groupResourceForKind(msg.ResourceType)
		if err != nil {
			return func() tea.Msg { return cluster.DescribeMsg{Err: err} }
		}
		return m.browser.operations.InspectResource(kube.ResourceReference{
			Resource:  resource,
			Namespace: defaultNamespace(msg.ResourceNS, m.namespace),
			Name:      msg.ResourceName,
		})
	case ScreenLogs:
		return m.logs.SetPodCmd()
	case ScreenDashboard, ScreenAnalysis, ScreenHelm, ScreenCRDs:
		return nil
	}
	return nil
}

func appendCommand(commands []tea.Cmd, command tea.Cmd) []tea.Cmd {
	if command == nil {
		return commands
	}
	return append(commands, command)
}

func defaultNamespace(resourceNamespace, activeNamespace string) string {
	if resourceNamespace != "" {
		return resourceNamespace
	}
	return activeNamespace
}

func browserResourceType(kind string) string {
	if _, ok := resourceCatalog[kind]; ok {
		return kind
	}
	for resourceType, binding := range resourceCatalog {
		if binding.Singular == kind {
			return resourceType
		}
	}
	return ""
}
