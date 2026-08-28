package service

import (
	"context"
	"testing"
)

func TestWatchIngresses_ReturnsNonNilWatcher(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	w := WatchIngresses(ctx, "ns")
	cancel()
	if w == nil {
		t.Error("WatchIngresses should return a non-nil watcher")
	}
}

func TestWatchNetworkPolicies_ReturnsNonNilWatcher(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	w := WatchNetworkPolicies(ctx, "ns")
	cancel()
	if w == nil {
		t.Error("WatchNetworkPolicies should return a non-nil watcher")
	}
}

func TestWatchPVCs_ReturnsNonNilWatcher(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	w := WatchPVCs(ctx, "ns")
	cancel()
	if w == nil {
		t.Error("WatchPVCs should return a non-nil watcher")
	}
}

func TestWatchCronJobs_ReturnsNonNilWatcher(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	w := WatchCronJobs(ctx, "ns")
	cancel()
	if w == nil {
		t.Error("WatchCronJobs should return a non-nil watcher")
	}
}

func TestWatchHPAs_ReturnsNonNilWatcher(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	w := WatchHPAs(ctx, "ns")
	cancel()
	if w == nil {
		t.Error("WatchHPAs should return a non-nil watcher")
	}
}

func TestWatchSecrets_ReturnsNonNilWatcher(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	w := WatchSecrets(ctx, "ns")
	cancel()
	if w == nil {
		t.Error("WatchSecrets should return a non-nil watcher")
	}
}

func TestWatchReplicaSets_ReturnsNonNilWatcher(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	w := WatchReplicaSets(ctx, "ns")
	cancel()
	if w == nil {
		t.Error("WatchReplicaSets should return a non-nil watcher")
	}
}
