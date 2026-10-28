// Package render builds the whole agent UI — live status, plan, asks, chunked
// replies and slash commands — on top of a small Surface that a channel
// implements. A channel only moves messages; everything it shows is decided
// here, so every channel (built-in or a skill bridge) looks and behaves alike.
package render

import (
	"context"

	"github.com/aspasskiy/gogogot/internal/transport"
)

// Format is how a surface displays message text.
type Format string

const (
	FormatMarkdown     Format = "markdown"      // the surface renders markdown itself
	FormatPlain        Format = "plain"         // markdown is flattened to plain text
	FormatTelegramHTML Format = "telegram_html" // markdown is converted to Telegram's HTML subset
)

const defaultMaxLen = 4000

// Caps describes what a surface can display.
type Caps struct {
	Format  Format
	MaxLen  int  // max characters per message; 0 means 4000
	CanEdit bool // sent messages can be edited and deleted (enables the live status message)
	Buttons bool // Ask shows options as buttons whose Value comes back as the reply
}

func (c Caps) maxLen() int {
	if c.MaxLen <= 0 {
		return defaultMaxLen
	}
	return c.MaxLen
}

// Text is one outgoing message. Plain marks a body that must be shown
// verbatim: status lines, and the fallback sent after the surface rejected the
// formatted version. Status marks the live status message, which is edited in
// place and finally replaced by the answer or deleted.
type Text struct {
	Body   string
	Plain  bool
	Status bool
}

// Surface is the minimal set of operations a channel provides. Chat and
// message IDs are opaque strings owned by the channel.
type Surface interface {
	Caps() Caps
	Send(ctx context.Context, chat string, t Text) (msgID string, err error)
	Edit(ctx context.Context, chat, msgID string, t Text) error
	Delete(ctx context.Context, chat, msgID string) error
	Typing(ctx context.Context, chat string) error
	SendFile(ctx context.Context, chat, path, caption string) error
	// Ask shows a question with options; called only when Caps.Buttons is set.
	Ask(ctx context.Context, chat string, t Text, options []transport.AskOption) error
}
