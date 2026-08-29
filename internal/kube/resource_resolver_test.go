package kube

import (
	"errors"
	"testing"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
	dynamicfake "k8s.io/client-go/dynamic/fake"
)

type mapperResult[T comparable] struct {
	value T
	err   error
}

type scriptedRESTMapper struct {
	meta.RESTMapper
	resources  []mapperResult[schema.GroupVersionResource]
	kinds      []mapperResult[schema.GroupVersionKind]
	mappings   []mapperResult[*meta.RESTMapping]
	resourceAt int
	kindAt     int
	mappingAt  int
	resetCalls int
}

func (m *scriptedRESTMapper) ResourceFor(schema.GroupVersionResource) (schema.GroupVersionResource, error) {
	result := m.resources[m.resourceAt]
	m.resourceAt++
	return result.value, result.err
}

func (m *scriptedRESTMapper) KindFor(schema.GroupVersionResource) (schema.GroupVersionKind, error) {
	result := m.kinds[m.kindAt]
	m.kindAt++
	return result.value, result.err
}

func (m *scriptedRESTMapper) RESTMapping(schema.GroupKind, ...string) (*meta.RESTMapping, error) {
	result := m.mappings[m.mappingAt]
	m.mappingAt++
	return result.value, result.err
}

func (m *scriptedRESTMapper) Reset() {
	m.resetCalls++
}

type trackingDiscovery struct {
	discovery.CachedDiscoveryInterface
	invalidations int
}

func (d *trackingDiscovery) Invalidate() {
	d.invalidations++
}

func TestResolveResourceMappingFailures(t *testing.T) {
	resource := schema.GroupResource{Group: "apps", Resource: "deployments"}
	versionedResource := schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"}
	kind := schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"}
	sentinel := errors.New("mapping failed")
	tests := []struct {
		name    string
		mapper  meta.RESTMapper
		wantErr error
	}{
		{name: "missing mapper", wantErr: ErrRESTMapperUnavailable},
		{
			name: "resource failure",
			mapper: &scriptedRESTMapper{
				resources: []mapperResult[schema.GroupVersionResource]{{err: sentinel}},
			},
			wantErr: sentinel,
		},
		{
			name: "kind failure",
			mapper: &scriptedRESTMapper{
				resources: []mapperResult[schema.GroupVersionResource]{{value: versionedResource}},
				kinds:     []mapperResult[schema.GroupVersionKind]{{err: sentinel}},
			},
			wantErr: sentinel,
		},
		{
			name: "mapping failure",
			mapper: &scriptedRESTMapper{
				resources: []mapperResult[schema.GroupVersionResource]{{value: versionedResource}},
				kinds:     []mapperResult[schema.GroupVersionKind]{{value: kind}},
				mappings:  []mapperResult[*meta.RESTMapping]{{err: sentinel}},
			},
			wantErr: sentinel,
		},
		{
			name: "empty mapping",
			mapper: &scriptedRESTMapper{
				resources: []mapperResult[schema.GroupVersionResource]{{value: versionedResource}},
				kinds:     []mapperResult[schema.GroupVersionKind]{{value: kind}},
				mappings:  []mapperResult[*meta.RESTMapping]{{}},
			},
			wantErr: ErrRESTMappingUnavailable,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := resolveResourceMapping(&Clients{mapper: test.mapper}, resource)
			if got != nil || !errors.Is(err, test.wantErr) {
				t.Fatalf("resolveResourceMapping() = (%v, %v), want error %v", got, err, test.wantErr)
			}
		})
	}
}

func TestResolveResourceMappingSucceeds(t *testing.T) {
	resource := schema.GroupResource{Group: "apps", Resource: "deployments"}
	versionedResource := schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"}
	kind := schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"}
	mapping := &meta.RESTMapping{Resource: versionedResource, GroupVersionKind: kind, Scope: meta.RESTScopeNamespace}
	mapper := &scriptedRESTMapper{
		resources: []mapperResult[schema.GroupVersionResource]{{value: versionedResource}},
		kinds:     []mapperResult[schema.GroupVersionKind]{{value: kind}},
		mappings:  []mapperResult[*meta.RESTMapping]{{value: mapping}},
	}
	got, err := resolveResourceMapping(&Clients{mapper: mapper}, resource)
	if got != mapping || err != nil {
		t.Fatalf("resolveResourceMapping() = (%v, %v), want mapping", got, err)
	}
}

func TestResolveResourceMappingRetriesWithoutResetDependencies(t *testing.T) {
	resource := schema.GroupResource{Group: "apps", Resource: "deployments"}
	versionedResource := schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"}
	kind := schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"}
	mapping := &meta.RESTMapping{Resource: versionedResource, GroupVersionKind: kind, Scope: meta.RESTScopeNamespace}
	noMatch := &meta.NoResourceMatchError{PartialResource: versionedResource}
	mapper := struct{ meta.RESTMapper }{RESTMapper: &scriptedRESTMapper{
		resources: []mapperResult[schema.GroupVersionResource]{{err: noMatch}, {value: versionedResource}},
		kinds:     []mapperResult[schema.GroupVersionKind]{{value: kind}},
		mappings:  []mapperResult[*meta.RESTMapping]{{value: mapping}},
	}}
	got, err := resolveResourceMapping(&Clients{mapper: mapper}, resource)
	if got != mapping || err != nil {
		t.Fatalf("resolveResourceMapping() = (%v, %v), want mapping", got, err)
	}
}

func TestResolveResourceMappingInvalidatesStaleDiscovery(t *testing.T) {
	resource := schema.GroupResource{Resource: "pods"}
	versionedResource := schema.GroupVersionResource{Version: "v1", Resource: "pods"}
	kind := schema.GroupVersionKind{Version: "v1", Kind: "Pod"}
	mapping := &meta.RESTMapping{Resource: versionedResource, GroupVersionKind: kind, Scope: meta.RESTScopeNamespace}
	noMatch := &meta.NoResourceMatchError{PartialResource: versionedResource}
	mapper := &scriptedRESTMapper{
		resources: []mapperResult[schema.GroupVersionResource]{{err: noMatch}, {value: versionedResource}},
		kinds:     []mapperResult[schema.GroupVersionKind]{{value: kind}},
		mappings:  []mapperResult[*meta.RESTMapping]{{value: mapping}},
	}
	discoveryCache := &trackingDiscovery{}
	got, err := resolveResourceMapping(&Clients{mapper: mapper, discovery: discoveryCache}, resource)
	if err != nil || got != mapping {
		t.Fatalf("resolveResourceMapping() = (%v, %v), want mapping", got, err)
	}
	if discoveryCache.invalidations != 1 || mapper.resetCalls != 1 || mapper.resourceAt != resourceMappingAttempts {
		t.Fatalf("retry state = invalidations:%d resets:%d attempts:%d", discoveryCache.invalidations, mapper.resetCalls, mapper.resourceAt)
	}
}

func TestResolveResourceMappingStopsAfterRetryLimit(t *testing.T) {
	resource := schema.GroupResource{Resource: "widgets"}
	versionedResource := schema.GroupVersionResource{Resource: "widgets"}
	noMatch := &meta.NoResourceMatchError{PartialResource: versionedResource}
	mapper := &scriptedRESTMapper{
		resources: []mapperResult[schema.GroupVersionResource]{{err: noMatch}, {err: noMatch}},
	}
	discoveryCache := &trackingDiscovery{}
	got, err := resolveResourceMapping(&Clients{mapper: mapper, discovery: discoveryCache}, resource)
	if got != nil || !errors.Is(err, noMatch) {
		t.Fatalf("resolveResourceMapping() = (%v, %v), want retry error", got, err)
	}
	if discoveryCache.invalidations != 1 || mapper.resetCalls != 1 || mapper.resourceAt != resourceMappingAttempts {
		t.Fatalf("retry state = invalidations:%d resets:%d attempts:%d", discoveryCache.invalidations, mapper.resetCalls, mapper.resourceAt)
	}
}

func TestResourceClientRejectsIncompleteInputs(t *testing.T) {
	clients := &Clients{dynamic: dynamicfake.NewSimpleDynamicClient(runtime.NewScheme())}
	resource := schema.GroupVersionResource{Version: "v1", Resource: "pods"}
	if client, err := resourceClient(clients, nil, "team-a"); client != nil || !errors.Is(err, ErrRESTMappingUnavailable) {
		t.Fatalf("resourceClient(nil mapping) = (%v, %v)", client, err)
	}
	if client, err := resourceClient(&Clients{}, &meta.RESTMapping{Resource: resource, Scope: meta.RESTScopeRoot}, ""); client != nil || !errors.Is(err, ErrDynamicClientUnavailable) {
		t.Fatalf("resourceClient(no dynamic client) = (%v, %v)", client, err)
	}
	if client, err := resourceClient(clients, &meta.RESTMapping{Resource: resource, Scope: meta.RESTScopeNamespace}, "  "); client != nil || !errors.Is(err, ErrNamespaceRequired) {
		t.Fatalf("resourceClient(blank namespace) = (%v, %v)", client, err)
	}
}

func TestResourceClientHonorsMappingScope(t *testing.T) {
	clients := &Clients{dynamic: dynamicfake.NewSimpleDynamicClient(runtime.NewScheme())}
	resource := schema.GroupVersionResource{Version: "v1", Resource: "pods"}
	if client, err := resourceClient(clients, &meta.RESTMapping{Resource: resource, Scope: meta.RESTScopeNamespace}, "team-a"); client == nil || err != nil {
		t.Fatalf("resourceClient(namespaced) = (%v, %v)", client, err)
	}
	if client, err := resourceClient(clients, &meta.RESTMapping{Resource: resource, Scope: meta.RESTScopeRoot}, "ignored"); client == nil || err != nil {
		t.Fatalf("resourceClient(cluster scoped) = (%v, %v)", client, err)
	}
}

func TestSecretMappingDetection(t *testing.T) {
	tests := []struct {
		name    string
		mapping *meta.RESTMapping
		want    bool
	}{
		{name: "missing mapping"},
		{name: "core secret", mapping: &meta.RESTMapping{Resource: schema.GroupVersionResource{Resource: "secrets"}}, want: true},
		{name: "other core resource", mapping: &meta.RESTMapping{Resource: schema.GroupVersionResource{Resource: "configmaps"}}},
		{name: "same plural in another group", mapping: &meta.RESTMapping{Resource: schema.GroupVersionResource{Group: "example.io", Resource: "secrets"}}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := isSecretMapping(test.mapping); got != test.want {
				t.Fatalf("isSecretMapping() = %t, want %t", got, test.want)
			}
		})
	}
}
