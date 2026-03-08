package agent

import (
	"gogogot/core/agent/event"
	"gogogot/core/agent/hook"
	"gogogot/core/agent/session"
)

// Session types re-exported for backward compatibility.
type (
	Session          = session.Session
	Message          = session.Message
	Usage            = session.Usage
	CompactionConfig = session.CompactionConfig
	Summarizer       = session.Summarizer
)

var (
	NewSession              = session.NewSession
	DefaultCompactionConfig = session.DefaultCompactionConfig
	EstimateTokens          = session.EstimateTokens
)

// Event types re-exported for backward compatibility.
type (
	Event           = event.Event
	EventKind       = event.Kind
	LLMResponseData = event.LLMResponseData
	LLMStreamData   = event.LLMStreamData
	ToolStartData   = event.ToolStartData
	ToolEndData     = event.ToolEndData
	ErrorData       = event.ErrorData
	DoneData        = event.DoneData
	CompactionData  = event.CompactionData
	LoopWarningData = event.LoopWarningData
	EvalRunData     = event.EvalRunData
	EvalResultData  = event.EvalResultData
)

const (
	EventLLMStart    = event.LLMStart
	EventLLMResponse = event.LLMResponse
	EventLLMStream   = event.LLMStream
	EventToolStart   = event.ToolStart
	EventToolEnd     = event.ToolEnd
	EventEvalRun     = event.EvalRun
	EventEvalResult  = event.EvalResult
	EventCompaction  = event.Compaction
	EventLoopWarning = event.LoopWarning
	EventError       = event.Error
	EventDone        = event.Done
)

// Hook types re-exported for backward compatibility.
type (
	ToolCallContext    = hook.ToolCallContext
	ToolCallResult     = hook.ToolCallResult
	LLMCallContext     = hook.LLMCallContext
	LLMCallResult      = hook.LLMCallResult
	BeforeToolCallFunc = hook.BeforeToolCallFunc
	AfterToolCallFunc  = hook.AfterToolCallFunc
	BeforeLLMCallFunc  = hook.BeforeLLMCallFunc
	AfterLLMCallFunc   = hook.AfterLLMCallFunc
	LoopDetector       = hook.LoopDetector
)

var (
	LoggingBeforeHook    = hook.LoggingBeforeHook
	LoggingAfterHook     = hook.LoggingAfterHook
	LLMLoggingBeforeHook = hook.LLMLoggingBeforeHook
	LLMLoggingAfterHook  = hook.LLMLoggingAfterHook
	NewLoopDetector      = hook.NewLoopDetector
)
