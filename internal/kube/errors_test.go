package kube

import (
	"context"
	"errors"
	"io/fs"
	"net"
	"net/http"
	"slices"
	"testing"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"

	"github.com/HediAbed/opsmate/failure"
)

func TestErrorFormattingAndUnwrap(t *testing.T) {
	sentinel := errors.New("denied")
	tests := []struct {
		name string
		err  *Error
		want string
	}{
		{name: "nil", err: nil, want: "kubernetes operation failed"},
		{name: "empty operation", err: &Error{}, want: "kubernetes operation"},
		{name: "operation", err: &Error{Operation: OperationConnect}, want: "kubernetes connect"},
		{name: "subject", err: &Error{Subject: SubjectClients}, want: "kubernetes operation clients"},
		{name: "context", err: &Error{Operation: OperationConnect, Subject: SubjectClients, ContextName: "staging"}, want: "kubernetes connect clients for context staging"},
		{name: "identifier", err: &Error{Operation: OperationRead, Subject: SubjectResource, Identifier: "team-a/configmaps/settings"}, want: "kubernetes read resource team-a/configmaps/settings"},
		{name: "cause", err: &Error{Operation: OperationConnect, Subject: SubjectClients, ContextName: "staging", Err: sentinel}, want: "kubernetes connect clients for context staging: denied"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := test.err.Error(); got != test.want {
				t.Fatalf("Error() = %q, want %q", got, test.want)
			}
		})
	}
	var nilError *Error
	if nilError.Unwrap() != nil {
		t.Fatal("nil Error.Unwrap() must return nil")
	}
	wrapped := &Error{Err: sentinel}
	if !errors.Is(wrapped, sentinel) {
		t.Fatalf("errors.Is(%v, sentinel) = false", wrapped)
	}
	if wrapped.FailureCode() != failure.CodeUnknown || nilError.FailureCode() != failure.CodeUnknown {
		t.Fatal("missing error codes must normalize to unknown")
	}
}

func TestOperationAliasesSharedVocabulary(t *testing.T) {
	pairs := []struct {
		kube   Operation
		shared failure.Operation
	}{
		{kube: OperationCreate, shared: failure.OperationCreate},
		{kube: OperationLoad, shared: failure.OperationLoad},
		{kube: OperationConnect, shared: failure.OperationConnect},
		{kube: OperationBuild, shared: failure.OperationBuild},
		{kube: OperationGet, shared: failure.OperationGet},
		{kube: OperationList, shared: failure.OperationList},
		{kube: OperationSet, shared: failure.OperationSet},
		{kube: OperationObserve, shared: failure.OperationObserve},
		{kube: OperationResolve, shared: failure.OperationResolve},
		{kube: OperationRead, shared: failure.OperationRead},
		{kube: OperationStream, shared: failure.OperationStream},
		{kube: OperationDelete, shared: failure.OperationDelete},
		{kube: OperationUpdate, shared: failure.OperationUpdate},
		{kube: OperationStart, shared: failure.OperationStart},
		{kube: OperationStop, shared: failure.OperationStop},
		{kube: OperationSend, shared: failure.OperationSend},
		{kube: OperationCollect, shared: failure.OperationCollect},
	}
	for _, pair := range pairs {
		if pair.kube != pair.shared {
			t.Fatalf("kube operation %q does not match shared operation %q", pair.kube, pair.shared)
		}
	}
}

type errorClassificationCase struct {
	name string
	err  error
	want failure.Code
}

func inputErrorClassificationCases() []errorClassificationCase {
	resource := schema.GroupResource{Group: "apps", Resource: "deployments"}
	kind := schema.GroupKind{Group: "apps", Kind: "Deployment"}
	return []errorClassificationCase{
		{name: "missing config source", err: ErrConfigSourceRequired, want: failure.CodeInvalidArgument},
		{name: "missing client builder", err: ErrClientBuilderRequired, want: failure.CodeInvalidArgument},
		{name: "missing REST config", err: ErrRESTConfigRequired, want: failure.CodeInvalidArgument},
		{name: "missing resource identifier", err: ErrResourceIdentifierRequired, want: failure.CodeInvalidArgument},
		{name: "missing context", err: ErrContextRequired, want: failure.CodeInvalidArgument},
		{name: "missing list watcher", err: ErrListWatcherRequired, want: failure.CodeInvalidArgument},
		{name: "missing resource prototype", err: ErrResourcePrototypeRequired, want: failure.CodeInvalidArgument},
		{name: "missing resource decoder", err: ErrResourceDecoderRequired, want: failure.CodeInvalidArgument},
		{name: "missing resource transform", err: ErrResourceTransformRequired, want: failure.CodeInvalidArgument},
		{name: "missing informer", err: ErrInformerRequired, want: failure.CodeInvalidArgument},
		{name: "missing namespace", err: ErrNamespaceRequired, want: failure.CodeInvalidArgument},
		{name: "missing resource name", err: ErrResourceNameRequired, want: failure.CodeInvalidArgument},
		{name: "missing resource names", err: ErrResourceNamesRequired, want: failure.CodeInvalidArgument},
		{name: "duplicate resource name", err: ErrDuplicateResourceName, want: failure.CodeInvalidArgument},
		{name: "missing pod name", err: ErrPodNameRequired, want: failure.CodeInvalidArgument},
		{name: "invalid log tail", err: ErrLogTailLinesInvalid, want: failure.CodeInvalidArgument},
		{name: "unsupported workload", err: ErrUnsupportedWorkloadKind, want: failure.CodeInvalidArgument},
		{name: "invalid replica count", err: ErrReplicaCountInvalid, want: failure.CodeInvalidArgument},
		{name: "missing helm release name", err: ErrHelmReleaseNameRequired, want: failure.CodeInvalidArgument},
		{name: "invalid", err: apierrors.NewInvalid(kind, "web", field.ErrorList{field.Invalid(field.NewPath("spec"), "bad", "invalid")}), want: failure.CodeInvalidArgument},
		{name: "bad request", err: apierrors.NewBadRequest("bad"), want: failure.CodeInvalidArgument},
		{name: "unknown context", err: ErrContextNotFound, want: failure.CodeNotFound},
		{name: "not found", err: apierrors.NewNotFound(resource, "web"), want: failure.CodeNotFound},
		{name: "already exists", err: apierrors.NewAlreadyExists(resource, "web"), want: failure.CodeAlreadyExists},
	}
}

func runtimeErrorClassificationCases() []errorClassificationCase {
	resource := schema.GroupResource{Group: "apps", Resource: "deployments"}
	return []errorClassificationCase{
		{name: "canceled", err: context.Canceled, want: failure.CodeCanceled},
		{name: "deadline", err: context.DeadlineExceeded, want: failure.CodeDeadlineExceeded},
		{name: "forbidden", err: apierrors.NewForbidden(resource, "web", errors.New("denied")), want: failure.CodePermissionDenied},
		{name: "file permission", err: fs.ErrPermission, want: failure.CodePermissionDenied},
		{name: "sensitive resource", err: ErrSensitiveResourceAccess, want: failure.CodePermissionDenied},
		{name: "unauthenticated", err: apierrors.NewUnauthorized("login required"), want: failure.CodeUnauthenticated},
		{name: "conflict", err: apierrors.NewConflict(resource, "web", errors.New("changed")), want: failure.CodeConflict},
		{name: "rate limited", err: apierrors.NewTooManyRequests("slow down", 1), want: failure.CodeRateLimited},
		{name: "missing clients", err: ErrClientUnavailable, want: failure.CodeUnavailable},
		{name: "network", err: &net.DNSError{Err: "offline", Name: "cluster.invalid"}, want: failure.CodeUnavailable},
		{name: "server timeout", err: apierrors.NewServerTimeout(resource, "list", 1), want: failure.CodeUnavailable},
		{name: "timeout", err: apierrors.NewTimeoutError("slow", 1), want: failure.CodeUnavailable},
		{name: "service unavailable", err: apierrors.NewServiceUnavailable("offline"), want: failure.CodeUnavailable},
		{name: "expired resource version", err: apierrors.NewResourceExpired("expired"), want: failure.CodeUnavailable},
		{name: "gone resource version", err: &apierrors.StatusError{ErrStatus: metav1.Status{Status: metav1.StatusFailure, Code: http.StatusGone, Reason: metav1.StatusReasonGone}}, want: failure.CodeUnavailable},
		{name: "internal", err: apierrors.NewInternalError(errors.New("broken")), want: failure.CodeInternal},
		{name: "missing connection check", err: ErrConnectionCheckUnavailable, want: failure.CodeInternal},
		{name: "unexpected resource object", err: ErrUnexpectedResourceObject, want: failure.CodeInternal},
		{name: "missing dynamic client", err: ErrDynamicClientUnavailable, want: failure.CodeInternal},
		{name: "missing REST mapper", err: ErrRESTMapperUnavailable, want: failure.CodeInternal},
		{name: "missing REST mapping", err: ErrRESTMappingUnavailable, want: failure.CodeInternal},
		{name: "missing typed client", err: ErrTypedClientUnavailable, want: failure.CodeInternal},
		{name: "missing pod log reader", err: ErrPodLogReaderUnavailable, want: failure.CodeInternal},
		{name: "missing pod log stream", err: ErrPodLogStreamUnavailable, want: failure.CodeInternal},
		{name: "missing helm storage", err: ErrHelmStorageUnavailable, want: failure.CodeInternal},
		{name: "invalid helm release", err: ErrHelmReleaseInvalid, want: failure.CodeInternal},
		{name: "unknown", err: errors.New("other"), want: failure.CodeUnknown},
		{name: "nil", want: failure.CodeUnknown},
	}
}

func TestErrorClassification(t *testing.T) {
	tests := slices.Concat(inputErrorClassificationCases(), runtimeErrorClassificationCases())
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := classifyError(test.err); got != test.want {
				t.Fatalf("classifyError(%v) = %q, want %q", test.err, got, test.want)
			}
			wrapped := newError(OperationConnect, SubjectClients, "staging", test.err)
			if wrapped.FailureCode() != test.want {
				t.Fatalf("newError().FailureCode() = %q, want %q", wrapped.FailureCode(), test.want)
			}
		})
	}
}
