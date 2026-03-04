package agent

import "time"

type EventKind string

const (
	EventUserMessage EventKind = "user_message"
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
	Source    string // "core-loop", "eval-loop", "compaction", "subagent:abc"
	Depth     int    // nesting level (0 = top agent, 1 = subagent, ...)
	Data      any
}

type EventBus struct {
	subscribers []func(Event)
}

func NewEventBus() *EventBus {
	return &EventBus{
		subscribers: make([]func(Event), 0),
	}
}

func (eb *EventBus) Emit(e Event) {
	if e.Timestamp.IsZero() {
		e.Timestamp = time.Now()
	}
	for _, sub := range eb.subscribers {
		sub(e)
	}
}

func (eb *EventBus) Subscribe(fn func(Event)) {
	eb.subscribers = append(eb.subscribers, fn)
}
