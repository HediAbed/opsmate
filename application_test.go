package main

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/HediAbed/opsmate/internal/model"
	"github.com/HediAbed/opsmate/internal/service"
)

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
	expectedOutput := "opsmate " + currentVersion() + "\n"
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
	expectedError := "level=ERROR msg=\"invalid arguments\" error=\"unknown option \\\"--unknown\\\"\"\n" + usageText
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
	expectedError := "level=ERROR msg=\"write output\" error=\"write failed\"\n"
	if errorOutput.String() != expectedError {
		t.Fatalf("stderr = %q, want %q", errorOutput.String(), expectedError)
	}
}

func TestApplicationRunsTerminalAndPersistsSession(t *testing.T) {
	output := &bytes.Buffer{}
	errorOutput := &bytes.Buffer{}
	application := newApplication(output, errorOutput)
	callCounts := runtimeCallCounts{}

	application.dependencies.loadEnvironment = func() error {
		callCounts.environmentLoads++
		return nil
	}
	application.dependencies.initializeProvider = func() error {
		callCounts.providerInitializations++
		return nil
	}
	application.dependencies.loadSession = func() (service.SessionState, error) {
		callCounts.sessionLoads++
		return service.SessionState{Namespace: "saved"}, nil
	}
	application.dependencies.newProgram = func(initialModel tea.Model) programRunner {
		callCounts.programStarts++
		root, ok := initialModel.(model.RootModel)
		if !ok {
			t.Fatalf("initial model = %T, want model.RootModel", initialModel)
		}
		return fakeProgramRunner{finalModel: root}
	}
	application.dependencies.saveSession = func(model.RootModel) error {
		callCounts.sessionSaves++
		return nil
	}
	application.dependencies.stopPortForwards = func() {
		callCounts.shutdowns++
	}

	exitCode := application.run(nil)

	if exitCode != exitSuccess {
		t.Fatalf("exit code = %d, want %d", exitCode, exitSuccess)
	}
	expectedCallCounts := runtimeCallCounts{
		environmentLoads:        1,
		providerInitializations: 1,
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
	application.dependencies.loadSession = func() (service.SessionState, error) {
		t.Fatal("invalid environment must stop startup before loading a session")
		return service.SessionState{}, nil
	}
	application.dependencies.initializeProvider = func() error {
		t.Fatal("invalid environment must stop startup before provider initialization")
		return nil
	}
	application.dependencies.newProgram = func(tea.Model) programRunner {
		t.Fatal("invalid environment must stop startup before launching the terminal")
		return nil
	}
	application.dependencies.saveSession = func(model.RootModel) error {
		t.Fatal("invalid environment must stop startup before saving a session")
		return nil
	}
	application.dependencies.stopPortForwards = func() {
		shutdowns++
	}

	exitCode := application.run(nil)

	if exitCode != exitFailure {
		t.Fatalf("exit code = %d, want %d", exitCode, exitFailure)
	}
	expectedError := "level=ERROR msg=\"load environment configuration\" error=\"invalid environment\"\n"
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
	application.dependencies.loadSession = func() (service.SessionState, error) {
		t.Fatal("invalid provider must stop startup before loading a session")
		return service.SessionState{}, nil
	}
	application.dependencies.newProgram = func(tea.Model) programRunner {
		t.Fatal("invalid provider must stop startup before launching the terminal")
		return nil
	}
	application.dependencies.saveSession = func(model.RootModel) error {
		t.Fatal("invalid provider must stop startup before saving a session")
		return nil
	}
	application.dependencies.stopPortForwards = func() {
		shutdowns++
	}

	exitCode := application.run(nil)

	if exitCode != exitFailure {
		t.Fatalf("exit code = %d, want %d", exitCode, exitFailure)
	}
	expectedError := "level=ERROR msg=\"initialize analysis provider\" error=\"provider executable not found\"\n"
	if errorOutput.String() != expectedError {
		t.Fatalf("stderr = %q, want %q", errorOutput.String(), expectedError)
	}
	if shutdowns != 0 {
		t.Fatalf("shutdowns = %d, want 0 before terminal startup", shutdowns)
	}
}

func TestApplicationRejectsUnreadableSession(t *testing.T) {
	errorOutput := &bytes.Buffer{}
	application := applicationWithSuccessfulTerminal(errorOutput)
	shutdowns := 0
	application.dependencies.loadSession = func() (service.SessionState, error) {
		return service.SessionState{}, errors.New("corrupt session")
	}
	application.dependencies.newProgram = func(tea.Model) programRunner {
		t.Fatal("invalid session must stop startup before launching the terminal")
		return nil
	}
	application.dependencies.saveSession = func(model.RootModel) error {
		t.Fatal("invalid session must stop startup before saving a session")
		return nil
	}
	application.dependencies.stopPortForwards = func() {
		shutdowns++
	}

	exitCode := application.run(nil)

	if exitCode != exitFailure {
		t.Fatalf("exit code = %d, want %d", exitCode, exitFailure)
	}
	expectedError := "level=ERROR msg=\"load session state\" error=\"corrupt session\"\n"
	if errorOutput.String() != expectedError {
		t.Fatalf("stderr = %q, want %q", errorOutput.String(), expectedError)
	}
	if shutdowns != 0 {
		t.Fatalf("shutdowns = %d, want 0 before terminal startup", shutdowns)
	}
}

func TestApplicationTreatsMissingSessionAsNormal(t *testing.T) {
	errorOutput := &bytes.Buffer{}
	application := applicationWithSuccessfulTerminal(errorOutput)
	application.dependencies.loadSession = func() (service.SessionState, error) {
		return service.SessionState{}, service.ErrNoSession
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
	application.dependencies.saveSession = func(model.RootModel) error {
		t.Fatal("failed terminal must not save session")
		return nil
	}
	application.dependencies.stopPortForwards = func() {
		shutdowns++
	}

	exitCode := application.run(nil)

	if exitCode != exitFailure {
		t.Fatalf("exit code = %d, want %d", exitCode, exitFailure)
	}
	if shutdowns != 1 {
		t.Fatalf("shutdowns = %d, want 1", shutdowns)
	}
	expectedError := "level=ERROR msg=\"terminal failed\" error=\"terminal error\"\n"
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
	application.dependencies.saveSession = func(model.RootModel) error {
		t.Fatal("unexpected final model must not save session")
		return nil
	}
	application.dependencies.stopPortForwards = func() {
		shutdowns++
	}

	exitCode := application.run(nil)

	if exitCode != exitFailure {
		t.Fatalf("exit code = %d, want %d", exitCode, exitFailure)
	}
	expectedError := "level=ERROR msg=\"terminal returned invalid state\" error=\"terminal returned main.alternateModel, want model.RootModel\"\n"
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
	application.dependencies.saveSession = func(model.RootModel) error {
		return errors.New("storage unavailable")
	}
	application.dependencies.stopPortForwards = func() {
		shutdowns++
	}

	exitCode := application.run(nil)

	if exitCode != exitFailure {
		t.Fatalf("exit code = %d, want %d", exitCode, exitFailure)
	}
	expectedError := "level=ERROR msg=\"save session state\" error=\"storage unavailable\"\n"
	if errorOutput.String() != expectedError {
		t.Fatalf("stderr = %q, want %q", errorOutput.String(), expectedError)
	}
	if shutdowns != 1 {
		t.Fatalf("shutdowns = %d, want 1 after terminal startup", shutdowns)
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
	root := model.NewRootModel("")
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
	var nilRoot *model.RootModel
	for _, testCase := range []struct {
		name          string
		finalModel    tea.Model
		expectedError string
	}{
		{name: "nil pointer", finalModel: nilRoot, expectedError: "terminal returned *model.RootModel, want model.RootModel"},
		{name: "other model", finalModel: alternateModel{}, expectedError: "terminal returned main.alternateModel, want model.RootModel"},
		{name: "nil model", finalModel: nil, expectedError: "terminal returned <nil>, want model.RootModel"},
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
		})
	}
}

func applicationWithSuccessfulTerminal(errorOutput *bytes.Buffer) application {
	application := newApplication(&bytes.Buffer{}, errorOutput)
	application.dependencies.loadEnvironment = func() error { return nil }
	application.dependencies.initializeProvider = func() error { return nil }
	application.dependencies.loadSession = func() (service.SessionState, error) {
		return service.SessionState{}, nil
	}
	application.dependencies.newProgram = func(initialModel tea.Model) programRunner {
		return fakeProgramRunner{finalModel: initialModel}
	}
	application.dependencies.saveSession = func(model.RootModel) error { return nil }
	application.dependencies.stopPortForwards = func() {}
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
	runner := newTerminalProgram(model.NewRootModel(""))
	if runner == nil {
		t.Fatal("newTerminalProgram returned nil")
	}
}
