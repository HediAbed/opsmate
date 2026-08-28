package service

import (
	"errors"
	"testing"
)

func TestKubectlHelpers_RejectEmptyNamespace(t *testing.T) {
	cases := []struct {
		name string
		run  func() error
	}{
		{"DescribeResource", func() error { return DescribeResource("", "pod", "x")().(DescribeMsg).Err }},
		{"GetYAML", func() error { return GetYAML("", "pod", "x")().(YAMLMsg).Err }},
		{"FetchContainerLogs", func() error { return FetchContainerLogs("", "x", "c", 10)().(LogsMsg).Err }},
		{"FetchContainers", func() error { return FetchContainers("", "x")().(ContainersMsg).Err }},
		{"ScaleResource", func() error { return ScaleResource("", "deployment", "x", 1)().(CommandResultMsg).Err }},
		{"DeleteResource", func() error { return DeleteResource("", "pod", "x")().(CommandResultMsg).Err }},
		{"RestartRollout", func() error { return RestartRollout("", "deployment", "x")().(CommandResultMsg).Err }},
		{"StartPortForward", func() error { return StartPortForward("", "pod", 8080, 80)().(PortForwardStartedMsg).Err }},
		{"DeleteResources", func() error { return DeleteResources("", "pod", []string{"a"})().(CommandResultMsg).Err }},
		{"RestartRollouts", func() error {
			return RestartRollouts("", "deployment", []string{"a"})().(CommandResultMsg).Err
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.run()
			if err == nil {
				t.Fatalf("%s with empty namespace should fail", tc.name)
			}
			if !errors.Is(err, ErrNamespaceRequired) {
				t.Errorf("%s err = %v; want ErrNamespaceRequired", tc.name, err)
			}
		})
	}
}

func TestDeleteResources_EmptyNamesReturnsError(t *testing.T) {
	cmd := DeleteResources("default", "pod", nil)
	msg := cmd()
	result, ok := msg.(CommandResultMsg)
	if !ok {
		t.Fatalf("expected CommandResultMsg, got %T", msg)
	}
	if result.Err == nil {
		t.Fatal("expected error for empty name slice")
	}
}

func TestRestartRollouts_EmptyNamesReturnsError(t *testing.T) {
	cmd := RestartRollouts("default", "deployment", nil)
	msg := cmd()
	result, ok := msg.(CommandResultMsg)
	if !ok {
		t.Fatalf("expected CommandResultMsg, got %T", msg)
	}
	if result.Err == nil {
		t.Fatal("expected error for empty name slice")
	}
}
