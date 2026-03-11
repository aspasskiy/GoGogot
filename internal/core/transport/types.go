package transport

import "context"

// Replier sends responses back to the user within a specific session.
// Each channel creates a Replier per conversation/chat.
type Replier interface {
	SendText(ctx context.Context, text string) error
	SendFile(ctx context.Context, path, caption string) error
	SendTyping(ctx context.Context) error
	SendAsk(ctx context.Context, prompt string, kind AskKind, options []AskOption) error

	// ConsumeEvents reads agent events and translates them into channel
	// interactions (typing, status updates, text). Returns the final text output.
	// replyInbox carries user responses for ask_user; pass nil to disable asking.
	ConsumeEvents(ctx context.Context, events <-chan Event, replyInbox <-chan string) string
}

// Phase represents a high-level stage of the agent's work.
type Phase string

const (
	PhaseThinking Phase = "thinking"
	PhasePlanning Phase = "planning"
	PhaseTool     Phase = "tool"
	PhaseWorking  Phase = "working"
	PhaseMessage  Phase = "message"
)

// AgentStatus carries structured information about what the agent is doing.
// Each channel renders it in its own style (emoji, spinner, animation, etc.).
type AgentStatus struct {
	Phase   Phase
	Tool    string     // raw tool name (empty when not in tool phase)
	Detail  string     // human-readable label: "Editing file", "go build", etc.
	Plan    []PlanTask // structured plan; nil when no plan is active
	Percent *int       // optional 0-100 progress for measurable operations
}

// TaskStatus represents the state of a plan task.
type TaskStatus string

const (
	TaskPending    TaskStatus = "pending"
	TaskInProgress TaskStatus = "in_progress"
	TaskCompleted  TaskStatus = "completed"
)

// PlanTask is a single step in the agent's structured plan.
type PlanTask struct {
	Title  string
	Status TaskStatus
}

// MessageLevel controls how a mid-run message is rendered by the channel.
type MessageLevel string

const (
	LevelInfo    MessageLevel = ""
	LevelSuccess MessageLevel = "success"
	LevelWarning MessageLevel = "warning"
)

// AskKind determines the interaction pattern for an ask_user call.
type AskKind string

const (
	AskFreeform AskKind = "freeform"
	AskConfirm  AskKind = "confirm"
	AskChoice   AskKind = "choice"
)

// AskOption is one selectable choice in a choice-type ask.
type AskOption struct {
	Value string // returned to the agent
	Label string // displayed to the user
}

// Attachment holds file data received from a channel message.
type Attachment struct {
	Filename string
	MimeType string
	Data     []byte
}
