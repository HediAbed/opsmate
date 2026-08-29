package ui

import (
	"context"
	"errors"
	"testing"

	"github.com/HediAbed/opsmate/failure"
)

func validRuntimeDependencies() RuntimeDependencies {
	operations := &testClusterOperations{}
	return RuntimeDependencies{
		Context:           context.Background(),
		ClusterContext:    &testContextManager{},
		ClusterResources:  &testResourceReader{},
		ClusterSnapshots:  &snapshotCollectorStub{},
		ClusterObserver:   &testResourceObserver{},
		ClusterOperations: operations,
		HelmReleases:      operations,
	}
}

func TestRuntimeDependenciesValidation(t *testing.T) {
	tests := []struct {
		name      string
		clear     func(*RuntimeDependencies)
		wantCause error
	}{
		{name: "missing context", clear: func(runtime *RuntimeDependencies) { runtime.Context = nil }, wantCause: ErrRuntimeContextRequired},
		{name: "missing cluster context", clear: func(runtime *RuntimeDependencies) { runtime.ClusterContext = nil }, wantCause: ErrClusterContextRequired},
		{name: "missing cluster resources", clear: func(runtime *RuntimeDependencies) { runtime.ClusterResources = nil }, wantCause: ErrClusterResourcesRequired},
		{name: "missing cluster snapshots", clear: func(runtime *RuntimeDependencies) { runtime.ClusterSnapshots = nil }, wantCause: ErrClusterSnapshotsRequired},
		{name: "missing cluster observer", clear: func(runtime *RuntimeDependencies) { runtime.ClusterObserver = nil }, wantCause: ErrClusterObserverRequired},
		{name: "missing cluster operations", clear: func(runtime *RuntimeDependencies) { runtime.ClusterOperations = nil }, wantCause: ErrClusterOperationsRequired},
		{name: "missing helm releases", clear: func(runtime *RuntimeDependencies) { runtime.HelmReleases = nil }, wantCause: ErrHelmReaderRequired},
		{name: "valid"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runtime := validRuntimeDependencies()
			if test.clear != nil {
				test.clear(&runtime)
			}
			err := runtime.validate()
			if !errors.Is(err, test.wantCause) {
				t.Fatalf("validate() error = %v, want cause %v", err, test.wantCause)
			}
			root, constructorErr := NewRootModel("default", runtime)
			if test.wantCause != nil {
				if !errors.Is(constructorErr, test.wantCause) {
					t.Fatalf("NewRootModel() error = %v, want cause %v", constructorErr, test.wantCause)
				}
				return
			}
			if constructorErr != nil || root.namespace != "default" {
				t.Fatalf("NewRootModel() = (%+v, %v)", root, constructorErr)
			}
		})
	}
}

func TestDependencyError(t *testing.T) {
	sentinel := errors.New("missing")
	tests := []struct {
		name string
		err  *DependencyError
		want string
	}{
		{name: "nil", want: "ui dependency is invalid"},
		{name: "without cause", err: &DependencyError{Dependency: "clock"}, want: "ui dependency clock is invalid"},
		{name: "with cause", err: &DependencyError{Dependency: "clock", Err: sentinel}, want: "ui dependency clock: missing"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := test.err.Error(); got != test.want {
				t.Fatalf("Error() = %q, want %q", got, test.want)
			}
		})
	}
	var nilError *DependencyError
	if nilError.Unwrap() != nil {
		t.Fatal("nil Unwrap() must return nil")
	}
	err := &DependencyError{Err: sentinel}
	if !errors.Is(err, sentinel) {
		t.Fatal("DependencyError must unwrap its cause")
	}
	if err.FailureCode() != failure.CodeInvalidArgument {
		t.Fatalf("FailureCode() = %q, want invalid_argument", err.FailureCode())
	}
}
