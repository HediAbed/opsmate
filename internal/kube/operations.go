package kube

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"k8s.io/apimachinery/pkg/runtime/schema"
)

const (
	minimumNetworkPort = 1
	maximumNetworkPort = 65535
)

type ResourceReference struct {
	Resource  schema.GroupResource
	Namespace string
	Name      string
}

func (r ResourceReference) Identifier() string {
	resource := r.Resource.String()
	if r.Namespace == "" {
		return resource + "/" + r.Name
	}
	return r.Namespace + "/" + resource + "/" + r.Name
}

type ResourceBatch struct {
	Resource  schema.GroupResource
	Namespace string
	Names     []string
}

type PodReference struct {
	Namespace string
	Name      string
}

func (r PodReference) Identifier() string {
	if r.Namespace == "" {
		return "pod/" + r.Name
	}
	return r.Namespace + "/pod/" + r.Name
}

type PodLogRequest struct {
	Pod       PodReference
	Container string
	TailLines int64
}

type ShellRequest struct {
	Pod       PodReference
	Container string
}

type ShellIdentity struct {
	ID        string
	Pod       PodReference
	Container string
}

type ShellOutput struct {
	SessionID string
	Line      string
	Stderr    bool
}

type ShellExit struct {
	SessionID string
	Err       error
}

type ShellSession interface {
	Identity() ShellIdentity
	Send(string) error
	Output() <-chan ShellOutput
	Exit() <-chan ShellExit
	Interrupt() error
	Close()
}

type NetworkPort struct {
	value uint16
}

func NewNetworkPort(value int) (NetworkPort, error) {
	if value < minimumNetworkPort || value > maximumNetworkPort {
		return NetworkPort{}, fmt.Errorf("%w: must be between %d and %d", ErrNetworkPortInvalid, minimumNetworkPort, maximumNetworkPort)
	}
	return NetworkPort{value: uint16(value)}, nil
}

func (p NetworkPort) Int() int {
	return int(p.value)
}

func (p NetworkPort) String() string {
	return fmt.Sprintf("%d", p.value)
}

type PortForwardRequest struct {
	Pod        PodReference
	LocalPort  NetworkPort
	RemotePort NetworkPort
}

type PortForwardStatus uint8

const (
	PortForwardRunning PortForwardStatus = iota + 1
)

func (s PortForwardStatus) String() string {
	if s == PortForwardRunning {
		return "running"
	}
	return "unknown"
}

type PortForwardSession struct {
	ID         string
	Pod        PodReference
	LocalPort  NetworkPort
	RemotePort NetworkPort
	StartedAt  time.Time
	Status     PortForwardStatus
}

type PortForwardExit struct {
	SessionID string
	Err       error
}

type PortForward interface {
	Session() PortForwardSession
	Exit() <-chan PortForwardExit
}

type HelmRelease struct {
	Name         string
	Namespace    string
	Revision     int
	Status       string
	ChartName    string
	ChartVersion string
	AppVersion   string
	UpdatedAt    time.Time
}

func (r HelmRelease) Identifier() string {
	if r.Namespace == "" {
		return "helmrelease/" + r.Name
	}
	return r.Namespace + "/helmrelease/" + r.Name
}

func (r HelmRelease) ChartLabel() string {
	switch {
	case r.ChartName == "":
		return r.ChartVersion
	case r.ChartVersion == "":
		return r.ChartName
	default:
		return r.ChartName + "-" + r.ChartVersion
	}
}

type HelmReleaseReference struct {
	Namespace string
	Name      string
}

func (r HelmReleaseReference) Identifier() string {
	if r.Namespace == "" {
		return "helmrelease/" + r.Name
	}
	return r.Namespace + "/helmrelease/" + r.Name
}

type WorkloadKind uint8

const (
	WorkloadDeployment WorkloadKind = iota + 1
	WorkloadStatefulSet
)

func (k WorkloadKind) String() string {
	switch k {
	case WorkloadDeployment:
		return "deployment"
	case WorkloadStatefulSet:
		return "statefulset"
	default:
		return "workload"
	}
}

type WorkloadReference struct {
	Kind      WorkloadKind
	Namespace string
	Name      string
}

func (r WorkloadReference) Identifier() string {
	if r.Namespace == "" {
		return r.Kind.String() + "/" + r.Name
	}
	return r.Namespace + "/" + r.Kind.String() + "/" + r.Name
}

type WorkloadBatch struct {
	Kind      WorkloadKind
	Namespace string
	Names     []string
}

type ScaleRequest struct {
	Workload WorkloadReference
	Replicas int32
}

type BatchFailure struct {
	Name string
	Err  error
}

type BatchOutcome struct {
	Succeeded []string
	Failed    []BatchFailure
}

func (o BatchOutcome) Failure() error {
	failures := make([]error, 0, len(o.Failed))
	for _, failure := range o.Failed {
		failures = append(failures, failure.Err)
	}
	return errors.Join(failures...)
}

type ResourceInspector interface {
	ResourceYAML(context.Context, ResourceReference) (string, error)
}

type PodReader interface {
	PodContainers(context.Context, PodReference) ([]string, error)
	OpenPodLogs(context.Context, PodLogRequest) (io.ReadCloser, error)
}

type PodSheller interface {
	StartShell(context.Context, ShellRequest) (ShellSession, error)
}

type HelmReader interface {
	ListHelmReleases(context.Context, string) ([]HelmRelease, error)
	HelmReleaseValues(context.Context, HelmReleaseReference) (string, error)
}

type PortForwardManager interface {
	StartPortForward(context.Context, PortForwardRequest) (PortForward, error)
	StopPortForward(context.Context, string) error
	StopAllPortForwards(context.Context) error
	PortForwards() []PortForwardSession
}

type ResourceWriter interface {
	Scale(context.Context, ScaleRequest) error
	Delete(context.Context, ResourceReference) error
	DeleteBatch(context.Context, ResourceBatch) (BatchOutcome, error)
	Restart(context.Context, WorkloadReference) error
	RestartBatch(context.Context, WorkloadBatch) (BatchOutcome, error)
}

type ClusterOperations interface {
	ResourceInspector
	PodReader
	PodSheller
	PortForwardManager
	ResourceWriter
}

func validateResourceReference(reference ResourceReference) error {
	switch {
	case strings.TrimSpace(reference.Resource.Resource) == "":
		return ErrResourceIdentifierRequired
	case strings.TrimSpace(reference.Name) == "":
		return ErrResourceNameRequired
	default:
		return nil
	}
}

func validatePodReference(reference PodReference) error {
	switch {
	case strings.TrimSpace(reference.Namespace) == "":
		return ErrNamespaceRequired
	case strings.TrimSpace(reference.Name) == "":
		return ErrPodNameRequired
	default:
		return nil
	}
}

func validatePortForwardRequest(request PortForwardRequest) error {
	if err := validatePodReference(request.Pod); err != nil {
		return err
	}
	if request.LocalPort.value == 0 || request.RemotePort.value == 0 {
		return ErrNetworkPortInvalid
	}
	return nil
}

func validateWorkloadReference(reference WorkloadReference) error {
	if reference.Kind != WorkloadDeployment && reference.Kind != WorkloadStatefulSet {
		return ErrUnsupportedWorkloadKind
	}
	if strings.TrimSpace(reference.Namespace) == "" {
		return ErrNamespaceRequired
	}
	if strings.TrimSpace(reference.Name) == "" {
		return ErrResourceNameRequired
	}
	return nil
}

func validateBatch(resource schema.GroupResource, names []string) error {
	if strings.TrimSpace(resource.Resource) == "" {
		return ErrResourceIdentifierRequired
	}
	if len(names) == 0 {
		return ErrResourceNamesRequired
	}
	seenNames := make(map[string]struct{}, len(names))
	for index, name := range names {
		if strings.TrimSpace(name) == "" {
			return fmt.Errorf("%w at index %d", ErrResourceNameRequired, index)
		}
		if _, exists := seenNames[name]; exists {
			return fmt.Errorf("%w: %s", ErrDuplicateResourceName, name)
		}
		seenNames[name] = struct{}{}
	}
	return nil
}

func validateWorkloadBatch(batch WorkloadBatch) error {
	if batch.Kind != WorkloadDeployment && batch.Kind != WorkloadStatefulSet {
		return ErrUnsupportedWorkloadKind
	}
	if strings.TrimSpace(batch.Namespace) == "" {
		return ErrNamespaceRequired
	}
	return validateBatch(schema.GroupResource{Resource: batch.Kind.String()}, batch.Names)
}
