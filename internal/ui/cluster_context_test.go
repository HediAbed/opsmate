package ui

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/HediAbed/opsmate/internal/cluster"
	"github.com/HediAbed/opsmate/internal/kube"
)

func rootModelWithContextManager(t *testing.T, namespace string, manager *testContextManager) RootModel {
	t.Helper()
	operations := &testClusterOperations{}
	root, err := NewRootModel(namespace, RuntimeDependencies{
		Context:           context.Background(),
		ClusterContext:    manager,
		ClusterResources:  &testResourceReader{},
		ClusterSnapshots:  &snapshotCollectorStub{},
		ClusterObserver:   &testResourceObserver{},
		ClusterOperations: operations,
		HelmReleases:      operations,
	})
	if err != nil {
		t.Fatalf("NewRootModel() error = %v", err)
	}
	return root
}

func TestFetchNamespacesCommand(t *testing.T) {
	root := rootModelWithContextManager(t, "default", &testContextManager{
		namespaces: func(ctx context.Context) ([]string, error) {
			assertCommandDeadline(ctx, t)
			return []string{"default", "platform"}, nil
		},
	})
	namespaces, ok := root.fetchNamespaces()().(cluster.NamespacesMsg)
	if !ok || namespaces.Err != nil || !slices.Equal(namespaces.Namespaces, []string{"default", "platform"}) {
		t.Fatalf("fetchNamespaces() = %+v", namespaces)
	}
}

func TestFetchCurrentContextCommand(t *testing.T) {
	root := rootModelWithContextManager(t, "default", &testContextManager{
		currentContext: func(ctx context.Context) (string, error) {
			assertCommandDeadline(ctx, t)
			return "primary", nil
		},
	})
	current, ok := root.fetchCurrentContext()().(cluster.CurrentContextMsg)
	if !ok || current.Err != nil || current.Name != "primary" {
		t.Fatalf("fetchCurrentContext() = %+v", current)
	}
}

func TestFetchContextsCommand(t *testing.T) {
	root := rootModelWithContextManager(t, "default", &testContextManager{
		contexts: func(ctx context.Context) ([]kube.ContextInfo, error) {
			assertCommandDeadline(ctx, t)
			return []kube.ContextInfo{{Name: "primary", Cluster: "main", Namespace: "default", Current: true}}, nil
		},
	})
	contexts, ok := root.fetchContexts()().(cluster.ContextsMsg)
	if !ok || contexts.Err != nil || len(contexts.Contexts) != 1 || contexts.Contexts[0].Name != "primary" || contexts.Contexts[0].Cluster != "main" || contexts.Contexts[0].Namespace != "default" || !contexts.Contexts[0].Current {
		t.Fatalf("fetchContexts() = %+v", contexts)
	}
}

func TestSwitchContextCommand(t *testing.T) {
	sentinel := errors.New("denied")
	root := rootModelWithContextManager(t, "default", &testContextManager{
		connect: func(ctx context.Context, name string) error {
			assertCommandDeadline(ctx, t)
			if name != "secondary" {
				t.Fatalf("Connect() name = %q, want secondary", name)
			}
			return sentinel
		},
	})
	switched, ok := root.switchContext("secondary")().(cluster.ContextSwitchedMsg)
	if !ok || switched.Name != "secondary" || !errors.Is(switched.Err, sentinel) {
		t.Fatalf("switchContext() = %+v", switched)
	}
}

func TestClusterContextCommandsReturnErrors(t *testing.T) {
	sentinel := errors.New("offline")
	root := rootModelWithContextManager(t, "", &testContextManager{
		namespaces:     func(context.Context) ([]string, error) { return nil, sentinel },
		currentContext: func(context.Context) (string, error) { return "", sentinel },
		contexts:       func(context.Context) ([]kube.ContextInfo, error) { return nil, sentinel },
	})
	if message := root.fetchNamespaces()().(cluster.NamespacesMsg); !errors.Is(message.Err, sentinel) {
		t.Fatalf("fetchNamespaces() error = %v", message.Err)
	}
	if message := root.fetchCurrentContext()().(cluster.CurrentContextMsg); !errors.Is(message.Err, sentinel) {
		t.Fatalf("fetchCurrentContext() error = %v", message.Err)
	}
	if message := root.fetchContexts()().(cluster.ContextsMsg); !errors.Is(message.Err, sentinel) || len(message.Contexts) != 0 {
		t.Fatalf("fetchContexts() = %+v", message)
	}
}

func TestContextMessagesPreserveEmptyInput(t *testing.T) {
	if messages := contextMessages(nil); messages == nil || len(messages) != 0 {
		t.Fatalf("contextMessages(nil) = %#v, want non-nil empty slice", messages)
	}
}

func assertCommandDeadline(ctx context.Context, t *testing.T) {
	t.Helper()
	if _, set := ctx.Deadline(); !set {
		t.Fatal("cluster command context has no deadline")
	}
}
