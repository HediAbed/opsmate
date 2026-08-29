package kube

import (
	"context"
	"io"

	corev1 "k8s.io/api/core/v1"
	apiextensionsclient "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/discovery/cached/memory"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/metadata"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/restmapper"
	metricsclient "k8s.io/metrics/pkg/client/clientset/versioned"
)

const clientUserAgent = "opsmate"

type Clients struct {
	config          *rest.Config
	kubernetes      kubernetes.Interface
	dynamic         dynamic.Interface
	metrics         metricsclient.Interface
	extensions      apiextensionsclient.Interface
	metadata        metadata.Interface
	discovery       discovery.CachedDiscoveryInterface
	mapper          meta.RESTMapper
	checkConnection func(context.Context) error
	openPodLogs     func(context.Context, string, string, *corev1.PodLogOptions) (io.ReadCloser, error)
}

type clientConstructors struct {
	kubernetes func(*rest.Config) (kubernetes.Interface, error)
	dynamic    func(*rest.Config) (dynamic.Interface, error)
	metrics    func(*rest.Config) (metricsclient.Interface, error)
	extensions func(*rest.Config) (apiextensionsclient.Interface, error)
	metadata   func(*rest.Config) (metadata.Interface, error)
	discovery  func(*rest.Config) (discovery.DiscoveryInterface, error)
}

var defaultClientConstructors = clientConstructors{
	kubernetes: func(config *rest.Config) (kubernetes.Interface, error) {
		return kubernetes.NewForConfig(config)
	},
	dynamic: func(config *rest.Config) (dynamic.Interface, error) {
		return dynamic.NewForConfig(config)
	},
	metrics: func(config *rest.Config) (metricsclient.Interface, error) {
		return metricsclient.NewForConfig(config)
	},
	extensions: func(config *rest.Config) (apiextensionsclient.Interface, error) {
		return apiextensionsclient.NewForConfig(config)
	},
	metadata: func(config *rest.Config) (metadata.Interface, error) {
		return metadata.NewForConfig(config)
	},
	discovery: func(config *rest.Config) (discovery.DiscoveryInterface, error) {
		return discovery.NewDiscoveryClientForConfig(config)
	},
}

func NewClients(config *rest.Config) (*Clients, error) {
	return buildClients(config, defaultClientConstructors)
}

func buildClients(config *rest.Config, constructors clientConstructors) (*Clients, error) {
	if config == nil {
		return nil, newError(OperationCreate, SubjectClients, "", ErrRESTConfigRequired)
	}
	clientConfig := rest.CopyConfig(config)
	clientConfig.UserAgent = clientUserAgent

	typedClient, err := constructors.kubernetes(clientConfig)
	if err != nil {
		return nil, clientCreationError(SubjectTypedClient, err)
	}
	dynamicClient, err := constructors.dynamic(clientConfig)
	if err != nil {
		return nil, clientCreationError(SubjectDynamicClient, err)
	}
	metricsClient, err := constructors.metrics(clientConfig)
	if err != nil {
		return nil, clientCreationError(SubjectMetricsClient, err)
	}
	extensionsClient, err := constructors.extensions(clientConfig)
	if err != nil {
		return nil, clientCreationError(SubjectExtensionsClient, err)
	}
	metadataClient, err := constructors.metadata(clientConfig)
	if err != nil {
		return nil, clientCreationError(SubjectMetadataClient, err)
	}
	discoveryClient, err := constructors.discovery(clientConfig)
	if err != nil {
		return nil, clientCreationError(SubjectDiscoveryClient, err)
	}
	cachedDiscovery := memory.NewMemCacheClient(discoveryClient)
	return &Clients{
		config:          clientConfig,
		kubernetes:      typedClient,
		dynamic:         dynamicClient,
		metrics:         metricsClient,
		extensions:      extensionsClient,
		metadata:        metadataClient,
		discovery:       cachedDiscovery,
		mapper:          restmapper.NewDeferredDiscoveryRESTMapper(cachedDiscovery),
		checkConnection: newConnectionCheck(typedClient),
		openPodLogs:     newPodLogOpener(typedClient),
	}, nil
}

func newPodLogOpener(client kubernetes.Interface) func(context.Context, string, string, *corev1.PodLogOptions) (io.ReadCloser, error) {
	if client == nil {
		return nil
	}
	return func(ctx context.Context, namespace, pod string, options *corev1.PodLogOptions) (io.ReadCloser, error) {
		return client.CoreV1().Pods(namespace).GetLogs(pod, options).Stream(ctx)
	}
}

func newConnectionCheck(client kubernetes.Interface) func(context.Context) error {
	if client == nil {
		return unavailableConnectionCheck
	}
	discoveryClient := client.Discovery()
	if discoveryClient == nil {
		return unavailableConnectionCheck
	}
	restClient := discoveryClient.RESTClient()
	if restClient == nil {
		return unavailableConnectionCheck
	}
	return func(ctx context.Context) error {
		return restClient.Get().AbsPath("/version").Do(ctx).Error()
	}
}

func unavailableConnectionCheck(context.Context) error {
	return ErrConnectionCheckUnavailable
}

func clientCreationError(subject Subject, err error) error {
	return newError(OperationCreate, subject, "", err)
}

func (c *Clients) RESTConfig() *rest.Config {
	if c == nil || c.config == nil {
		return nil
	}
	return rest.CopyConfig(c.config)
}

func (c *Clients) Kubernetes() kubernetes.Interface {
	if c == nil {
		return nil
	}
	return c.kubernetes
}

func (c *Clients) Dynamic() dynamic.Interface {
	if c == nil {
		return nil
	}
	return c.dynamic
}

func (c *Clients) Metrics() metricsclient.Interface {
	if c == nil {
		return nil
	}
	return c.metrics
}

func (c *Clients) Extensions() apiextensionsclient.Interface {
	if c == nil {
		return nil
	}
	return c.extensions
}

func (c *Clients) Metadata() metadata.Interface {
	if c == nil {
		return nil
	}
	return c.metadata
}

func (c *Clients) Discovery() discovery.CachedDiscoveryInterface {
	if c == nil {
		return nil
	}
	return c.discovery
}

func (c *Clients) RESTMapper() meta.RESTMapper {
	if c == nil {
		return nil
	}
	return c.mapper
}

func (c *Clients) CheckConnection(ctx context.Context) error {
	if c == nil || c.checkConnection == nil {
		return ErrConnectionCheckUnavailable
	}
	return c.checkConnection(ctx)
}

func (c *Clients) OpenPodLogs(ctx context.Context, namespace, pod string, options *corev1.PodLogOptions) (io.ReadCloser, error) {
	if c == nil || c.openPodLogs == nil {
		return nil, ErrPodLogReaderUnavailable
	}
	return c.openPodLogs(ctx, namespace, pod, options)
}

func coreRESTClient(clients *Clients) rest.Interface {
	if clients == nil || clients.Kubernetes() == nil {
		return nil
	}
	client := clients.Kubernetes().CoreV1().RESTClient()
	if concrete, ok := client.(*rest.RESTClient); ok && concrete == nil {
		return nil
	}
	return client
}
