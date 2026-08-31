package analysis

import "testing"

func TestAnalysisAnalyze_NoProviderReturnsError(t *testing.T) {
	msg := NewService(nil).Analyze("sys", "user")().(AnalysisMsg)
	if msg.Err == nil {
		t.Error("no-provider Analyze must return an error")
	}
}

func TestAnalysisGenerateCommand_NoProviderReturnsError(t *testing.T) {
	msg := NewService(nil).GenerateCommand("scale", "ns")().(GeneratedCommandMsg)
	if msg.Err == nil {
		t.Error("no-provider GenerateCommand must return an error")
	}
}

func TestAnalysisExplainLogLine_NoProviderReturnsError(t *testing.T) {
	msg := NewService(nil).ExplainLogLine("line", "ctx", "pod")().(LogExplanationMsg)
	if msg.Err == nil {
		t.Error("no-provider ExplainLogLine must return an error")
	}
}

func TestAnalysisClusterHealth_NoProviderReturnsError(t *testing.T) {
	msg := NewService(nil).ClusterHealth("ctx")().(DashboardHealthMsg)
	if msg.Err == nil {
		t.Error("no-provider ClusterHealth must return an error")
	}
}

func TestAnalysisDescribeSummary_NoProviderReturnsError(t *testing.T) {
	msg := NewService(nil).DescribeSummary("pod", "name", "describe")().(DescribeSummaryMsg)
	if msg.Err == nil {
		t.Error("no-provider DescribeSummary must return an error")
	}
}
