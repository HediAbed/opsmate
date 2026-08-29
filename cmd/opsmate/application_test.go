package main

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/HediAbed/opsmate"
	"github.com/HediAbed/opsmate/failure"
	"github.com/HediAbed/opsmate/internal/kube"
	"github.com/HediAbed/opsmate/internal/session"
	"github.com/HediAbed/opsmate/internal/ui"
)

type fakeContextManager struct {
	kube.ResourceReader
	kube.ResourceObserver
	kube.HelmReader
	kube.ClusterOperations
}

func (*fakeContextManager) Connect(context.Context, string) error {
	return nil
}

func (*fakeContextManager) Contexts(context.Context) ([]kube.ContextInfo, error) {
	return nil, nil
}

func (*fakeContextManager) CurrentContext(context.Context) (string, error) {
	return "test", nil
}

func (*fakeContextManager) Namespaces(context.Context) ([]string, error) {
	return nil, nil
}

func (*fakeContextManager) StopAllPortForwards(context.Context) error {
	return nil
}

type fakeProgramRunner struct {
	finalModel tea.Model
	err        error
}

func (runner fakeProgramRunner) Run() (tea.Model, error) {
	return runner.finalModel, runner.err
}

type alternateModel struct {
	tea.Model
}

type failingWriter struct {
	writes int
}

type runtimeCallCounts struct {
	environmentLoads        int
	providerInitializations int
	clusterConnections      int
	snapshotCollectors      int
	sessionLoads            int
	programStarts           int
	sessionSaves            int
	shutdowns               int
}

func (writer *failingWriter) Write(_ []byte) (int, error) {
	writer.writes++
	return 0, errors.New("write failed")
}

func TestApplicationShowsHelpWithoutStartingRuntime(t *testing.T) {
	output := &bytes.Buffer{}
	errorOutput := &bytes.Buffer{}
	application := newApplication(output, errorOutput)
	application.dependencies.loadEnvironment = func() error {
		t.Fatal("help must not load environment configuration")
		return nil
	}

	exitCode := application.run([]string{"--help"})

	if exitCode != exitSuccess {
		t.Fatalf("exit code = %d, want %d", exitCode, exitSuccess)
	}
	if output.String() != usageText {
		t.Fatalf("output = %q, want %q", output.String(), usageText)
	}
	if errorOutput.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", errorOutput.String())
	}
}

func TestApplicationShowsVersionWithoutStartingRuntime(t *testing.T) {
	output := &bytes.Buffer{}
	errorOutput := &bytes.Buffer{}
	application := newApplication(output, errorOutput)
	application.dependencies.loadEnvironment = func() error {
		t.Fatal("version must not load environment configuration")
		return nil
	}

	exitCode := application.run([]string{"--version"})

	if exitCode != exitSuccess {
		t.Fatalf("exit code = %d, want %d", exitCode, exitSuccess)
	}
	expectedOutput := "opsmate " + opsmate.Version() + "\n"
	if output.String() != expectedOutput {
		t.Fatalf("output = %q, want %q", output.String(), expectedOutput)
	}
	if errorOutput.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", errorOutput.String())
	}
}

func TestApplicationRejectsInvalidArguments(t *testing.T) {
	output := &bytes.Buffer{}
	errorOutput := &bytes.Buffer{}
	application := newApplication(output, errorOutput)

	exitCode := application.run([]string{"--unknown"})

	if exitCode != exitUsage {
		t.Fatalf("exit code = %d, want %d", exitCode, exitUsage)
	}
	if output.Len() != 0 {
		t.Fatalf("stdout = %q, want empty", output.String())
	}
	expectedError := "level=ERROR msg=\"invalid arguments\" code=invalid_argument error=\"unknown option \\\"--unknown\\\"\"\n" + usageText
	if errorOutput.String() != expectedError {
		t.Fatalf("stderr = %q, want %q", errorOutput.String(), expectedError)
	}
}

func TestApplicationReturnsUsageWhenErrorOutputFails(t *testing.T) {
	errorOutput := &failingWriter{}
	application := newApplication(&bytes.Buffer{}, errorOutput)

	exitCode := application.run([]string{"one", "two"})

	if exitCode != exitUsage {
		t.Fatalf("exit code = %d, want %d", exitCode, exitUsage)
	}
	if errorOutput.writes == 0 {
		t.Fatal("application did not attempt to report the argument error")
	}
}

func TestApplicationReturnsFailureWhenStandardOutputFails(t *testing.T) {
	output := &failingWriter{}
	errorOutput := &bytes.Buffer{}
	application := newApplication(output, errorOutput)

	exitCode := application.run([]string{"--help"})

	if exitCode != exitFailure {
		t.Fatalf("exit code = %d, want %d", exitCode, exitFailure)
	}
	if output.writes != 1 {
		t.Fatalf("write attempts = %d, want 1", output.writes)
	}
	expectedError := "level=ERROR msg=\"write output\" code=unknown error=\"write failed\"\n"
	if errorOutput.String() != expectedError {
		t.Fatalf("stderr = %q, want %q", errorOutput.String(), expectedError)
	}
}

func applicationCountingRuntimeCalls(t *testing.T, output, errorOutput *bytes.Buffer, callCounts *runtimeCallCounts) application {
	t.Helper()
	application := newApplication(output, errorOutput)
	application.dependencies.loadEnvironment = func() error {
		callCounts.environmentLoads++
		return nil
	}
	application.dependencies.initializeProvider = func() error {
		callCounts.providerInitializations++
		return nil
	}
	application.dependencies.connectCluster = func(context.Context) (kube.Cluster, error) {
		callCounts.clusterConnections++
		return &fakeContextManager{}, nil
	}
	application.dependencies.newSnapshotCollector = func(
		contexts kube.ContextManager,
		resources kube.ResourceReader,
	) (*kube.SnapshotCollector, error) {
		callCounts.snapshotCollectors++
		return kube.NewSnapshotCollector(contexts, resources)
	}
	application.dependencies.loadSession = func() (session.SessionState, error) {
		callCounts.sessionLoads++
		return session.SessionState{Namespace: "saved"}, nil
	}
	application.dependencies.newProgram = func(initialModel tea.Model) programRunner {
		callCounts.programStarts++
		root, ok := initialModel.(ui.RootModel)
		if !ok {
			t.Fatalf("initial model = %T, want ui.RootModel", initialModel)
		}
		return fakeProgramRunner{finalModel: root}
	}
	application.dependencies.saveSession = func(ui.RootModel) error {
		callCounts.sessionSaves++
		return nil
	}
	application.dependencies.stopPortForwards = func(ctx context.Context, manager kube.PortForwardManager) error {
		callCounts.shutdowns++
		if manager == nil {
			t.Fatal("shutdown manager is nil")
		}
		if _, hasDeadline := ctx.Deadline(); !hasDeadline {
			t.Fatal("shutdown context has no deadline")
		}
		return nil
	}
	return application
}

func TestApplicationRunsTerminalAndPersistsSession(t *testing.T) {
	output := &bytes.Buffer{}
	errorOutput := &bytes.Buffer{}
	callCounts := runtimeCallCounts{}
	application := applicationCountingRuntimeCalls(t, output, errorOutput, &callCounts)

	exitCode := application.run(nil)

	if exitCode != exitSuccess {
		t.Fatalf("exit code = %d, want %d", exitCode, exitSuccess)
	}
	expectedCallCounts := runtimeCallCounts{
		environmentLoads:        1,
		providerInitializations: 1,
		clusterConnections:      1,
		snapshotCollectors:      1,
		sessionLoads:            1,
		programStarts:           1,
		sessionSaves:            1,
		shutdowns:               1,
	}
	if callCounts != expectedCallCounts {
		t.Fatalf("call counts = %+v, want %+v", callCounts, expectedCallCounts)
	}
	if output.Len() != 0 || errorOutput.Len() != 0 {
		t.Fatalf("stdout = %q, stderr = %q, want both empty", output.String(), errorOutput.String())
	}
}

func TestApplicationRejectsInvalidEnvironmentConfiguration(t *testing.T) {
	errorOutput := &bytes.Buffer{}
	application := applicationWithSuccessfulTerminal(errorOutput)
	shutdowns := 0
	application.dependencies.loadEnvironment = func() error {
		return errors.New("invalid environment")
	}
	application.dependencies.loadSession = func() (session.SessionState, error) {
		t.Fatal("invalid environment must stop startup before loading a session")
		return session.SessionState{}, nil
	}
	application.dependencies.initializeProvider = func() error {
		t.Fatal("invalid environment must stop startup before provider initialization")
		return nil
	}
	application.dependencies.newProgram = func(tea.Model) programRunner {
		t.Fatal("invalid environment must stop startup before launching the terminal")
		return nil
	}
	application.dependencies.saveSession = func(ui.RootModel) error {
		t.Fatal("invalid environment must stop startup before saving a session")
		return nil
	}
	application.dependencies.stopPortForwards = func(context.Context, kube.PortForwardManager) error {
		shutdowns++
		return nil
	}

	exitCode := application.run(nil)

	if exitCode != exitFailure {
		t.Fatalf("exit code = %d, want %d", exitCode, exitFailure)
	}
	expectedError := "level=ERROR msg=\"load environment configuration\" code=unknown error=\"invalid environment\"\n"
	if errorOutput.String() != expectedError {
		t.Fatalf("stderr = %q, want %q", errorOutput.String(), expectedError)
	}
	if shutdowns != 0 {
		t.Fatalf("shutdowns = %d, want 0 before terminal startup", shutdowns)
	}
}

func TestApplicationRejectsInvalidProviderConfiguration(t *testing.T) {
	errorOutput := &bytes.Buffer{}
	application := applicationWithSuccessfulTerminal(errorOutput)
	shutdowns := 0
	application.dependencies.initializeProvider = func() error {
		return errors.New("provider executable not found")
	}
	application.dependencies.loadSession = func() (session.SessionState, error) {
		t.Fatal("invalid provider must stop startup before loading a session")
		return session.SessionState{}, nil
	}
	application.dependencies.newProgram = func(tea.Model) programRunner {
		t.Fatal("invalid provider must stop startup before launching the terminal")
		return nil
	}
	application.dependencies.saveSession = func(ui.RootModel) error {
		t.Fatal("invalid provider must stop startup before saving a session")
		return nil
	}
	application.dependencies.stopPortForwards = func(context.Context, kube.PortForwardManager) error {
		shutdowns++
		return nil
	}

	exitCode := application.run(nil)

	if exitCode != exitFailure {
		t.Fatalf("exit code = %d, want %d", exitCode, exitFailure)
	}
	expectedError := "level=ERROR msg=\"initialize analysis provider\" code=unknown error=\"provider executable not found\"\n"
	if errorOutput.String() != expectedError {
		t.Fatalf("stderr = %q, want %q", errorOutput.String(), expectedError)
	}
	if shutdowns != 0 {
		t.Fatalf("shutdowns = %d, want 0 before terminal startup", shutdowns)
	}
}

func TestApplicationRejectsClusterConnectionFailure(t *testing.T) {
	errorOutput := &bytes.Buffer{}
	application := applicationWithSuccessfulTerminal(errorOutput)
	application.dependencies.connectCluster = func(context.Context) (kube.Cluster, error) {
		return nil, errors.New("configuration unavailable")
	}
	application.dependencies.loadSession = func() (session.SessionState, error) {
		t.Fatal("cluster failure must stop startup before loading a session")
		return session.SessionState{}, nil
	}
	application.dependencies.newProgram = func(tea.Model) programRunner {
		t.Fatal("cluster failure must stop startup before launching the terminal")
		return nil
	}

	exitCode := application.run(nil)

	if exitCode != exitFailure {
		t.Fatalf("exit code = %d, want %d", exitCode, exitFailure)
	}
	expectedError := "level=ERROR msg=\"connect cluster\" code=unknown error=\"configuration unavailable\"\n"
	if errorOutput.String() != expectedError {
		t.Fatalf("stderr = %q, want %q", errorOutput.String(), expectedError)
	}
}

func TestApplicationRejectsSnapshotCollectorFailure(t *testing.T) {
	errorOutput := &bytes.Buffer{}
	application := applicationWithSuccessfulTerminal(errorOutput)
	sentinel := errors.New("snapshot collector unavailable")
	application.dependencies.newSnapshotCollector = func(
		kube.ContextManager,
		kube.ResourceReader,
	) (*kube.SnapshotCollector, error) {
		return nil, sentinel
	}
	application.dependencies.loadSession = func() (session.SessionState, error) {
		t.Fatal("snapshot collector failure must stop startup before loading a session")
		return session.SessionState{}, nil
	}

	exitCode := application.run(nil)

	if exitCode != exitFailure {
		t.Fatalf("exit code = %d, want %d", exitCode, exitFailure)
	}
	expectedError := "level=ERROR msg=\"create cluster snapshot collector\" code=unknown error=\"snapshot collector unavailable\"\n"
	if errorOutput.String() != expectedError {
		t.Fatalf("stderr = %q, want %q", errorOutput.String(), expectedError)
	}
}

func TestApplicationRejectsUnreadableSession(t *testing.T) {
	errorOutput := &bytes.Buffer{}
	application := applicationWithSuccessfulTerminal(errorOutput)
	shutdowns := 0
	application.dependencies.loadSession = func() (session.SessionState, error) {
		return session.SessionState{}, errors.New("corrupt session")
	}
	application.dependencies.newProgram = func(tea.Model) programRunner {
		t.Fatal("invalid session must stop startup before launching the terminal")
		return nil
	}
	application.dependencies.saveSession = func(ui.RootModel) error {
		t.Fatal("invalid session must stop startup before saving a session")
		return nil
	}
	application.dependencies.stopPortForwards = func(context.Context, kube.PortForwardManager) error {
		shutdowns++
		return nil
	}

	exitCode := application.run(nil)

	if exitCode != exitFailure {
		t.Fatalf("exit code = %d, want %d", exitCode, exitFailure)
	}
	expectedError := "level=ERROR msg=\"load session state\" code=unknown error=\"corrupt session\"\n"
	if errorOutput.String() != expectedError {
		t.Fatalf("stderr = %q, want %q", errorOutput.String(), expectedError)
	}
	if shutdowns != 0 {
		t.Fatalf("shutdowns = %d, want 0 before terminal startup", shutdowns)
	}
}

func TestApplicationRejectsModelConstructionFailure(t *testing.T) {
	errorOutput := &bytes.Buffer{}
	application := applicationWithSuccessfulTerminal(errorOutput)
	application.dependencies.newRootModel = func(string, ui.RuntimeDependencies) (ui.RootModel, error) {
		return ui.RootModel{}, errors.New("invalid runtime dependencies")
	}
	application.dependencies.newProgram = func(tea.Model) programRunner {
		t.Fatal("model construction failure must stop before launching the terminal")
		return nil
	}
	application.dependencies.saveSession = func(ui.RootModel) error {
		t.Fatal("model construction failure must stop before saving a session")
		return nil
	}

	exitCode := application.run(nil)

	if exitCode != exitFailure {
		t.Fatalf("exit code = %d, want %d", exitCode, exitFailure)
	}
	expectedError := "level=ERROR msg=\"create terminal model\" code=unknown error=\"invalid runtime dependencies\"\n"
	if errorOutput.String() != expectedError {
		t.Fatalf("stderr = %q, want %q", errorOutput.String(), expectedError)
	}
}

func TestApplicationTreatsMissingSessionAsNormal(t *testing.T) {
	errorOutput := &bytes.Buffer{}
	application := applicationWithSuccessfulTerminal(errorOutput)
	application.dependencies.loadSession = func() (session.SessionState, error) {
		return session.SessionState{}, session.ErrNoSession
	}

	exitCode := application.run(nil)

	if exitCode != exitSuccess {
		t.Fatalf("exit code = %d, want %d", exitCode, exitSuccess)
	}
	if errorOutput.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", errorOutput.String())
	}
}

func TestApplicationReturnsFailureWhenTerminalFails(t *testing.T) {
	errorOutput := &bytes.Buffer{}
	application := applicationWithSuccessfulTerminal(errorOutput)
	shutdowns := 0
	application.dependencies.newProgram = func(tea.Model) programRunner {
		return fakeProgramRunner{err: errors.New("terminal error")}
	}
	application.dependencies.saveSession = func(ui.RootModel) error {
		t.Fatal("failed terminal must not save session")
		return nil
	}
	application.dependencies.stopPortForwards = func(context.Context, kube.PortForwardManager) error {
		shutdowns++
		return nil
	}

	exitCode := application.run(nil)

	if exitCode != exitFailure {
		t.Fatalf("exit code = %d, want %d", exitCode, exitFailure)
	}
	if shutdowns != 1 {
		t.Fatalf("shutdowns = %d, want 1", shutdowns)
	}
	expectedError := "level=ERROR msg=\"terminal failed\" code=unknown error=\"terminal error\"\n"
	if errorOutput.String() != expectedError {
		t.Fatalf("stderr = %q, want %q", errorOutput.String(), expectedError)
	}
}

func TestApplicationRejectsUnexpectedFinalModel(t *testing.T) {
	errorOutput := &bytes.Buffer{}
	application := applicationWithSuccessfulTerminal(errorOutput)
	shutdowns := 0
	application.dependencies.newProgram = func(tea.Model) programRunner {
		return fakeProgramRunner{finalModel: alternateModel{}}
	}
	application.dependencies.saveSession = func(ui.RootModel) error {
		t.Fatal("unexpected final model must not save session")
		return nil
	}
	application.dependencies.stopPortForwards = func(context.Context, kube.PortForwardManager) error {
		shutdowns++
		return nil
	}

	exitCode := application.run(nil)

	if exitCode != exitFailure {
		t.Fatalf("exit code = %d, want %d", exitCode, exitFailure)
	}
	expectedError := "level=ERROR msg=\"terminal returned invalid state\" code=internal error=\"terminal returned main.alternateModel, want ui.RootModel\"\n"
	if errorOutput.String() != expectedError {
		t.Fatalf("stderr = %q, want %q", errorOutput.String(), expectedError)
	}
	if shutdowns != 1 {
		t.Fatalf("shutdowns = %d, want 1 after terminal startup", shutdowns)
	}
}

func TestApplicationReportsSessionSaveFailure(t *testing.T) {
	errorOutput := &bytes.Buffer{}
	application := applicationWithSuccessfulTerminal(errorOutput)
	shutdowns := 0
	application.dependencies.saveSession = func(ui.RootModel) error {
		return errors.New("storage unavailable")
	}
	application.dependencies.stopPortForwards = func(context.Context, kube.PortForwardManager) error {
		shutdowns++
		return nil
	}

	exitCode := application.run(nil)

	if exitCode != exitFailure {
		t.Fatalf("exit code = %d, want %d", exitCode, exitFailure)
	}
	expectedError := "level=ERROR msg=\"save session state\" code=unknown error=\"storage unavailable\"\n"
	if errorOutput.String() != expectedError {
		t.Fatalf("stderr = %q, want %q", errorOutput.String(), expectedError)
	}
	if shutdowns != 1 {
		t.Fatalf("shutdowns = %d, want 1 after terminal startup", shutdowns)
	}
}

func TestApplicationReportsPortForwardShutdownFailure(t *testing.T) {
	errorOutput := &bytes.Buffer{}
	application := applicationWithSuccessfulTerminal(errorOutput)
	application.dependencies.stopPortForwards = func(context.Context, kube.PortForwardManager) error {
		return errors.New("shutdown unavailable")
	}

	exitCode := application.run(nil)

	if exitCode != exitFailure {
		t.Fatalf("exit code = %d, want %d", exitCode, exitFailure)
	}
	expectedError := "level=ERROR msg=\"stop port forwards\" code=unknown error=\"shutdown unavailable\"\n"
	if errorOutput.String() != expectedError {
		t.Fatalf("stderr = %q, want %q", errorOutput.String(), expectedError)
	}
}

func TestResolveStartupNamespace(t *testing.T) {
	if namespace := resolveStartupNamespace("explicit", "saved"); namespace != "explicit" {
		t.Fatalf("namespace with override = %q, want %q", namespace, "explicit")
	}
	if namespace := resolveStartupNamespace("", "saved"); namespace != "saved" {
		t.Fatalf("namespace without override = %q, want %q", namespace, "saved")
	}
}

func TestRootFromFinalModelAcceptsValueAndPointer(t *testing.T) {
	root := newApplicationRootModel(t, "")
	fromValue, err := rootFromFinalModel(root)
	if err != nil {
		t.Fatalf("root value: %v", err)
	}
	if fromValue.View().Content != root.View().Content {
		t.Fatal("root value was not preserved")
	}
	fromPointer, err := rootFromFinalModel(&root)
	if err != nil {
		t.Fatalf("root pointer: %v", err)
	}
	if fromPointer.View().Content != root.View().Content {
		t.Fatal("root pointer was not preserved")
	}
}

func TestRootFromFinalModelRejectsNilPointerAndOtherModel(t *testing.T) {
	var nilRoot *ui.RootModel
	for _, testCase := range []struct {
		name          string
		finalModel    tea.Model
		expectedError string
	}{
		{name: "nil pointer", finalModel: nilRoot, expectedError: "terminal returned *ui.RootModel, want ui.RootModel"},
		{name: "other model", finalModel: alternateModel{}, expectedError: "terminal returned main.alternateModel, want ui.RootModel"},
		{name: "nil model", finalModel: nil, expectedError: "terminal returned <nil>, want ui.RootModel"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			_, err := rootFromFinalModel(testCase.finalModel)
			var typedError *finalModelError
			if !errors.As(err, &typedError) {
				t.Fatalf("error = %T, want *finalModelError", err)
			}
			if err.Error() != testCase.expectedError {
				t.Fatalf("error = %q, want %q", err, testCase.expectedError)
			}
			if code := failure.CodeOf(err); code != failure.CodeInternal {
				t.Fatalf("failure code = %q, want %q", code, failure.CodeInternal)
			}
		})
	}
}

func TestFinalModelErrorHandlesMissingType(t *testing.T) {
	var nilError *finalModelError
	for _, err := range []*finalModelError{nilError, {}} {
		if got := err.Error(); got != "terminal returned an invalid model" {
			t.Fatalf("Error() = %q, want generic final-model error", got)
		}
	}
}

func applicationWithSuccessfulTerminal(errorOutput *bytes.Buffer) application {
	application := newApplication(&bytes.Buffer{}, errorOutput)
	application.dependencies.loadEnvironment = func() error { return nil }
	application.dependencies.initializeProvider = func() error { return nil }
	application.dependencies.connectCluster = func(context.Context) (kube.Cluster, error) {
		return &fakeContextManager{}, nil
	}
	application.dependencies.loadSession = func() (session.SessionState, error) {
		return session.SessionState{}, nil
	}
	application.dependencies.newProgram = func(initialModel tea.Model) programRunner {
		return fakeProgramRunner{finalModel: initialModel}
	}
	application.dependencies.saveSession = func(ui.RootModel) error { return nil }
	return application
}

func TestStructuredLoggerOmitsTimestamp(t *testing.T) {
	errorOutput := &bytes.Buffer{}
	application := newApplication(&bytes.Buffer{}, errorOutput)
	application.logger.Info("message", "field", "value")

	actual := errorOutput.String()
	if strings.Contains(actual, "time=") {
		t.Fatalf("log output contains timestamp: %q", actual)
	}
	if actual != "level=INFO msg=message field=value\n" {
		t.Fatalf("log output = %q, want structured fields", actual)
	}
}

func TestNewTerminalProgramReturnsRunner(t *testing.T) {
	runner := newTerminalProgram(newApplicationRootModel(t, ""))
	if runner == nil {
		t.Fatal("newTerminalProgram returned nil")
	}
}

func newApplicationRootModel(t *testing.T, namespace string) ui.RootModel {
	t.Helper()
	cluster := &fakeContextManager{}
	snapshots, err := kube.NewSnapshotCollector(cluster, cluster)
	if err != nil {
		t.Fatalf("NewSnapshotCollector() error = %v", err)
	}
	root, err := ui.NewRootModel(namespace, ui.RuntimeDependencies{
		Context:           context.Background(),
		ClusterContext:    cluster,
		ClusterResources:  cluster,
		ClusterSnapshots:  snapshots,
		ClusterObserver:   cluster,
		ClusterOperations: cluster,
		HelmReleases:      cluster,
	})
	if err != nil {
		t.Fatalf("NewRootModel() error = %v", err)
	}
	return root
}
