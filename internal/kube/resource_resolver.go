package kube

import (
	"strings"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
)

const resourceMappingAttempts = 2

type resettableRESTMapper interface {
	Reset()
}

func resolveResourceMapping(clients *Clients, resource schema.GroupResource) (*meta.RESTMapping, error) {
	mapper := clients.RESTMapper()
	if mapper == nil {
		return nil, ErrRESTMapperUnavailable
	}
	var mappingError error
	for attempt := 0; attempt < resourceMappingAttempts; attempt++ {
		mapping, err := resolveResourceMappingOnce(mapper, resource)
		if err == nil {
			return mapping, nil
		}
		mappingError = err
		if attempt == 0 && meta.IsNoMatchError(mappingError) {
			refreshResourceMappings(clients, mapper)
			continue
		}
		break
	}
	return nil, mappingError
}

func resolveResourceMappingOnce(mapper meta.RESTMapper, resource schema.GroupResource) (*meta.RESTMapping, error) {
	versionedResource, err := mapper.ResourceFor(schema.GroupVersionResource{
		Group:    resource.Group,
		Resource: resource.Resource,
	})
	if err != nil {
		return nil, err
	}
	kind, err := mapper.KindFor(versionedResource)
	if err != nil {
		return nil, err
	}
	mapping, err := mapper.RESTMapping(kind.GroupKind(), kind.Version)
	if err != nil {
		return nil, err
	}
	if mapping == nil {
		return nil, ErrRESTMappingUnavailable
	}
	return mapping, nil
}

func refreshResourceMappings(clients *Clients, mapper meta.RESTMapper) {
	if discovery := clients.Discovery(); discovery != nil {
		discovery.Invalidate()
	}
	if resetter, ok := mapper.(resettableRESTMapper); ok {
		resetter.Reset()
	}
}

func resourceClient(clients *Clients, mapping *meta.RESTMapping, namespace string) (dynamic.ResourceInterface, error) {
	if mapping == nil {
		return nil, ErrRESTMappingUnavailable
	}
	dynamicClient := clients.Dynamic()
	if dynamicClient == nil {
		return nil, ErrDynamicClientUnavailable
	}
	resource := dynamicClient.Resource(mapping.Resource)
	if mapping.Scope.Name() != meta.RESTScopeNameNamespace {
		return resource, nil
	}
	if strings.TrimSpace(namespace) == "" {
		return nil, ErrNamespaceRequired
	}
	return resource.Namespace(namespace), nil
}

func isSecretMapping(mapping *meta.RESTMapping) bool {
	return mapping != nil && mapping.Resource.Group == "" && mapping.Resource.Resource == "secrets"
}
