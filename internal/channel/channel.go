package channel

import (
	"context"
	"gogogot/internal/core/transport"
)

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
	Data    map[string]string
	Payload any // typed data for channel-specific formatting
	Error   error
}

type Message struct {
	Text        string
	Attachments []transport.Attachment
	Command     *Command
	Reply       transport.Replier
}

type Handler func(ctx context.Context, msg Message)

// Channel is the interface every communication channel must implement.
type Channel interface {
	Name() string
	OwnerReplier() transport.Replier
	Run(ctx context.Context, handler Handler) error
}
