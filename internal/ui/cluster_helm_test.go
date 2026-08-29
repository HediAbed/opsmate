package ui

import (
	"context"
	"errors"
	"testing"

	"github.com/HediAbed/opsmate/internal/kube"
)

func TestNativeHelmCommands(t *testing.T) {
	sentinel := errors.New("helm unavailable")
	reader := &testClusterOperations{
		helmReleases: []kube.HelmRelease{{Name: "gateway", Namespace: "edge", Revision: 2}},
		helmValues:   "replicas: 3\n",
	}
	commands := newNativeHelmCommands(context.Background(), reader)

	releases := commands.ListReleases("edge")().(helmReleasesMsg)
	if releases.Err != nil || len(releases.Releases) != 1 || releases.Releases[0].Name != "gateway" {
		t.Fatalf("ListReleases() = %+v", releases)
	}
	reference := kube.HelmReleaseReference{Namespace: "edge", Name: "gateway"}
	values := commands.GetValues(reference)().(helmValuesMsg)
	if values.Err != nil || values.Release != reference.Name || values.Namespace != reference.Namespace || values.Values != "replicas: 3\n" {
		t.Fatalf("GetValues() = %+v", values)
	}

	reader.helmErr = sentinel
	if message := commands.ListReleases("edge")().(helmReleasesMsg); !errors.Is(message.Err, sentinel) {
		t.Fatalf("ListReleases() error = %v", message.Err)
	}
	if message := commands.GetValues(reference)().(helmValuesMsg); !errors.Is(message.Err, sentinel) {
		t.Fatalf("GetValues() error = %v", message.Err)
	}
}
