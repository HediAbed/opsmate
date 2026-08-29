package kube

import (
	"os"
	"path/filepath"
	"testing"

	"k8s.io/client-go/tools/clientcmd"
)

func deferredConfigSourceWithTwoContexts(t *testing.T) *DeferredConfigSource {
	t.Helper()
	kubeconfigPath := filepath.Join(t.TempDir(), "config")
	kubeconfig := []byte(`apiVersion: v1
kind: Config
current-context: first
clusters:
- name: first-cluster
  cluster:
    server: https://first.invalid
- name: second-cluster
  cluster:
    server: https://second.invalid
contexts:
- name: first
  context:
    cluster: first-cluster
    user: first-user
- name: second
  context:
    cluster: second-cluster
    user: second-user
users:
- name: first-user
  user:
    token: first-token
- name: second-user
  user:
    token: second-token
`)
	if err := os.WriteFile(kubeconfigPath, kubeconfig, 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	return deferredConfigSourceWithRules(&clientcmd.ClientConfigLoadingRules{ExplicitPath: kubeconfigPath})
}

func TestDeferredConfigSourceLoadsRawAndOverriddenConfigs(t *testing.T) {
	source := deferredConfigSourceWithTwoContexts(t)
	rawConfig, err := source.RawConfig()
	if err != nil {
		t.Fatalf("RawConfig() error = %v", err)
	}
	if rawConfig.CurrentContext != "first" || len(rawConfig.Contexts) != 2 {
		t.Fatalf("RawConfig() = %+v, want first and two contexts", rawConfig)
	}
	config, err := source.RESTConfig("second")
	if err != nil {
		t.Fatalf("RESTConfig(second) error = %v", err)
	}
	if config.Host != "https://second.invalid" || config.BearerToken != "second-token" {
		t.Fatalf("RESTConfig(second) = %+v", config)
	}
}

func TestDeferredConfigSourcePersistsCurrentContextOverride(t *testing.T) {
	source := deferredConfigSourceWithTwoContexts(t)
	if err := source.SetCurrentContext("second"); err != nil {
		t.Fatalf("SetCurrentContext(second) error = %v", err)
	}
	updatedConfig, err := source.RawConfig()
	if err != nil {
		t.Fatalf("RawConfig() after update error = %v", err)
	}
	if updatedConfig.CurrentContext != "second" {
		t.Fatalf("CurrentContext = %q, want second", updatedConfig.CurrentContext)
	}
}

func TestDeferredConfigSourceReportsLoadFailureWhenSettingContext(t *testing.T) {
	missingPath := filepath.Join(t.TempDir(), "missing")
	source := deferredConfigSourceWithRules(&clientcmd.ClientConfigLoadingRules{ExplicitPath: missingPath})
	if err := source.SetCurrentContext("second"); err == nil {
		t.Fatal("SetCurrentContext() error = nil, want load failure")
	}
}

func TestDefaultConstructors(t *testing.T) {
	source := NewDeferredConfigSource()
	if source == nil || source.loadingRules == nil {
		t.Fatal("NewDeferredConfigSource() returned no loading rules")
	}
	builder := DefaultClientBuilder{}
	if clients, err := builder.Build(nil); clients != nil || err == nil {
		t.Fatalf("Build(nil) = (%v, %v), want error", clients, err)
	}
}
