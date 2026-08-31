package app

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"k8s.io/client-go/rest"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"

	"github.com/HediAbed/opsmate/internal/kube"
)

type staticConfigSource struct {
	raw     clientcmdapi.Config
	rest    *rest.Config
	rawErr  error
	restErr error
}

func (s staticConfigSource) RawConfig() (clientcmdapi.Config, error) {
	return s.raw, s.rawErr
}

func (s staticConfigSource) RESTConfig(string) (*rest.Config, error) {
	return s.rest, s.restErr
}

func (staticConfigSource) SetCurrentContext(string) error {
	return nil
}

type clientBuilderFunc func(*rest.Config) (*kube.Clients, error)

func (build clientBuilderFunc) Build(config *rest.Config) (*kube.Clients, error) {
	return build(config)
}

func TestConnectClusterWithDependencies(t *testing.T) {
	server := newKubernetesVersionServer(t)
	validSource := staticConfigSource{
		raw: clientcmdapi.Config{
			CurrentContext: "primary",
			Contexts:       map[string]*clientcmdapi.Context{"primary": {}},
		},
		rest: &rest.Config{Host: server.URL},
	}
	t.Run("manager construction", func(t *testing.T) {
		cluster, err := connectClusterWith(context.Background(), nil, kube.DefaultClientBuilder{})
		if cluster != nil || !errors.Is(err, kube.ErrConfigSourceRequired) {
			t.Fatalf("connectClusterWith() = (%v, %v), want config-source error", cluster, err)
		}
	})
	t.Run("connection", func(t *testing.T) {
		sentinel := errors.New("client failure")
		builder := clientBuilderFunc(func(*rest.Config) (*kube.Clients, error) {
			return nil, sentinel
		})
		cluster, err := connectClusterWith(context.Background(), validSource, builder)
		if cluster != nil || !errors.Is(err, sentinel) {
			t.Fatalf("connectClusterWith() = (%v, %v), want sentinel", cluster, err)
		}
	})
	t.Run("success", func(t *testing.T) {
		cluster, err := connectClusterWith(context.Background(), validSource, kube.DefaultClientBuilder{})
		if err != nil || cluster == nil {
			t.Fatalf("connectClusterWith() = (%v, %v)", cluster, err)
		}
		name, currentErr := cluster.CurrentContext(context.Background())
		if currentErr != nil || name != "primary" {
			t.Fatalf("CurrentContext() = (%q, %v)", name, currentErr)
		}
	})
}

func TestConnectClusterUsesDefaultKubeconfigLoading(t *testing.T) {
	server := newKubernetesVersionServer(t)
	kubeconfigPath := filepath.Join(t.TempDir(), "config")
	content := []byte(`apiVersion: v1
kind: Config
current-context: primary
clusters:
- name: main
  cluster:
    server: ` + server.URL + `
contexts:
- name: primary
  context:
    cluster: main
    user: operator
users:
- name: operator
  user:
    token: test-token
`)
	if err := os.WriteFile(kubeconfigPath, content, 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	t.Setenv("KUBECONFIG", kubeconfigPath)
	cluster, err := connectCluster(context.Background())
	if err != nil || cluster == nil {
		t.Fatalf("connectCluster() = (%v, %v)", cluster, err)
	}
}

func newKubernetesVersionServer(t *testing.T) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/version" {
			t.Errorf("request path = %q, want /version", request.URL.Path)
		}
		response.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)
	return server
}
