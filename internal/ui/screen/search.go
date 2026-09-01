package screen

type ResourceKind string

const (
	ResourceKindPod           ResourceKind = "pod"
	ResourceKindDeployment    ResourceKind = "deployment"
	ResourceKindService       ResourceKind = "service"
	ResourceKindStatefulSet   ResourceKind = "statefulset"
	ResourceKindDaemonSet     ResourceKind = "daemonset"
	ResourceKindConfigMap     ResourceKind = "configmap"
	ResourceKindNode          ResourceKind = "node"
	ResourceKindJob           ResourceKind = "job"
	ResourceKindIngress       ResourceKind = "ingress"
	ResourceKindNetworkPolicy ResourceKind = "networkpolicy"
	ResourceKindPVC           ResourceKind = "pvc"
	ResourceKindCronJob       ResourceKind = "cronjob"
	ResourceKindHPA           ResourceKind = "hpa"
	ResourceKindSecret        ResourceKind = "secret"
	ResourceKindReplicaSet    ResourceKind = "replicaset"
	ResourceKindRBAC          ResourceKind = "rbac"
)

func (kind ResourceKind) Valid() bool {
	switch kind {
	case ResourceKindPod,
		ResourceKindDeployment,
		ResourceKindService,
		ResourceKindStatefulSet,
		ResourceKindDaemonSet,
		ResourceKindConfigMap,
		ResourceKindNode,
		ResourceKindJob,
		ResourceKindIngress,
		ResourceKindNetworkPolicy,
		ResourceKindPVC,
		ResourceKindCronJob,
		ResourceKindHPA,
		ResourceKindSecret,
		ResourceKindReplicaSet,
		ResourceKindRBAC:
		return true
	default:
		return false
	}
}

type SearchItem struct {
	Kind      ResourceKind
	Name      string
	Namespace string
}

func (item SearchItem) Valid() bool {
	return item.Kind.Valid() && item.Name != ""
}
