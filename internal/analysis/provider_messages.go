package analysis

type AnalysisMsg struct {
	Response string
	Err      error
}

type GeneratedCommandMsg struct {
	Command     string
	Explanation string
	Err         error
}

type LogExplanationMsg struct {
	Explanation string
	Err         error
}

type DashboardHealthMsg struct {
	Summary string
	Err     error
}

type DescribeSummaryMsg struct {
	Summary string
	Err     error
}

type StreamChunkMsg struct {
	Chunk string
	Done  bool
	Err   error
}
