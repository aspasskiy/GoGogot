package event

import (
	"time"

	"gogogot/core/agent/session"
)

type Kind string

const (
	LLMStart    Kind = "llm_start"
	LLMResponse Kind = "llm_response"
	LLMStream   Kind = "llm_stream"
	ToolStart   Kind = "tool_start"
	ToolEnd     Kind = "tool_end"
	EvalRun     Kind = "eval_run"
	EvalResult  Kind = "eval_result"
	Compaction  Kind = "compaction"
	LoopWarning Kind = "loop_warning"
	Error       Kind = "error"
	Done        Kind = "done"
)

type Event struct {
	Timestamp time.Time
	Kind      Kind
	Source    string
	Depth     int
	Data      any
}

type LLMResponseData struct {
	Usage session.Usage
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
	Usage session.Usage
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
