package cluster

import "time"

type NamespacesMsg struct {
	Namespaces []string
	Err        error
}

type Pod struct {
	Name       string
	Namespace  string
	Status     string
	Ready      string
	Restarts   int
	Age        string
	CPU        string
	Memory     string
	Node       string
	IP         string
	Containers []string
}

type PodsMsg struct {
	Pods []Pod
	Err  error
}

type Deployment struct {
	Name       string
	Namespace  string
	Ready      string
	UpToDate   int
	Available  int
	Age        string
	Containers []string
	Images     []string
	Selector   string
}

type DeploymentsMsg struct {
	Deployments []Deployment
	Err         error
}

type Event struct {
	Name          string
	UID           string
	Namespace     string
	Type          string
	Reason        string
	Object        string
	Message       string
	Age           string
	Count         int
	LastTimestamp time.Time
}

type EventsMsg struct {
	Events []Event
	Err    error
}

type DescribeMsg struct {
	Output string
	Err    error
}

type LogsMsg struct {
	Lines []string
	Err   error
}

type ContainersMsg struct {
	Containers []string
	Err        error
}

type YAMLMsg struct {
	Output string
	Err    error
}

type MutationResultMsg struct {
	Output string
	Err    error
}

type MetricsMsg struct {
	PodMetrics []PodMetric
	Err        error
}

type PodMetric struct {
	Name      string
	Namespace string
	CPU       string
	Memory    string
}

type Service struct {
	Name       string
	Namespace  string
	Type       string
	ClusterIP  string
	ExternalIP string
	Ports      string
	Age        string
	Selector   string
}

type ServicesMsg struct {
	Services []Service
	Err      error
}

type StatefulSet struct {
	Name      string
	Namespace string
	Ready     string
	Replicas  int
	Age       string
}

type StatefulSetsMsg struct {
	StatefulSets []StatefulSet
	Err          error
}

type DaemonSet struct {
	Name      string
	Namespace string
	Desired   int
	Current   int
	Ready     int
	Available int
	Age       string
}

type DaemonSetsMsg struct {
	DaemonSets []DaemonSet
	Err        error
}

type ConfigMap struct {
	Name      string
	Namespace string
	Data      int
	Age       string
}

type ConfigMapsMsg struct {
	ConfigMaps []ConfigMap
	Err        error
}

type KubeContext struct {
	Name      string
	Cluster   string
	Namespace string
	Current   bool
}

type ContextsMsg struct {
	Contexts []KubeContext
	Err      error
}

type ContextSwitchedMsg struct {
	Name string
	Err  error
}

type CurrentContextMsg struct {
	Name string
	Err  error
}

type Node struct {
	Name       string
	Status     string
	Roles      string
	Version    string
	Age        string
	InternalIP string
	OSImage    string
	Kernel     string
	Runtime    string
}

type NodesMsg struct {
	Nodes []Node
	Err   error
}

type Job struct {
	Name        string
	Namespace   string
	Completions string
	Duration    string
	Status      string
	Age         string
}

type JobsMsg struct {
	Jobs []Job
	Err  error
}

type Ingress struct {
	Name      string
	Namespace string
	Class     string
	Hosts     string
	Address   string
	Ports     string
	Age       string
}

type IngressesMsg struct {
	Ingresses []Ingress
	Err       error
}

type NetworkPolicy struct {
	Name        string
	Namespace   string
	PodSelector map[string]string
	PolicyTypes []string
	Age         string
}

type NetworkPoliciesMsg struct {
	NetworkPolicies []NetworkPolicy
	Err             error
}

type PersistentVolumeClaim struct {
	Name         string
	Namespace    string
	Status       string
	Volume       string
	Capacity     string
	AccessModes  []string
	StorageClass string
	Age          string
}

type PVCsMsg struct {
	PVCs []PersistentVolumeClaim
	Err  error
}

type CronJob struct {
	Name         string
	Namespace    string
	Schedule     string
	Suspend      bool
	Active       int
	LastSchedule string
	Age          string
}

type CronJobsMsg struct {
	CronJobs []CronJob
	Err      error
}

type ScaleTargetRef struct {
	Kind string
	Name string
}

func (r ScaleTargetRef) String() string {
	if r.Kind == "" || r.Name == "" {
		return ""
	}
	return r.Kind + "/" + r.Name
}

type HPAMetricPair struct {
	Current string
	Target  string
}

func (p HPAMetricPair) String() string {
	if p.Current == "" {
		return p.Target
	}
	if p.Target == "" {
		return p.Current
	}
	return p.Current + "/" + p.Target
}

type HPA struct {
	Name        string
	Namespace   string
	Reference   ScaleTargetRef
	Targets     []HPAMetricPair
	MinReplicas int
	MaxReplicas int
	Replicas    int
	Age         string
}

type HPAsMsg struct {
	HPAs []HPA
	Err  error
}

type Secret struct {
	Name      string
	Namespace string
	Type      string
	Data      int
	Age       string
}

type SecretsMsg struct {
	Secrets []Secret
	Err     error
}

type ReplicaSet struct {
	Name      string
	Namespace string
	Desired   int
	Current   int
	Ready     int
	Age       string
}

type ReplicaSetsMsg struct {
	ReplicaSets []ReplicaSet
	Err         error
}

type RBAC struct {
	Kind      string
	Name      string
	Namespace string
	Count     int
	Scope     string
	Age       string
}

type RBACMsg struct {
	RBAC []RBAC
	Err  error
}
