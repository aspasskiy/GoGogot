package channel

import "context"

const (
	CmdNewEpisode = "new_episode"
	CmdStop       = "stop"
	CmdHistory    = "history"
	CmdMemory     = "memory"
)

type Command struct {
	Name   string
	Args   map[string]string
	Result *CommandResult
}

type CommandResult struct {
	Data  map[string]string
	Error error
}

type Message struct {
	SessionID   string
	Text        string
	Attachments []Attachment
	Command     *Command
	Reply       Replier
}

type Attachment struct {
	Filename string
	MimeType string
	Data     []byte
}

type Handler func(ctx context.Context, msg Message)

// Replier sends responses back to the user within a specific session.
// Each channel creates a Replier per conversation/chat.
type Replier interface {
	SendText(ctx context.Context, text string) error
	SendFile(ctx context.Context, path, caption string) error
	SendTyping(ctx context.Context) error

	SendStatus(ctx context.Context, status AgentStatus) (statusID string, err error)
	UpdateStatus(ctx context.Context, statusID string, status AgentStatus) error
	DeleteStatus(ctx context.Context, statusID string) error
}

// Channel is the interface every communication channel must implement.
type Channel interface {
	Name() string
	OwnerSession() (sessionID string, reply Replier)
	Run(ctx context.Context, handler Handler) error
}

// Phase represents a high-level stage of the agent's work.
type Phase string

const (
	PhaseThinking Phase = "thinking"
	PhasePlanning Phase = "planning"
	PhaseTool     Phase = "tool"
)

// AgentStatus carries structured information about what the agent is doing.
// Each channel renders it in its own style (emoji, spinner, animation, etc.).
type AgentStatus struct {
	Phase  Phase
	Tool   string // raw tool name (empty when not in tool phase)
	Detail string // human-readable label: "Editing file", "go build", etc.
}
