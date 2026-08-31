package ui

import (
	"context"
	"errors"
	"fmt"

	"github.com/HediAbed/opsmate/internal/analysis"
	"github.com/HediAbed/opsmate/internal/failure"
	"github.com/HediAbed/opsmate/internal/kube"
)

var (
	ErrRuntimeContextRequired    = errors.New("runtime context is required")
	ErrClusterContextRequired    = errors.New("cluster context manager is required")
	ErrClusterResourcesRequired  = errors.New("cluster resource reader is required")
	ErrClusterSnapshotsRequired  = errors.New("cluster snapshot collector is required")
	ErrClusterObserverRequired   = errors.New("cluster resource observer is required")
	ErrClusterOperationsRequired = errors.New("cluster operations are required")
	ErrHelmReaderRequired        = errors.New("helm release reader is required")
)

type ClusterSnapshotCollector interface {
	Collect(context.Context, string) (kube.ClusterSnapshot, error)
}

type RuntimeDependencies struct {
	Context           context.Context
	ClusterContext    kube.ContextManager
	ClusterResources  kube.ResourceReader
	ClusterSnapshots  ClusterSnapshotCollector
	ClusterObserver   kube.ResourceObserver
	ClusterOperations kube.ClusterOperations
	HelmReleases      kube.HelmReader
	Analysis          analysis.Service
}

type DependencyError struct {
	Dependency string
	Err        error
}

func (e *DependencyError) Error() string {
	if e == nil {
		return "ui dependency is invalid"
	}
	if e.Err == nil {
		return "ui dependency " + e.Dependency + " is invalid"
	}
	return fmt.Sprintf("ui dependency %s: %v", e.Dependency, e.Err)
}

func (e *DependencyError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

func (*DependencyError) FailureCode() failure.Code {
	return failure.CodeInvalidArgument
}

func (d RuntimeDependencies) validate() error {
	if d.Context == nil {
		return &DependencyError{Dependency: "context", Err: ErrRuntimeContextRequired}
	}
	if d.ClusterContext == nil {
		return &DependencyError{Dependency: "cluster context", Err: ErrClusterContextRequired}
	}
	if d.ClusterResources == nil {
		return &DependencyError{Dependency: "cluster resources", Err: ErrClusterResourcesRequired}
	}
	if d.ClusterSnapshots == nil {
		return &DependencyError{Dependency: "cluster snapshots", Err: ErrClusterSnapshotsRequired}
	}
	if d.ClusterObserver == nil {
		return &DependencyError{Dependency: "cluster observer", Err: ErrClusterObserverRequired}
	}
	if d.ClusterOperations == nil {
		return &DependencyError{Dependency: "cluster operations", Err: ErrClusterOperationsRequired}
	}
	if d.HelmReleases == nil {
		return &DependencyError{Dependency: "helm releases", Err: ErrHelmReaderRequired}
	}
	return nil
}
