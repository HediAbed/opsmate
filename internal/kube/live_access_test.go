package kube

import (
	"context"
	"errors"
	"testing"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	cachetesting "k8s.io/client-go/tools/cache/testing"

	"github.com/HediAbed/opsmate/internal/failure"
)

func TestLiveSetStopsAfterForbiddenWatch(t *testing.T) {
	source := cachetesting.NewFakeControllerSource()
	observed, err := newLiveSet(
		context.Background(),
		SubjectPods,
		source,
		&corev1.Pod{},
		decodeLivePod,
		stripManagedFields,
	)
	if err != nil {
		t.Fatalf("newLiveSet() error = %v", err)
	}
	concrete := observed.(*liveSet[corev1.Pod])
	waitForLiveState(t, observed, func(state LiveState[corev1.Pod]) bool { return state.Ready })

	forbidden := apierrors.NewForbidden(schema.GroupResource{Resource: "pods"}, "", errors.New("denied"))
	concrete.handleWatchError(context.Background(), nil, forbidden)
	assertChangesClosed(t, observed.Changes())

	state := observed.State()
	if !failure.IsPermissionDenied(state.Err) {
		t.Fatalf("state error after forbidden watch = %v, want permission denied", state.Err)
	}
	var typed *Error
	if !errors.As(state.Err, &typed) || typed.Operation != OperationObserve || typed.Subject != SubjectPods {
		t.Fatalf("state error = %#v, want observe pods error", state.Err)
	}
	if concrete.ctx.Err() == nil {
		t.Fatal("forbidden watch must stop the live set instead of retrying")
	}
}
