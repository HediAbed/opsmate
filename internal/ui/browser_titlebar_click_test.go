package ui

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"

	"github.com/HediAbed/opsmate/internal/theme"
)

func TestHandleTitleBarClick_ClickOnTitleNoOps(t *testing.T) {
	m := newTestBrowserModel("ns")
	m.SetSize(220, 40)
	out, cmd := m.handleTitleBarClick(0)
	if out.resourceType != m.resourceType {
		t.Error("clicking the title text should not switch resource type")
	}
	if cmd != nil {
		t.Error("clicking the title text should not return a cmd")
	}
}

func TestHandleTitleBarClick_ClickOnDeploymentsTabSwitches(t *testing.T) {
	m := newTestBrowserModel("ns")
	m.SetSize(220, 40)
	if m.resourceType == "deployments" {
		t.Fatal("test prereq: starting kind must not be deployments")
	}

	titleX := lipgloss.Width(theme.Title.Render("KUBERNETES BROWSER")) + 2
	cumX := 0
	for _, rt := range allResourceTypes {
		label := " " + strings.ToUpper(rt) + " "
		var w int
		if rt == m.resourceType {
			w = lipgloss.Width(theme.StatusBarActive.Render(label))
		} else {
			w = lipgloss.Width(theme.StatusBarItem.Render(label))
		}
		if rt == "deployments" {
			out, _ := m.handleTitleBarClick(titleX + cumX + 1)
			if out.resourceType != "deployments" {
				t.Errorf("click on deployments tab should switch resource type; got %q", out.resourceType)
			}
			return
		}
		cumX += w
	}
	t.Fatal("could not locate deployments tab in allResourceTypes")
}

func TestHandleTitleBarClick_ClickOnActiveTabIsNoOp(t *testing.T) {
	m := newTestBrowserModel("ns")
	m.SetSize(220, 40)
	m.SetResourceType("services")

	titleX := lipgloss.Width(theme.Title.Render("KUBERNETES BROWSER")) + 2
	cumX := 0
	for _, rt := range allResourceTypes {
		label := " " + strings.ToUpper(rt) + " "
		var w int
		if rt == m.resourceType {
			w = lipgloss.Width(theme.StatusBarActive.Render(label))
		} else {
			w = lipgloss.Width(theme.StatusBarItem.Render(label))
		}
		if rt == "services" {
			out, cmd := m.handleTitleBarClick(titleX + cumX + 1)
			if out.resourceType != "services" {
				t.Errorf("click on already-active tab must not change resource; got %q", out.resourceType)
			}
			if cmd != nil {
				t.Error("click on active tab must not return a cmd")
			}
			return
		}
		cumX += w
	}
}
