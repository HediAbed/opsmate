package model

import (
	"strings"
	"testing"

	"github.com/HediAbed/opsmate/internal/service"
)

func TestSelectColSpecs_NormalFallback(t *testing.T) {
	specs, ok := selectColSpecs("pods", false)
	if !ok {
		t.Fatal("pods must resolve in normal mode")
	}
	for _, c := range specs {
		if c.Title == "IP" {
			t.Errorf("normal pods columns must not contain IP; got %+v", specs)
		}
	}
}

func TestSelectColSpecs_WideIncludesExtras(t *testing.T) {
	cases := []struct {
		resource string
		extra    string
	}{
		{"pods", "IP"},
		{"services", "EXTERNAL-IP"},
		{"services", "SELECTOR"},
		{"deployments", "CONTAINERS"},
		{"deployments", "IMAGES"},
		{"deployments", "SELECTOR"},
		{"nodes", "INTERNAL-IP"},
		{"nodes", "OS-IMAGE"},
	}
	for _, c := range cases {
		t.Run(c.resource+"/"+c.extra, func(t *testing.T) {
			specs, ok := selectColSpecs(c.resource, true)
			if !ok {
				t.Fatalf("%s must resolve in wide mode", c.resource)
			}
			found := false
			for _, s := range specs {
				if s.Title == c.extra {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("wide %s must contain %q; got %+v", c.resource, c.extra, specs)
			}
		})
	}
}

func TestSelectColSpecs_WideFallsBackForUncoveredTypes(t *testing.T) {
	specs, ok := selectColSpecs("statefulsets", true)
	if !ok {
		t.Fatal("statefulsets fallback must resolve in wide mode")
	}
	normal, _ := selectColSpecs("statefulsets", false)
	if len(specs) != len(normal) {
		t.Errorf("uncovered resource type must reuse normal columns; wide=%d normal=%d", len(specs), len(normal))
	}
}

func TestPodRowsWide_AppendsIPColumn(t *testing.T) {
	m := NewBrowserModel("default")
	m.pods = []service.Pod{
		{Name: "alpha", Status: "Running", Ready: "1/1", Restarts: 0, Age: "1m", IP: "10.0.0.1", Node: "n1"},
	}
	rows := podRowsWide(&m)
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	if len(rows[0]) != len(resourceColSpecsWide["pods"]) {
		t.Errorf("wide row width %d must match wide colspec width %d", len(rows[0]), len(resourceColSpecsWide["pods"]))
	}
	joined := strings.Join(rows[0], "|")
	if !strings.Contains(joined, "10.0.0.1") {
		t.Errorf("wide pod row must contain IP; got %q", joined)
	}
}

func TestDeploymentRowsWide_JoinsContainersAndImages(t *testing.T) {
	m := NewBrowserModel("default")
	m.deployments = []service.Deployment{
		{
			Name:       "web",
			Ready:      "2/2",
			UpToDate:   2,
			Available:  2,
			Age:        "5m",
			Containers: []string{"nginx", "sidecar"},
			Images:     []string{"nginx:1.25", "envoy:1.30"},
			Selector:   "app=web",
		},
	}
	rows := deploymentRowsWide(&m)
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	if len(rows[0]) != len(resourceColSpecsWide["deployments"]) {
		t.Errorf("wide row width %d must match wide colspec width %d", len(rows[0]), len(resourceColSpecsWide["deployments"]))
	}
	joined := strings.Join(rows[0], "|")
	for _, want := range []string{"nginx,sidecar", "nginx:1.25,envoy:1.30", "app=web"} {
		if !strings.Contains(joined, want) {
			t.Errorf("wide deployment row must contain %q; got %q", want, joined)
		}
	}
}

func TestServiceRowsWide_IncludesExternalIPAndSelector(t *testing.T) {
	m := NewBrowserModel("default")
	m.services = []service.Service{
		{Name: "api", Type: "LoadBalancer", ClusterIP: "10.0.0.1", ExternalIP: "1.2.3.4", Ports: "80/TCP", Age: "1d", Selector: "app=api"},
	}
	rows := serviceRowsWide(&m)
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	if len(rows[0]) != len(resourceColSpecsWide["services"]) {
		t.Errorf("wide row width %d must match wide colspec width %d", len(rows[0]), len(resourceColSpecsWide["services"]))
	}
	joined := strings.Join(rows[0], "|")
	for _, want := range []string{"1.2.3.4", "app=api"} {
		if !strings.Contains(joined, want) {
			t.Errorf("wide service row must contain %q; got %q", want, joined)
		}
	}
}

func TestNodeRowsWide_AppendsIPOSKernelRuntime(t *testing.T) {
	m := NewBrowserModel("default")
	m.nodes = []service.Node{
		{
			Name: "node-1", Status: "Ready", Roles: "control-plane", Version: "v1.30",
			Age: "10d", InternalIP: "10.1.1.1", OSImage: "Ubuntu 22.04", Kernel: "6.5.0", Runtime: "containerd://1.7",
		},
	}
	rows := nodeRowsWide(&m)
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	if len(rows[0]) != len(resourceColSpecsWide["nodes"]) {
		t.Errorf("wide row width %d must match wide colspec width %d", len(rows[0]), len(resourceColSpecsWide["nodes"]))
	}
	joined := strings.Join(rows[0], "|")
	for _, want := range []string{"10.1.1.1", "Ubuntu 22.04", "6.5.0", "containerd://1.7"} {
		if !strings.Contains(joined, want) {
			t.Errorf("wide node row must contain %q; got %q", want, joined)
		}
	}
}

func TestBrowser_ToggleWide_FlipsPositionalColumnCount(t *testing.T) {
	m := NewBrowserModel("default")
	m.resourceType = "pods"
	m.pods = []service.Pod{{Name: "p", Status: "Running", Ready: "1/1", Age: "1m", IP: "10.0.0.1", Node: "n"}}
	normalCols := len(resourceColSpecs["pods"])
	wideCols := len(resourceColSpecsWide["pods"])
	if normalCols == wideCols {
		t.Fatal("wide and normal pods specs must differ in width — fixture invalid")
	}

	if got := len(m.currentResourceRows()[0]); got != normalCols {
		t.Errorf("default row width = %d, want %d (normal)", got, normalCols)
	}

	m.wide = true
	if got := len(m.currentResourceRows()[0]); got != wideCols {
		t.Errorf("wide row width = %d, want %d (wide)", got, wideCols)
	}
}

func TestBrowser_WideAccessors_RoundTrip(t *testing.T) {
	m := NewBrowserModel("default")
	if m.Wide() {
		t.Error("Wide() must be false by default")
	}
	m.SetWide(true)
	if !m.Wide() {
		t.Error("SetWide(true) must persist")
	}
	m.SetWide(false)
	if m.Wide() {
		t.Error("SetWide(false) must clear")
	}
}
