package cluster

import (
	"strings"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/HediAbed/opsmate/internal/kube"
)

func TestProjectIngresses(t *testing.T) {
	now := time.Date(2026, time.August, 28, 12, 0, 0, 0, time.UTC)
	className := "public"
	ingresses := projectIngresses([]networkingv1.Ingress{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "shop", Namespace: "edge"},
			Spec: networkingv1.IngressSpec{
				IngressClassName: &className,
				Rules:            []networkingv1.IngressRule{{Host: "shop.example.invalid"}, {Host: "shop.example.invalid"}, {}},
				TLS:              []networkingv1.IngressTLS{{Hosts: []string{"shop.example.invalid"}}},
			},
			Status: networkingv1.IngressStatus{LoadBalancer: networkingv1.IngressLoadBalancerStatus{Ingress: []networkingv1.IngressLoadBalancerIngress{
				{Hostname: "lb.example.invalid"}, {IP: "203.0.113.2"}, {},
			}}},
		},
		{ObjectMeta: metav1.ObjectMeta{Name: "internal"}},
	}, now)
	if ingresses[0].Class != "public" || ingresses[0].Hosts != "shop.example.invalid" || ingresses[0].Address != "lb.example.invalid,203.0.113.2" || ingresses[0].Ports != ingressHTTPSPorts || ingresses[1].Class != "" || ingresses[1].Ports != ingressHTTPPorts {
		t.Fatalf("projectIngresses() = %+v", ingresses)
	}
	if projectedIngressHosts(nil) != "" || projectedIngressAddresses(nil) != "" {
		t.Fatal("empty ingress formatters must return empty strings")
	}
}

func TestProjectNetworkPolicies(t *testing.T) {
	now := time.Date(2026, time.August, 28, 12, 0, 0, 0, time.UTC)
	policies := projectNetworkPolicies([]networkingv1.NetworkPolicy{{
		ObjectMeta: metav1.ObjectMeta{Name: "deny", Namespace: "edge"},
		Spec: networkingv1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{MatchLabels: map[string]string{"app": "shop"}},
			PolicyTypes: []networkingv1.PolicyType{networkingv1.PolicyTypeIngress, networkingv1.PolicyTypeEgress},
		},
	}}, now)
	if len(policies) != 1 || policies[0].PodSelector["app"] != "shop" || strings.Join(policies[0].PolicyTypes, ",") != "Ingress,Egress" {
		t.Fatalf("projectNetworkPolicies() = %+v", policies)
	}
}

func TestProjectPersistentVolumeClaims(t *testing.T) {
	now := time.Date(2026, time.August, 28, 12, 0, 0, 0, time.UTC)
	storageClass := "fast"
	claims := projectPVCs([]corev1.PersistentVolumeClaim{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "bound", Namespace: "edge"},
			Spec: corev1.PersistentVolumeClaimSpec{
				VolumeName: "volume-a", StorageClassName: &storageClass,
				AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
				Resources:   corev1.VolumeResourceRequirements{Requests: corev1.ResourceList{corev1.ResourceStorage: resource.MustParse("1Gi")}},
			},
			Status: corev1.PersistentVolumeClaimStatus{Phase: corev1.ClaimBound, Capacity: corev1.ResourceList{corev1.ResourceStorage: resource.MustParse("2Gi")}},
		},
		{ObjectMeta: metav1.ObjectMeta{Name: "pending"}, Spec: corev1.PersistentVolumeClaimSpec{Resources: corev1.VolumeResourceRequirements{Requests: corev1.ResourceList{corev1.ResourceStorage: resource.MustParse("3Gi")}}}},
		{ObjectMeta: metav1.ObjectMeta{Name: "empty"}},
	}, now)
	if claims[0].Capacity != "2Gi" || claims[0].StorageClass != "fast" || strings.Join(claims[0].AccessModes, ",") != "ReadWriteOnce" || claims[1].Capacity != "3Gi" || claims[1].StorageClass != "" || claims[2].Capacity != "" {
		t.Fatalf("projectPVCs() = %+v", claims)
	}
	zeroCapacityClaim := corev1.PersistentVolumeClaim{
		Status: corev1.PersistentVolumeClaimStatus{Capacity: corev1.ResourceList{corev1.ResourceStorage: resource.MustParse("0")}},
		Spec:   corev1.PersistentVolumeClaimSpec{Resources: corev1.VolumeResourceRequirements{Requests: corev1.ResourceList{corev1.ResourceStorage: resource.MustParse("4Gi")}}},
	}
	if got := projectedPVCCapacity(zeroCapacityClaim); got != "4Gi" {
		t.Fatalf("projectedPVCCapacity(zero status) = %q", got)
	}
}

func TestProjectCronJobs(t *testing.T) {
	now := time.Date(2026, time.August, 28, 12, 0, 0, 0, time.UTC)
	lastSchedule := metav1.NewTime(now.Add(-time.Hour))
	cronJobs := projectCronJobs([]batchv1.CronJob{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "report"},
			Spec:       batchv1.CronJobSpec{Schedule: "0 * * * *", Suspend: pointerTo(true)},
			Status:     batchv1.CronJobStatus{LastScheduleTime: &lastSchedule, Active: []corev1.ObjectReference{{Name: "report-1"}}},
		},
		{ObjectMeta: metav1.ObjectMeta{Name: "new"}},
	}, now)
	if cronJobs[0].Schedule != "0 * * * *" || !cronJobs[0].Suspend || cronJobs[0].Active != 1 || cronJobs[0].LastSchedule != "1h" || cronJobs[1].Suspend || cronJobs[1].LastSchedule != projectedNever {
		t.Fatalf("projectCronJobs() = %+v", cronJobs)
	}
	zeroTime := metav1.Time{}
	if projectedCronJobLastSchedule(&zeroTime, now) != projectedNever {
		t.Fatal("zero last schedule must be normalized to never")
	}
}

func TestProjectReplicaSets(t *testing.T) {
	now := time.Date(2026, time.August, 28, 12, 0, 0, 0, time.UTC)
	replicas := int32(3)
	replicaSets := projectReplicaSets([]appsv1.ReplicaSet{
		{ObjectMeta: metav1.ObjectMeta{Name: "set-a"}, Spec: appsv1.ReplicaSetSpec{Replicas: &replicas}, Status: appsv1.ReplicaSetStatus{Replicas: 2, ReadyReplicas: 1}},
		{ObjectMeta: metav1.ObjectMeta{Name: "set-b"}},
	}, now)
	if replicaSets[0].Desired != 3 || replicaSets[0].Current != 2 || replicaSets[0].Ready != 1 || replicaSets[1].Desired != 0 {
		t.Fatalf("projectReplicaSets() = %+v", replicaSets)
	}
}

func TestProjectMetadataOnlyResources(t *testing.T) {
	now := time.Date(2026, time.August, 28, 12, 0, 0, 0, time.UTC)
	secrets := projectSecrets([]kube.ResourceMetadata{{Name: "credentials", Namespace: "edge", CreatedAt: now.Add(-time.Hour)}}, now)
	if len(secrets) != 1 || secrets[0].Type != metadataOnlyLabel || secrets[0].Data != 0 || secrets[0].Age != "1h" {
		t.Fatalf("projectSecrets() = %+v", secrets)
	}
	instances := projectCRDInstances([]kube.ResourceMetadata{{Name: "widget-a", Namespace: "edge", CreatedAt: now.Add(-time.Hour)}}, now)
	if len(instances) != 1 || instances[0].Name != "widget-a" || instances[0].Namespace != "edge" || instances[0].Age != "1h" {
		t.Fatalf("projectCRDInstances() = %+v", instances)
	}
}

func hpaMetricSpecsForTest() []autoscalingv2.MetricSpec {
	utilizationTarget := int32(70)
	averageTarget := resource.MustParse("500m")
	valueTarget := resource.MustParse("20")
	return []autoscalingv2.MetricSpec{
		{
			Type: autoscalingv2.ResourceMetricSourceType,
			Resource: &autoscalingv2.ResourceMetricSource{
				Name:   corev1.ResourceCPU,
				Target: autoscalingv2.MetricTarget{Type: autoscalingv2.UtilizationMetricType, AverageUtilization: &utilizationTarget},
			},
		},
		{
			Type: autoscalingv2.ContainerResourceMetricSourceType,
			ContainerResource: &autoscalingv2.ContainerResourceMetricSource{
				Name: corev1.ResourceCPU, Container: "app",
				Target: autoscalingv2.MetricTarget{Type: autoscalingv2.AverageValueMetricType, AverageValue: &averageTarget},
			},
		},
		{
			Type: autoscalingv2.PodsMetricSourceType,
			Pods: &autoscalingv2.PodsMetricSource{
				Metric: autoscalingv2.MetricIdentifier{Name: "requests"},
				Target: autoscalingv2.MetricTarget{Type: autoscalingv2.AverageValueMetricType, AverageValue: &averageTarget},
			},
		},
		{
			Type: autoscalingv2.ObjectMetricSourceType,
			Object: &autoscalingv2.ObjectMetricSource{
				Metric: autoscalingv2.MetricIdentifier{Name: "queue"},
				Target: autoscalingv2.MetricTarget{Type: autoscalingv2.ValueMetricType, Value: &valueTarget},
			},
		},
		{
			Type: autoscalingv2.ExternalMetricSourceType,
			External: &autoscalingv2.ExternalMetricSource{
				Metric: autoscalingv2.MetricIdentifier{Name: "lag"},
				Target: autoscalingv2.MetricTarget{},
			},
		},
		{Type: autoscalingv2.MetricSourceType("Custom")},
	}
}

func hpaMetricStatusesForTest() []autoscalingv2.MetricStatus {
	utilizationCurrent := int32(55)
	averageCurrent := resource.MustParse("350m")
	valueCurrent := resource.MustParse("12")
	return []autoscalingv2.MetricStatus{
		{
			Type: autoscalingv2.ResourceMetricSourceType,
			Resource: &autoscalingv2.ResourceMetricStatus{
				Name:    corev1.ResourceCPU,
				Current: autoscalingv2.MetricValueStatus{AverageUtilization: &utilizationCurrent},
			},
		},
		{
			Type: autoscalingv2.ContainerResourceMetricSourceType,
			ContainerResource: &autoscalingv2.ContainerResourceMetricStatus{
				Name: corev1.ResourceCPU, Container: "app",
				Current: autoscalingv2.MetricValueStatus{AverageValue: &averageCurrent},
			},
		},
		{
			Type: autoscalingv2.PodsMetricSourceType,
			Pods: &autoscalingv2.PodsMetricStatus{
				Metric:  autoscalingv2.MetricIdentifier{Name: "requests"},
				Current: autoscalingv2.MetricValueStatus{AverageValue: &averageCurrent},
			},
		},
		{
			Type: autoscalingv2.ObjectMetricSourceType,
			Object: &autoscalingv2.ObjectMetricStatus{
				Metric:  autoscalingv2.MetricIdentifier{Name: "queue"},
				Current: autoscalingv2.MetricValueStatus{Value: &valueCurrent},
			},
		},
		{
			Type: autoscalingv2.ExternalMetricSourceType,
			External: &autoscalingv2.ExternalMetricStatus{
				Metric:  autoscalingv2.MetricIdentifier{Name: "lag"},
				Current: autoscalingv2.MetricValueStatus{},
			},
		},
		{Type: autoscalingv2.MetricSourceType("Custom")},
	}
}

func TestProjectedHPATargetsPairSpecsWithStatuses(t *testing.T) {
	pairs := projectedHPATargets(hpaMetricSpecsForTest(), hpaMetricStatusesForTest())
	wantPairs := []string{"55%/70%", "350m/500m", "350m/500m", "12/20", projectedUnknown + "/" + projectedUnknown, projectedUnknown + "/" + projectedUnknown}
	if len(pairs) != len(wantPairs) {
		t.Fatalf("projectedHPATargets() count = %d", len(pairs))
	}
	for index, want := range wantPairs {
		if got := pairs[index].String(); got != want {
			t.Fatalf("pair %d = %q, want %q", index, got, want)
		}
	}
	if empty := projectedHPATargets(nil, nil); len(empty) != 1 || empty[0].Target != projectedNone {
		t.Fatalf("projectedHPATargets(empty) = %+v", empty)
	}
}

func TestProjectHPAs(t *testing.T) {
	now := time.Date(2026, time.August, 28, 12, 0, 0, 0, time.UTC)
	specs := hpaMetricSpecsForTest()
	statuses := hpaMetricStatusesForTest()
	minimum := int32(2)
	hpas := projectHPAs([]autoscalingv2.HorizontalPodAutoscaler{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "api", Namespace: "edge"},
			Spec: autoscalingv2.HorizontalPodAutoscalerSpec{
				ScaleTargetRef: autoscalingv2.CrossVersionObjectReference{Kind: "Deployment", Name: "api"},
				MinReplicas:    &minimum, MaxReplicas: 10, Metrics: specs,
			},
			Status: autoscalingv2.HorizontalPodAutoscalerStatus{CurrentReplicas: 3, CurrentMetrics: statuses},
		},
		{ObjectMeta: metav1.ObjectMeta{Name: "default-min"}},
	}, now)
	if hpas[0].Reference.String() != "Deployment/api" || hpas[0].MinReplicas != 2 || hpas[0].MaxReplicas != 10 || hpas[0].Replicas != 3 || len(hpas[0].Targets) != len(specs) || hpas[1].MinReplicas != defaultHPAReplicaCount {
		t.Fatalf("projectHPAs() = %+v", hpas)
	}
}

func TestProjectedMetricFallbacksNormalizeToUnknown(t *testing.T) {
	for _, metric := range []autoscalingv2.MetricSpec{
		{Type: autoscalingv2.ResourceMetricSourceType},
		{Type: autoscalingv2.ContainerResourceMetricSourceType},
		{Type: autoscalingv2.PodsMetricSourceType},
		{Type: autoscalingv2.ObjectMetricSourceType},
		{Type: autoscalingv2.ExternalMetricSourceType},
	} {
		if projectedMetricTarget(metric) != projectedUnknown {
			t.Fatalf("nil metric target for %s was not unknown", metric.Type)
		}
	}
	for _, metric := range []autoscalingv2.MetricStatus{
		{Type: autoscalingv2.ResourceMetricSourceType},
		{Type: autoscalingv2.ContainerResourceMetricSourceType},
		{Type: autoscalingv2.PodsMetricSourceType},
		{Type: autoscalingv2.ObjectMetricSourceType},
		{Type: autoscalingv2.ExternalMetricSourceType},
	} {
		if projectedMetricStatus(metric) != projectedUnknown {
			t.Fatalf("nil metric status for %s was not unknown", metric.Type)
		}
	}
	if key := metricStatusKey(autoscalingv2.MetricStatus{Type: autoscalingv2.ResourceMetricSourceType}); key != string(autoscalingv2.ResourceMetricSourceType) {
		t.Fatalf("nil resource metric status key = %q", key)
	}
}

func projectionMetadataForTest(now time.Time) func(name, namespace string) metav1.ObjectMeta {
	created := metav1.NewTime(now.Add(-time.Hour))
	return func(name, namespace string) metav1.ObjectMeta {
		return metav1.ObjectMeta{Name: name, Namespace: namespace, CreationTimestamp: created}
	}
}

func TestProjectRBAC(t *testing.T) {
	now := time.Date(2026, time.August, 28, 12, 0, 0, 0, time.UTC)
	metadata := projectionMetadataForTest(now)
	resources := kube.RBACResources{
		ServiceAccounts: []corev1.ServiceAccount{{ObjectMeta: metadata("builder", "edge")}},
		Roles:           []rbacv1.Role{{ObjectMeta: metadata("reader", "edge"), Rules: []rbacv1.PolicyRule{{}, {}}}},
		RoleBindings:    []rbacv1.RoleBinding{{ObjectMeta: metadata("readers", "edge"), Subjects: []rbacv1.Subject{{}}}},
		ClusterRoles:    []rbacv1.ClusterRole{{ObjectMeta: metadata("viewer", ""), Rules: []rbacv1.PolicyRule{{}}}},
		ClusterRoleBindings: []rbacv1.ClusterRoleBinding{{
			ObjectMeta: metadata("viewers", ""), Subjects: []rbacv1.Subject{{}, {}},
		}},
	}
	rows := projectRBAC(resources, now)
	if len(rows) != 5 || rows[0].Kind != "ServiceAccount" || rows[1].Count != 2 || rows[2].Count != 1 || rows[3].Scope != "Cluster" || rows[4].Count != 2 || rows[4].Age != "1h" {
		t.Fatalf("projectRBAC() = %+v", rows)
	}
}

func TestProjectCRDs(t *testing.T) {
	now := time.Date(2026, time.August, 28, 12, 0, 0, 0, time.UTC)
	metadata := projectionMetadataForTest(now)
	crds := projectCRDs([]apiextensionsv1.CustomResourceDefinition{
		{
			ObjectMeta: metadata("widgets.example.invalid", ""),
			Spec: apiextensionsv1.CustomResourceDefinitionSpec{
				Group:    "example.invalid",
				Names:    apiextensionsv1.CustomResourceDefinitionNames{Plural: "widgets", Singular: "widget", Kind: "Widget"},
				Scope:    apiextensionsv1.NamespaceScoped,
				Versions: []apiextensionsv1.CustomResourceDefinitionVersion{{Name: "v1", Served: true}, {Name: "v2", Served: true, Storage: true}},
			},
		},
		{ObjectMeta: metadata("empty", ""), Spec: apiextensionsv1.CustomResourceDefinitionSpec{Versions: []apiextensionsv1.CustomResourceDefinitionVersion{{Name: "v1alpha1", Served: true}}}},
	}, now)
	if len(crds) != 2 || crds[0].Resource != "widgets.example.invalid" || strings.Join(crds[0].Versions, ",") != "v1,v2" || crds[0].PreferredVersion != "v2" || crds[0].Scope != "Namespaced" || crds[1].Resource != "" || crds[1].PreferredVersion != "v1alpha1" {
		t.Fatalf("projectCRDs() = %+v", crds)
	}
}
