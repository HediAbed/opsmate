package service

import "testing"

func TestAIAnalyze_NoProviderReturnsError(t *testing.T) {
	withCleanProvider(t)
	msg := AIAnalyze("sys", "user")().(AnalysisMsg)
	if msg.Err == nil {
		t.Error("no-provider AIAnalyze must return an error")
	}
}

func TestAIGenerateCommand_NoProviderReturnsError(t *testing.T) {
	withCleanProvider(t)
	msg := AIGenerateCommand("scale", "ns")().(GeneratedCommandMsg)
	if msg.Err == nil {
		t.Error("no-provider AIGenerateCommand must return an error")
	}
}

func TestAIExplainLogLine_NoProviderReturnsError(t *testing.T) {
	withCleanProvider(t)
	msg := AIExplainLogLine("line", "ctx", "pod")().(LogExplainMsg)
	if msg.Err == nil {
		t.Error("no-provider AIExplainLogLine must return an error")
	}
}

func TestAIClusterHealth_NoProviderReturnsError(t *testing.T) {
	withCleanProvider(t)
	msg := AIClusterHealth("ctx")().(DashHealthMsg)
	if msg.Err == nil {
		t.Error("no-provider AIClusterHealth must return an error")
	}
}

func TestAIDescribeSummary_NoProviderReturnsError(t *testing.T) {
	withCleanProvider(t)
	msg := AIDescribeSummary("pod", "name", "describe")().(DescribeSummaryMsg)
	if msg.Err == nil {
		t.Error("no-provider AIDescribeSummary must return an error")
	}
}
