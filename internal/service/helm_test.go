package service

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestHelmReleaseFromRaw_PopulatesEveryColumn(t *testing.T) {
	got, err := helmReleaseFromRaw(rawHelmRelease{
		Name:       "ingress-nginx",
		Namespace:  "ingress",
		Revision:   "5",
		Updated:    "2026-04-12 10:00:00.0 +0000 UTC",
		Status:     "deployed",
		Chart:      "ingress-nginx-4.10.0",
		AppVersion: "1.10.0",
	})
	if err != nil {
		t.Fatalf("project release: %v", err)
	}
	want := HelmRelease{
		Name: "ingress-nginx", Namespace: "ingress", Revision: 5,
		Status: "deployed", Chart: "ingress-nginx-4.10.0", AppVersion: "1.10.0",
		Updated: "2026-04-12 10:00:00.0 +0000 UTC",
	}
	if got != want {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestHelmReleaseFromRawRejectsInvalidRevision(t *testing.T) {
	for _, revision := range []string{"garbage", "0"} {
		_, err := helmReleaseFromRaw(rawHelmRelease{Name: "x", Revision: revision})
		if !errors.Is(err, ErrInvalidHelmRevision) {
			t.Errorf("revision %q error = %v", revision, err)
		}
	}
}

func TestHelmNamespaceArgs_EmptyUsesAllNamespaces(t *testing.T) {
	if got := helmNamespaceArgs(""); !slices.Equal(got, []string{"-A"}) {
		t.Errorf("empty ns should use -A; got %v", got)
	}
}

func TestHelmNamespaceArgs_NonEmptyUsesDashN(t *testing.T) {
	if got := helmNamespaceArgs("kube-system"); !slices.Equal(got, []string{"-n", "kube-system"}) {
		t.Errorf("non-empty ns should use -n <ns>; got %v", got)
	}
}

func TestListHelmReleases_HappyPath(t *testing.T) {
	withFakeHelm(t, `printf '[{"name":"r1","namespace":"ns","revision":"1","updated":"now","status":"deployed","chart":"c-1.0.0","app_version":"1.0.0"}]'`)

	msg, ok := ListHelmReleases("ns")().(HelmReleasesMsg)
	if !ok {
		t.Fatalf("expected HelmReleasesMsg, got %T", ListHelmReleases("ns")())
	}
	if msg.Err != nil {
		t.Fatalf("unexpected err: %v", msg.Err)
	}
	if len(msg.Releases) != 1 || msg.Releases[0].Status != "deployed" {
		t.Errorf("payload wrong: %+v", msg.Releases)
	}
}

func TestListHelmReleases_BinaryMissingAbsolutePath(t *testing.T) {
	prev := helmBinary
	helmBinary = "/nonexistent/helm-not-installed-here"
	t.Cleanup(func() { helmBinary = prev })

	msg, ok := ListHelmReleases("ns")().(HelmReleasesMsg)
	if !ok {
		t.Fatalf("expected HelmReleasesMsg, got %T", ListHelmReleases("ns")())
	}
	if !errors.Is(msg.Err, ErrHelmBinaryMissing) {
		t.Errorf("absolute path that doesn't exist should produce ErrHelmBinaryMissing; got %v", msg.Err)
	}
}

func TestListHelmReleases_BinaryMissingBarePath(t *testing.T) {
	prev := helmBinary
	helmBinary = "helm-definitely-not-installed-anywhere-xyz123"
	t.Cleanup(func() { helmBinary = prev })

	msg, ok := ListHelmReleases("ns")().(HelmReleasesMsg)
	if !ok {
		t.Fatalf("expected HelmReleasesMsg, got %T", ListHelmReleases("ns")())
	}
	if !errors.Is(msg.Err, ErrHelmBinaryMissing) {
		t.Errorf("bare name that's not on PATH should produce ErrHelmBinaryMissing; got %v", msg.Err)
	}
}

func TestListHelmReleases_HelmFailureKeepsStderrInError(t *testing.T) {
	withFakeHelm(t, `printf 'release ingress not found' 1>&2; exit 1`)

	msg, _ := ListHelmReleases("ns")().(HelmReleasesMsg)
	if msg.Err == nil {
		t.Fatal("expected non-nil error")
	}
	if !strings.Contains(msg.Err.Error(), "release ingress not found") {
		t.Errorf("error should preserve stderr; got %v", msg.Err)
	}
	var helmErr *HelmError
	if !errors.As(msg.Err, &helmErr) {
		t.Errorf("error should wrap *HelmError; got %T", msg.Err)
	}
}

func TestListHelmReleases_MalformedJSONErrorsClearly(t *testing.T) {
	withFakeHelm(t, `printf 'not json'`)
	msg, _ := ListHelmReleases("ns")().(HelmReleasesMsg)
	if msg.Err == nil || !strings.Contains(msg.Err.Error(), "parse helm list output") {
		t.Errorf("malformed JSON should surface a parse-helm error; got %v", msg.Err)
	}
}

func TestListHelmReleasesRejectsMalformedRevision(t *testing.T) {
	withFakeHelm(t, `printf '[{"name":"broken","revision":"unknown"}]'`)

	message := ListHelmReleases("ns")().(HelmReleasesMsg)

	if !errors.Is(message.Err, ErrInvalidHelmRevision) {
		t.Fatalf("error = %v, want ErrInvalidHelmRevision", message.Err)
	}
}

func TestListHelmReleases_ArgvUsesNamespaceAndJSONOutput(t *testing.T) {
	argvPath := withFakeHelmCapturing(t, `printf '[]'`)
	if _, ok := ListHelmReleases("ingress")().(HelmReleasesMsg); !ok {
		t.Fatal("expected HelmReleasesMsg")
	}
	got := readArgv(t, argvPath)
	for _, want := range []string{"list", "-o", "json", "-n", "ingress"} {
		if !strings.Contains(got, want) {
			t.Errorf("argv missing %q; full argv: %q", want, got)
		}
	}
	if strings.Contains(got, "-A") {
		t.Errorf("non-empty namespace must not pass -A; got %q", got)
	}
}

func TestHelmBinaryAvailableFindsBareExecutable(t *testing.T) {
	directory := t.TempDir()
	writeTestExecutable(t, filepath.Join(directory, "helm-test"), "#!/bin/sh\nexit 0\n")
	t.Setenv("PATH", directory+string(os.PathListSeparator)+os.Getenv("PATH"))
	previous := helmBinary
	helmBinary = "helm-test"
	t.Cleanup(func() { helmBinary = previous })

	if err := helmBinaryAvailable(); err != nil {
		t.Fatalf("resolve executable: %v", err)
	}
}

func TestHelmErrorFormatsEveryFailureShape(t *testing.T) {
	sentinel := errors.New("failed")
	if got := (&HelmError{Err: sentinel}).Error(); got != "helm: failed" {
		t.Fatalf("error = %q", got)
	}
	if got := (&HelmError{}).Error(); got != "helm: unknown error" {
		t.Fatalf("unknown error = %q", got)
	}
	if !errors.Is(&HelmError{Err: sentinel}, sentinel) {
		t.Fatal("helm error did not unwrap its cause")
	}
	if got := (&HelmError{Operation: "parse list", Err: sentinel}).Error(); got != "helm parse list: failed" {
		t.Fatalf("operation error = %q", got)
	}
}

func TestGetHelmValuesReturnsContentAndArguments(t *testing.T) {
	argumentsPath := withFakeHelmCapturing(t, `printf 'replicas: 2'`)

	message := GetHelmValues("web", "operations")().(HelmValuesMsg)

	if message.Err != nil || message.Values != "replicas: 2" {
		t.Fatalf("message = %#v", message)
	}
	if message.Release != "web" || message.Namespace != "operations" {
		t.Fatalf("identity = %s/%s", message.Namespace, message.Release)
	}
	arguments := readArgv(t, argumentsPath)
	for _, expected := range []string{"get", "values", "web", "-o", "yaml", "-n", "operations"} {
		if !strings.Contains(arguments, expected) {
			t.Errorf("arguments %q missing %q", arguments, expected)
		}
	}
}

func TestGetHelmValuesOmitsNamespaceAndPropagatesFailure(t *testing.T) {
	argumentsPath := withFakeHelmCapturing(t, `printf 'failed' >&2; exit 1`)

	message := GetHelmValues("web", "")().(HelmValuesMsg)

	if message.Err == nil || message.Release != "web" || message.Namespace != "" {
		t.Fatalf("message = %#v, want release identity and failure", message)
	}
	if arguments := readArgv(t, argumentsPath); strings.Contains(arguments, " -n ") {
		t.Fatalf("all-namespace request included namespace flag: %q", arguments)
	}
}

func withFakeHelm(t *testing.T, script string) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "helm")
	writeTestExecutable(t, path, "#!/bin/sh\n"+script+"\n")
	prev := helmBinary
	helmBinary = path
	t.Cleanup(func() { helmBinary = prev })
}

func withFakeHelmCapturing(t *testing.T, script string) string {
	t.Helper()
	dir := t.TempDir()
	argvPath := filepath.Join(dir, "argv.txt")
	binPath := filepath.Join(dir, "helm")
	contents := "#!/bin/sh\necho \"$@\" > " + argvPath + "\n" + script + "\n"
	writeTestExecutable(t, binPath, contents)
	prev := helmBinary
	helmBinary = binPath
	t.Cleanup(func() { helmBinary = prev })
	return argvPath
}
