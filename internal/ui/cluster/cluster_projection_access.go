package cluster

import (
	"time"

	model "github.com/HediAbed/opsmate/internal/cluster"
	"github.com/HediAbed/opsmate/internal/kube"
	appsv1 "k8s.io/api/apps/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func projectSecrets(items []kube.ResourceMetadata, now time.Time) []model.Secret {
	return projectSlice(items, now, func(item kube.ResourceMetadata, now time.Time) model.Secret {
		return model.Secret{
			Name:      item.Name,
			Namespace: item.Namespace,
			Type:      metadataOnlyLabel,
			Age:       projectedAge(now, item.CreatedAt),
		}
	})
}

func projectCRDInstances(items []kube.ResourceMetadata, now time.Time) []model.CRDInstance {
	return projectSlice(items, now, func(item kube.ResourceMetadata, now time.Time) model.CRDInstance {
		return model.CRDInstance{
			Name:      item.Name,
			Namespace: item.Namespace,
			Age:       projectedAge(now, item.CreatedAt),
		}
	})
}

func projectReplicaSets(items []appsv1.ReplicaSet, now time.Time) []model.ReplicaSet {
	return projectSlice(items, now, func(item appsv1.ReplicaSet, now time.Time) model.ReplicaSet {
		desired := int32(0)
		if item.Spec.Replicas != nil {
			desired = *item.Spec.Replicas
		}
		return model.ReplicaSet{
			Name:      item.Name,
			Namespace: item.Namespace,
			Desired:   int(desired),
			Current:   int(item.Status.Replicas),
			Ready:     int(item.Status.ReadyReplicas),
			Age:       projectedAge(now, item.CreationTimestamp.Time),
		}
	})
}

func projectRBAC(resources kube.RBACResources, now time.Time) []model.RBAC {
	capacity := len(resources.ServiceAccounts) + len(resources.Roles) + len(resources.RoleBindings) + len(resources.ClusterRoles) + len(resources.ClusterRoleBindings)
	items := make([]model.RBAC, 0, capacity)
	for index := range resources.ServiceAccounts {
		items = append(items, projectedRBACRow("ServiceAccount", &resources.ServiceAccounts[index], 0, "Namespace", now))
	}
	for index := range resources.Roles {
		items = append(items, projectedRBACRow("Role", &resources.Roles[index], len(resources.Roles[index].Rules), "Namespace", now))
	}
	for index := range resources.RoleBindings {
		items = append(items, projectedRBACRow("RoleBinding", &resources.RoleBindings[index], len(resources.RoleBindings[index].Subjects), "Namespace", now))
	}
	for index := range resources.ClusterRoles {
		items = append(items, projectedRBACRow("ClusterRole", &resources.ClusterRoles[index], len(resources.ClusterRoles[index].Rules), "Cluster", now))
	}
	for index := range resources.ClusterRoleBindings {
		items = append(items, projectedRBACRow("ClusterRoleBinding", &resources.ClusterRoleBindings[index], len(resources.ClusterRoleBindings[index].Subjects), "Cluster", now))
	}
	return items
}

func projectedRBACRow(kind string, object metav1.Object, count int, scope string, now time.Time) model.RBAC {
	return model.RBAC{
		Kind:      kind,
		Name:      object.GetName(),
		Namespace: object.GetNamespace(),
		Count:     count,
		Scope:     scope,
		Age:       projectedAge(now, object.GetCreationTimestamp().Time),
	}
}

func projectCRDs(items []apiextensionsv1.CustomResourceDefinition, now time.Time) []model.CRD {
	return projectSlice(items, now, projectCRD)
}

func projectCRD(item apiextensionsv1.CustomResourceDefinition, now time.Time) model.CRD {
	versions, preferredVersion := projectCRDVersions(item.Spec.Versions)
	resourceName := ""
	if item.Spec.Names.Plural != "" && item.Spec.Group != "" {
		resourceName = item.Spec.Names.Plural + "." + item.Spec.Group
	}
	return model.CRD{
		Name:             item.Name,
		Group:            item.Spec.Group,
		Plural:           item.Spec.Names.Plural,
		Singular:         item.Spec.Names.Singular,
		Kind:             item.Spec.Names.Kind,
		Scope:            string(item.Spec.Scope),
		Versions:         versions,
		PreferredVersion: preferredVersion,
		Resource:         resourceName,
		Age:              projectedAge(now, item.CreationTimestamp.Time),
	}
}

func projectCRDVersions(definitions []apiextensionsv1.CustomResourceDefinitionVersion) ([]string, string) {
	versions := make([]string, 0, len(definitions))
	preferredVersion := ""
	for _, definition := range definitions {
		if definition.Served {
			versions = append(versions, definition.Name)
		}
		if definition.Storage {
			preferredVersion = definition.Name
		}
	}
	if preferredVersion == "" && len(versions) > 0 {
		preferredVersion = versions[0]
	}
	return versions, preferredVersion
}
