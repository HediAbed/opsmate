package ui

import (
	"context"

	tea "charm.land/bubbletea/v2"

	"github.com/HediAbed/opsmate/internal/kube"
)

type helmCommands interface {
	ListReleases(string) tea.Cmd
	GetValues(kube.HelmReleaseReference) tea.Cmd
}

type nativeHelmCommands struct {
	parent context.Context
	reader kube.HelmReader
}

type helmReleasesMsg struct {
	Releases []kube.HelmRelease
	Err      error
}

type helmValuesMsg struct {
	Release   string
	Namespace string
	Values    string
	Err       error
}

func newNativeHelmCommands(parent context.Context, reader kube.HelmReader) nativeHelmCommands {
	return nativeHelmCommands{parent: parent, reader: reader}
}

func (c nativeHelmCommands) ListReleases(namespace string) tea.Cmd {
	return func() tea.Msg {
		releases, err := c.reader.ListHelmReleases(c.parent, namespace)
		return helmReleasesMsg{Releases: releases, Err: err}
	}
}

func (c nativeHelmCommands) GetValues(reference kube.HelmReleaseReference) tea.Cmd {
	return func() tea.Msg {
		values, err := c.reader.HelmReleaseValues(c.parent, reference)
		return helmValuesMsg{
			Release:   reference.Name,
			Namespace: reference.Namespace,
			Values:    values,
			Err:       err,
		}
	}
}
