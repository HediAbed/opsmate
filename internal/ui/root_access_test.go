package ui

import (
	"io/fs"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/HediAbed/opsmate/internal/cluster"
	"github.com/HediAbed/opsmate/internal/failure"
	"github.com/HediAbed/opsmate/internal/kube"
	"github.com/HediAbed/opsmate/internal/session"
)

func namespaceListDeniedError() error {
	return &kube.Error{Operation: kube.OperationList, Subject: kube.SubjectNamespaces, Code: failure.CodePermissionDenied}
}

func TestRootDeniedNamespaceListFallsBackToContextNamespace(t *testing.T) {
	model := newTestRootModel(t, "")
	model.width = 200
	model.height = 50
	model.nsLoading = true

	updated, _ := model.Update(cluster.ContextsMsg{Contexts: []cluster.KubeContext{
		{Name: "prod", Namespace: "checkout", Current: true},
		{Name: "staging", Namespace: "staging-ns"},
	}})
	root := updated.(RootModel)
	if root.namespace != "" {
		t.Fatalf("context list alone changed namespace to %q", root.namespace)
	}

	updated, command := root.Update(cluster.NamespacesMsg{Err: namespaceListDeniedError()})
	root = updated.(RootModel)
	if root.err != nil {
		t.Fatalf("denied namespace list surfaced as error: %v", root.err)
	}
	if root.namespace != "checkout" || !root.namespaceListDenied || root.nsLoading {
		t.Fatalf("fallback state = namespace:%q denied:%v loading:%v", root.namespace, root.namespaceListDenied, root.nsLoading)
	}
	if command == nil {
		t.Fatal("namespace fallback must reload the active screen")
	}
	if !strings.Contains(root.notice, "showing namespace checkout") {
		t.Fatalf("notice = %q, want context namespace fallback", root.notice)
	}
	if root.dashboard.Namespace() != "checkout" {
		t.Fatalf("dashboard namespace = %q, want checkout", root.dashboard.Namespace())
	}
}

func TestRootDeniedNamespaceListWithoutContextNamespaceUsesDefault(t *testing.T) {
	model := newTestRootModel(t, "")
	model.width = 200
	model.height = 50
	updated, _ := model.Update(cluster.ContextsMsg{Contexts: []cluster.KubeContext{{Name: "prod", Current: true}}})
	updated, _ = updated.(RootModel).Update(cluster.NamespacesMsg{Err: namespaceListDeniedError()})
	root := updated.(RootModel)
	if root.namespace != defaultClusterNamespace {
		t.Fatalf("namespace = %q, want %q", root.namespace, defaultClusterNamespace)
	}
}

func TestRootDeniedNamespaceListKeepsExplicitNamespace(t *testing.T) {
	model := newTestRootModel(t, "payments")
	model.width = 200
	model.height = 50
	updated, _ := model.Update(cluster.ContextsMsg{Contexts: []cluster.KubeContext{{Name: "prod", Namespace: "checkout", Current: true}}})
	updated, command := updated.(RootModel).Update(cluster.NamespacesMsg{Err: namespaceListDeniedError()})
	root := updated.(RootModel)
	if root.namespace != "payments" || command != nil || root.notice != "" {
		t.Fatalf("explicit namespace changed: namespace=%q command=%v notice=%q", root.namespace, command != nil, root.notice)
	}
}

func TestRootNonPermissionNamespaceFailureStaysAnError(t *testing.T) {
	model := freshRoot(t)
	updated, _ := model.Update(cluster.NamespacesMsg{Err: errStub("namespace list unavailable")})
	root := updated.(RootModel)
	if root.err == nil || root.namespaceListDenied || root.notice != "" {
		t.Fatalf("plain failure state = err:%v denied:%v notice:%q", root.err, root.namespaceListDenied, root.notice)
	}
}

func TestRootDeniedPickerAcceptsTypedNamespace(t *testing.T) {
	model := freshRoot(t)
	model.namespaceListDenied = true
	command := model.openNamespacePicker()
	if command == nil || !model.showNSPicker {
		t.Fatalf("denied picker open = command:%v visible:%v", command != nil, model.showNSPicker)
	}
	view := stripAnsiForTest(model.renderNSPicker(model.height))
	if !strings.Contains(view, namespaceListDeniedNotice) || !strings.Contains(view, namespaceTypePrompt) {
		t.Fatalf("denied picker view = %q, want typed namespace prompt", view)
	}

	var current tea.Model = model
	for _, letter := range "team-a" {
		current, _ = current.(RootModel).handleNSPicker(string(letter), tea.KeyPressMsg{Code: letter, Text: string(letter)})
	}
	current, command = current.(RootModel).handleNSPicker("enter", tea.KeyPressMsg{Code: tea.KeyEnter})
	root := current.(RootModel)
	if root.namespace != "team-a" || root.showNSPicker || command == nil {
		t.Fatalf("typed namespace result = namespace:%q visible:%v command:%v", root.namespace, root.showNSPicker, command != nil)
	}
}

func TestRootDeniedPickerIgnoresBlankAndEscape(t *testing.T) {
	model := freshRoot(t)
	model.namespaceListDenied = true
	model.openNamespacePicker()
	current, command := model.handleNSPicker("enter", tea.KeyPressMsg{Code: tea.KeyEnter})
	root := current.(RootModel)
	if command != nil || !root.showNSPicker || root.namespace != "default" {
		t.Fatalf("blank submit changed state: command=%v visible=%v namespace=%q", command != nil, root.showNSPicker, root.namespace)
	}
	current, _ = root.handleNSPicker("esc", tea.KeyPressMsg{Code: tea.KeyEscape})
	if current.(RootModel).showNSPicker {
		t.Fatal("esc must close the typed namespace picker")
	}
}

func TestRootDeniedPickerIgnoresMouse(t *testing.T) {
	model := freshRoot(t)
	model.namespaceListDenied = true
	model.openNamespacePicker()
	current, command := model.handleNSPickerMouse(tea.MouseClickMsg{Button: tea.MouseLeft, X: 10, Y: 10})
	root := current.(RootModel)
	if command != nil || !root.showNSPicker || root.namespace != "default" {
		t.Fatalf("mouse on typed picker changed state: command=%v visible=%v namespace=%q", command != nil, root.showNSPicker, root.namespace)
	}
}

func TestRootDeniedFailuresRenderAsNoticeNotError(t *testing.T) {
	model := freshRoot(t)
	denied := &kube.Error{Operation: kube.OperationList, Subject: kube.SubjectPods, Code: failure.CodePermissionDenied}
	model.setError(denied)
	if model.err != nil || model.notice != "Not permitted: list pods" {
		t.Fatalf("denied setError state = err:%v notice:%q", model.err, model.notice)
	}
	footer := stripAnsiForTest(model.renderRootFooter())
	if !strings.Contains(footer, "Not permitted: list pods") || strings.Contains(footer, "ERROR") {
		t.Fatalf("footer = %q, want calm notice without error banner", footer)
	}
	model.dismissFooterMessages()
	if model.notice != "" || strings.Contains(stripAnsiForTest(model.renderRootFooter()), "Not permitted") {
		t.Fatal("dismiss must clear the notice")
	}
}

func TestRootLocalPermissionFailuresStayErrors(t *testing.T) {
	model := freshRoot(t)
	sessionFailure := &session.SessionError{Operation: session.OperationWriteTemporary, Path: "/tmp/state", Err: fs.ErrPermission}
	model.setError(sessionFailure)
	if model.err == nil || model.notice != "" {
		t.Fatalf("session permission failure = err:%v notice:%q, want error banner", model.err, model.notice)
	}
	if footer := stripAnsiForTest(model.renderRootFooter()); !strings.Contains(footer, "ERROR") || !strings.Contains(footer, "permission denied") {
		t.Fatalf("footer = %q, want the diagnostic error banner", footer)
	}
}

func TestRootDenialArrivingOnOpenPickerFocusesTypedInput(t *testing.T) {
	model := freshRoot(t)
	command := model.openNamespacePicker()
	if command == nil || !model.nsLoading {
		t.Fatalf("picker open before namespaces = command:%v loading:%v", command != nil, model.nsLoading)
	}
	updated, _ := model.Update(cluster.NamespacesMsg{Err: namespaceListDeniedError()})
	root := updated.(RootModel)
	if !root.showNSPicker || !root.namespaceListDenied || root.nsLoading {
		t.Fatalf("picker after denial = visible:%v denied:%v loading:%v", root.showNSPicker, root.namespaceListDenied, root.nsLoading)
	}
	var current tea.Model = root
	for _, letter := range "ops" {
		current, _ = current.(RootModel).handleNSPicker(string(letter), tea.KeyPressMsg{Code: letter, Text: string(letter)})
	}
	current, command = current.(RootModel).handleNSPicker("enter", tea.KeyPressMsg{Code: tea.KeyEnter})
	if namespace := current.(RootModel).namespace; namespace != "ops" || command == nil {
		t.Fatalf("typed namespace after late denial = %q command:%v, want ops", namespace, command != nil)
	}
}

func TestRootTypedNamespaceClearsFallbackNotice(t *testing.T) {
	model := freshRoot(t)
	model.namespaceListDenied = true
	model.setNotice("Cluster-wide access is not permitted; showing namespace default")
	model.openNamespacePicker()
	var current tea.Model = model
	for _, letter := range "team-b" {
		current, _ = current.(RootModel).handleNSPicker(string(letter), tea.KeyPressMsg{Code: letter, Text: string(letter)})
	}
	current, _ = current.(RootModel).handleNSPicker("enter", tea.KeyPressMsg{Code: tea.KeyEnter})
	if root := current.(RootModel); root.notice != "" || root.namespace != "team-b" {
		t.Fatalf("after typed switch = notice:%q namespace:%q, want cleared notice", root.notice, root.namespace)
	}
}

func TestRootContextSwitchAdoptsContextNamespace(t *testing.T) {
	model := freshRoot(t)
	model.namespaceListDenied = true
	model.contexts = []cluster.KubeContext{
		{Name: "prod", Namespace: "checkout", Current: true},
		{Name: "lab"},
	}
	updated, command := model.Update(cluster.ContextSwitchedMsg{Name: "prod"})
	root := updated.(RootModel)
	if root.namespace != "checkout" || root.namespaceListDenied || command == nil {
		t.Fatalf("switch to prod = namespace:%q denied:%v command:%v", root.namespace, root.namespaceListDenied, command != nil)
	}
	updated, _ = root.Update(cluster.ContextSwitchedMsg{Name: "lab"})
	if namespace := updated.(RootModel).namespace; namespace != "" {
		t.Fatalf("switch to lab = namespace:%q, want all namespaces", namespace)
	}
}
