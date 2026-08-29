package kube

import (
	"cmp"
	"context"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/rest"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

type ContextInfo struct {
	Name      string
	Cluster   string
	User      string
	Namespace string
	Current   bool
}

type ConfigSource interface {
	RawConfig() (clientcmdapi.Config, error)
	RESTConfig(contextName string) (*rest.Config, error)
	SetCurrentContext(contextName string) error
}

type ClientBuilder interface {
	Build(*rest.Config) (*Clients, error)
}

type ContextManager interface {
	Connect(context.Context, string) error
	Contexts(context.Context) ([]ContextInfo, error)
	CurrentContext(context.Context) (string, error)
	Namespaces(context.Context) ([]string, error)
}

type Manager struct {
	connectMu       sync.Mutex
	mu              sync.RWMutex
	configSource    ConfigSource
	clientBuilder   ClientBuilder
	clients         *Clients
	currentContext  string
	clientLifetime  context.Context
	cancelClients   context.CancelFunc
	clock           func() time.Time
	encodeResource  func(*unstructured.Unstructured) ([]byte, error)
	newShellStream  shellExecutorFactory
	shellSequence   atomic.Uint64
	newPortForward  portForwardFactory
	portForwards    *portForwardRegistry
	forwardTimeout  time.Duration
	forwardSequence atomic.Uint64
	newHelmStorage  helmReleaseStorageFactory
}

type connectionCandidate struct {
	contextName     string
	previousContext string
	clients         *Clients
}

func NewManager(configSource ConfigSource, clientBuilder ClientBuilder) (*Manager, error) {
	if configSource == nil {
		return nil, newError(OperationCreate, SubjectManager, "", ErrConfigSourceRequired)
	}
	if clientBuilder == nil {
		return nil, newError(OperationCreate, SubjectManager, "", ErrClientBuilderRequired)
	}
	return &Manager{
		configSource:   configSource,
		clientBuilder:  clientBuilder,
		clock:          time.Now,
		encodeResource: encodeResourceYAML,
		newShellStream: defaultShellExecutor,
		newPortForward: defaultPortForwarder,
		portForwards:   newPortForwardRegistry(),
		forwardTimeout: defaultPortForwardReadinessTimeout,
		newHelmStorage: newHelmSecretReleaseStorage,
	}, nil
}

func (m *Manager) Connect(ctx context.Context, contextName string) error {
	m.connectMu.Lock()
	defer m.connectMu.Unlock()
	if ctx == nil {
		return newError(OperationConnect, SubjectClients, contextName, ErrContextRequired)
	}
	if err := ctx.Err(); err != nil {
		return newError(OperationConnect, SubjectClients, contextName, err)
	}
	candidate, err := m.prepareConnection(contextName)
	if err != nil {
		return err
	}
	if err := candidate.clients.CheckConnection(ctx); err != nil {
		return newError(OperationConnect, SubjectAPIServer, candidate.contextName, err)
	}
	if err := ctx.Err(); err != nil {
		return newError(OperationConnect, SubjectAPIServer, candidate.contextName, err)
	}
	if candidate.previousContext != candidate.contextName {
		if err := m.configSource.SetCurrentContext(candidate.contextName); err != nil {
			return newError(OperationSet, SubjectCurrentContext, candidate.contextName, err)
		}
	}
	m.installConnection(candidate)
	return nil
}

func (m *Manager) prepareConnection(contextName string) (connectionCandidate, error) {
	rawConfig, err := m.configSource.RawConfig()
	if err != nil {
		return connectionCandidate{}, newError(OperationLoad, SubjectConfiguration, contextName, err)
	}
	selectedContext := contextName
	if selectedContext == "" {
		selectedContext = rawConfig.CurrentContext
	}
	if _, found := rawConfig.Contexts[selectedContext]; !found {
		return connectionCandidate{}, newError(OperationConnect, SubjectClients, selectedContext, ErrContextNotFound)
	}
	restConfig, err := m.configSource.RESTConfig(selectedContext)
	if err != nil {
		return connectionCandidate{}, newError(OperationBuild, SubjectRESTConfig, selectedContext, err)
	}
	clients, err := m.clientBuilder.Build(restConfig)
	if err != nil {
		return connectionCandidate{}, newError(OperationBuild, SubjectClients, selectedContext, err)
	}
	if clients == nil {
		return connectionCandidate{}, newError(OperationBuild, SubjectClients, selectedContext, ErrClientUnavailable)
	}
	return connectionCandidate{
		contextName:     selectedContext,
		previousContext: rawConfig.CurrentContext,
		clients:         clients,
	}, nil
}

func (m *Manager) installConnection(candidate connectionCandidate) {
	clientLifetime, cancelClients := context.WithCancel(context.Background())
	m.mu.Lock()
	previousCancel := m.cancelClients
	m.clients = candidate.clients
	m.currentContext = candidate.contextName
	m.clientLifetime = clientLifetime
	m.cancelClients = cancelClients
	m.mu.Unlock()
	if previousCancel != nil {
		previousCancel()
	}
}

func (m *Manager) Clients() (*Clients, error) {
	m.mu.RLock()
	clients := m.clients
	contextName := m.currentContext
	m.mu.RUnlock()
	if clients == nil {
		return nil, newError(OperationGet, SubjectClients, contextName, ErrClientUnavailable)
	}
	return clients, nil
}

func (m *Manager) observationSession() (*Clients, context.Context, error) {
	m.mu.RLock()
	clients := m.clients
	clientLifetime := m.clientLifetime
	contextName := m.currentContext
	m.mu.RUnlock()
	if clients == nil {
		return nil, nil, newError(OperationGet, SubjectClients, contextName, ErrClientUnavailable)
	}
	if clientLifetime == nil {
		clientLifetime = context.Background()
	}
	return clients, clientLifetime, nil
}

func (m *Manager) clientSession(parent context.Context) (*Clients, context.Context, context.CancelFunc, error) {
	if parent == nil {
		return nil, nil, nil, ErrContextRequired
	}
	clients, lifetime, err := m.observationSession()
	if err != nil {
		return nil, nil, nil, err
	}
	ctx, cancel := linkedContext(parent, lifetime)
	return clients, ctx, cancel, nil
}

func (m *Manager) CurrentContext(ctx context.Context) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", newError(OperationLoad, SubjectCurrentContext, "", err)
	}
	m.mu.RLock()
	contextName := m.currentContext
	m.mu.RUnlock()
	if contextName != "" {
		return contextName, nil
	}
	rawConfig, err := m.configSource.RawConfig()
	if err != nil {
		return "", newError(OperationLoad, SubjectCurrentContext, "", err)
	}
	if _, found := rawConfig.Contexts[rawConfig.CurrentContext]; !found {
		return "", newError(OperationLoad, SubjectCurrentContext, rawConfig.CurrentContext, ErrContextNotFound)
	}
	return rawConfig.CurrentContext, nil
}

func (m *Manager) Contexts(ctx context.Context) ([]ContextInfo, error) {
	if err := ctx.Err(); err != nil {
		return nil, newError(OperationList, SubjectContexts, "", err)
	}
	rawConfig, err := m.configSource.RawConfig()
	if err != nil {
		return nil, newError(OperationList, SubjectContexts, "", err)
	}
	m.mu.RLock()
	currentContext := m.currentContext
	m.mu.RUnlock()
	if currentContext == "" {
		currentContext = rawConfig.CurrentContext
	}
	contexts := make([]ContextInfo, 0, len(rawConfig.Contexts))
	for name, contextConfig := range rawConfig.Contexts {
		if contextConfig == nil {
			contextConfig = &clientcmdapi.Context{}
		}
		contexts = append(contexts, ContextInfo{
			Name:      name,
			Cluster:   contextConfig.Cluster,
			User:      contextConfig.AuthInfo,
			Namespace: contextConfig.Namespace,
			Current:   name == currentContext,
		})
	}
	slices.SortFunc(contexts, func(left, right ContextInfo) int {
		return cmp.Compare(left.Name, right.Name)
	})
	return contexts, nil
}

func (m *Manager) Namespaces(ctx context.Context) ([]string, error) {
	clients, err := m.Clients()
	if err != nil {
		return nil, newError(OperationList, SubjectNamespaces, "", err)
	}
	list, err := clients.Kubernetes().CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, newError(OperationList, SubjectNamespaces, "", err)
	}
	namespaces := make([]string, 0, len(list.Items))
	for _, namespace := range list.Items {
		namespaces = append(namespaces, namespace.Name)
	}
	slices.Sort(namespaces)
	return namespaces, nil
}
