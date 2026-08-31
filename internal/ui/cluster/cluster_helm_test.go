package cluster

import (
	"context"
	"errors"
	"testing"

	"github.com/HediAbed/opsmate/internal/kube"
)

type testHelmReader struct {
	releases []kube.HelmRelease
	values   string
	err      error
}

func (r *testHelmReader) ListHelmReleases(context.Context, string) ([]kube.HelmRelease, error) {
	return append([]kube.HelmRelease(nil), r.releases...), r.err
}

func (r *testHelmReader) HelmReleaseValues(context.Context, kube.HelmReleaseReference) (string, error) {
	return r.values, r.err
}

func TestNativeHelmCommands(t *testing.T) {
	sentinel := errors.New("helm unavailable")
	reader := &testHelmReader{
		releases: []kube.HelmRelease{{Name: "gateway", Namespace: "edge", Revision: 2}},
		values:   "replicas: 3\n",
	}
	commands := NewHelmCommands(context.Background(), reader)

	releases := commands.ListReleases("edge")().(HelmReleasesMsg)
	if releases.Err != nil || len(releases.Releases) != 1 || releases.Releases[0].Name != "gateway" {
		t.Fatalf("ListReleases() = %+v", releases)
	}
	reference := kube.HelmReleaseReference{Namespace: "edge", Name: "gateway"}
	values := commands.GetValues(reference)().(HelmValuesMsg)
	if values.Err != nil || values.Release != reference.Name || values.Namespace != reference.Namespace || values.Values != "replicas: 3\n" {
		t.Fatalf("GetValues() = %+v", values)
	}

	reader.err = sentinel
	if message := commands.ListReleases("edge")().(HelmReleasesMsg); !errors.Is(message.Err, sentinel) {
		t.Fatalf("ListReleases() error = %v", message.Err)
	}
	if message := commands.GetValues(reference)().(HelmValuesMsg); !errors.Is(message.Err, sentinel) {
		t.Fatalf("GetValues() error = %v", message.Err)
	}
}
