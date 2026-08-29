package kube

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"net"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/HediAbed/opsmate/failure"
)

var (
	ErrClientUnavailable          = errors.New("kubernetes client is unavailable")
	ErrContextNotFound            = errors.New("kubernetes context was not found")
	ErrConfigSourceRequired       = errors.New("kubernetes config source is required")
	ErrClientBuilderRequired      = errors.New("kubernetes client builder is required")
	ErrRESTConfigRequired         = errors.New("kubernetes REST config is required")
	ErrConnectionCheckUnavailable = errors.New("kubernetes connection check is unavailable")
	ErrResourceIdentifierRequired = errors.New("kubernetes resource identifier is required")
	ErrContextRequired            = errors.New("context is required")
	ErrListWatcherRequired        = errors.New("kubernetes list watcher is required")
	ErrResourcePrototypeRequired  = errors.New("kubernetes resource prototype is required")
	ErrResourceDecoderRequired    = errors.New("kubernetes resource decoder is required")
	ErrResourceTransformRequired  = errors.New("kubernetes resource transform is required")
	ErrInformerRequired           = errors.New("kubernetes informer is required")
	ErrUnexpectedResourceObject   = errors.New("unexpected kubernetes resource object")
	ErrNamespaceRequired          = errors.New("kubernetes namespace is required")
	ErrResourceNameRequired       = errors.New("kubernetes resource name is required")
	ErrResourceNamesRequired      = errors.New("at least one kubernetes resource name is required")
	ErrDuplicateResourceName      = errors.New("kubernetes resource names must be unique")
	ErrPodNameRequired            = errors.New("kubernetes pod name is required")
	ErrLogTailLinesInvalid        = errors.New("kubernetes log tail line count must be positive")
	ErrUnsupportedWorkloadKind    = errors.New("unsupported kubernetes workload kind")
	ErrReplicaCountInvalid        = errors.New("kubernetes replica count cannot be negative")
	ErrSensitiveResourceAccess    = errors.New("reading kubernetes Secret payloads is disabled")
	ErrDynamicClientUnavailable   = errors.New("kubernetes dynamic client is unavailable")
	ErrRESTMapperUnavailable      = errors.New("kubernetes REST mapper is unavailable")
	ErrRESTMappingUnavailable     = errors.New("kubernetes REST mapping is unavailable")
	ErrTypedClientUnavailable     = errors.New("kubernetes typed client is unavailable")
	ErrPodLogReaderUnavailable    = errors.New("kubernetes pod log reader is unavailable")
	ErrPodLogStreamUnavailable    = errors.New("kubernetes pod log stream is unavailable")
	ErrPodExecutorUnavailable     = errors.New("kubernetes pod executor is unavailable")
	ErrPortForwarderUnavailable   = errors.New("kubernetes port forwarder is unavailable")
	ErrShellSessionClosed         = errors.New("kubernetes shell session is closed")
	ErrShellInputBackpressure     = errors.New("kubernetes shell input queue is full")
	ErrShellOutputLineTooLong     = errors.New("kubernetes shell output line is too long")
	ErrNetworkPortInvalid         = errors.New("kubernetes network port is invalid")
	ErrPortForwardIDRequired      = errors.New("kubernetes port-forward session ID is required")
	ErrPortForwardReadiness       = errors.New("kubernetes port forward did not become ready")
	ErrHelmStorageUnavailable     = errors.New("helm release storage is unavailable")
	ErrHelmReleaseNameRequired    = errors.New("helm release name is required")
	ErrHelmReleaseInvalid         = errors.New("helm release metadata is invalid")
	ErrContextManagerRequired     = errors.New("kubernetes context manager is required")
	ErrResourceReaderRequired     = errors.New("kubernetes resource reader is required")
	ErrSnapshotCollectorRequired  = errors.New("kubernetes snapshot collector is required")
)

// Operation aliases the shared vocabulary so kube call sites stay concise.
type Operation = failure.Operation

const (
	OperationCreate  = failure.OperationCreate
	OperationLoad    = failure.OperationLoad
	OperationConnect = failure.OperationConnect
	OperationBuild   = failure.OperationBuild
	OperationGet     = failure.OperationGet
	OperationList    = failure.OperationList
	OperationSet     = failure.OperationSet
	OperationObserve = failure.OperationObserve
	OperationResolve = failure.OperationResolve
	OperationRead    = failure.OperationRead
	OperationStream  = failure.OperationStream
	OperationDelete  = failure.OperationDelete
	OperationUpdate  = failure.OperationUpdate
	OperationStart   = failure.OperationStart
	OperationStop    = failure.OperationStop
	OperationSend    = failure.OperationSend
	OperationCollect = failure.OperationCollect
)

type Subject string

const (
	SubjectClients             Subject = "clients"
	SubjectTypedClient         Subject = "typed client"
	SubjectDynamicClient       Subject = "dynamic client"
	SubjectMetricsClient       Subject = "metrics client"
	SubjectExtensionsClient    Subject = "extensions client"
	SubjectDiscoveryClient     Subject = "discovery client"
	SubjectMetadataClient      Subject = "metadata client"
	SubjectManager             Subject = "manager"
	SubjectConfiguration       Subject = "configuration"
	SubjectRESTConfig          Subject = "REST configuration"
	SubjectAPIServer           Subject = "API server"
	SubjectCurrentContext      Subject = "current context"
	SubjectContexts            Subject = "contexts"
	SubjectNamespaces          Subject = "namespaces"
	SubjectPods                Subject = "pods"
	SubjectDeployments         Subject = "deployments"
	SubjectEvents              Subject = "events"
	SubjectServices            Subject = "services"
	SubjectStatefulSets        Subject = "stateful sets"
	SubjectDaemonSets          Subject = "daemon sets"
	SubjectConfigMaps          Subject = "config maps"
	SubjectNodes               Subject = "nodes"
	SubjectJobs                Subject = "jobs"
	SubjectIngresses           Subject = "ingresses"
	SubjectNetworkPolicies     Subject = "network policies"
	SubjectPVCs                Subject = "persistent volume claims"
	SubjectCronJobs            Subject = "cron jobs"
	SubjectHPAs                Subject = "horizontal pod autoscalers"
	SubjectSecrets             Subject = "secrets"
	SubjectReplicaSets         Subject = "replica sets"
	SubjectServiceAccounts     Subject = "service accounts"
	SubjectRoles               Subject = "roles"
	SubjectRoleBindings        Subject = "role bindings"
	SubjectClusterRoles        Subject = "cluster roles"
	SubjectClusterRoleBindings Subject = "cluster role bindings"
	SubjectRBAC                Subject = "role-based access control resources"
	SubjectCRDs                Subject = "custom resource definitions"
	SubjectPodMetrics          Subject = "pod metrics"
	SubjectResourceMetadata    Subject = "resource metadata"
	SubjectResource            Subject = "resource"
	SubjectPod                 Subject = "pod"
	SubjectPodLogs             Subject = "pod logs"
	SubjectWorkload            Subject = "workload"
	SubjectPodShell            Subject = "pod shell"
	SubjectPortForward         Subject = "port forward"
	SubjectHelmReleases        Subject = "helm releases"
	SubjectHelmValues          Subject = "helm release values"
	SubjectClusterSnapshot     Subject = "cluster snapshot"
)

type Error struct {
	Operation   Operation
	Subject     Subject
	ContextName string
	Identifier  string
	Code        failure.Code
	Err         error
}

func (e *Error) Error() string {
	if e == nil {
		return "kubernetes operation failed"
	}
	prefix := "kubernetes operation"
	if e.Operation != "" {
		prefix = "kubernetes " + string(e.Operation)
	}
	if e.Subject != "" {
		prefix += " " + string(e.Subject)
	}
	if e.ContextName != "" {
		prefix += " for context " + e.ContextName
	}
	if e.Identifier != "" {
		prefix += " " + e.Identifier
	}
	if e.Err == nil {
		return prefix
	}
	return fmt.Sprintf("%s: %v", prefix, e.Err)
}

func newResourceError(operation Operation, subject Subject, identifier string, err error) *Error {
	wrapped := newError(operation, subject, "", err)
	wrapped.Identifier = identifier
	return wrapped
}

func (e *Error) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

func (e *Error) FailureCode() failure.Code {
	if e == nil || e.Code == "" {
		return failure.CodeUnknown
	}
	return e.Code
}

func newError(operation Operation, subject Subject, contextName string, err error) *Error {
	return &Error{
		Operation:   operation,
		Subject:     subject,
		ContextName: contextName,
		Code:        classifyError(err),
		Err:         err,
	}
}

func classifyError(err error) failure.Code {
	if code, found := classifyLocalError(err); found {
		return code
	}
	return classifyAPIError(err)
}

func classifyLocalError(err error) (failure.Code, bool) {
	if matchesAnyError(err, context.Canceled, ErrShellSessionClosed) {
		return failure.CodeCanceled, true
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return failure.CodeDeadlineExceeded, true
	}
	if matchesAnyError(err, invalidArgumentErrors()...) {
		return failure.CodeInvalidArgument, true
	}
	if errors.Is(err, ErrContextNotFound) {
		return failure.CodeNotFound, true
	}
	if matchesAnyError(err, fs.ErrPermission, ErrSensitiveResourceAccess) {
		return failure.CodePermissionDenied, true
	}
	if errors.Is(err, ErrShellInputBackpressure) {
		return failure.CodeRateLimited, true
	}
	if matchesAnyError(err, internalErrors()...) {
		return failure.CodeInternal, true
	}
	if matchesAnyError(err, ErrClientUnavailable, ErrPortForwardReadiness) || isNetworkError(err) {
		return failure.CodeUnavailable, true
	}
	return failure.CodeUnknown, false
}

func classifyAPIError(err error) failure.Code {
	reason := apierrors.ReasonForError(err)
	if code, found := classifyAPIRequestReason(reason); found {
		return code
	}
	return classifyAPIServerReason(reason)
}

func classifyAPIRequestReason(reason metav1.StatusReason) (failure.Code, bool) {
	if reason == metav1.StatusReasonInvalid || reason == metav1.StatusReasonBadRequest {
		return failure.CodeInvalidArgument, true
	}
	if reason == metav1.StatusReasonNotFound {
		return failure.CodeNotFound, true
	}
	if reason == metav1.StatusReasonAlreadyExists {
		return failure.CodeAlreadyExists, true
	}
	if reason == metav1.StatusReasonForbidden {
		return failure.CodePermissionDenied, true
	}
	if reason == metav1.StatusReasonUnauthorized {
		return failure.CodeUnauthenticated, true
	}
	if reason == metav1.StatusReasonConflict {
		return failure.CodeConflict, true
	}
	if reason == metav1.StatusReasonTooManyRequests {
		return failure.CodeRateLimited, true
	}
	return failure.CodeUnknown, false
}

func classifyAPIServerReason(reason metav1.StatusReason) failure.Code {
	if reason == metav1.StatusReasonInternalError {
		return failure.CodeInternal
	}
	if reason == metav1.StatusReasonServerTimeout ||
		reason == metav1.StatusReasonTimeout ||
		reason == metav1.StatusReasonServiceUnavailable ||
		reason == metav1.StatusReasonExpired ||
		reason == metav1.StatusReasonGone {
		return failure.CodeUnavailable
	}
	return failure.CodeUnknown
}

func matchesAnyError(err error, targets ...error) bool {
	for _, target := range targets {
		if errors.Is(err, target) {
			return true
		}
	}
	return false
}

func invalidArgumentErrors() []error {
	return []error{
		ErrConfigSourceRequired,
		ErrClientBuilderRequired,
		ErrRESTConfigRequired,
		ErrResourceIdentifierRequired,
		ErrContextRequired,
		ErrListWatcherRequired,
		ErrResourcePrototypeRequired,
		ErrResourceDecoderRequired,
		ErrResourceTransformRequired,
		ErrInformerRequired,
		ErrNamespaceRequired,
		ErrResourceNameRequired,
		ErrResourceNamesRequired,
		ErrDuplicateResourceName,
		ErrPodNameRequired,
		ErrLogTailLinesInvalid,
		ErrUnsupportedWorkloadKind,
		ErrReplicaCountInvalid,
		ErrNetworkPortInvalid,
		ErrPortForwardIDRequired,
		ErrHelmReleaseNameRequired,
		ErrContextManagerRequired,
		ErrResourceReaderRequired,
		ErrSnapshotCollectorRequired,
	}
}

func internalErrors() []error {
	return []error{
		ErrConnectionCheckUnavailable,
		ErrUnexpectedResourceObject,
		ErrDynamicClientUnavailable,
		ErrRESTMapperUnavailable,
		ErrRESTMappingUnavailable,
		ErrTypedClientUnavailable,
		ErrPodLogReaderUnavailable,
		ErrPodLogStreamUnavailable,
		ErrPodExecutorUnavailable,
		ErrPortForwarderUnavailable,
		ErrShellOutputLineTooLong,
		ErrHelmStorageUnavailable,
		ErrHelmReleaseInvalid,
	}
}

func isNetworkError(err error) bool {
	var networkError net.Error
	return errors.As(err, &networkError)
}
