package browser

import (
	"errors"
	"testing"

	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/HediAbed/opsmate/internal/failure"
	"github.com/HediAbed/opsmate/internal/kube"
)

func TestGroupResourceForKind(t *testing.T) {
	tests := []struct {
		kind string
		want schema.GroupResource
	}{
		{kind: resourceKindPod, want: schema.GroupResource{Resource: resourceTypePods}},
		{kind: resourceKindDeployment, want: schema.GroupResource{Group: apiGroupApps, Resource: resourceTypeDeployments}},
		{kind: resourceKindService, want: schema.GroupResource{Resource: resourceTypeServices}},
		{kind: resourceKindStatefulSet, want: schema.GroupResource{Group: apiGroupApps, Resource: resourceTypeStatefulSets}},
		{kind: resourceKindDaemonSet, want: schema.GroupResource{Group: apiGroupApps, Resource: resourceTypeDaemonSets}},
		{kind: resourceKindConfigMap, want: schema.GroupResource{Resource: resourceTypeConfigMaps}},
		{kind: resourceKindNode, want: schema.GroupResource{Resource: resourceTypeNodes}},
		{kind: resourceKindJob, want: schema.GroupResource{Group: apiGroupBatch, Resource: resourceTypeJobs}},
		{kind: resourceKindIngress, want: schema.GroupResource{Group: apiGroupNetworking, Resource: resourceTypeIngresses}},
		{kind: resourceKindNetworkPolicy, want: schema.GroupResource{Group: apiGroupNetworking, Resource: resourceTypeNetworkPolicies}},
		{kind: resourceKindPVC, want: schema.GroupResource{Resource: resourceTypePersistentVolumeClaims}},
		{kind: resourceKindCronJob, want: schema.GroupResource{Group: apiGroupBatch, Resource: resourceTypeCronJobs}},
		{kind: resourceKindHPA, want: schema.GroupResource{Group: apiGroupAutoscaling, Resource: resourceTypeHorizontalAutoscalers}},
		{kind: resourceKindSecret, want: schema.GroupResource{Resource: resourceTypeSecrets}},
		{kind: resourceKindReplicaSet, want: schema.GroupResource{Group: apiGroupApps, Resource: resourceTypeReplicaSets}},
		{kind: resourceKindServiceAccount, want: schema.GroupResource{Resource: resourceTypeServiceAccounts}},
		{kind: resourceKindRole, want: schema.GroupResource{Group: apiGroupRBAC, Resource: resourceTypeRoles}},
		{kind: resourceKindRoleBinding, want: schema.GroupResource{Group: apiGroupRBAC, Resource: resourceTypeRoleBindings}},
		{kind: resourceKindClusterRole, want: schema.GroupResource{Group: apiGroupRBAC, Resource: resourceTypeClusterRoles}},
		{kind: resourceKindClusterRoleBinding, want: schema.GroupResource{Group: apiGroupRBAC, Resource: resourceTypeClusterRoleBindings}},
	}
	for _, test := range tests {
		t.Run(test.kind, func(t *testing.T) {
			got, err := groupResourceForKind(test.kind)
			if err != nil || got != test.want {
				t.Fatalf("groupResourceForKind(%q) = (%v, %v), want %v", test.kind, got, err, test.want)
			}
		})
	}
}

func TestGroupResourceForKindNormalizesAliases(t *testing.T) {
	tests := []struct {
		kind string
		want schema.GroupResource
	}{
		{kind: " PODS ", want: schema.GroupResource{Resource: resourceTypePods}},
		{kind: resourceTypePersistentVolumeClaims, want: schema.GroupResource{Resource: resourceTypePersistentVolumeClaims}},
		{kind: resourceTypePVCs, want: schema.GroupResource{Resource: resourceTypePersistentVolumeClaims}},
		{kind: resourceKindHorizontalAutoscaler, want: schema.GroupResource{Group: apiGroupAutoscaling, Resource: resourceTypeHorizontalAutoscalers}},
		{kind: resourceTypeHPAs, want: schema.GroupResource{Group: apiGroupAutoscaling, Resource: resourceTypeHorizontalAutoscalers}},
	}
	for _, test := range tests {
		t.Run(test.kind, func(t *testing.T) {
			got, err := groupResourceForKind(test.kind)
			if err != nil || got != test.want {
				t.Fatalf("groupResourceForKind(%q) = (%v, %v), want %v", test.kind, got, err, test.want)
			}
		})
	}
}

func TestGroupResourceForKindRejectsInvalidInput(t *testing.T) {
	tests := []struct {
		name    string
		kind    string
		wantErr error
	}{
		{name: "empty", kind: "  ", wantErr: ErrResourceKindRequired},
		{name: "unsupported", kind: "widget", wantErr: ErrUnsupportedResourceKind},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			resource, err := groupResourceForKind(test.kind)
			if resource != (schema.GroupResource{}) || !errors.Is(err, test.wantErr) {
				t.Fatalf("groupResourceForKind(%q) = (%v, %v), want %v", test.kind, resource, err, test.wantErr)
			}
			if failure.CodeOf(err) != failure.CodeInvalidArgument {
				t.Fatalf("failure code = %q, want %q", failure.CodeOf(err), failure.CodeInvalidArgument)
			}
		})
	}
}

func TestResourceKindErrorBehavior(t *testing.T) {
	var nilError *ResourceKindError
	if nilError.Error() != "resource kind is invalid" || nilError.Unwrap() != nil || nilError.FailureCode() != failure.CodeUnknown {
		t.Fatal("nil ResourceKindError behavior is inconsistent")
	}
	zero := &ResourceKindError{}
	if zero.Error() != "resource kind is invalid" {
		t.Fatalf("zero Error() = %q", zero.Error())
	}
	withoutCause := &ResourceKindError{Kind: "widget"}
	if withoutCause.Error() != `resource kind "widget" is invalid` {
		t.Fatalf("Error() = %q", withoutCause.Error())
	}
	missing := &ResourceKindError{Err: ErrResourceKindRequired}
	if missing.Error() != ErrResourceKindRequired.Error() {
		t.Fatalf("Error() = %q", missing.Error())
	}
	unsupported := &ResourceKindError{Kind: "widget", Err: ErrUnsupportedResourceKind}
	if unsupported.Error() != `resource kind "widget": resource kind is not supported` || !errors.Is(unsupported, ErrUnsupportedResourceKind) {
		t.Fatalf("unsupported error = %v", unsupported)
	}
}

func TestWorkloadKindForResource(t *testing.T) {
	tests := []struct {
		name    string
		kind    string
		want    kube.WorkloadKind
		wantErr error
	}{
		{name: "deployment", kind: " Deployment ", want: kube.WorkloadDeployment},
		{name: "deployment plural", kind: resourceTypeDeployments, want: kube.WorkloadDeployment},
		{name: "stateful set", kind: resourceKindStatefulSet, want: kube.WorkloadStatefulSet},
		{name: "stateful set plural", kind: resourceTypeStatefulSets, want: kube.WorkloadStatefulSet},
		{name: "empty", kind: " ", wantErr: ErrResourceKindRequired},
		{name: "unsupported", kind: resourceKindPod, wantErr: ErrUnsupportedResourceKind},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := workloadKindForResource(test.kind)
			if got != test.want || !errors.Is(err, test.wantErr) {
				t.Fatalf("workloadKindForResource(%q) = (%v, %v), want (%v, %v)", test.kind, got, err, test.want, test.wantErr)
			}
		})
	}
}

func TestResourceReferenceForIdentity(t *testing.T) {
	identity := resourceIdentity{Kind: resourceKindDeployment, Namespace: "team-a", Name: "web"}
	reference, err := resourceReferenceForIdentity(identity)
	want := kube.ResourceReference{
		Resource:  schema.GroupResource{Group: apiGroupApps, Resource: resourceTypeDeployments},
		Namespace: identity.Namespace,
		Name:      identity.Name,
	}
	if err != nil || reference != want {
		t.Fatalf("resourceReferenceForIdentity() = (%+v, %v), want %+v", reference, err, want)
	}
	if reference, err := resourceReferenceForIdentity(resourceIdentity{Kind: "widget"}); reference != (kube.ResourceReference{}) || !errors.Is(err, ErrUnsupportedResourceKind) {
		t.Fatalf("resourceReferenceForIdentity(unsupported) = (%+v, %v)", reference, err)
	}
}

func TestWorkloadReferenceForIdentity(t *testing.T) {
	identity := resourceIdentity{Kind: resourceKindStatefulSet, Namespace: "team-a", Name: "database"}
	reference, err := workloadReferenceForIdentity(identity)
	want := kube.WorkloadReference{
		Kind:      kube.WorkloadStatefulSet,
		Namespace: identity.Namespace,
		Name:      identity.Name,
	}
	if err != nil || reference != want {
		t.Fatalf("workloadReferenceForIdentity() = (%+v, %v), want %+v", reference, err, want)
	}
	if reference, err := workloadReferenceForIdentity(resourceIdentity{Kind: resourceKindPod}); reference != (kube.WorkloadReference{}) || !errors.Is(err, ErrUnsupportedResourceKind) {
		t.Fatalf("workloadReferenceForIdentity(unsupported) = (%+v, %v)", reference, err)
	}
}
