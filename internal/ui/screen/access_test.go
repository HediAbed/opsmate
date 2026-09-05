package screen

import (
	"errors"
	"testing"

	"github.com/HediAbed/opsmate/internal/failure"
	"github.com/HediAbed/opsmate/internal/kube"
	clusterui "github.com/HediAbed/opsmate/internal/ui/cluster"
)

func TestAccessNoticeDescribesDeniedOperation(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want string
	}{
		{
			name: "observe reads as list",
			err:  &kube.Error{Operation: kube.OperationObserve, Subject: kube.SubjectPods, Code: failure.CodePermissionDenied},
			want: "Not permitted: list pods",
		},
		{
			name: "identifier is included",
			err:  &kube.Error{Operation: kube.OperationDelete, Subject: kube.SubjectPod, Identifier: "payments/api", Code: failure.CodePermissionDenied},
			want: "Not permitted: delete pod payments/api",
		},
		{
			name: "wrapped kube error is unwrapped",
			err:  errors.Join(errors.New("outer"), &kube.Error{Operation: kube.OperationList, Subject: kube.SubjectNamespaces, Code: failure.CodePermissionDenied}),
			want: "Not permitted: list namespaces",
		},
		{
			name: "plain error falls back",
			err:  errors.New("forbidden"),
			want: "Not permitted for this account",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := AccessNotice(test.err); got != test.want {
				t.Fatalf("AccessNotice() = %q, want %q", got, test.want)
			}
		})
	}
}

type localPermissionError struct{}

func (localPermissionError) Error() string {
	return "session write: permission denied"
}

func (localPermissionError) FailureCode() failure.Code {
	return failure.CodePermissionDenied
}

func TestAccessDeniedOnlyMatchesKubernetesPermissionFailures(t *testing.T) {
	denied := &kube.Error{Operation: kube.OperationList, Subject: kube.SubjectPods, Code: failure.CodePermissionDenied}
	if !AccessDenied(denied) {
		t.Fatal("kubernetes permission denied error must be classified as access denied")
	}
	unavailable := &kube.Error{Operation: kube.OperationList, Subject: kube.SubjectPods, Code: failure.CodeUnavailable}
	if AccessDenied(unavailable) || AccessDenied(nil) {
		t.Fatal("non-permission failures must not be classified as access denied")
	}
	if AccessDenied(localPermissionError{}) {
		t.Fatal("local file permission failures must keep the regular error path")
	}
}

func TestLiveStopErrorKeepsPermissionDenial(t *testing.T) {
	denied := &kube.Error{Operation: kube.OperationObserve, Subject: kube.SubjectPods, Code: failure.CodePermissionDenied}
	closed := LiveMessage{Closed: true, Payload: LiveSnapshot[string]{State: clusterui.LiveState[string]{Err: denied}}}
	if got := LiveStopError(closed); !errors.Is(got, denied) {
		t.Fatalf("LiveStopError(denied) = %v, want the denial", got)
	}
	plain := LiveMessage{Closed: true, Payload: LiveSnapshot[string]{State: clusterui.LiveState[string]{Err: errors.New("gone")}}}
	if got := LiveStopError(plain); !errors.Is(got, ErrLiveUpdatesStopped) {
		t.Fatalf("LiveStopError(plain) = %v, want %v", got, ErrLiveUpdatesStopped)
	}
	if got := LiveStopError(LiveMessage{Closed: true}); !errors.Is(got, ErrLiveUpdatesStopped) {
		t.Fatalf("LiveStopError(no payload) = %v, want %v", got, ErrLiveUpdatesStopped)
	}
}
