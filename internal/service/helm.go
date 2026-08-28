package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
)

// HelmRelease contains the fields displayed from `helm list`.
type HelmRelease struct {
	Name       string
	Namespace  string
	Revision   int
	Status     string
	Chart      string
	AppVersion string
	Updated    string
}

// HelmReleasesMsg carries a release list or its failure.
type HelmReleasesMsg struct {
	Releases []HelmRelease
	Err      error
}

var (
	// ErrHelmBinaryMissing indicates that the helm executable could not be resolved.
	ErrHelmBinaryMissing = errors.New("helm CLI not found on PATH")
	// ErrInvalidHelmRevision indicates malformed release metadata.
	ErrInvalidHelmRevision = errors.New("invalid helm revision")
)

const helmReadTimeout = 30 * time.Second

var helmBinary = "helm"

type rawHelmRelease struct {
	Name       string `json:"name"`
	Namespace  string `json:"namespace"`
	Revision   string `json:"revision"`
	Updated    string `json:"updated"`
	Status     string `json:"status"`
	Chart      string `json:"chart"`
	AppVersion string `json:"app_version"`
}

// ListHelmReleases fetches releases in one namespace or across the cluster.
func ListHelmReleases(namespace string) tea.Cmd {
	return func() tea.Msg {
		args := append([]string{"list", "-o", "json"}, helmNamespaceArgs(namespace)...)
		out, err := runHelm(helmReadTimeout, args...)
		if err != nil {
			return HelmReleasesMsg{Err: err}
		}
		var raw []rawHelmRelease
		if err := json.Unmarshal(out, &raw); err != nil {
			return HelmReleasesMsg{Err: &HelmError{Operation: "parse helm list output", Err: err}}
		}
		releases := make([]HelmRelease, 0, len(raw))
		for _, rawRelease := range raw {
			release, err := helmReleaseFromRaw(rawRelease)
			if err != nil {
				return HelmReleasesMsg{Err: err}
			}
			releases = append(releases, release)
		}
		return HelmReleasesMsg{Releases: releases}
	}
}

func helmReleaseFromRaw(raw rawHelmRelease) (HelmRelease, error) {
	revision, err := strconv.Atoi(raw.Revision)
	if err != nil || revision < 1 {
		return HelmRelease{}, &HelmError{
			Operation: "parse release revision",
			Err:       fmt.Errorf("%w %q for release %q", ErrInvalidHelmRevision, raw.Revision, raw.Name),
		}
	}
	return HelmRelease{
		Name:       raw.Name,
		Namespace:  raw.Namespace,
		Revision:   revision,
		Status:     raw.Status,
		Chart:      raw.Chart,
		AppVersion: raw.AppVersion,
		Updated:    raw.Updated,
	}, nil
}

func helmNamespaceArgs(namespace string) []string {
	if namespace == "" {
		return []string{"-A"}
	}
	return []string{"-n", namespace}
}

func runHelm(timeout time.Duration, args ...string) ([]byte, error) {
	if err := helmBinaryAvailable(); err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	var stdout, stderr bytes.Buffer
	cmd := newExternalCommandContext(ctx, helmBinary, args...)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, &HelmError{Stderr: strings.TrimSpace(stderr.String()), Err: err}
	}
	return stdout.Bytes(), nil
}

func helmBinaryAvailable() error {
	if _, err := exec.LookPath(helmBinary); err != nil {
		return ErrHelmBinaryMissing
	}
	return nil
}

// HelmError adds operation and stderr context to a helm failure.
type HelmError struct {
	Operation string
	Stderr    string
	Err       error
}

func (e *HelmError) Error() string {
	prefix := "helm"
	if e.Operation != "" {
		prefix += " " + e.Operation
	}
	if e.Stderr != "" {
		return prefix + ": " + e.Stderr
	}
	if e.Err != nil {
		return prefix + ": " + e.Err.Error()
	}
	return prefix + ": unknown error"
}

func (e *HelmError) Unwrap() error { return e.Err }

// HelmValuesMsg carries configured values for one release.
type HelmValuesMsg struct {
	Release   string
	Namespace string
	Values    string
	Err       error
}

// GetHelmValues fetches user-supplied values without merged chart defaults.
func GetHelmValues(release, namespace string) tea.Cmd {
	return func() tea.Msg {
		args := []string{"get", "values", release, "-o", "yaml"}
		if namespace != "" {
			args = append(args, "-n", namespace)
		}
		out, err := runHelm(helmReadTimeout, args...)
		if err != nil {
			return HelmValuesMsg{Release: release, Namespace: namespace, Err: err}
		}
		return HelmValuesMsg{Release: release, Namespace: namespace, Values: string(out)}
	}
}
