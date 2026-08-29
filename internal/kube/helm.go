package kube

import (
	"cmp"
	"context"
	"fmt"
	"reflect"
	"slices"
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	typedcorev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	"sigs.k8s.io/yaml"

	chart "helm.sh/helm/v4/pkg/chart/v2"
	helmrelease "helm.sh/helm/v4/pkg/release"
	releasev1 "helm.sh/helm/v4/pkg/release/v1"
	"helm.sh/helm/v4/pkg/storage"
	"helm.sh/helm/v4/pkg/storage/driver"
)

type helmReleaseStorage interface {
	List() ([]*releasev1.Release, error)
	Last(string) (*releasev1.Release, error)
}

type helmReleaseStorageFactory func(context.Context, kubernetes.Interface, string) (helmReleaseStorage, error)

type sdkHelmReleaseStorage struct {
	storage *storage.Storage
}

type contextualSecretClient struct {
	typedcorev1.SecretInterface
	ctx context.Context
}

func (m *Manager) ListHelmReleases(parent context.Context, namespace string) ([]HelmRelease, error) {
	clients, ctx, cancel, err := m.clientSession(parent)
	if err != nil {
		return nil, newError(OperationList, SubjectHelmReleases, "", err)
	}
	defer cancel()
	if err := ctx.Err(); err != nil {
		return nil, newError(OperationList, SubjectHelmReleases, "", err)
	}

	releaseStorage, err := m.helmStorage(ctx, clients, namespace)
	if err != nil {
		return nil, newError(OperationList, SubjectHelmReleases, "", err)
	}
	records, err := releaseStorage.List()
	if err != nil {
		return nil, newError(OperationList, SubjectHelmReleases, "", err)
	}
	releases, err := projectHelmReleases(records)
	if err != nil {
		return nil, newError(OperationList, SubjectHelmReleases, "", err)
	}
	return latestHelmReleases(releases), nil
}

func (m *Manager) HelmReleaseValues(parent context.Context, reference HelmReleaseReference) (string, error) {
	if err := validateHelmReleaseReference(reference); err != nil {
		return "", newResourceError(OperationRead, SubjectHelmValues, reference.Identifier(), err)
	}
	clients, ctx, cancel, err := m.clientSession(parent)
	if err != nil {
		return "", newResourceError(OperationRead, SubjectHelmValues, reference.Identifier(), err)
	}
	defer cancel()
	if err := ctx.Err(); err != nil {
		return "", newResourceError(OperationRead, SubjectHelmValues, reference.Identifier(), err)
	}

	releaseStorage, err := m.helmStorage(ctx, clients, reference.Namespace)
	if err != nil {
		return "", newResourceError(OperationRead, SubjectHelmValues, reference.Identifier(), err)
	}
	record, err := releaseStorage.Last(reference.Name)
	if err != nil {
		return "", newResourceError(OperationRead, SubjectHelmValues, reference.Identifier(), err)
	}
	values, err := encodeHelmValues(record)
	if err != nil {
		return "", newResourceError(OperationRead, SubjectHelmValues, reference.Identifier(), err)
	}
	return values, nil
}

func (m *Manager) helmStorage(ctx context.Context, clients *Clients, namespace string) (helmReleaseStorage, error) {
	if m == nil || m.newHelmStorage == nil {
		return nil, ErrHelmStorageUnavailable
	}
	if clients == nil || isNilKubernetesClient(clients.Kubernetes()) {
		return nil, ErrTypedClientUnavailable
	}
	return m.newHelmStorage(ctx, clients.Kubernetes(), namespace)
}

func newHelmSecretReleaseStorage(
	ctx context.Context,
	client kubernetes.Interface,
	namespace string,
) (helmReleaseStorage, error) {
	if ctx == nil {
		return nil, ErrContextRequired
	}
	if isNilKubernetesClient(client) {
		return nil, ErrTypedClientUnavailable
	}
	coreClient := client.CoreV1()
	if isNilCoreClient(coreClient) {
		return nil, ErrTypedClientUnavailable
	}
	secretClient := coreClient.Secrets(namespace)
	if isNilSecretClient(secretClient) {
		return nil, ErrHelmStorageUnavailable
	}
	secretDriver := driver.NewSecrets(&contextualSecretClient{SecretInterface: secretClient, ctx: ctx})
	return &sdkHelmReleaseStorage{storage: storage.Init(secretDriver)}, nil
}

func (s *sdkHelmReleaseStorage) List() ([]*releasev1.Release, error) {
	if s == nil || s.storage == nil {
		return nil, ErrHelmStorageUnavailable
	}
	records, err := s.storage.ListReleases()
	if err != nil {
		return nil, err
	}
	return typedHelmReleases(records)
}

func (s *sdkHelmReleaseStorage) Last(name string) (*releasev1.Release, error) {
	if s == nil || s.storage == nil {
		return nil, ErrHelmStorageUnavailable
	}
	record, err := s.storage.Last(name)
	if err != nil {
		return nil, err
	}
	return typedHelmRelease(record)
}

func typedHelmReleases(records []helmrelease.Releaser) ([]*releasev1.Release, error) {
	typed := make([]*releasev1.Release, 0, len(records))
	for _, record := range records {
		release, err := typedHelmRelease(record)
		if err != nil {
			return nil, err
		}
		typed = append(typed, release)
	}
	return typed, nil
}

func typedHelmRelease(record helmrelease.Releaser) (*releasev1.Release, error) {
	switch release := record.(type) {
	case releasev1.Release:
		return &release, nil
	case *releasev1.Release:
		if release != nil {
			return release, nil
		}
	}
	return nil, fmt.Errorf("%w: unexpected record type %T", ErrHelmReleaseInvalid, record)
}

func projectHelmReleases(records []*releasev1.Release) ([]HelmRelease, error) {
	releases := make([]HelmRelease, 0, len(records))
	for _, record := range records {
		release, err := projectHelmRelease(record)
		if err != nil {
			return nil, err
		}
		releases = append(releases, release)
	}
	return releases, nil
}

func projectHelmRelease(record *releasev1.Release) (HelmRelease, error) {
	if err := validateStoredHelmRelease(record); err != nil {
		return HelmRelease{}, err
	}
	metadata := record.Chart.Metadata
	return HelmRelease{
		Name:         record.Name,
		Namespace:    record.Namespace,
		Revision:     record.Version,
		Status:       record.Info.Status.String(),
		ChartName:    metadata.Name,
		ChartVersion: metadata.Version,
		AppVersion:   metadata.AppVersion,
		UpdatedAt:    record.Info.LastDeployed,
	}, nil
}

func validateStoredHelmRelease(record *releasev1.Release) error {
	if record == nil {
		return fmt.Errorf("%w: record is nil", ErrHelmReleaseInvalid)
	}
	if err := validateHelmReleaseIdentity(record); err != nil {
		return err
	}
	return validateHelmReleaseChart(record.Chart)
}

func validateHelmReleaseIdentity(record *releasev1.Release) error {
	switch {
	case strings.TrimSpace(record.Name) == "":
		return fmt.Errorf("%w: name is empty", ErrHelmReleaseInvalid)
	case strings.TrimSpace(record.Namespace) == "":
		return fmt.Errorf("%w: namespace is empty", ErrHelmReleaseInvalid)
	case record.Version < 1:
		return fmt.Errorf("%w: revision must be positive", ErrHelmReleaseInvalid)
	case record.Info == nil:
		return fmt.Errorf("%w: status is missing", ErrHelmReleaseInvalid)
	default:
		return nil
	}
}

func validateHelmReleaseChart(releaseChart *chart.Chart) error {
	switch {
	case releaseChart == nil || releaseChart.Metadata == nil:
		return fmt.Errorf("%w: chart metadata is missing", ErrHelmReleaseInvalid)
	case strings.TrimSpace(releaseChart.Metadata.Name) == "":
		return fmt.Errorf("%w: chart name is empty", ErrHelmReleaseInvalid)
	case strings.TrimSpace(releaseChart.Metadata.Version) == "":
		return fmt.Errorf("%w: chart version is empty", ErrHelmReleaseInvalid)
	default:
		return nil
	}
}

func latestHelmReleases(releases []HelmRelease) []HelmRelease {
	type releaseKey struct {
		namespace string
		name      string
	}
	latest := make(map[releaseKey]HelmRelease, len(releases))
	for _, release := range releases {
		key := releaseKey{namespace: release.Namespace, name: release.Name}
		current, found := latest[key]
		if !found || release.Revision > current.Revision {
			latest[key] = release
		}
	}
	result := make([]HelmRelease, 0, len(latest))
	for _, release := range latest {
		result = append(result, release)
	}
	slices.SortFunc(result, func(left, right HelmRelease) int {
		if namespaceOrder := cmp.Compare(left.Namespace, right.Namespace); namespaceOrder != 0 {
			return namespaceOrder
		}
		return cmp.Compare(left.Name, right.Name)
	})
	return result
}

func validateHelmReleaseReference(reference HelmReleaseReference) error {
	if strings.TrimSpace(reference.Namespace) == "" {
		return ErrNamespaceRequired
	}
	if strings.TrimSpace(reference.Name) == "" {
		return ErrHelmReleaseNameRequired
	}
	return nil
}

func encodeHelmValues(record *releasev1.Release) (string, error) {
	if record == nil {
		return "", fmt.Errorf("%w: record is nil", ErrHelmReleaseInvalid)
	}
	if len(record.Config) == 0 {
		return "", nil
	}
	encoded, err := yaml.Marshal(record.Config)
	if err != nil {
		return "", fmt.Errorf("encode helm values: %w", err)
	}
	return string(encoded), nil
}

func (c *contextualSecretClient) Get(
	_ context.Context,
	name string,
	options metav1.GetOptions,
) (*corev1.Secret, error) {
	if c == nil || c.SecretInterface == nil || c.ctx == nil {
		return nil, ErrHelmStorageUnavailable
	}
	if err := c.ctx.Err(); err != nil {
		return nil, err
	}
	return c.SecretInterface.Get(c.ctx, name, options)
}

func (c *contextualSecretClient) List(
	_ context.Context,
	options metav1.ListOptions,
) (*corev1.SecretList, error) {
	if c == nil || c.SecretInterface == nil || c.ctx == nil {
		return nil, ErrHelmStorageUnavailable
	}
	if err := c.ctx.Err(); err != nil {
		return nil, err
	}
	return c.SecretInterface.List(c.ctx, options)
}

func isNilKubernetesClient(client kubernetes.Interface) bool {
	if client == nil {
		return true
	}
	value := reflect.ValueOf(client)
	return value.Kind() == reflect.Pointer && value.IsNil()
}

func isNilSecretClient(client typedcorev1.SecretInterface) bool {
	if client == nil {
		return true
	}
	value := reflect.ValueOf(client)
	return value.Kind() == reflect.Pointer && value.IsNil()
}

func isNilCoreClient(client typedcorev1.CoreV1Interface) bool {
	if client == nil {
		return true
	}
	value := reflect.ValueOf(client)
	return value.Kind() == reflect.Pointer && value.IsNil()
}
