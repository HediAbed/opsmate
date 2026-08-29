package kube

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	apiextensionsclient "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	apiextensionsfake "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset/fake"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/discovery"
	discoveryfake "k8s.io/client-go/discovery/fake"
	"k8s.io/client-go/dynamic"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	"k8s.io/client-go/kubernetes"
	kubernetesfake "k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/metadata"
	metadatafake "k8s.io/client-go/metadata/fake"
	"k8s.io/client-go/rest"
	clienttesting "k8s.io/client-go/testing"
	metricsclient "k8s.io/metrics/pkg/client/clientset/versioned"
	metricsfake "k8s.io/metrics/pkg/client/clientset/versioned/fake"
)

type discoverylessKubernetesClient struct {
	kubernetes.Interface
}

func (discoverylessKubernetesClient) Discovery() discovery.DiscoveryInterface {
	return nil
}

func TestNewClientsRejectsNilConfig(t *testing.T) {
	clients, err := NewClients(nil)
	if clients != nil || !errors.Is(err, ErrRESTConfigRequired) {
		t.Fatalf("NewClients(nil) = (%v, %v), want nil and ErrRESTConfigRequired", clients, err)
	}
}

func clientAccessorNilness(clients *Clients) map[string]bool {
	return map[string]bool{
		"RESTConfig": clients.RESTConfig() == nil,
		"Kubernetes": clients.Kubernetes() == nil,
		"Dynamic":    clients.Dynamic() == nil,
		"Metrics":    clients.Metrics() == nil,
		"Extensions": clients.Extensions() == nil,
		"Metadata":   clients.Metadata() == nil,
		"Discovery":  clients.Discovery() == nil,
		"RESTMapper": clients.RESTMapper() == nil,
	}
}

func requireCompleteClientBundle(t *testing.T, clients *Clients) {
	t.Helper()
	for accessor, isNil := range clientAccessorNilness(clients) {
		if isNil {
			t.Errorf("%s() = nil, want a constructed client", accessor)
		}
	}
}

func requireEmptyClientBundle(t *testing.T, clients *Clients) {
	t.Helper()
	for accessor, isNil := range clientAccessorNilness(clients) {
		if !isNil {
			t.Errorf("%s() != nil, want nil", accessor)
		}
	}
}

func TestNewClientsBuildsIndependentClientBundle(t *testing.T) {
	input := &rest.Config{Host: "https://cluster.invalid", UserAgent: "caller"}
	clients, err := NewClients(input)
	if err != nil {
		t.Fatalf("NewClients() error = %v", err)
	}
	requireCompleteClientBundle(t, clients)
	config := clients.RESTConfig()
	if config == input {
		t.Fatal("RESTConfig() returned the caller-owned pointer")
	}
	if config.UserAgent != clientUserAgent {
		t.Fatalf("UserAgent = %q, want %q", config.UserAgent, clientUserAgent)
	}
	config.Host = "https://changed.invalid"
	if clients.RESTConfig().Host != input.Host {
		t.Fatal("RESTConfig() exposed mutable internal configuration")
	}
	if input.UserAgent != "caller" {
		t.Fatalf("input UserAgent = %q, want caller", input.UserAgent)
	}
}

func TestNewClientsReportsConstructorStage(t *testing.T) {
	sentinel := errors.New("failed")
	stages := []struct {
		name     string
		subject  Subject
		sabotage func(*clientConstructors)
	}{
		{name: "typed", subject: SubjectTypedClient, sabotage: func(constructors *clientConstructors) {
			constructors.kubernetes = func(*rest.Config) (kubernetes.Interface, error) { return nil, sentinel }
		}},
		{name: "dynamic", subject: SubjectDynamicClient, sabotage: func(constructors *clientConstructors) {
			constructors.dynamic = func(*rest.Config) (dynamic.Interface, error) { return nil, sentinel }
		}},
		{name: "metrics", subject: SubjectMetricsClient, sabotage: func(constructors *clientConstructors) {
			constructors.metrics = func(*rest.Config) (metricsclient.Interface, error) { return nil, sentinel }
		}},
		{name: "extensions", subject: SubjectExtensionsClient, sabotage: func(constructors *clientConstructors) {
			constructors.extensions = func(*rest.Config) (apiextensionsclient.Interface, error) { return nil, sentinel }
		}},
		{name: "metadata", subject: SubjectMetadataClient, sabotage: func(constructors *clientConstructors) {
			constructors.metadata = func(*rest.Config) (metadata.Interface, error) { return nil, sentinel }
		}},
		{name: "discovery", subject: SubjectDiscoveryClient, sabotage: func(constructors *clientConstructors) {
			constructors.discovery = func(*rest.Config) (discovery.DiscoveryInterface, error) { return nil, sentinel }
		}},
	}
	for _, stage := range stages {
		t.Run(stage.name, func(t *testing.T) {
			constructors := successfulConstructors()
			stage.sabotage(&constructors)
			clients, err := buildClients(&rest.Config{Host: "https://cluster.invalid"}, constructors)
			if clients != nil {
				t.Fatalf("buildClients() clients = %v, want nil", clients)
			}
			var clientErr *Error
			if !errors.As(err, &clientErr) || clientErr.Operation != OperationCreate || clientErr.Subject != stage.subject || !errors.Is(err, sentinel) {
				t.Fatalf("buildClients() error = %v, want create %q wrapping sentinel", err, stage.subject)
			}
		})
	}
}

func TestClientsNilAccessors(t *testing.T) {
	var clients *Clients
	requireEmptyClientBundle(t, clients)
	if !errors.Is(clients.CheckConnection(context.Background()), ErrConnectionCheckUnavailable) {
		t.Fatal("nil Clients connection check must be unavailable")
	}
	if (&Clients{}).RESTConfig() != nil {
		t.Fatal("RESTConfig() with no configuration must return nil")
	}
	if !errors.Is((&Clients{}).CheckConnection(context.Background()), ErrConnectionCheckUnavailable) {
		t.Fatal("empty Clients connection check must be unavailable")
	}
}

func TestConnectionCheckValidatesClientShape(t *testing.T) {
	tests := []struct {
		name   string
		client kubernetes.Interface
	}{
		{name: "missing client"},
		{name: "missing discovery", client: discoverylessKubernetesClient{}},
		{name: "missing REST client", client: kubernetesfake.NewSimpleClientset()},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			check := newConnectionCheck(test.client)
			if err := check(context.Background()); !errors.Is(err, ErrConnectionCheckUnavailable) {
				t.Fatalf("connection check error = %v, want unavailable", err)
			}
		})
	}
}

func TestClientsCheckConnectionUsesRequestContext(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/version" {
			t.Errorf("request path = %q, want /version", request.URL.Path)
		}
		response.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)
	clients, err := NewClients(&rest.Config{Host: server.URL})
	if err != nil {
		t.Fatalf("NewClients() error = %v", err)
	}
	if err := clients.CheckConnection(context.Background()); err != nil {
		t.Fatalf("CheckConnection() error = %v", err)
	}
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if err := clients.CheckConnection(cancelled); !errors.Is(err, context.Canceled) {
		t.Fatalf("CheckConnection(cancelled) error = %v, want context.Canceled", err)
	}
}

func successfulConstructors() clientConstructors {
	discoveryClient := &discoveryfake.FakeDiscovery{Fake: &clienttesting.Fake{}}
	return clientConstructors{
		kubernetes: func(*rest.Config) (kubernetes.Interface, error) {
			return kubernetesfake.NewSimpleClientset(), nil
		},
		dynamic: func(*rest.Config) (dynamic.Interface, error) {
			return dynamicfake.NewSimpleDynamicClient(runtime.NewScheme()), nil
		},
		metrics: func(*rest.Config) (metricsclient.Interface, error) {
			return metricsfake.NewSimpleClientset(), nil
		},
		extensions: func(*rest.Config) (apiextensionsclient.Interface, error) {
			return apiextensionsfake.NewSimpleClientset(), nil
		},
		metadata: func(*rest.Config) (metadata.Interface, error) {
			return metadatafake.NewSimpleMetadataClient(runtime.NewScheme()), nil
		},
		discovery: func(*rest.Config) (discovery.DiscoveryInterface, error) {
			return discoveryClient, nil
		},
	}
}
