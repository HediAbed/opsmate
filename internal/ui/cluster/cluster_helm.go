package cluster

import (
	"context"

	tea "charm.land/bubbletea/v2"

	"github.com/HediAbed/opsmate/internal/kube"
)

type HelmCommands interface {
	ListReleases(string) tea.Cmd
	GetValues(kube.HelmReleaseReference) tea.Cmd
}

type HelmAdapter struct {
	parent context.Context
	reader kube.HelmReader
}

type HelmReleasesMsg struct {
	Releases []kube.HelmRelease
	Err      error
}

type HelmValuesMsg struct {
	Release   string
	Namespace string
	Values    string
	Err       error
}

func NewHelmCommands(parent context.Context, reader kube.HelmReader) HelmAdapter {
	return HelmAdapter{parent: parent, reader: reader}
}

func (c HelmAdapter) ListReleases(namespace string) tea.Cmd {
	return func() tea.Msg {
		releases, err := c.reader.ListHelmReleases(c.parent, namespace)
		return HelmReleasesMsg{Releases: releases, Err: err}
	}
}

func (c HelmAdapter) GetValues(reference kube.HelmReleaseReference) tea.Cmd {
	return func() tea.Msg {
		values, err := c.reader.HelmReleaseValues(c.parent, reference)
		return HelmValuesMsg{
			Release:   reference.Name,
			Namespace: reference.Namespace,
			Values:    values,
			Err:       err,
		}
	}
}
