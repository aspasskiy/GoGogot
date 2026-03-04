package transport

import "context"

const (
	CmdNewChat    = "new_chat"
	CmdSwitchChat = "switch_chat"
	CmdStop       = "stop"
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

// Transport is the core interface every communication channel must implement.
type Transport interface {
	Name() string
	Run(ctx context.Context, handler Handler) error
	SendText(ctx context.Context, channelID string, text string) error
}

// FileSender is implemented by transports that can deliver files.
type FileSender interface {
	SendFile(ctx context.Context, channelID, path, caption string) error
}

// TypingNotifier is implemented by transports that support typing indicators.
type TypingNotifier interface {
	SendTyping(ctx context.Context, channelID string) error
}

// StatusUpdater is implemented by transports that can post/edit/delete status messages.
type StatusUpdater interface {
	SendStatus(ctx context.Context, channelID, text string) (statusID string, err error)
	UpdateStatus(ctx context.Context, channelID, statusID, text string) error
	DeleteStatus(ctx context.Context, channelID, statusID string) error
}
