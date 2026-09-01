package browser

import (
	"strings"
	"testing"

	"github.com/HediAbed/opsmate/internal/cluster"
)

func TestApplyFetchResult_HappyPathPopulatesCacheAndClearsBanners(t *testing.T) {
	m := newTestBrowserModel("ns")
	m.SetSize(120, 40)
	m.loading = true
	m.errBanner = "stale"
	m.err = errStub("old")

	pods := []cluster.Pod{{Name: "a", Namespace: "ns"}, {Name: "b", Namespace: "ns"}}
	applyTypedFetchResult(&m, "pods", &m.pods, pods, nil)

	if m.loading {
		t.Error("loading should be cleared after successful fetch")
	}
	if m.err != nil {
		t.Errorf("err should be nil after success; got %v", m.err)
	}
	if m.errBanner != "" {
		t.Errorf("errBanner should be cleared; got %q", m.errBanner)
	}
	if len(m.pods) != 2 {
		t.Errorf("pods slice should be replaced with payload; got %d", len(m.pods))
	}
	if m.statusMsg == "" {
		t.Error("statusMsg should report Loaded N pods")
	}
}

func TestApplyFetchResult_ErrorKeepsCacheAndSetsBanner(t *testing.T) {
	m := newTestBrowserModel("ns")
	m.pods = []cluster.Pod{{Name: "existing", Namespace: "ns"}}
	m.loading = true

	applyTypedFetchResult(&m, "pods", &m.pods, nil, errStub("boom"))

	if m.loading {
		t.Error("loading should clear even on error")
	}
	if m.err == nil {
		t.Error("error path must populate err")
	}
	if m.errBanner == "" {
		t.Error("error path must populate errBanner")
	}
	if len(m.pods) != 1 {
		t.Errorf("error path must not overwrite cache; got %d pods", len(m.pods))
	}
}

func TestApplyFetchResult_DispatchedKindAppearsInStatus(t *testing.T) {
	m := newTestBrowserModel("ns")
	m.SetSize(120, 40)
	applyTypedFetchResult(&m, "cronjobs", &m.cronjobs, []cluster.CronJob{{Name: "n"}, {Name: "n2"}}, nil)
	if !strings.Contains(m.statusMsg, "2") || !strings.Contains(m.statusMsg, "cronjobs") {
		t.Errorf("status should mention count and kind; got %q", m.statusMsg)
	}
}

func TestApplyFetchResult_SingleItemUsesSingularNoun(t *testing.T) {
	m := newTestBrowserModel("ns")
	m.SetSize(120, 40)
	applyTypedFetchResult(&m, "pods", &m.pods, []cluster.Pod{{Name: "only", Namespace: "ns"}}, nil)
	if strings.Contains(m.statusMsg, "1 pods") {
		t.Errorf("status should pluralize correctly for count=1; got %q", m.statusMsg)
	}
	if !strings.Contains(m.statusMsg, "1 pod") {
		t.Errorf("status should read 'Loaded 1 pod'; got %q", m.statusMsg)
	}
}

func TestResourceNounUsesPluralForCountOtherThanOne(t *testing.T) {
	cases := []struct {
		kind  string
		count int
		want  string
	}{
		{"pods", 0, "pods"},
		{"pods", 5, "pods"},
		{"ingresses", 12, "ingresses"},
	}
	for _, c := range cases {
		if got := resourceNoun(c.kind, c.count); got != c.want {
			t.Errorf("resourceNoun(%q, %d) = %q; want %q", c.kind, c.count, got, c.want)
		}
	}
}

func TestResourceNounUsesCatalogSingularForOne(t *testing.T) {
	cases := []struct {
		kind, want string
	}{
		{"pods", "pod"},
		{"deployments", "deployment"},
		{"cronjobs", "cronjob"},
		{"pvcs", "pvc"},
		{"ingresses", "ingress"},
		{"networkpolicies", "networkpolicy"},
	}
	for _, c := range cases {
		if got := resourceNoun(c.kind, 1); got != c.want {
			t.Errorf("resourceNoun(%q, 1) = %q; want %q", c.kind, got, c.want)
		}
	}
}
