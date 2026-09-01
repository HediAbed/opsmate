package browser

import (
	"strings"
	"testing"
)

func TestRenderBrowserTabStripFitsAllTabs(t *testing.T) {
	got := renderBrowserTabStrip(resourceTypePods, 500)
	previous := -1
	for _, resourceType := range allResourceTypes {
		label := strings.ToUpper(resourceType)
		index := strings.Index(got, label)
		if index < 0 {
			t.Errorf("missing tab %q", resourceType)
		}
		if index < previous {
			t.Errorf("tab %q is out of order", resourceType)
		}
		previous = index
	}
	if strings.Contains(got, "‹") || strings.Contains(got, "›") {
		t.Fatal("wide tab strip contains overflow hints")
	}
}

func TestRenderBrowserTabStripKeepsActiveTabVisible(t *testing.T) {
	got := renderBrowserTabStrip(resourceTypeHPAs, 30)
	if !strings.Contains(got, strings.ToUpper(resourceTypeHPAs)) {
		t.Errorf("active tab is missing: %q", got)
	}
	if !strings.Contains(got, "‹") || !strings.Contains(got, "›") {
		t.Errorf("overflow hints are missing: %q", got)
	}
	if strings.Contains(got, "PODS") && strings.Contains(got, "RBAC") {
		t.Errorf("narrow tab strip did not hide tabs: %q", got)
	}
}

func TestRenderBrowserTabStripHonorsMinimumWidth(t *testing.T) {
	if got := renderBrowserTabStrip(resourceTypePods, browserTabStripMinimumViable-1); got != "" {
		t.Errorf("tab strip below minimum width = %q", got)
	}
	if got := renderBrowserTabStrip(resourceTypePods, browserTabStripMinimumViable); got == "" {
		t.Fatal("tab strip at minimum width is empty")
	}
}

func TestRenderBrowserTabStripShowsOnlyRightHintForFirstTab(t *testing.T) {
	got := renderBrowserTabStrip(allResourceTypes[0], 30)
	if strings.Contains(got, "‹") || !strings.Contains(got, "›") {
		t.Errorf("first tab hints = %q", got)
	}
}

func TestRenderBrowserTabStripShowsOnlyLeftHintForLastTab(t *testing.T) {
	got := renderBrowserTabStrip(allResourceTypes[len(allResourceTypes)-1], 30)
	if strings.Contains(got, "›") || !strings.Contains(got, "‹") {
		t.Errorf("last tab hints = %q", got)
	}
}

func TestRenderBrowserTabStripShowsMiddleActiveTab(t *testing.T) {
	got := renderBrowserTabStrip(resourceTypeServices, 80)
	if !strings.Contains(got, strings.ToUpper(resourceTypeServices)) {
		t.Errorf("active tab is missing: %q", got)
	}
}
