package kube

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
	typedcorev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	clienttesting "k8s.io/client-go/testing"

	chartcommon "helm.sh/helm/v4/pkg/chart/common"
	chartv2 "helm.sh/helm/v4/pkg/chart/v2"
	helmrelease "helm.sh/helm/v4/pkg/release"
	releasecommon "helm.sh/helm/v4/pkg/release/common"
	releasev1 "helm.sh/helm/v4/pkg/release/v1"
	"helm.sh/helm/v4/pkg/storage"
	"helm.sh/helm/v4/pkg/storage/driver"
)

type helmReleaseStorageStub struct {
	releases []*releasev1.Release
	last     *releasev1.Release
	listErr  error
	lastErr  error
}

func (s helmReleaseStorageStub) List() ([]*releasev1.Release, error) {
	return s.releases, s.listErr
}

func (s helmReleaseStorageStub) Last(string) (*releasev1.Release, error) {
	return s.last, s.lastErr
}

type corelessKubernetesClient struct {
	kubernetes.Interface
	core typedcorev1.CoreV1Interface
}

func (c corelessKubernetesClient) CoreV1() typedcorev1.CoreV1Interface {
	return c.core
}

type secretlessCoreClient struct {
	typedcorev1.CoreV1Interface
	secrets typedcorev1.SecretInterface
}

func (c secretlessCoreClient) Secrets(string) typedcorev1.SecretInterface {
	return c.secrets
}

type pointerCoreClient struct {
	typedcorev1.CoreV1Interface
}

type pointerSecretClient struct {
	typedcorev1.SecretInterface
}

type recordingSecretClient struct {
	typedcorev1.SecretInterface
	getContext  context.Context
	listContext context.Context
	getResult   *corev1.Secret
	listResult  *corev1.SecretList
}

func (c *recordingSecretClient) Get(
	ctx context.Context,
	_ string,
	_ metav1.GetOptions,
) (*corev1.Secret, error) {
	c.getContext = ctx
	return c.getResult, nil
}

func (c *recordingSecretClient) List(
	ctx context.Context,
	_ metav1.ListOptions,
) (*corev1.SecretList, error) {
	c.listContext = ctx
	return c.listResult, nil
}

func TestManagerReadsHelmReleasesAndValues(t *testing.T) {
	client := fake.NewSimpleClientset()
	first := testStoredHelmRelease("api", "team-a", 1, "1.0.0", chartcommon.Values{"replicas": 1})
	latest := testStoredHelmRelease("api", "team-a", 2, "1.1.0", chartcommon.Values{"replicas": 3})
	worker := testStoredHelmRelease("worker", "team-b", 1, "2.0.0", nil)
	for _, release := range []*releasev1.Release{first, latest, worker} {
		storeHelmRelease(t, client, release)
	}
	manager := managerWithClientsForTest(t, &Clients{kubernetes: client})

	releases, err := manager.ListHelmReleases(context.Background(), "")
	if err != nil {
		t.Fatalf("ListHelmReleases() error = %v", err)
	}
	want := []HelmRelease{
		projectStoredHelmRelease(t, latest),
		projectStoredHelmRelease(t, worker),
	}
	if !slices.Equal(releases, want) {
		t.Fatalf("ListHelmReleases() = %+v, want %+v", releases, want)
	}

	namespaced, err := manager.ListHelmReleases(context.Background(), "team-a")
	if err != nil {
		t.Fatalf("namespaced ListHelmReleases() error = %v", err)
	}
	if !slices.Equal(namespaced, want[:1]) {
		t.Fatalf("namespaced ListHelmReleases() = %+v, want %+v", namespaced, want[:1])
	}

	values, err := manager.HelmReleaseValues(context.Background(), HelmReleaseReference{Namespace: "team-a", Name: "api"})
	if err != nil {
		t.Fatalf("HelmReleaseValues() error = %v", err)
	}
	if values != "replicas: 3\n" {
		t.Fatalf("HelmReleaseValues() = %q, want replicas YAML", values)
	}
	emptyValues, err := manager.HelmReleaseValues(context.Background(), HelmReleaseReference{Namespace: "team-b", Name: "worker"})
	if err != nil || emptyValues != "" {
		t.Fatalf("empty HelmReleaseValues() = (%q, %v), want empty values", emptyValues, err)
	}
}

func TestManagerHelmFailuresAreContextual(t *testing.T) {
	sentinel := errors.New("storage failed")
	tests := []struct {
		name      string
		configure func(*Manager)
		wantCause error
	}{
		{
			name: "missing factory",
			configure: func(manager *Manager) {
				manager.newHelmStorage = nil
			},
			wantCause: ErrHelmStorageUnavailable,
		},
		{
			name: "factory failure",
			configure: func(manager *Manager) {
				manager.newHelmStorage = func(context.Context, kubernetes.Interface, string) (helmReleaseStorage, error) {
					return nil, sentinel
				}
			},
			wantCause: sentinel,
		},
		{
			name: "list failure",
			configure: func(manager *Manager) {
				manager.newHelmStorage = helmStorageFactory(helmReleaseStorageStub{listErr: sentinel})
			},
			wantCause: sentinel,
		},
		{
			name: "invalid release",
			configure: func(manager *Manager) {
				manager.newHelmStorage = helmStorageFactory(helmReleaseStorageStub{releases: []*releasev1.Release{nil}})
			},
			wantCause: ErrHelmReleaseInvalid,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			manager := managerWithClientsForTest(t, &Clients{kubernetes: fake.NewSimpleClientset()})
			test.configure(manager)
			releases, err := manager.ListHelmReleases(context.Background(), "team-a")
			if releases != nil || !errors.Is(err, test.wantCause) {
				t.Fatalf("ListHelmReleases() = (%+v, %v), want %v", releases, err, test.wantCause)
			}
			var typedError *Error
			if !errors.As(err, &typedError) || typedError.Subject != SubjectHelmReleases {
				t.Fatalf("error = %#v, want helm releases Error", err)
			}
		})
	}
}

func TestListHelmReleasesRequiresConnectedClient(t *testing.T) {
	unavailable, err := NewManager(&fakeConfigSource{}, &fakeClientBuilder{})
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}
	if releases, listErr := unavailable.ListHelmReleases(context.Background(), ""); releases != nil || !errors.Is(listErr, ErrClientUnavailable) {
		t.Fatalf("unconnected ListHelmReleases() = (%+v, %v)", releases, listErr)
	}
}

func TestListHelmReleasesHonorsCanceledContext(t *testing.T) {
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	valid := testStoredHelmRelease("api", "team-a", 1, "1.0.0", nil)
	manager := managerWithClientsForTest(t, &Clients{kubernetes: fake.NewSimpleClientset()})
	manager.newHelmStorage = helmStorageFactory(helmReleaseStorageStub{releases: []*releasev1.Release{valid}})
	if releases, listErr := manager.ListHelmReleases(canceled, ""); releases != nil || !errors.Is(listErr, context.Canceled) {
		t.Fatalf("canceled ListHelmReleases() = (%+v, %v)", releases, listErr)
	}
}

func TestListHelmReleasesRejectsNilContext(t *testing.T) {
	manager := managerWithClientsForTest(t, &Clients{kubernetes: fake.NewSimpleClientset()})
	var missingContext context.Context
	listErr := errorWithoutPanic(t, "ListHelmReleases without a context", func() error {
		_, err := manager.ListHelmReleases(missingContext, "")
		return err
	})
	if !errors.Is(listErr, ErrContextRequired) {
		t.Fatalf("ListHelmReleases(nil) error = %v, want context-required error", listErr)
	}
}

func TestManagerHelmValuesFailuresAreContextual(t *testing.T) {
	sentinel := errors.New("values failed")
	invalidValues := testStoredHelmRelease("api", "team-a", 1, "1.0.0", chartcommon.Values{"bad": make(chan int)})
	tests := []struct {
		name      string
		reference HelmReleaseReference
		configure func(*Manager)
		wantCause error
	}{
		{name: "missing namespace", reference: HelmReleaseReference{Name: "api"}, wantCause: ErrNamespaceRequired},
		{name: "missing name", reference: HelmReleaseReference{Namespace: "team-a"}, wantCause: ErrHelmReleaseNameRequired},
		{
			name:      "factory failure",
			reference: HelmReleaseReference{Namespace: "team-a", Name: "api"},
			configure: func(manager *Manager) {
				manager.newHelmStorage = func(context.Context, kubernetes.Interface, string) (helmReleaseStorage, error) {
					return nil, sentinel
				}
			},
			wantCause: sentinel,
		},
		{
			name:      "last failure",
			reference: HelmReleaseReference{Namespace: "team-a", Name: "api"},
			configure: func(manager *Manager) {
				manager.newHelmStorage = helmStorageFactory(helmReleaseStorageStub{lastErr: sentinel})
			},
			wantCause: sentinel,
		},
		{
			name:      "encoding failure",
			reference: HelmReleaseReference{Namespace: "team-a", Name: "api"},
			configure: func(manager *Manager) {
				manager.newHelmStorage = helmStorageFactory(helmReleaseStorageStub{last: invalidValues})
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			manager := managerWithClientsForTest(t, &Clients{kubernetes: fake.NewSimpleClientset()})
			if test.configure != nil {
				test.configure(manager)
			}
			values, err := manager.HelmReleaseValues(context.Background(), test.reference)
			if values != "" || err == nil {
				t.Fatalf("HelmReleaseValues() = (%q, %v), want failure", values, err)
			}
			if test.wantCause != nil && !errors.Is(err, test.wantCause) {
				t.Fatalf("HelmReleaseValues() error = %v, want %v", err, test.wantCause)
			}
			var typedError *Error
			if !errors.As(err, &typedError) || typedError.Subject != SubjectHelmValues {
				t.Fatalf("error = %#v, want helm values Error", err)
			}
		})
	}
}

func TestHelmReleaseValuesRequireConnectedClient(t *testing.T) {
	unavailable, err := NewManager(&fakeConfigSource{}, &fakeClientBuilder{})
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}
	reference := HelmReleaseReference{Namespace: "team-a", Name: "api"}
	if values, readErr := unavailable.HelmReleaseValues(context.Background(), reference); values != "" || !errors.Is(readErr, ErrClientUnavailable) {
		t.Fatalf("unconnected HelmReleaseValues() = (%q, %v)", values, readErr)
	}
}

func TestHelmReleaseValuesHonorCanceledContext(t *testing.T) {
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	manager := managerWithClientsForTest(t, &Clients{kubernetes: fake.NewSimpleClientset()})
	reference := HelmReleaseReference{Namespace: "team-a", Name: "api"}
	if values, readErr := manager.HelmReleaseValues(canceled, reference); values != "" || !errors.Is(readErr, context.Canceled) {
		t.Fatalf("canceled HelmReleaseValues() = (%q, %v)", values, readErr)
	}
}

func helmSecretReleaseStorageForTest(t *testing.T, client kubernetes.Interface) helmReleaseStorage {
	t.Helper()
	releaseStorage, err := newHelmSecretReleaseStorage(context.Background(), client, "team-a")
	if err != nil {
		t.Fatalf("newHelmSecretReleaseStorage() error = %v", err)
	}
	return releaseStorage
}

func TestHelmSecretReleaseStorageReadsStoredReleases(t *testing.T) {
	client := fake.NewSimpleClientset()
	storeHelmRelease(t, client, testStoredHelmRelease("api", "team-a", 1, "1.0.0", nil))
	releaseStorage := helmSecretReleaseStorageForTest(t, client)
	releases, err := releaseStorage.List()
	if err != nil || len(releases) != 1 || releases[0].Name != "api" {
		t.Fatalf("List() = (%+v, %v)", releases, err)
	}
	last, err := releaseStorage.Last("api")
	if err != nil || last.Name != "api" {
		t.Fatalf("Last() = (%+v, %v)", last, err)
	}
	if missing, lastErr := releaseStorage.Last("missing"); missing != nil || lastErr == nil {
		t.Fatalf("missing Last() = (%+v, %v), want error", missing, lastErr)
	}
}

func TestSDKHelmReleaseStorageSurfacesDriverListFailure(t *testing.T) {
	client := fake.NewSimpleClientset()
	sdkStorage := helmSecretReleaseStorageForTest(t, client).(*sdkHelmReleaseStorage)
	client.PrependReactor("list", "secrets", func(_ clienttesting.Action) (bool, runtime.Object, error) {
		return true, nil, errors.New("list failed")
	})
	if listed, listErr := sdkStorage.List(); listed != nil || listErr == nil {
		t.Fatalf("failing SDK List() = (%+v, %v)", listed, listErr)
	}
}

func TestSDKHelmReleaseStorageRequiresBackingStorage(t *testing.T) {
	for _, unavailable := range []*sdkHelmReleaseStorage{nil, {}} {
		if listed, listErr := unavailable.List(); listed != nil || !errors.Is(listErr, ErrHelmStorageUnavailable) {
			t.Fatalf("List() on %#v = (%+v, %v)", unavailable, listed, listErr)
		}
		if last, lastErr := unavailable.Last("api"); last != nil || !errors.Is(lastErr, ErrHelmStorageUnavailable) {
			t.Fatalf("Last() on %#v = (%+v, %v)", unavailable, last, lastErr)
		}
	}
}

func TestHelmStorageConstructionRejectsUnavailableDependencies(t *testing.T) {
	var typedNilClient *fake.Clientset
	var typedNilCore *pointerCoreClient
	var typedNilSecrets *pointerSecretClient
	tests := []struct {
		name   string
		ctx    context.Context
		client kubernetes.Interface
		cause  error
	}{
		{name: "missing context", client: fake.NewSimpleClientset(), cause: ErrContextRequired},
		{name: "missing client", ctx: context.Background(), cause: ErrTypedClientUnavailable},
		{name: "typed nil client", ctx: context.Background(), client: typedNilClient, cause: ErrTypedClientUnavailable},
		{name: "missing core client", ctx: context.Background(), client: corelessKubernetesClient{}, cause: ErrTypedClientUnavailable},
		{name: "typed nil core client", ctx: context.Background(), client: corelessKubernetesClient{core: typedNilCore}, cause: ErrTypedClientUnavailable},
		{name: "missing secret client", ctx: context.Background(), client: corelessKubernetesClient{core: secretlessCoreClient{}}, cause: ErrHelmStorageUnavailable},
		{name: "typed nil secret client", ctx: context.Background(), client: corelessKubernetesClient{core: secretlessCoreClient{secrets: typedNilSecrets}}, cause: ErrHelmStorageUnavailable},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			releaseStorage, err := newHelmSecretReleaseStorage(test.ctx, test.client, "team-a")
			if releaseStorage != nil || !errors.Is(err, test.cause) {
				t.Fatalf("newHelmSecretReleaseStorage() = (%v, %v), want %v", releaseStorage, err, test.cause)
			}
		})
	}

	var nilManager *Manager
	if releaseStorage, err := nilManager.helmStorage(context.Background(), &Clients{}, ""); releaseStorage != nil || !errors.Is(err, ErrHelmStorageUnavailable) {
		t.Fatalf("nil manager helmStorage() = (%v, %v)", releaseStorage, err)
	}
	manager := managerWithClientsForTest(t, &Clients{})
	if releaseStorage, err := manager.helmStorage(context.Background(), &Clients{}, ""); releaseStorage != nil || !errors.Is(err, ErrTypedClientUnavailable) {
		t.Fatalf("missing client helmStorage() = (%v, %v)", releaseStorage, err)
	}
}

func recordingSecretClientWithRelease() *recordingSecretClient {
	return &recordingSecretClient{
		getResult:  &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "release"}},
		listResult: &corev1.SecretList{Items: []corev1.Secret{{ObjectMeta: metav1.ObjectMeta{Name: "release"}}}},
	}
}

func TestContextualSecretClientUsesCallerContext(t *testing.T) {
	type requestContextKey struct{}
	requestContext := context.WithValue(context.Background(), requestContextKey{}, "request")
	recorder := recordingSecretClientWithRelease()
	client := &contextualSecretClient{SecretInterface: recorder, ctx: requestContext}
	secret, err := client.Get(context.Background(), "release", metav1.GetOptions{})
	if err != nil || secret.Name != "release" || recorder.getContext != requestContext {
		t.Fatalf("Get() = (%+v, %v), context propagated = %t", secret, err, recorder.getContext == requestContext)
	}
	secrets, err := client.List(context.Background(), metav1.ListOptions{})
	if err != nil || len(secrets.Items) != 1 || recorder.listContext != requestContext {
		t.Fatalf("List() = (%+v, %v), context propagated = %t", secrets, err, recorder.listContext == requestContext)
	}
}

func TestContextualSecretClientPropagatesCancellation(t *testing.T) {
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	canceledClient := &contextualSecretClient{SecretInterface: recordingSecretClientWithRelease(), ctx: canceled}
	if secret, getErr := canceledClient.Get(context.Background(), "release", metav1.GetOptions{}); secret != nil || !errors.Is(getErr, context.Canceled) {
		t.Fatalf("canceled Get() = (%+v, %v)", secret, getErr)
	}
	if secrets, listErr := canceledClient.List(context.Background(), metav1.ListOptions{}); secrets != nil || !errors.Is(listErr, context.Canceled) {
		t.Fatalf("canceled List() = (%+v, %v)", secrets, listErr)
	}
}

func TestContextualSecretClientRequiresContextAndDelegate(t *testing.T) {
	incomplete := []*contextualSecretClient{nil, {}, {SecretInterface: recordingSecretClientWithRelease()}}
	for _, unavailable := range incomplete {
		if secret, getErr := unavailable.Get(context.Background(), "release", metav1.GetOptions{}); secret != nil || !errors.Is(getErr, ErrHelmStorageUnavailable) {
			t.Fatalf("unavailable Get() = (%+v, %v)", secret, getErr)
		}
		if secrets, listErr := unavailable.List(context.Background(), metav1.ListOptions{}); secrets != nil || !errors.Is(listErr, ErrHelmStorageUnavailable) {
			t.Fatalf("unavailable List() = (%+v, %v)", secrets, listErr)
		}
	}
}

func TestHelmReleaseProjectionLabelsAndIdentifiers(t *testing.T) {
	valid := testStoredHelmRelease("api", "team-a", 2, "1.2.3", nil)
	projected, err := projectHelmRelease(valid)
	if err != nil {
		t.Fatalf("projectHelmRelease() error = %v", err)
	}
	if projected.ChartLabel() != "api-chart-1.2.3" || projected.Identifier() != "team-a/helmrelease/api" {
		t.Fatalf("projected release = %+v", projected)
	}
	if (HelmRelease{Name: "api", ChartVersion: "1.0.0"}).ChartLabel() != "1.0.0" {
		t.Fatal("version-only chart label is incorrect")
	}
	if (HelmRelease{Name: "api", ChartName: "chart"}).ChartLabel() != "chart" {
		t.Fatal("name-only chart label is incorrect")
	}
	if (HelmRelease{Name: "api"}).Identifier() != "helmrelease/api" {
		t.Fatal("cluster-scoped release identifier is incorrect")
	}
	if (HelmReleaseReference{Name: "api"}).Identifier() != "helmrelease/api" ||
		(HelmReleaseReference{Namespace: "team-a", Name: "api"}).Identifier() != "team-a/helmrelease/api" {
		t.Fatal("release reference identifier is incorrect")
	}
}

func TestHelmReleaseProjectionRejectsIncompleteRecords(t *testing.T) {
	valid := testStoredHelmRelease("api", "team-a", 2, "1.2.3", nil)
	invalidCases := []*releasev1.Release{
		nil,
		{},
		{Name: "api"},
		{Name: "api", Namespace: "team-a"},
		{Name: "api", Namespace: "team-a", Version: 1},
		{Name: "api", Namespace: "team-a", Version: 1, Info: &releasev1.Info{}},
		{Name: "api", Namespace: "team-a", Version: 1, Info: &releasev1.Info{}, Chart: &chartv2.Chart{}},
		{Name: "api", Namespace: "team-a", Version: 1, Info: &releasev1.Info{}, Chart: &chartv2.Chart{Metadata: &chartv2.Metadata{}}},
		{Name: "api", Namespace: "team-a", Version: 1, Info: &releasev1.Info{}, Chart: &chartv2.Chart{Metadata: &chartv2.Metadata{Name: "chart"}}},
	}
	for index, record := range invalidCases {
		if release, projectionErr := projectHelmRelease(record); release != (HelmRelease{}) || !errors.Is(projectionErr, ErrHelmReleaseInvalid) {
			t.Fatalf("invalid case %d = (%+v, %v)", index, release, projectionErr)
		}
	}
	if releases, projectionErr := projectHelmReleases([]*releasev1.Release{valid, nil}); releases != nil || !errors.Is(projectionErr, ErrHelmReleaseInvalid) {
		t.Fatalf("projectHelmReleases() = (%+v, %v)", releases, projectionErr)
	}
}

func TestHelmReleaseRecordConversion(t *testing.T) {
	record := testStoredHelmRelease("api", "team-a", 1, "1.0.0", nil)
	fromPointer, err := typedHelmRelease(record)
	if err != nil || fromPointer != record {
		t.Fatalf("typedHelmRelease(pointer) = (%+v, %v)", fromPointer, err)
	}
	fromValue, err := typedHelmRelease(*record)
	if err != nil || fromValue.Name != record.Name {
		t.Fatalf("typedHelmRelease(value) = (%+v, %v)", fromValue, err)
	}
	var nilRecord *releasev1.Release
	for _, invalid := range []helmrelease.Releaser{nilRecord, "invalid"} {
		if converted, conversionErr := typedHelmRelease(invalid); converted != nil || !errors.Is(conversionErr, ErrHelmReleaseInvalid) {
			t.Fatalf("typedHelmRelease(%T) = (%+v, %v)", invalid, converted, conversionErr)
		}
	}
	if converted, conversionErr := typedHelmReleases([]helmrelease.Releaser{record, "invalid"}); converted != nil || !errors.Is(conversionErr, ErrHelmReleaseInvalid) {
		t.Fatalf("typedHelmReleases() = (%+v, %v)", converted, conversionErr)
	}
}

func TestLatestHelmReleasesAreDeterministic(t *testing.T) {
	releases := []HelmRelease{
		{Name: "worker", Namespace: "team-b", Revision: 1},
		{Name: "api", Namespace: "team-a", Revision: 2},
		{Name: "api", Namespace: "team-a", Revision: 1},
		{Name: "worker", Namespace: "team-a", Revision: 1},
	}
	want := []HelmRelease{releases[1], releases[3], releases[0]}
	if latest := latestHelmReleases(releases); !slices.Equal(latest, want) {
		t.Fatalf("latestHelmReleases() = %+v, want %+v", latest, want)
	}
	if latest := latestHelmReleases(nil); latest == nil || len(latest) != 0 {
		t.Fatalf("latestHelmReleases(nil) = %+v, want non-nil empty slice", latest)
	}
}

func TestEncodeHelmValues(t *testing.T) {
	if values, err := encodeHelmValues(nil); values != "" || !errors.Is(err, ErrHelmReleaseInvalid) {
		t.Fatalf("encodeHelmValues(nil) = (%q, %v)", values, err)
	}
	if values, err := encodeHelmValues(&releasev1.Release{}); values != "" || err != nil {
		t.Fatalf("encodeHelmValues(empty) = (%q, %v)", values, err)
	}
	valid := &releasev1.Release{Config: chartcommon.Values{"enabled": true}}
	if values, err := encodeHelmValues(valid); values != "enabled: true\n" || err != nil {
		t.Fatalf("encodeHelmValues(valid) = (%q, %v)", values, err)
	}
	invalid := &releasev1.Release{Config: chartcommon.Values{"bad": make(chan int)}}
	if values, err := encodeHelmValues(invalid); values != "" || err == nil {
		t.Fatalf("encodeHelmValues(invalid) = (%q, %v)", values, err)
	}
}

func helmStorageFactory(releaseStorage helmReleaseStorage) helmReleaseStorageFactory {
	return func(context.Context, kubernetes.Interface, string) (helmReleaseStorage, error) {
		return releaseStorage, nil
	}
}

func testStoredHelmRelease(
	name string,
	namespace string,
	revision int,
	chartVersion string,
	values chartcommon.Values,
) *releasev1.Release {
	return &releasev1.Release{
		Name:      name,
		Namespace: namespace,
		Version:   revision,
		Info: &releasev1.Info{
			Status:       releasecommon.StatusDeployed,
			LastDeployed: time.Date(2026, time.August, revision, 12, 30, 0, 0, time.UTC),
		},
		Chart: &chartv2.Chart{Metadata: &chartv2.Metadata{
			Name:       name + "-chart",
			Version:    chartVersion,
			AppVersion: "2026.8",
		}},
		Config: values,
	}
}

func storeHelmRelease(t *testing.T, client kubernetes.Interface, release *releasev1.Release) {
	t.Helper()
	releaseStorage := storage.Init(driver.NewSecrets(client.CoreV1().Secrets(release.Namespace)))
	if err := releaseStorage.Create(release); err != nil {
		t.Fatalf("store Helm release %s revision %d: %v", release.Name, release.Version, err)
	}
}

func projectStoredHelmRelease(t *testing.T, record *releasev1.Release) HelmRelease {
	t.Helper()
	release, err := projectHelmRelease(record)
	if err != nil {
		t.Fatalf("projectHelmRelease() error = %v", err)
	}
	return release
}
