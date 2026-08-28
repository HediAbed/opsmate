//go:build !windows

package model

import (
	"testing"

	"github.com/HediAbed/opsmate/internal/service"
)

func TestBrowserDeactivateClosesShellSession(t *testing.T) {
	m := NewBrowserModel("ns")
	m.state = stateShell
	m.shellSession = makeFakeShellSession(t)

	m.Deactivate()

	if m.shellSession != nil {
		t.Fatal("shell session was not cleared")
	}
	if m.state != stateBrowsing {
		t.Fatalf("state = %v, want %v", m.state, stateBrowsing)
	}
}

func TestBrowserRoutesOwnedSupervisedWatchMessage(t *testing.T) {
	m := NewBrowserModel("ns")
	watcher := newFakeWatcher[service.Pod]()
	m.podWatcher.Set(watcher)
	message := supervisedWatchMsg{
		generation: m.podWatcher.Generation(),
		payload: service.WatchEventMsg[service.Pod]{Event: service.WatchEvent[service.Pod]{
			Kind: service.WatchAdded,
			Item: service.Pod{Name: "web", Namespace: "ns"},
		}},
	}

	if !m.ownsSupervisedWatchMessage(message) {
		t.Fatal("active supervisor did not claim its message")
	}
	updated, command := m.handleSupervisedWatchMessage(message)
	if len(updated.pods) != 1 || updated.pods[0].Name != "web" {
		t.Fatalf("pods = %#v, want the watched pod", updated.pods)
	}
	if command == nil {
		t.Fatal("owned event did not request the next watch item")
	}
}

func TestBrowserWatchClosedIgnoresResourceWithoutWatcher(t *testing.T) {
	m := NewBrowserModel("ns")
	m.active = true
	m.resourceType = "services"

	updated, command := m.handleSharedWatchClosed("services")

	if command != nil || updated.resourceType != "services" {
		t.Fatal("resource without watcher should remain unchanged")
	}
}

func TestBrowserReconnectIgnoresUnregisteredResource(t *testing.T) {
	m := NewBrowserModel("ns")
	m.active = true
	m.resourceType = "services"

	updated, command := m.handleSharedReconnect(browserReconnectMsg{
		Kind:      "services",
		Namespace: "ns",
	})

	if command != nil || updated.resourceType != "services" {
		t.Fatal("unregistered resource should not start a watcher")
	}
}

func TestBrowserReconnectIgnoresStaleGeneration(t *testing.T) {
	m := NewBrowserModel("ns")
	m.active = true
	m.resourceType = "pods"
	m.podWatcher.Set(newFakeWatcher[service.Pod]())
	staleGeneration := m.podWatcher.Generation() + 1

	updated, command := m.handleSharedReconnect(browserReconnectMsg{
		Kind:       "pods",
		Namespace:  "ns",
		Generation: staleGeneration,
	})

	if command != nil || updated.podWatcher.Generation() != m.podWatcher.Generation() {
		t.Fatal("stale reconnect replaced the active watcher")
	}
}
