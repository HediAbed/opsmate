package model

import (
	"testing"
)

func TestScreenConstants(t *testing.T) {
	if ScreenDashboard != 0 {
		t.Errorf("ScreenDashboard = %d; want 0", ScreenDashboard)
	}
	if ScreenBrowser != 1 {
		t.Errorf("ScreenBrowser = %d; want 1", ScreenBrowser)
	}
	if ScreenLogs != 2 {
		t.Errorf("ScreenLogs = %d; want 2", ScreenLogs)
	}
	if ScreenAI != 3 {
		t.Errorf("ScreenAI = %d; want 3", ScreenAI)
	}
}

func TestNewRootModel_Defaults(t *testing.T) {
	m := NewRootModel("test-ns")
	if m.namespace != "test-ns" {
		t.Errorf("namespace = %q; want %q", m.namespace, "test-ns")
	}
	if m.screen != ScreenDashboard {
		t.Errorf("screen = %d; want ScreenDashboard(%d)", m.screen, ScreenDashboard)
	}
	if m.showHelp {
		t.Error("showHelp should be false initially")
	}
	if m.showNSPicker {
		t.Error("showNSPicker should be false initially")
	}
}

func TestDrillDownMsg_Fields(t *testing.T) {
	msg := DrillDownMsg{
		Screen:       ScreenBrowser,
		ResourceType: "pod",
		ResourceName: "nginx-abc",
	}
	if msg.Screen != ScreenBrowser {
		t.Errorf("Screen = %d; want ScreenBrowser", msg.Screen)
	}
	if msg.ResourceType != "pod" {
		t.Errorf("ResourceType = %q; want %q", msg.ResourceType, "pod")
	}
	if msg.ResourceName != "nginx-abc" {
		t.Errorf("ResourceName = %q; want %q", msg.ResourceName, "nginx-abc")
	}
}
