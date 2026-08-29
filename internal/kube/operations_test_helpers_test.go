package kube

import (
	"context"
	"testing"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func managerWithClientsForTest(t *testing.T, clients *Clients) *Manager {
	t.Helper()
	manager, err := NewManager(&fakeConfigSource{}, &fakeClientBuilder{})
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}
	clientLifetime, cancelClients := context.WithCancel(context.Background())
	manager.clients = clients
	manager.currentContext = "primary"
	manager.clientLifetime = clientLifetime
	manager.cancelClients = cancelClients
	t.Cleanup(cancelClients)
	return manager
}

func mapperForResource(kind schema.GroupVersionKind, resource, singular schema.GroupVersionResource, scope meta.RESTScope) meta.RESTMapper {
	mapper := meta.NewDefaultRESTMapper([]schema.GroupVersion{kind.GroupVersion()})
	mapper.AddSpecific(kind, resource, singular, scope)
	return mapper
}
