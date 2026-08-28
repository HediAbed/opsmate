package main

import (
	"errors"
	"testing"
)

func TestParseArgumentsDefaultsToInteractiveMode(t *testing.T) {
	parsed, err := parseArguments(nil)
	if err != nil {
		t.Fatalf("parseArguments(nil): %v", err)
	}
	interactive, ok := parsed.(interactiveCommand)
	if !ok {
		t.Fatalf("parseArguments(nil) returned %T, want interactiveCommand", parsed)
	}
	if interactive.namespace != "" {
		t.Fatalf("namespace = %q, want empty", interactive.namespace)
	}
}

func TestParseArgumentsAcceptsNamespace(t *testing.T) {
	parsed, err := parseArguments([]string{"kube-system"})
	if err != nil {
		t.Fatalf("parse namespace: %v", err)
	}
	interactive, ok := parsed.(interactiveCommand)
	if !ok {
		t.Fatalf("parse namespace returned %T, want interactiveCommand", parsed)
	}
	if interactive.namespace != "kube-system" {
		t.Fatalf("namespace = %q, want %q", interactive.namespace, "kube-system")
	}
}

func TestParseArgumentsAcceptsHelpAliases(t *testing.T) {
	for _, argument := range []string{"-h", "--help"} {
		t.Run(argument, func(t *testing.T) {
			parsed, err := parseArguments([]string{argument})
			if err != nil {
				t.Fatalf("parse %q: %v", argument, err)
			}
			if _, ok := parsed.(helpCommand); !ok {
				t.Fatalf("parse %q returned %T, want helpCommand", argument, parsed)
			}
		})
	}
}

func TestParseArgumentsAcceptsVersionAliases(t *testing.T) {
	for _, argument := range []string{"-v", "--version"} {
		t.Run(argument, func(t *testing.T) {
			parsed, err := parseArguments([]string{argument})
			if err != nil {
				t.Fatalf("parse %q: %v", argument, err)
			}
			if _, ok := parsed.(versionCommand); !ok {
				t.Fatalf("parse %q returned %T, want versionCommand", argument, parsed)
			}
		})
	}
}

func TestParseArgumentsRejectsInvalidInput(t *testing.T) {
	testCases := []struct {
		name      string
		arguments []string
		expected  string
	}{
		{name: "too many", arguments: []string{"one", "two"}, expected: "expected at most one argument"},
		{name: "empty namespace", arguments: []string{""}, expected: "namespace cannot be empty"},
		{name: "unknown option", arguments: []string{"--unknown"}, expected: `unknown option "--unknown"`},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			parsed, err := parseArguments(testCase.arguments)
			if parsed != nil {
				t.Fatalf("parsed command = %T, want nil", parsed)
			}
			var typedError *argumentError
			if !errors.As(err, &typedError) {
				t.Fatalf("error = %T, want *argumentError", err)
			}
			if err.Error() != testCase.expected {
				t.Fatalf("error = %q, want %q", err, testCase.expected)
			}
		})
	}
}
