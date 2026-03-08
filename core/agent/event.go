package agent

import "time"

type EventKind string

const (
	EventLLMStart    EventKind = "llm_start"
	EventLLMResponse EventKind = "llm_response"
	EventLLMStream   EventKind = "llm_stream"
	EventToolStart   EventKind = "tool_start"
	EventToolEnd     EventKind = "tool_end"
	EventEvalRun     EventKind = "eval_run"
	EventEvalResult  EventKind = "eval_result"
	EventCompaction  EventKind = "compaction"
	EventLoopWarning EventKind = "loop_warning"
	EventError       EventKind = "error"
	EventDone        EventKind = "done"
)

type Event struct {
	Timestamp time.Time
	Kind      EventKind
	Source    string
	Depth     int
	Data      any
}

type LLMResponseData struct {
	Usage Usage
}

type LLMStreamData struct {
	Text string
}

type ToolStartData struct {
	Name   string
	Detail string
}

type ToolEndData struct {
	Name       string
	Result     string
	DurationMs int64
}

type ErrorData struct {
	Error string
}

type DoneData struct {
	Usage Usage
}

type CompactionData struct {
	BeforeTokens int
	AfterTokens  int
}

type LoopWarningData struct {
	Name   string
	Reason string
}

type EvalRunData struct {
	Iteration int
}

type EvalResultData struct {
	Iteration int
	Passed    bool
	Feedback  string
}
