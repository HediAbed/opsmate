package kube

import (
	"context"
	"errors"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	"sigs.k8s.io/yaml"

	"github.com/HediAbed/opsmate/internal/failure"
)

func TestResourceYAMLValidatesRequestAndDependencies(t *testing.T) {
	reference := ResourceReference{
		Resource:  schema.GroupResource{Resource: "configmaps"},
		Namespace: "team-a",
		Name:      "settings",
	}
	manager, err := NewManager(&fakeConfigSource{}, &fakeClientBuilder{})
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}
	tests := []struct {
		name      string
		ctx       context.Context
		reference ResourceReference
		wantErr   error
	}{
		{name: "missing context", reference: reference, wantErr: ErrContextRequired},
		{name: "missing resource", ctx: context.Background(), reference: ResourceReference{Name: "settings"}, wantErr: ErrResourceIdentifierRequired},
		{name: "missing clients", ctx: context.Background(), reference: reference, wantErr: ErrClientUnavailable},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			payload, readErr := manager.ResourceYAML(test.ctx, test.reference)
			if payload != "" || !errors.Is(readErr, test.wantErr) {
				t.Fatalf("ResourceYAML() = (%q, %v), want error %v", payload, readErr, test.wantErr)
			}
			var resourceErr *Error
			if !errors.As(readErr, &resourceErr) || resourceErr.Identifier != test.reference.Identifier() {
				t.Fatalf("ResourceYAML() error = %#v, want identifier %q", resourceErr, test.reference.Identifier())
			}
		})
	}
}

func TestResourceYAMLReportsResolutionAndReadFailures(t *testing.T) {
	reference := ResourceReference{
		Resource:  schema.GroupResource{Resource: "configmaps"},
		Namespace: "team-a",
		Name:      "settings",
	}
	sentinel := errors.New("encode failed")
	object := &corev1.ConfigMap{
		TypeMeta:   metav1.TypeMeta{APIVersion: "v1", Kind: "ConfigMap"},
		ObjectMeta: metav1.ObjectMeta{Name: reference.Name, Namespace: reference.Namespace},
	}
	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("AddToScheme() error = %v", err)
	}
	dynamicClient := dynamicfake.NewSimpleDynamicClient(scheme, object)
	kind := schema.GroupVersionKind{Version: "v1", Kind: "ConfigMap"}
	resource := schema.GroupVersionResource{Version: "v1", Resource: "configmaps"}
	singular := schema.GroupVersionResource{Version: "v1", Resource: "configmap"}
	tests := []struct {
		name      string
		clients   *Clients
		configure func(*Manager)
		wantErr   error
		wantCode  failure.Code
		operation Operation
	}{
		{name: "missing mapper", clients: &Clients{dynamic: dynamicClient}, wantErr: ErrRESTMapperUnavailable, operation: OperationResolve},
		{name: "missing dynamic client", clients: &Clients{mapper: mapperForResource(kind, resource, singular, meta.RESTScopeNamespace)}, wantErr: ErrDynamicClientUnavailable, operation: OperationRead},
		{name: "resource not found", clients: &Clients{mapper: mapperForResource(kind, resource, singular, meta.RESTScopeNamespace), dynamic: dynamicfake.NewSimpleDynamicClient(scheme)}, wantCode: failure.CodeNotFound, operation: OperationRead},
		{
			name:    "encoding failure",
			clients: &Clients{mapper: mapperForResource(kind, resource, singular, meta.RESTScopeNamespace), dynamic: dynamicClient},
			configure: func(manager *Manager) {
				manager.encodeResource = func(*unstructured.Unstructured) ([]byte, error) { return nil, sentinel }
			},
			wantErr:   sentinel,
			operation: OperationRead,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			manager := managerWithClientsForTest(t, test.clients)
			if test.configure != nil {
				test.configure(manager)
			}
			payload, readErr := manager.ResourceYAML(context.Background(), reference)
			if payload != "" {
				t.Fatalf("ResourceYAML() payload = %q, want empty", payload)
			}
			requireResourceYAMLFailure(t, readErr, test.wantErr, test.wantCode, test.operation)
		})
	}
}

func requireResourceYAMLFailure(t *testing.T, readErr error, wantErr error, wantCode failure.Code, operation Operation) {
	t.Helper()
	if readErr == nil {
		t.Fatal("ResourceYAML() error = nil, want failure")
	}
	if wantErr != nil && !errors.Is(readErr, wantErr) {
		t.Fatalf("ResourceYAML() error = %v, want %v", readErr, wantErr)
	}
	if wantCode != "" && failure.CodeOf(readErr) != wantCode {
		t.Fatalf("ResourceYAML() failure code = %q, want %q", failure.CodeOf(readErr), wantCode)
	}
	var resourceErr *Error
	if !errors.As(readErr, &resourceErr) || resourceErr.Operation != operation {
		t.Fatalf("ResourceYAML() error = %#v, want operation %q", resourceErr, operation)
	}
}

func TestResourceYAMLBlocksCoreSecrets(t *testing.T) {
	kind := schema.GroupVersionKind{Version: "v1", Kind: "Secret"}
	resource := schema.GroupVersionResource{Version: "v1", Resource: "secrets"}
	singular := schema.GroupVersionResource{Version: "v1", Resource: "secret"}
	manager := managerWithClientsForTest(t, &Clients{mapper: mapperForResource(kind, resource, singular, meta.RESTScopeNamespace)})
	reference := ResourceReference{
		Resource:  schema.GroupResource{Resource: resource.Resource},
		Namespace: "team-a",
		Name:      "credentials",
	}
	payload, err := manager.ResourceYAML(context.Background(), reference)
	if payload != "" || !errors.Is(err, ErrSensitiveResourceAccess) {
		t.Fatalf("ResourceYAML(secret) = (%q, %v), want access denied", payload, err)
	}
}

func TestResourceYAMLReturnsSanitizedYAML(t *testing.T) {
	kind := schema.GroupVersionKind{Version: "v1", Kind: "ConfigMap"}
	resource := schema.GroupVersionResource{Version: "v1", Resource: "configmaps"}
	singular := schema.GroupVersionResource{Version: "v1", Resource: "configmap"}
	configMap := &corev1.ConfigMap{
		TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "ConfigMap"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "settings",
			Namespace: "team-a",
			ManagedFields: []metav1.ManagedFieldsEntry{
				{Manager: "controller", Operation: metav1.ManagedFieldsOperationUpdate, APIVersion: "v1"},
			},
		},
		Data: map[string]string{"mode": "demo"},
	}
	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("AddToScheme() error = %v", err)
	}
	manager := managerWithClientsForTest(t, &Clients{
		mapper:  mapperForResource(kind, resource, singular, meta.RESTScopeNamespace),
		dynamic: dynamicfake.NewSimpleDynamicClient(scheme, configMap),
	})
	payload, err := manager.ResourceYAML(context.Background(), ResourceReference{
		Resource:  schema.GroupResource{Resource: resource.Resource},
		Namespace: configMap.Namespace,
		Name:      configMap.Name,
	})
	if err != nil {
		t.Fatalf("ResourceYAML() error = %v", err)
	}
	if strings.Contains(payload, "managedFields") {
		t.Fatalf("ResourceYAML() retained managed fields:\n%s", payload)
	}
	var decoded corev1.ConfigMap
	if err := yaml.Unmarshal([]byte(payload), &decoded); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if decoded.Name != configMap.Name || decoded.Namespace != configMap.Namespace || decoded.Data["mode"] != "demo" {
		t.Fatalf("decoded ConfigMap = %#v", decoded)
	}
}
