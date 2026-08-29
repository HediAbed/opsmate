package kube

import (
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

type DeferredConfigSource struct {
	loadingRules *clientcmd.ClientConfigLoadingRules
}

func NewDeferredConfigSource() *DeferredConfigSource {
	return &DeferredConfigSource{loadingRules: clientcmd.NewDefaultClientConfigLoadingRules()}
}

func deferredConfigSourceWithRules(loadingRules *clientcmd.ClientConfigLoadingRules) *DeferredConfigSource {
	return &DeferredConfigSource{loadingRules: loadingRules}
}

func (s *DeferredConfigSource) RawConfig() (clientcmdapi.Config, error) {
	return s.deferredClientConfig("").RawConfig()
}

func (s *DeferredConfigSource) RESTConfig(contextName string) (*rest.Config, error) {
	return s.deferredClientConfig(contextName).ClientConfig()
}

func (s *DeferredConfigSource) SetCurrentContext(contextName string) error {
	config, err := s.RawConfig()
	if err != nil {
		return err
	}
	config.CurrentContext = contextName
	return clientcmd.ModifyConfig(s.loadingRules, config, false)
}

func (s *DeferredConfigSource) deferredClientConfig(contextName string) clientcmd.ClientConfig {
	overrides := &clientcmd.ConfigOverrides{CurrentContext: contextName}
	return clientcmd.NewNonInteractiveDeferredLoadingClientConfig(s.loadingRules, overrides)
}

type DefaultClientBuilder struct{}

func (DefaultClientBuilder) Build(config *rest.Config) (*Clients, error) {
	return NewClients(config)
}
