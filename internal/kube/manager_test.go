package kube

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	kubernetesfake "k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/rest"
	clienttesting "k8s.io/client-go/testing"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"

	"github.com/HediAbed/opsmate/internal/failure"
)

type fakeConfigSource struct {
	rawConfig clientcmdapi.Config
	rawErr    error
	rest      *rest.Config
	restErr   error
	restCalls []string
	setErr    error
	setCalls  []string
}

func (f *fakeConfigSource) RawConfig() (clientcmdapi.Config, error) {
	return f.rawConfig, f.rawErr
}

func (f *fakeConfigSource) RESTConfig(contextName string) (*rest.Config, error) {
	f.restCalls = append(f.restCalls, contextName)
	return f.rest, f.restErr
}

func (f *fakeConfigSource) SetCurrentContext(contextName string) error {
	f.setCalls = append(f.setCalls, contextName)
	return f.setErr
}

type fakeClientBuilder struct {
	clients *Clients
	err     error
	configs []*rest.Config
	after   func()
}

func (f *fakeClientBuilder) Build(config *rest.Config) (*Clients, error) {
	f.configs = append(f.configs, config)
	if f.after != nil {
		f.after()
	}
	return f.clients, f.err
}

func TestNewManagerValidatesDependencies(t *testing.T) {
	source := &fakeConfigSource{}
	builder := &fakeClientBuilder{}
	if manager, err := NewManager(nil, builder); manager != nil || err == nil {
		t.Fatalf("NewManager(nil, builder) = (%v, %v), want error", manager, err)
	}
	if manager, err := NewManager(source, nil); manager != nil || err == nil {
		t.Fatalf("NewManager(source, nil) = (%v, %v), want error", manager, err)
	}
	manager, err := NewManager(source, builder)
	if err != nil || manager == nil {
		t.Fatalf("NewManager() = (%v, %v), want manager", manager, err)
	}
}

func TestManagerConnectRequiresContext(t *testing.T) {
	manager, err := NewManager(&fakeConfigSource{}, &fakeClientBuilder{})
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}
	var missingContext context.Context
	if err := manager.Connect(missingContext, "primary"); !errors.Is(err, ErrContextRequired) {
		t.Fatalf("Connect(nil) error = %v, want context-required error", err)
	}
}

const maximumConnectionCheckBudget = time.Minute

func TestManagerConnectBoundsConnectionCheck(t *testing.T) {
	t.Run("bounds callers without a deadline", func(t *testing.T) {
		observed := connectWithObservedCheckContext(context.Background(), t)
		deadline, bounded := observed.Deadline()
		if !bounded {
			t.Fatal("Connect() handed an unbounded context to the connection check")
		}
		if budget := time.Until(deadline); budget <= 0 || budget > maximumConnectionCheckBudget {
			t.Fatalf("connection check budget = %s, want a positive budget of at most %s", budget, maximumConnectionCheckBudget)
		}
	})
	t.Run("keeps an earlier caller deadline", func(t *testing.T) {
		callerDeadline := time.Now().Add(750 * time.Millisecond)
		ctx, cancel := context.WithDeadline(context.Background(), callerDeadline)
		defer cancel()
		observed := connectWithObservedCheckContext(ctx, t)
		deadline, bounded := observed.Deadline()
		if !bounded || !deadline.Equal(callerDeadline) {
			t.Fatalf("connection check deadline = (%s, %t), want the caller deadline %s", deadline, bounded, callerDeadline)
		}
	})
}

func connectWithObservedCheckContext(ctx context.Context, t *testing.T) context.Context {
	t.Helper()
	var observed context.Context
	clients := testClientsWithCheck(func(checkContext context.Context) error {
		observed = checkContext
		return nil
	})
	manager, err := NewManager(primaryContextSource(), &fakeClientBuilder{clients: clients})
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}
	if err := manager.Connect(ctx, "primary"); err != nil {
		t.Fatalf("Connect() error = %v", err)
	}
	if observed == nil {
		t.Fatal("Connect() did not run the connection check")
	}
	return observed
}

func TestManagerRejectsNilContext(t *testing.T) {
	manager := connectedManagerForTest(t, kubernetesfake.NewSimpleClientset())
	var missingContext context.Context
	calls := []struct {
		name string
		call func() error
	}{
		{name: "CurrentContext", call: func() error {
			_, err := manager.CurrentContext(missingContext)
			return err
		}},
		{name: "Contexts", call: func() error {
			_, err := manager.Contexts(missingContext)
			return err
		}},
		{name: "Namespaces", call: func() error {
			_, err := manager.Namespaces(missingContext)
			return err
		}},
	}
	for _, test := range calls {
		t.Run(test.name, func(t *testing.T) {
			err := errorWithoutPanic(t, test.name+" without a context", test.call)
			if !errors.Is(err, ErrContextRequired) {
				t.Fatalf("%s(nil) error = %v, want context-required error", test.name, err)
			}
		})
	}
}

func TestManagerConnect(t *testing.T) {
	sentinel := errors.New("failed")
	validConfig := clientcmdapi.Config{
		CurrentContext: "primary",
		Contexts: map[string]*clientcmdapi.Context{
			"primary": {Cluster: "main"},
			"other":   {Cluster: "secondary"},
		},
	}
	tests := []struct {
		name        string
		contextName string
		source      *fakeConfigSource
		builder     *fakeClientBuilder
		wantContext string
		wantCause   error
	}{
		{name: "raw config failure", contextName: "other", source: &fakeConfigSource{rawErr: sentinel}, builder: &fakeClientBuilder{}, wantCause: sentinel},
		{name: "missing default", source: &fakeConfigSource{rawConfig: clientcmdapi.Config{Contexts: map[string]*clientcmdapi.Context{}}}, builder: &fakeClientBuilder{}, wantCause: ErrContextNotFound},
		{name: "missing explicit", contextName: "absent", source: &fakeConfigSource{rawConfig: validConfig}, builder: &fakeClientBuilder{}, wantCause: ErrContextNotFound},
		{name: "rest config failure", source: &fakeConfigSource{rawConfig: validConfig, restErr: sentinel}, builder: &fakeClientBuilder{}, wantCause: sentinel},
		{name: "client failure", source: &fakeConfigSource{rawConfig: validConfig, rest: &rest.Config{Host: "https://cluster.invalid"}}, builder: &fakeClientBuilder{err: sentinel}, wantCause: sentinel},
		{name: "missing clients", source: &fakeConfigSource{rawConfig: validConfig, rest: &rest.Config{Host: "https://cluster.invalid"}}, builder: &fakeClientBuilder{}, wantCause: ErrClientUnavailable},
		{name: "missing connection check", source: &fakeConfigSource{rawConfig: validConfig, rest: &rest.Config{Host: "https://cluster.invalid"}}, builder: &fakeClientBuilder{clients: &Clients{}}, wantCause: ErrConnectionCheckUnavailable},
		{name: "connection failure", source: &fakeConfigSource{rawConfig: validConfig, rest: &rest.Config{Host: "https://cluster.invalid"}}, builder: &fakeClientBuilder{clients: testClientsWithCheck(func(context.Context) error { return sentinel })}, wantCause: sentinel},
		{name: "persistence failure", contextName: "other", source: &fakeConfigSource{rawConfig: validConfig, rest: &rest.Config{Host: "https://cluster.invalid"}, setErr: sentinel}, builder: &fakeClientBuilder{clients: healthyTestClients()}, wantCause: sentinel},
		{name: "default success", source: &fakeConfigSource{rawConfig: validConfig, rest: &rest.Config{Host: "https://cluster.invalid"}}, builder: &fakeClientBuilder{clients: healthyTestClients()}, wantContext: "primary"},
		{name: "explicit success", contextName: "other", source: &fakeConfigSource{rawConfig: validConfig, rest: &rest.Config{Host: "https://cluster.invalid"}}, builder: &fakeClientBuilder{clients: healthyTestClients()}, wantContext: "other"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			manager, err := NewManager(test.source, test.builder)
			if err != nil {
				t.Fatalf("NewManager() error = %v", err)
			}
			err = manager.Connect(context.Background(), test.contextName)
			if test.wantCause != nil {
				requireConnectFailure(t, manager, err, test.wantCause)
				return
			}
			if err != nil {
				t.Fatalf("Connect() error = %v", err)
			}
			requireConnectRequests(t, test.source, test.builder, test.wantContext)
			requireConnectedManagerState(t, manager, test.builder.clients, test.wantContext)
		})
	}
}

func requireConnectFailure(t *testing.T, manager *Manager, err error, wantCause error) {
	t.Helper()
	if !errors.Is(err, wantCause) {
		t.Fatalf("Connect() error = %v, want cause %v", err, wantCause)
	}
	if clients, clientsErr := manager.Clients(); clients != nil || !errors.Is(clientsErr, ErrClientUnavailable) {
		t.Fatalf("Clients() after failed connect = (%v, %v), want unavailable", clients, clientsErr)
	}
}

func requireConnectRequests(t *testing.T, source *fakeConfigSource, builder *fakeClientBuilder, wantContext string) {
	t.Helper()
	if !slices.Equal(source.restCalls, []string{wantContext}) {
		t.Fatalf("RESTConfig calls = %v, want %q", source.restCalls, wantContext)
	}
	if len(builder.configs) != 1 || builder.configs[0] != source.rest {
		t.Fatalf("Build configs = %v, want source REST config", builder.configs)
	}
	wantSetCalls := []string{}
	if wantContext != source.rawConfig.CurrentContext {
		wantSetCalls = []string{wantContext}
	}
	if !slices.Equal(source.setCalls, wantSetCalls) {
		t.Fatalf("SetCurrentContext calls = %v, want %v", source.setCalls, wantSetCalls)
	}
}

func requireConnectedManagerState(t *testing.T, manager *Manager, wantClients *Clients, wantContext string) {
	t.Helper()
	clients, clientsErr := manager.Clients()
	if clientsErr != nil || clients != wantClients {
		t.Fatalf("Clients() = (%v, %v), want connected clients", clients, clientsErr)
	}
	currentContext, currentErr := manager.CurrentContext(context.Background())
	if currentErr != nil || currentContext != wantContext {
		t.Fatalf("CurrentContext() = (%q, %v), want %q", currentContext, currentErr, wantContext)
	}
}

func TestManagerWithoutConnection(t *testing.T) {
	sentinel := errors.New("failed")
	tests := []struct {
		name      string
		source    *fakeConfigSource
		want      string
		wantCause error
	}{
		{name: "current", source: &fakeConfigSource{rawConfig: clientcmdapi.Config{CurrentContext: "primary", Contexts: map[string]*clientcmdapi.Context{"primary": {}}}}, want: "primary"},
		{name: "load failure", source: &fakeConfigSource{rawErr: sentinel}, wantCause: sentinel},
		{name: "missing", source: &fakeConfigSource{rawConfig: clientcmdapi.Config{CurrentContext: "absent", Contexts: map[string]*clientcmdapi.Context{}}}, wantCause: ErrContextNotFound},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			manager, err := NewManager(test.source, &fakeClientBuilder{})
			if err != nil {
				t.Fatalf("NewManager() error = %v", err)
			}
			if clients, clientsErr := manager.Clients(); clients != nil || !errors.Is(clientsErr, ErrClientUnavailable) {
				t.Fatalf("Clients() = (%v, %v), want unavailable", clients, clientsErr)
			}
			contextName, currentErr := manager.CurrentContext(context.Background())
			if contextName != test.want || !errors.Is(currentErr, test.wantCause) {
				t.Fatalf("CurrentContext() = (%q, %v), want (%q, %v)", contextName, currentErr, test.want, test.wantCause)
			}
		})
	}
}

func TestManagerContextsReportsLoadFailure(t *testing.T) {
	sentinel := errors.New("failed")
	failingManager, err := NewManager(&fakeConfigSource{rawErr: sentinel}, &fakeClientBuilder{})
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}
	if contexts, contextsErr := failingManager.Contexts(context.Background()); contexts != nil || !errors.Is(contextsErr, sentinel) {
		t.Fatalf("Contexts() = (%v, %v), want load failure", contexts, contextsErr)
	}
}

func TestManagerContexts(t *testing.T) {
	source := &fakeConfigSource{rawConfig: clientcmdapi.Config{
		CurrentContext: "zeta",
		Contexts: map[string]*clientcmdapi.Context{
			"zeta":  nil,
			"alpha": {Cluster: "cluster-a", AuthInfo: "user-a", Namespace: "team-a"},
		},
	}, rest: &rest.Config{Host: "https://cluster.invalid"}}
	manager, err := NewManager(source, &fakeClientBuilder{clients: healthyTestClients()})
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}
	contexts, err := manager.Contexts(context.Background())
	if err != nil {
		t.Fatalf("Contexts() error = %v", err)
	}
	want := []ContextInfo{
		{Name: "alpha", Cluster: "cluster-a", User: "user-a", Namespace: "team-a"},
		{Name: "zeta", Current: true},
	}
	if !slices.Equal(contexts, want) {
		t.Fatalf("Contexts() = %+v, want %+v", contexts, want)
	}
	if err := manager.Connect(context.Background(), "alpha"); err != nil {
		t.Fatalf("Connect(alpha) error = %v", err)
	}
	contexts, err = manager.Contexts(context.Background())
	if err != nil {
		t.Fatalf("Contexts() after connect error = %v", err)
	}
	if !contexts[0].Current || contexts[1].Current {
		t.Fatalf("Contexts() current flags = %+v, want alpha current", contexts)
	}
}

func TestManagerHonorsContextCancellation(t *testing.T) {
	source := primaryContextSource()
	manager, err := NewManager(source, &fakeClientBuilder{clients: healthyTestClients()})
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if err := manager.Connect(cancelled, "primary"); !errors.Is(err, context.Canceled) {
		t.Fatalf("Connect(cancelled) error = %v, want context.Canceled", err)
	}
	if current, currentErr := manager.CurrentContext(cancelled); current != "" || !errors.Is(currentErr, context.Canceled) {
		t.Fatalf("CurrentContext(cancelled) = (%q, %v)", current, currentErr)
	}
	if contexts, contextsErr := manager.Contexts(cancelled); contexts != nil || !errors.Is(contextsErr, context.Canceled) {
		t.Fatalf("Contexts(cancelled) = (%v, %v)", contexts, contextsErr)
	}
}

func TestManagerDoesNotPublishClientsAfterCancellation(t *testing.T) {
	source := primaryContextSource()
	ctx, cancel := context.WithCancel(context.Background())
	builder := &fakeClientBuilder{clients: healthyTestClients(), after: cancel}
	manager, err := NewManager(source, builder)
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}
	if err := manager.Connect(ctx, "primary"); !errors.Is(err, context.Canceled) {
		t.Fatalf("Connect() error = %v, want context.Canceled", err)
	}
	if clients, clientsErr := manager.Clients(); clients != nil || !errors.Is(clientsErr, ErrClientUnavailable) {
		t.Fatalf("Clients() = (%v, %v), want unavailable", clients, clientsErr)
	}
}

func TestManagerChecksCancellationAfterConnectionCheck(t *testing.T) {
	source := primaryContextSource()
	ctx, cancel := context.WithCancel(context.Background())
	clients := testClientsWithCheck(func(context.Context) error {
		cancel()
		return nil
	})
	manager, err := NewManager(source, &fakeClientBuilder{clients: clients})
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}
	if err := manager.Connect(ctx, "primary"); !errors.Is(err, context.Canceled) {
		t.Fatalf("Connect() error = %v, want context.Canceled", err)
	}
	if connected, clientsErr := manager.Clients(); connected != nil || !errors.Is(clientsErr, ErrClientUnavailable) {
		t.Fatalf("Clients() = (%v, %v), want unavailable", connected, clientsErr)
	}
}

func TestManagerNamespaces(t *testing.T) {
	client := kubernetesfake.NewSimpleClientset(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "zeta"}},
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "alpha"}},
	)
	manager := connectedManagerForTest(t, client)
	namespaces, err := manager.Namespaces(context.Background())
	if err != nil {
		t.Fatalf("Namespaces() error = %v", err)
	}
	if !slices.Equal(namespaces, []string{"alpha", "zeta"}) {
		t.Fatalf("Namespaces() = %v, want sorted names", namespaces)
	}
}

func TestManagerNamespaceFailures(t *testing.T) {
	unavailable, err := NewManager(&fakeConfigSource{}, &fakeClientBuilder{})
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}
	if namespaces, listErr := unavailable.Namespaces(context.Background()); namespaces != nil || failure.CodeOf(listErr) != failure.CodeUnavailable {
		t.Fatalf("Namespaces() = (%v, %v), want unavailable", namespaces, listErr)
	}

	sentinel := errors.New("denied")
	client := kubernetesfake.NewSimpleClientset()
	client.PrependReactor("list", "namespaces", func(clienttesting.Action) (bool, runtime.Object, error) {
		return true, nil, sentinel
	})
	manager := connectedManagerForTest(t, client)
	if namespaces, listErr := manager.Namespaces(context.Background()); namespaces != nil || !errors.Is(listErr, sentinel) {
		t.Fatalf("Namespaces() = (%v, %v), want sentinel", namespaces, listErr)
	}
}

func connectedManagerForTest(t *testing.T, client *kubernetesfake.Clientset) *Manager {
	t.Helper()
	source := primaryContextSource()
	clients := healthyTestClients()
	clients.kubernetes = client
	manager, err := NewManager(source, &fakeClientBuilder{clients: clients})
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}
	if err := manager.Connect(context.Background(), "primary"); err != nil {
		t.Fatalf("Connect() error = %v", err)
	}
	return manager
}

func primaryContextSource() *fakeConfigSource {
	return &fakeConfigSource{rawConfig: clientcmdapi.Config{
		CurrentContext: "primary",
		Contexts:       map[string]*clientcmdapi.Context{"primary": {}},
	}, rest: &rest.Config{Host: "https://cluster.invalid"}}
}

func healthyTestClients() *Clients {
	return testClientsWithCheck(func(ctx context.Context) error {
		return ctx.Err()
	})
}

func testClientsWithCheck(check func(context.Context) error) *Clients {
	return &Clients{checkConnection: check}
}
