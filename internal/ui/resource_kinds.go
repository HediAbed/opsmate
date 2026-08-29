package ui

import (
	"errors"
	"fmt"
	"strings"

	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/HediAbed/opsmate/failure"
	"github.com/HediAbed/opsmate/internal/kube"
)

const (
	resourceKindPod                   = "pod"
	resourceKindDeployment            = "deployment"
	resourceKindService               = "service"
	resourceKindStatefulSet           = "statefulset"
	resourceKindDaemonSet             = "daemonset"
	resourceKindConfigMap             = "configmap"
	resourceKindNode                  = "node"
	resourceKindJob                   = "job"
	resourceKindIngress               = "ingress"
	resourceKindNetworkPolicy         = "networkpolicy"
	resourceKindPersistentVolumeClaim = "persistentvolumeclaim"
	resourceKindPVC                   = "pvc"
	resourceKindCronJob               = "cronjob"
	resourceKindHorizontalAutoscaler  = "horizontalpodautoscaler"
	resourceKindHPA                   = "hpa"
	resourceKindSecret                = "secret"
	resourceKindReplicaSet            = "replicaset"
	resourceKindServiceAccount        = "serviceaccount"
	resourceKindRole                  = "role"
	resourceKindRoleBinding           = "rolebinding"
	resourceKindClusterRole           = "clusterrole"
	resourceKindClusterRoleBinding    = "clusterrolebinding"
	resourceKindRBAC                  = "rbac"
)

const (
	resourceTypePods                   = "pods"
	resourceTypeDeployments            = "deployments"
	resourceTypeServices               = "services"
	resourceTypeStatefulSets           = "statefulsets"
	resourceTypeDaemonSets             = "daemonsets"
	resourceTypeConfigMaps             = "configmaps"
	resourceTypeNodes                  = "nodes"
	resourceTypeJobs                   = "jobs"
	resourceTypeIngresses              = "ingresses"
	resourceTypeNetworkPolicies        = "networkpolicies"
	resourceTypePersistentVolumeClaims = "persistentvolumeclaims"
	resourceTypePVCs                   = "pvcs"
	resourceTypeCronJobs               = "cronjobs"
	resourceTypeHorizontalAutoscalers  = "horizontalpodautoscalers"
	resourceTypeHPAs                   = "hpas"
	resourceTypeSecrets                = "secrets"
	resourceTypeReplicaSets            = "replicasets"
	resourceTypeServiceAccounts        = "serviceaccounts"
	resourceTypeRoles                  = "roles"
	resourceTypeRoleBindings           = "rolebindings"
	resourceTypeClusterRoles           = "clusterroles"
	resourceTypeClusterRoleBindings    = "clusterrolebindings"
	resourceTypeRBAC                   = "rbac"
)

const (
	apiGroupApps        = "apps"
	apiGroupBatch       = "batch"
	apiGroupNetworking  = "networking.k8s.io"
	apiGroupAutoscaling = "autoscaling"
	apiGroupRBAC        = "rbac.authorization.k8s.io"
)

var (
	ErrResourceKindRequired    = errors.New("resource kind is required")
	ErrUnsupportedResourceKind = errors.New("resource kind is not supported")
	ErrMixedResourceSelection  = errors.New("batch actions require one resource kind and namespace")
)

type ResourceKindError struct {
	Kind string
	Err  error
}

func (e *ResourceKindError) Error() string {
	if e == nil {
		return "resource kind is invalid"
	}
	if e.Err == nil {
		if e.Kind == "" {
			return "resource kind is invalid"
		}
		return fmt.Sprintf("resource kind %q is invalid", e.Kind)
	}
	if e.Kind == "" {
		return e.Err.Error()
	}
	return fmt.Sprintf("resource kind %q: %v", e.Kind, e.Err)
}

func (e *ResourceKindError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

func (e *ResourceKindError) FailureCode() failure.Code {
	if e == nil {
		return failure.CodeUnknown
	}
	return failure.CodeInvalidArgument
}

func groupResourceForKind(kind string) (schema.GroupResource, error) {
	normalizedKind := strings.ToLower(strings.TrimSpace(kind))
	if normalizedKind == "" {
		return schema.GroupResource{}, &ResourceKindError{Err: ErrResourceKindRequired}
	}
	resource, found := knownGroupResource(normalizedKind)
	if !found {
		return schema.GroupResource{}, &ResourceKindError{Kind: normalizedKind, Err: ErrUnsupportedResourceKind}
	}
	return resource, nil
}

func knownGroupResource(kind string) (schema.GroupResource, bool) {
	if resource, found := knownCoreGroupResource(kind); found {
		return resource, true
	}
	if resource, found := knownAppsGroupResource(kind); found {
		return resource, true
	}
	if resource, found := knownWorkloadGroupResource(kind); found {
		return resource, true
	}
	if resource, found := knownNetworkGroupResource(kind); found {
		return resource, true
	}
	return knownRBACGroupResource(kind)
}

func knownCoreGroupResource(kind string) (schema.GroupResource, bool) {
	switch kind {
	case resourceKindPod, resourceTypePods:
		return schema.GroupResource{Resource: resourceTypePods}, true
	case resourceKindService, resourceTypeServices:
		return schema.GroupResource{Resource: resourceTypeServices}, true
	case resourceKindConfigMap, resourceTypeConfigMaps:
		return schema.GroupResource{Resource: resourceTypeConfigMaps}, true
	case resourceKindNode, resourceTypeNodes:
		return schema.GroupResource{Resource: resourceTypeNodes}, true
	case resourceKindPVC, resourceKindPersistentVolumeClaim, resourceTypePVCs, resourceTypePersistentVolumeClaims:
		return schema.GroupResource{Resource: resourceTypePersistentVolumeClaims}, true
	case resourceKindSecret, resourceTypeSecrets:
		return schema.GroupResource{Resource: resourceTypeSecrets}, true
	case resourceKindServiceAccount, resourceTypeServiceAccounts:
		return schema.GroupResource{Resource: resourceTypeServiceAccounts}, true
	default:
		return schema.GroupResource{}, false
	}
}

func knownAppsGroupResource(kind string) (schema.GroupResource, bool) {
	switch kind {
	case resourceKindDeployment, resourceTypeDeployments:
		return schema.GroupResource{Group: apiGroupApps, Resource: resourceTypeDeployments}, true
	case resourceKindStatefulSet, resourceTypeStatefulSets:
		return schema.GroupResource{Group: apiGroupApps, Resource: resourceTypeStatefulSets}, true
	case resourceKindDaemonSet, resourceTypeDaemonSets:
		return schema.GroupResource{Group: apiGroupApps, Resource: resourceTypeDaemonSets}, true
	case resourceKindReplicaSet, resourceTypeReplicaSets:
		return schema.GroupResource{Group: apiGroupApps, Resource: resourceTypeReplicaSets}, true
	default:
		return schema.GroupResource{}, false
	}
}

func knownWorkloadGroupResource(kind string) (schema.GroupResource, bool) {
	switch kind {
	case resourceKindJob, resourceTypeJobs:
		return schema.GroupResource{Group: apiGroupBatch, Resource: resourceTypeJobs}, true
	case resourceKindCronJob, resourceTypeCronJobs:
		return schema.GroupResource{Group: apiGroupBatch, Resource: resourceTypeCronJobs}, true
	case resourceKindHPA, resourceKindHorizontalAutoscaler, resourceTypeHPAs, resourceTypeHorizontalAutoscalers:
		return schema.GroupResource{Group: apiGroupAutoscaling, Resource: resourceTypeHorizontalAutoscalers}, true
	default:
		return schema.GroupResource{}, false
	}
}

func knownNetworkGroupResource(kind string) (schema.GroupResource, bool) {
	switch kind {
	case resourceKindIngress, resourceTypeIngresses:
		return schema.GroupResource{Group: apiGroupNetworking, Resource: resourceTypeIngresses}, true
	case resourceKindNetworkPolicy, resourceTypeNetworkPolicies:
		return schema.GroupResource{Group: apiGroupNetworking, Resource: resourceTypeNetworkPolicies}, true
	default:
		return schema.GroupResource{}, false
	}
}

func knownRBACGroupResource(kind string) (schema.GroupResource, bool) {
	switch kind {
	case resourceKindRole, resourceTypeRoles:
		return schema.GroupResource{Group: apiGroupRBAC, Resource: resourceTypeRoles}, true
	case resourceKindRoleBinding, resourceTypeRoleBindings:
		return schema.GroupResource{Group: apiGroupRBAC, Resource: resourceTypeRoleBindings}, true
	case resourceKindClusterRole, resourceTypeClusterRoles:
		return schema.GroupResource{Group: apiGroupRBAC, Resource: resourceTypeClusterRoles}, true
	case resourceKindClusterRoleBinding, resourceTypeClusterRoleBindings:
		return schema.GroupResource{Group: apiGroupRBAC, Resource: resourceTypeClusterRoleBindings}, true
	default:
		return schema.GroupResource{}, false
	}
}

func workloadKindForResource(kind string) (kube.WorkloadKind, error) {
	normalizedKind := strings.ToLower(strings.TrimSpace(kind))
	switch normalizedKind {
	case resourceKindDeployment, resourceTypeDeployments:
		return kube.WorkloadDeployment, nil
	case resourceKindStatefulSet, resourceTypeStatefulSets:
		return kube.WorkloadStatefulSet, nil
	case "":
		return 0, &ResourceKindError{Err: ErrResourceKindRequired}
	default:
		return 0, &ResourceKindError{Kind: normalizedKind, Err: ErrUnsupportedResourceKind}
	}
}

func resourceReferenceForIdentity(identity resourceIdentity) (kube.ResourceReference, error) {
	resource, err := groupResourceForKind(identity.Kind)
	if err != nil {
		return kube.ResourceReference{}, err
	}
	return kube.ResourceReference{
		Resource:  resource,
		Namespace: identity.Namespace,
		Name:      identity.Name,
	}, nil
}

func workloadReferenceForIdentity(identity resourceIdentity) (kube.WorkloadReference, error) {
	kind, err := workloadKindForResource(identity.Kind)
	if err != nil {
		return kube.WorkloadReference{}, err
	}
	return kube.WorkloadReference{
		Kind:      kind,
		Namespace: identity.Namespace,
		Name:      identity.Name,
	}, nil
}
