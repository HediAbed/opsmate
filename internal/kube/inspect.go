package kube

import (
	"context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/yaml"
)

func encodeResourceYAML(object *unstructured.Unstructured) ([]byte, error) {
	return yaml.Marshal(object.Object)
}

func (m *Manager) ResourceYAML(parent context.Context, reference ResourceReference) (string, error) {
	identifier := reference.Identifier()
	if parent == nil {
		return "", newResourceError(OperationRead, SubjectResource, identifier, ErrContextRequired)
	}
	if err := validateResourceReference(reference); err != nil {
		return "", newResourceError(OperationRead, SubjectResource, identifier, err)
	}
	clients, ctx, cancel, err := m.clientSession(parent)
	if err != nil {
		return "", newResourceError(OperationRead, SubjectResource, identifier, err)
	}
	defer cancel()
	mapping, err := resolveResourceMapping(clients, reference.Resource)
	if err != nil {
		return "", newResourceError(OperationResolve, SubjectResource, identifier, err)
	}
	if isSecretMapping(mapping) {
		return "", newResourceError(OperationRead, SubjectResource, identifier, ErrSensitiveResourceAccess)
	}
	resource, err := resourceClient(clients, mapping, reference.Namespace)
	if err != nil {
		return "", newResourceError(OperationRead, SubjectResource, identifier, err)
	}
	object, err := resource.Get(ctx, reference.Name, metav1.GetOptions{})
	if err != nil {
		return "", newResourceError(OperationRead, SubjectResource, identifier, err)
	}
	object = object.DeepCopy()
	object.SetManagedFields(nil)
	payload, err := m.encodeResource(object)
	if err != nil {
		return "", newResourceError(OperationRead, SubjectResource, identifier, err)
	}
	return string(payload), nil
}

var _ ResourceInspector = (*Manager)(nil)
