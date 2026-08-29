package ui

import (
	"context"

	tea "charm.land/bubbletea/v2"

	"github.com/HediAbed/opsmate/internal/cluster"
	"github.com/HediAbed/opsmate/internal/kube"
)

func (m RootModel) fetchNamespaces() tea.Cmd {
	parent := m.runtime.Context
	clusterContext := m.runtime.ClusterContext
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(parent, clusterReadTimeout)
		defer cancel()
		namespaces, err := clusterContext.Namespaces(ctx)
		return cluster.NamespacesMsg{Namespaces: namespaces, Err: err}
	}
}

func (m RootModel) fetchCurrentContext() tea.Cmd {
	parent := m.runtime.Context
	clusterContext := m.runtime.ClusterContext
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(parent, clusterReadTimeout)
		defer cancel()
		name, err := clusterContext.CurrentContext(ctx)
		return cluster.CurrentContextMsg{Name: name, Err: err}
	}
}

func (m RootModel) fetchContexts() tea.Cmd {
	parent := m.runtime.Context
	clusterContext := m.runtime.ClusterContext
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(parent, clusterReadTimeout)
		defer cancel()
		contexts, err := clusterContext.Contexts(ctx)
		return cluster.ContextsMsg{Contexts: contextMessages(contexts), Err: err}
	}
}

func (m RootModel) switchContext(name string) tea.Cmd {
	parent := m.runtime.Context
	clusterContext := m.runtime.ClusterContext
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(parent, clusterActionTimeout)
		defer cancel()
		err := clusterContext.Connect(ctx, name)
		return cluster.ContextSwitchedMsg{Name: name, Err: err}
	}
}

func contextMessages(contexts []kube.ContextInfo) []cluster.KubeContext {
	messages := make([]cluster.KubeContext, 0, len(contexts))
	for _, clusterContext := range contexts {
		messages = append(messages, cluster.KubeContext{
			Name:      clusterContext.Name,
			Cluster:   clusterContext.Cluster,
			Namespace: clusterContext.Namespace,
			Current:   clusterContext.Current,
		})
	}
	return messages
}
