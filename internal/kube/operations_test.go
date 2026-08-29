package kube

import (
	"errors"
	"testing"

	"k8s.io/apimachinery/pkg/runtime/schema"
)

var invalidWorkloadKind = WorkloadStatefulSet + 1

func TestResourceReferenceIdentifier(t *testing.T) {
	tests := []struct {
		name      string
		reference ResourceReference
		want      string
	}{
		{name: "cluster scoped", reference: ResourceReference{Resource: schema.GroupResource{Resource: "nodes"}, Name: "worker-1"}, want: "nodes/worker-1"},
		{name: "namespaced", reference: ResourceReference{Resource: schema.GroupResource{Resource: "pods"}, Namespace: "team-a", Name: "web"}, want: "team-a/pods/web"},
		{name: "namespaced with group", reference: ResourceReference{Resource: schema.GroupResource{Group: "apps", Resource: "deployments"}, Namespace: "team-a", Name: "web"}, want: "team-a/deployments.apps/web"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := test.reference.Identifier(); got != test.want {
				t.Fatalf("Identifier() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestPodReferenceIdentifier(t *testing.T) {
	tests := []struct {
		name      string
		reference PodReference
		want      string
	}{
		{name: "without namespace", reference: PodReference{Name: "web-0"}, want: "pod/web-0"},
		{name: "with namespace", reference: PodReference{Namespace: "team-a", Name: "web-0"}, want: "team-a/pod/web-0"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := test.reference.Identifier(); got != test.want {
				t.Fatalf("Identifier() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestWorkloadKindString(t *testing.T) {
	tests := []struct {
		name string
		kind WorkloadKind
		want string
	}{
		{name: "deployment", kind: WorkloadDeployment, want: "deployment"},
		{name: "stateful set", kind: WorkloadStatefulSet, want: "statefulset"},
		{name: "zero value", kind: WorkloadKind(0), want: "workload"},
		{name: "unknown value", kind: invalidWorkloadKind, want: "workload"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := test.kind.String(); got != test.want {
				t.Fatalf("String() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestWorkloadReferenceIdentifier(t *testing.T) {
	tests := []struct {
		name      string
		reference WorkloadReference
		want      string
	}{
		{name: "without namespace", reference: WorkloadReference{Kind: WorkloadDeployment, Name: "web"}, want: "deployment/web"},
		{name: "with namespace", reference: WorkloadReference{Kind: WorkloadStatefulSet, Namespace: "team-a", Name: "db"}, want: "team-a/statefulset/db"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := test.reference.Identifier(); got != test.want {
				t.Fatalf("Identifier() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestBatchOutcomeFailure(t *testing.T) {
	first := errors.New("first failed")
	second := errors.New("second failed")

	t.Run("no failures", func(t *testing.T) {
		outcome := BatchOutcome{Succeeded: []string{"web"}}
		if err := outcome.Failure(); err != nil {
			t.Fatalf("Failure() = %v, want nil", err)
		}
	})

	t.Run("single failure", func(t *testing.T) {
		outcome := BatchOutcome{Failed: []BatchFailure{{Name: "web", Err: first}}}
		err := outcome.Failure()
		if !errors.Is(err, first) {
			t.Fatalf("errors.Is(%v, first) = false", err)
		}
		if err.Error() != "first failed" {
			t.Fatalf("Failure().Error() = %q, want %q", err.Error(), "first failed")
		}
	})

	t.Run("multiple failures", func(t *testing.T) {
		outcome := BatchOutcome{
			Succeeded: []string{"api"},
			Failed: []BatchFailure{
				{Name: "web", Err: first},
				{Name: "db", Err: second},
			},
		}
		err := outcome.Failure()
		if !errors.Is(err, first) || !errors.Is(err, second) {
			t.Fatalf("Failure() = %v, want both causes joined", err)
		}
		if err.Error() != "first failed\nsecond failed" {
			t.Fatalf("Failure().Error() = %q, want %q", err.Error(), "first failed\nsecond failed")
		}
	})
}

func TestValidateResourceReference(t *testing.T) {
	tests := []struct {
		name      string
		reference ResourceReference
		want      error
	}{
		{name: "missing resource", reference: ResourceReference{Name: "web"}, want: ErrResourceIdentifierRequired},
		{name: "whitespace resource", reference: ResourceReference{Resource: schema.GroupResource{Resource: "   "}, Name: "web"}, want: ErrResourceIdentifierRequired},
		{name: "missing name", reference: ResourceReference{Resource: schema.GroupResource{Resource: "pods"}}, want: ErrResourceNameRequired},
		{name: "whitespace name", reference: ResourceReference{Resource: schema.GroupResource{Resource: "pods"}, Name: " \t"}, want: ErrResourceNameRequired},
		{name: "valid cluster scoped", reference: ResourceReference{Resource: schema.GroupResource{Resource: "nodes"}, Name: "worker-1"}},
		{name: "valid namespaced", reference: ResourceReference{Resource: schema.GroupResource{Resource: "pods"}, Namespace: "team-a", Name: "web"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := validateResourceReference(test.reference); !errors.Is(got, test.want) {
				t.Fatalf("validateResourceReference() = %v, want %v", got, test.want)
			}
		})
	}
}

func TestValidatePodReference(t *testing.T) {
	tests := []struct {
		name      string
		reference PodReference
		want      error
	}{
		{name: "missing namespace", reference: PodReference{Name: "web-0"}, want: ErrNamespaceRequired},
		{name: "whitespace namespace", reference: PodReference{Namespace: "  ", Name: "web-0"}, want: ErrNamespaceRequired},
		{name: "missing name", reference: PodReference{Namespace: "team-a"}, want: ErrPodNameRequired},
		{name: "whitespace name", reference: PodReference{Namespace: "team-a", Name: "\t"}, want: ErrPodNameRequired},
		{name: "valid", reference: PodReference{Namespace: "team-a", Name: "web-0"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := validatePodReference(test.reference); !errors.Is(got, test.want) {
				t.Fatalf("validatePodReference() = %v, want %v", got, test.want)
			}
		})
	}
}

func TestValidateWorkloadReference(t *testing.T) {
	tests := []struct {
		name      string
		reference WorkloadReference
		want      error
	}{
		{name: "zero kind", reference: WorkloadReference{Namespace: "team-a", Name: "web"}, want: ErrUnsupportedWorkloadKind},
		{name: "unknown kind", reference: WorkloadReference{Kind: invalidWorkloadKind, Namespace: "team-a", Name: "web"}, want: ErrUnsupportedWorkloadKind},
		{name: "missing namespace", reference: WorkloadReference{Kind: WorkloadDeployment, Name: "web"}, want: ErrNamespaceRequired},
		{name: "whitespace namespace", reference: WorkloadReference{Kind: WorkloadDeployment, Namespace: " ", Name: "web"}, want: ErrNamespaceRequired},
		{name: "missing name", reference: WorkloadReference{Kind: WorkloadStatefulSet, Namespace: "team-a"}, want: ErrResourceNameRequired},
		{name: "whitespace name", reference: WorkloadReference{Kind: WorkloadStatefulSet, Namespace: "team-a", Name: "  "}, want: ErrResourceNameRequired},
		{name: "valid deployment", reference: WorkloadReference{Kind: WorkloadDeployment, Namespace: "team-a", Name: "web"}},
		{name: "valid stateful set", reference: WorkloadReference{Kind: WorkloadStatefulSet, Namespace: "team-a", Name: "db"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := validateWorkloadReference(test.reference); !errors.Is(got, test.want) {
				t.Fatalf("validateWorkloadReference() = %v, want %v", got, test.want)
			}
		})
	}
}

func TestValidateBatch(t *testing.T) {
	pods := schema.GroupResource{Resource: "pods"}
	tests := []struct {
		name        string
		resource    schema.GroupResource
		names       []string
		want        error
		wantMessage string
	}{
		{name: "missing resource", resource: schema.GroupResource{}, names: []string{"web"}, want: ErrResourceIdentifierRequired},
		{name: "whitespace resource", resource: schema.GroupResource{Resource: "  "}, names: []string{"web"}, want: ErrResourceIdentifierRequired},
		{name: "nil names", resource: pods, want: ErrResourceNamesRequired},
		{name: "empty names", resource: pods, names: []string{}, want: ErrResourceNamesRequired},
		{name: "empty name entry", resource: pods, names: []string{"web", ""}, want: ErrResourceNameRequired, wantMessage: "kubernetes resource name is required at index 1"},
		{name: "whitespace name entry", resource: pods, names: []string{" \t", "web"}, want: ErrResourceNameRequired, wantMessage: "kubernetes resource name is required at index 0"},
		{name: "duplicate name", resource: pods, names: []string{"web", "api", "web"}, want: ErrDuplicateResourceName, wantMessage: "kubernetes resource names must be unique: web"},
		{name: "valid single", resource: pods, names: []string{"web"}},
		{name: "valid multiple", resource: pods, names: []string{"web", "api"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := validateBatch(test.resource, test.names)
			if !errors.Is(got, test.want) {
				t.Fatalf("validateBatch() = %v, want %v", got, test.want)
			}
			if test.wantMessage != "" && got.Error() != test.wantMessage {
				t.Fatalf("validateBatch().Error() = %q, want %q", got.Error(), test.wantMessage)
			}
		})
	}
}

func TestValidateWorkloadBatch(t *testing.T) {
	tests := []struct {
		name        string
		batch       WorkloadBatch
		want        error
		wantMessage string
	}{
		{name: "zero kind", batch: WorkloadBatch{Namespace: "team-a", Names: []string{"web"}}, want: ErrUnsupportedWorkloadKind},
		{name: "unknown kind", batch: WorkloadBatch{Kind: invalidWorkloadKind, Namespace: "team-a", Names: []string{"web"}}, want: ErrUnsupportedWorkloadKind},
		{name: "missing namespace", batch: WorkloadBatch{Kind: WorkloadDeployment, Names: []string{"web"}}, want: ErrNamespaceRequired},
		{name: "whitespace namespace", batch: WorkloadBatch{Kind: WorkloadDeployment, Namespace: "\t", Names: []string{"web"}}, want: ErrNamespaceRequired},
		{name: "missing names", batch: WorkloadBatch{Kind: WorkloadDeployment, Namespace: "team-a"}, want: ErrResourceNamesRequired},
		{name: "whitespace name entry", batch: WorkloadBatch{Kind: WorkloadDeployment, Namespace: "team-a", Names: []string{"web", "  "}}, want: ErrResourceNameRequired, wantMessage: "kubernetes resource name is required at index 1"},
		{name: "duplicate name", batch: WorkloadBatch{Kind: WorkloadStatefulSet, Namespace: "team-a", Names: []string{"db", "db"}}, want: ErrDuplicateResourceName, wantMessage: "kubernetes resource names must be unique: db"},
		{name: "valid deployment batch", batch: WorkloadBatch{Kind: WorkloadDeployment, Namespace: "team-a", Names: []string{"web", "api"}}},
		{name: "valid stateful set batch", batch: WorkloadBatch{Kind: WorkloadStatefulSet, Namespace: "team-a", Names: []string{"db"}}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := validateWorkloadBatch(test.batch)
			if !errors.Is(got, test.want) {
				t.Fatalf("validateWorkloadBatch() = %v, want %v", got, test.want)
			}
			if test.wantMessage != "" && got.Error() != test.wantMessage {
				t.Fatalf("validateWorkloadBatch().Error() = %q, want %q", got.Error(), test.wantMessage)
			}
		})
	}
}
