package transport

import (
	"github.com/aspasskiy/gogogot/internal/domain"
)

// Event is a sealed interface implemented by every agent event. The unexported
// marker method (promoted from BaseEvent) keeps the set of event types under
// this package's control, so a type switch over Event is exhaustive in practice.
//
// Consumers switch on the concrete type instead of inspecting an untyped
// Data any payload — the compiler now catches mismatched field access.
// Which events are persisted is decided by the orchestrator's sink.
type Event interface {
	isEvent()
}

// BaseEvent is embedded by every concrete event and supplies the sealing marker.
type BaseEvent struct{}

func (BaseEvent) isEvent() {}

// --- Persistent events (written to the store) ---

// MessageEvent carries a finished conversation turn (user, assistant, or
// tool-result) that must be appended to history.
type MessageEvent struct {
	BaseEvent
	Turn domain.Turn
}

// CompactionEvent carries the rewritten history after the compaction hook
// summarized older messages. Messages is the new full slice the store rewrites
// in place (store.ReplaceMessages); BeforeTokens/AfterTokens are for logging.
type CompactionEvent struct {
	BaseEvent
	Messages     []domain.Turn
	BeforeTokens int
	AfterTokens  int
}

// --- UI events (rendered by the channel, never persisted) ---

// LLMStartEvent signals the start of an LLM iteration (drives the thinking UI).
type LLMStartEvent struct {
	BaseEvent
}

// LLMStreamEvent carries assistant text. Today it holds the full text of one
// iteration; token-level deltas can be emitted more frequently later.
type LLMStreamEvent struct {
	BaseEvent
	Text string
}

// ToolStartEvent signals a tool is about to run.
type ToolStartEvent struct {
	BaseEvent
	Name   string
	Label  string
	Detail string
	Phase  string
}

// ToolEndEvent signals a tool finished.
type ToolEndEvent struct {
	BaseEvent
	Name       string
	Result     string
	DurationMs int64
}

// ProgressEvent carries plan/status updates emitted by tools.
type ProgressEvent struct {
	BaseEvent
	Tasks   []PlanTask
	Status  string
	Percent *int
}

// MidMessageEvent is an out-of-band message sent mid-run (send_message tool).
type MidMessageEvent struct {
	BaseEvent
	Text  string
	Level MessageLevel
}

// LoopWarningEvent signals a detected tool-call loop (identical args repeated).
type LoopWarningEvent struct {
	BaseEvent
	Name   string
	Reason string
}

// AskEvent asks the user a question and waits for the reply on ReplyCh. It must
// not be dropped — the orchestrator forwards it with delivery guaranteed.
type AskEvent struct {
	BaseEvent
	Prompt  string
	Kind    AskKind
	Options []AskOption
	ReplyCh chan<- string
}

// ErrorEvent carries a fatal run error and terminates the run.
type ErrorEvent struct {
	BaseEvent
	Error string
}

// DoneEvent is the terminal event of a successful run. Text is the authoritative
// final assistant message (the last text the model produced) — consumers render
// it without having to reconstruct it from the LLMStream events, which are
// reserved for live streaming. Usage is the run's aggregated token/cost
// accounting, which the orchestrator folds into the chat.
type DoneEvent struct {
	BaseEvent
	Text  string
	Usage domain.Usage
}

// Sink receives every event the agent emits. The orchestrator implements it:
// persistent events are written to the store synchronously (never dropped),
// while UI events are forwarded to a Bus. Emit is called from the agent's
// goroutine, so persistence happens inline — there is no lossy hop in between.
//
// Decoupling the agent behind this interface is the point of the design: the
// agent depends only on transport.Sink, never on the store or the channel.
type Sink interface {
	Emit(ev Event)
}
