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
	ChannelID   string
	Text        string
	Attachments []Attachment
	Command     *Command
}

type Attachment struct {
	Filename string
	MimeType string
	Data     []byte
}

type Handler func(ctx context.Context, msg Message)

// Channel is the interface every communication channel must implement.
type Channel interface {
	Name() string
	OwnerChannelID() string
	Run(ctx context.Context, handler Handler) error

	SendText(ctx context.Context, channelID string, text string) error
	SendFile(ctx context.Context, channelID, path, caption string) error
	SendTyping(ctx context.Context, channelID string) error

	SendStatus(ctx context.Context, channelID string, status AgentStatus) (statusID string, err error)
	UpdateStatus(ctx context.Context, channelID, statusID string, status AgentStatus) error
	DeleteStatus(ctx context.Context, channelID, statusID string) error
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
