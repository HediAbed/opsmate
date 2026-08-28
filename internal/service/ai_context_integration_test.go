//go:build !windows

package service

import (
	"context"
	"testing"
	"time"
)

func TestKubectlAIContextRunnerExecutesReadOnlyCommand(t *testing.T) {
	installFakeKubectl(t, "#!/bin/sh\nprintf 'pod-a\\n'\n")

	output, err := (kubectlAIContextRunner{}).RunText(context.Background(), time.Second, "get", "pods")
	if err != nil {
		t.Fatalf("run command: %v", err)
	}
	if output != "pod-a\n" {
		t.Fatalf("output = %q, want pod-a with trailing newline", output)
	}
}

func TestAIAnalyzeWithClusterSearchUsesConfiguredProvider(t *testing.T) {
	installFakeKubectl(t, "#!/bin/sh\nprintf 'healthy\\n'\n")
	provider := &capturingContextProvider{response: "healthy cluster"}
	installConfiguredProvider(t, provider)

	message := AIAnalyzeWithClusterSearch("system", "show health", "", "default")().(AnalysisMsg)

	if message.Err != nil || message.Response != "healthy cluster" {
		t.Fatalf("analysis = %#v, want successful response", message)
	}
}
