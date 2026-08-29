package kube

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	kubernetesfake "k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/rest"
)

type trackingReadCloser struct {
	reader     io.Reader
	closeCalls int
	closeErr   error
}

func (c *trackingReadCloser) Read(buffer []byte) (int, error) {
	return c.reader.Read(buffer)
}

func (c *trackingReadCloser) Close() error {
	c.closeCalls++
	return c.closeErr
}

func TestPodContainersValidatesRequestAndDependencies(t *testing.T) {
	reference := PodReference{Namespace: "team-a", Name: "web"}
	unavailable, err := NewManager(&fakeConfigSource{}, &fakeClientBuilder{})
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}
	tests := []struct {
		name      string
		manager   *Manager
		ctx       context.Context
		reference PodReference
		wantErr   error
	}{
		{name: "missing context", manager: unavailable, reference: reference, wantErr: ErrContextRequired},
		{name: "invalid pod", manager: unavailable, ctx: context.Background(), reference: PodReference{Name: "web"}, wantErr: ErrNamespaceRequired},
		{name: "missing clients", manager: unavailable, ctx: context.Background(), reference: reference, wantErr: ErrClientUnavailable},
		{name: "missing typed client", manager: managerWithClientsForTest(t, &Clients{}), ctx: context.Background(), reference: reference, wantErr: ErrTypedClientUnavailable},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			containers, readErr := test.manager.PodContainers(test.ctx, test.reference)
			if containers != nil || !errors.Is(readErr, test.wantErr) {
				t.Fatalf("PodContainers() = (%v, %v), want error %v", containers, readErr, test.wantErr)
			}
		})
	}
}

func TestPodContainersReportsReadFailure(t *testing.T) {
	manager := managerWithClientsForTest(t, &Clients{kubernetes: kubernetesfake.NewSimpleClientset()})
	containers, err := manager.PodContainers(context.Background(), PodReference{Namespace: "team-a", Name: "missing"})
	if containers != nil || err == nil {
		t.Fatalf("PodContainers() = (%v, %v), want read failure", containers, err)
	}
	var resourceErr *Error
	if !errors.As(err, &resourceErr) || resourceErr.Operation != OperationGet || resourceErr.Subject != SubjectPod {
		t.Fatalf("PodContainers() error = %#v, want get pod error", resourceErr)
	}
}

func TestPodContainersReturnsEveryContainerTypeInDeclaredOrder(t *testing.T) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "web", Namespace: "team-a"},
		Spec: corev1.PodSpec{
			InitContainers: []corev1.Container{{Name: "migrate"}},
			Containers:     []corev1.Container{{Name: "app"}, {Name: "proxy"}},
			EphemeralContainers: []corev1.EphemeralContainer{
				{EphemeralContainerCommon: corev1.EphemeralContainerCommon{Name: "debug"}},
			},
		},
	}
	manager := managerWithClientsForTest(t, &Clients{kubernetes: kubernetesfake.NewSimpleClientset(pod)})
	containers, err := manager.PodContainers(context.Background(), PodReference{Namespace: pod.Namespace, Name: pod.Name})
	if err != nil {
		t.Fatalf("PodContainers() error = %v", err)
	}
	want := []string{"migrate", "app", "proxy", "debug"}
	if !slices.Equal(containers, want) {
		t.Fatalf("PodContainers() = %v, want %v", containers, want)
	}
}

func TestOpenPodLogsValidatesRequestAndDependencies(t *testing.T) {
	request := PodLogRequest{
		Pod:       PodReference{Namespace: "team-a", Name: "web"},
		Container: "app",
		TailLines: 20,
	}
	unavailable, err := NewManager(&fakeConfigSource{}, &fakeClientBuilder{})
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}
	tests := []struct {
		name    string
		manager *Manager
		ctx     context.Context
		request PodLogRequest
		wantErr error
	}{
		{name: "missing context", manager: unavailable, request: request, wantErr: ErrContextRequired},
		{name: "invalid pod", manager: unavailable, ctx: context.Background(), request: PodLogRequest{Pod: PodReference{Name: "web"}, TailLines: 20}, wantErr: ErrNamespaceRequired},
		{name: "invalid tail", manager: unavailable, ctx: context.Background(), request: PodLogRequest{Pod: request.Pod}, wantErr: ErrLogTailLinesInvalid},
		{name: "missing clients", manager: unavailable, ctx: context.Background(), request: request, wantErr: ErrClientUnavailable},
		{name: "missing reader", manager: managerWithClientsForTest(t, &Clients{}), ctx: context.Background(), request: request, wantErr: ErrPodLogReaderUnavailable},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			stream, openErr := test.manager.OpenPodLogs(test.ctx, test.request)
			if stream != nil || !errors.Is(openErr, test.wantErr) {
				t.Fatalf("OpenPodLogs() = (%v, %v), want error %v", stream, openErr, test.wantErr)
			}
		})
	}
}

func TestOpenPodLogsHandlesReaderFailures(t *testing.T) {
	request := PodLogRequest{Pod: PodReference{Namespace: "team-a", Name: "web"}, TailLines: 20}
	sentinel := errors.New("stream failed")
	tests := []struct {
		name    string
		opener  func(context.Context, string, string, *corev1.PodLogOptions) (io.ReadCloser, error)
		wantErr error
	}{
		{name: "open failure", opener: func(context.Context, string, string, *corev1.PodLogOptions) (io.ReadCloser, error) {
			return nil, sentinel
		}, wantErr: sentinel},
		{name: "empty stream", opener: func(context.Context, string, string, *corev1.PodLogOptions) (io.ReadCloser, error) { return nil, nil }, wantErr: ErrPodLogStreamUnavailable},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			manager := managerWithClientsForTest(t, &Clients{openPodLogs: test.opener})
			stream, err := manager.OpenPodLogs(context.Background(), request)
			if stream != nil || !errors.Is(err, test.wantErr) {
				t.Fatalf("OpenPodLogs() = (%v, %v), want error %v", stream, err, test.wantErr)
			}
		})
	}
}

func TestOpenPodLogsPassesRequestOptions(t *testing.T) {
	request := PodLogRequest{
		Pod:       PodReference{Namespace: "team-a", Name: "web"},
		Container: "app",
		TailLines: 20,
	}
	underlying := &trackingReadCloser{reader: strings.NewReader("line one\n")}
	manager := managerWithClientsForTest(t, &Clients{openPodLogs: func(
		_ context.Context,
		namespace string,
		pod string,
		options *corev1.PodLogOptions,
	) (io.ReadCloser, error) {
		if namespace != request.Pod.Namespace || pod != request.Pod.Name {
			t.Fatalf("log target = %s/%s, want %s/%s", namespace, pod, request.Pod.Namespace, request.Pod.Name)
		}
		if options.Container != request.Container || options.TailLines == nil || *options.TailLines != request.TailLines {
			t.Fatalf("log options = %#v, want container %q and tail %d", options, request.Container, request.TailLines)
		}
		return underlying, nil
	}})
	stream, err := manager.OpenPodLogs(context.Background(), request)
	if err != nil {
		t.Fatalf("OpenPodLogs() error = %v", err)
	}
	payload, err := io.ReadAll(stream)
	if err != nil || string(payload) != "line one\n" {
		t.Fatalf("ReadAll() = (%q, %v)", payload, err)
	}
	if err := stream.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
}

func TestOpenPodLogsOwnsStreamLifetime(t *testing.T) {
	request := PodLogRequest{Pod: PodReference{Namespace: "team-a", Name: "web"}, TailLines: 20}
	closeErr := errors.New("close failed")
	underlying := &trackingReadCloser{reader: strings.NewReader("line one\n"), closeErr: closeErr}
	var streamContext context.Context
	manager := managerWithClientsForTest(t, &Clients{openPodLogs: func(
		ctx context.Context,
		_ string,
		_ string,
		_ *corev1.PodLogOptions,
	) (io.ReadCloser, error) {
		streamContext = ctx
		return underlying, nil
	}})
	stream, err := manager.OpenPodLogs(context.Background(), request)
	if err != nil {
		t.Fatalf("OpenPodLogs() error = %v", err)
	}
	manager.cancelClients()
	select {
	case <-streamContext.Done():
	case <-time.After(time.Second):
		t.Fatal("stream context was not canceled with the client lifetime")
	}
	if err := stream.Close(); !errors.Is(err, closeErr) {
		t.Fatalf("Close() error = %v, want %v", err, closeErr)
	}
	if err := stream.Close(); !errors.Is(err, closeErr) || underlying.closeCalls != 1 {
		t.Fatalf("second Close() = %v, calls = %d", err, underlying.closeCalls)
	}
	var nilStream *sessionReadCloser
	if err := nilStream.Close(); err != nil {
		t.Fatalf("nil Close() error = %v", err)
	}
}

func TestClientsOpenPodLogsDelegates(t *testing.T) {
	var nilClients *Clients
	if stream, err := nilClients.OpenPodLogs(context.Background(), "team-a", "web", &corev1.PodLogOptions{}); stream != nil || !errors.Is(err, ErrPodLogReaderUnavailable) {
		t.Fatalf("nil OpenPodLogs() = (%v, %v)", stream, err)
	}
	if stream, err := (&Clients{}).OpenPodLogs(context.Background(), "team-a", "web", &corev1.PodLogOptions{}); stream != nil || !errors.Is(err, ErrPodLogReaderUnavailable) {
		t.Fatalf("empty OpenPodLogs() = (%v, %v)", stream, err)
	}
	want := &trackingReadCloser{reader: strings.NewReader("")}
	clients := &Clients{openPodLogs: func(context.Context, string, string, *corev1.PodLogOptions) (io.ReadCloser, error) {
		return want, nil
	}}
	stream, err := clients.OpenPodLogs(context.Background(), "team-a", "web", &corev1.PodLogOptions{})
	if err != nil || stream != want {
		t.Fatalf("OpenPodLogs() = (%v, %v), want delegated stream", stream, err)
	}
}

func TestPodLogOpenerUsesKubernetesLogEndpoint(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/api/v1/namespaces/team-a/pods/web/log" {
			t.Errorf("request path = %q", request.URL.Path)
		}
		if request.URL.Query().Get("container") != "app" || request.URL.Query().Get("tailLines") != "20" {
			t.Errorf("request query = %q", request.URL.RawQuery)
		}
		response.Header().Set("Content-Type", "text/plain")
		_, _ = response.Write([]byte("ready\n"))
	}))
	t.Cleanup(server.Close)
	client, err := kubernetes.NewForConfig(&rest.Config{Host: server.URL})
	if err != nil {
		t.Fatalf("NewForConfig() error = %v", err)
	}
	if opener := newPodLogOpener(nil); opener != nil {
		t.Fatal("newPodLogOpener(nil) returned a function")
	}
	tailLines := int64(20)
	opener := newPodLogOpener(client)
	stream, err := opener(context.Background(), "team-a", "web", &corev1.PodLogOptions{Container: "app", TailLines: &tailLines})
	if err != nil {
		t.Fatalf("open pod logs error = %v", err)
	}
	payload, err := io.ReadAll(stream)
	if err != nil || string(payload) != "ready\n" {
		t.Fatalf("ReadAll() = (%q, %v)", payload, err)
	}
	if err := stream.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
}
