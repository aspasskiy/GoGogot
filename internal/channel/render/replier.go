package render

import (
	"context"
	"strings"
	"unicode/utf8"

	"github.com/aspasskiy/gogogot/internal/format"
	"github.com/aspasskiy/gogogot/internal/transport"

	"github.com/rs/zerolog/log"
)

// NewReplier returns the transport.Replier for one chat on a surface.
func NewReplier(s Surface, chat string) transport.Replier {
	return &replier{s: s, chat: chat}
}

type replier struct {
	s    Surface
	chat string
}

// chunk is one message-sized piece of a reply: body in the surface's format
// and its plain-text fallback.
type chunk struct {
	body  string
	plain string
}

func (r *replier) plainOnly() bool { return r.s.Caps().Format == FormatPlain }

func (r *replier) chunks(text string) []chunk {
	caps := r.s.Caps()
	var out []chunk
	switch caps.Format {
	case FormatTelegramHTML:
		for _, c := range format.FormatHTMLChunks(text, caps.maxLen()) {
			out = append(out, chunk{body: c.HTML, plain: c.Text})
		}
	case FormatPlain:
		for _, c := range format.FormatHTMLChunks(text, caps.maxLen()) {
			out = append(out, chunk{body: c.Text, plain: c.Text})
		}
	default:
		for _, p := range splitText(text, caps.maxLen()) {
			out = append(out, chunk{body: p, plain: p})
		}
	}
	return out
}

// sendChunk sends one chunk, retrying as plain text if the surface rejects
// the formatted body. It returns the sent message ID ("" on failure).
func (r *replier) sendChunk(ctx context.Context, c chunk) string {
	plain := r.plainOnly()
	id, err := r.s.Send(ctx, r.chat, Text{Body: c.body, Plain: plain})
	if err != nil && !plain {
		log.Warn().Err(err).Msg("render: formatted send failed, falling back to plain text")
		id, err = r.s.Send(ctx, r.chat, Text{Body: c.plain, Plain: true})
	}
	if err != nil {
		log.Error().Err(err).Msg("render: send failed")
		return ""
	}
	return id
}

func (r *replier) sendLong(ctx context.Context, text string) {
	for _, c := range r.chunks(text) {
		r.sendChunk(ctx, c)
	}
}

func (r *replier) SendText(ctx context.Context, text string) error {
	r.sendLong(ctx, text)
	return nil
}

func (r *replier) SendFile(ctx context.Context, path, caption string) error {
	return r.s.SendFile(ctx, r.chat, path, caption)
}

func (r *replier) SendTyping(ctx context.Context) error {
	return r.s.Typing(ctx, r.chat)
}

func (r *replier) SendAsk(ctx context.Context, prompt string, kind transport.AskKind, options []transport.AskOption) error {
	text := "❓ " + prompt

	if kind == transport.AskConfirm {
		options = []transport.AskOption{{Value: "yes", Label: "✅ Yes"}, {Value: "no", Label: "❌ No"}}
	} else if kind != transport.AskChoice {
		options = nil
	}

	if len(options) == 0 {
		r.sendLong(ctx, text)
		return nil
	}
	if r.s.Caps().Buttons {
		return r.s.Ask(ctx, r.chat, Text{Body: text, Plain: true}, options)
	}

	// No buttons: list the options and let the user type a value back.
	var b strings.Builder
	b.WriteString(text)
	b.WriteString("\n\nReply with one of:")
	for _, opt := range options {
		b.WriteString("\n• " + opt.Value)
		if opt.Label != "" && opt.Label != opt.Value {
			b.WriteString(" — " + opt.Label)
		}
	}
	_, err := r.s.Send(ctx, r.chat, Text{Body: b.String(), Plain: true})
	return err
}

// sendStatus posts a new status message and returns its ID. Surfaces that
// cannot edit get no status message — only the typing indicator.
func (r *replier) sendStatus(ctx context.Context, status transport.AgentStatus) string {
	if !r.s.Caps().CanEdit {
		return ""
	}
	id, err := r.s.Send(ctx, r.chat, Text{Body: formatStatus(status), Plain: true, Status: true})
	if err != nil {
		log.Warn().Err(err).Msg("render: send status failed")
		return ""
	}
	return id
}

func (r *replier) updateStatus(ctx context.Context, msgID string, status transport.AgentStatus) {
	if msgID == "" {
		return
	}
	if err := r.s.Edit(ctx, r.chat, msgID, Text{Body: formatStatus(status), Plain: true, Status: true}); err != nil {
		log.Warn().Err(err).Str("msg_id", msgID).Str("phase", string(status.Phase)).Msg("render: edit status failed")
	}
}

func (r *replier) deleteStatus(ctx context.Context, msgID string) {
	if msgID == "" {
		return
	}
	if err := r.s.Delete(ctx, r.chat, msgID); err != nil {
		log.Warn().Err(err).Str("msg_id", msgID).Msg("render: delete status failed")
	}
}

// editToFinal replaces the status message with the final response text.
// If the text fits in one message, it edits in-place. Otherwise it edits the
// status into the first chunk and sends the rest as new messages.
func (r *replier) editToFinal(ctx context.Context, msgID, text string) {
	if msgID == "" {
		r.sendLong(ctx, text)
		return
	}

	chunks := r.chunks(text)
	if len(chunks) == 0 {
		r.deleteStatus(ctx, msgID)
		return
	}

	plain := r.plainOnly()
	err := r.s.Edit(ctx, r.chat, msgID, Text{Body: chunks[0].body, Plain: plain})
	if err != nil && !plain {
		log.Warn().Err(err).Msg("render: edit to final failed, trying plain text")
		err = r.s.Edit(ctx, r.chat, msgID, Text{Body: chunks[0].plain, Plain: true})
	}
	if err != nil {
		log.Warn().Err(err).Msg("render: edit to final failed, falling back to delete+send")
		r.deleteStatus(ctx, msgID)
		r.sendLong(ctx, text)
		return
	}
	for _, c := range chunks[1:] {
		r.sendChunk(ctx, c)
	}
}

// splitText cuts s into pieces of at most limit runes, preferring to break
// at a newline in the second half of each window.
func splitText(s string, limit int) []string {
	s = strings.TrimRight(s, "\n ")
	if s == "" {
		return nil
	}
	var out []string
	for utf8.RuneCountInString(s) > limit {
		runes := []rune(s)
		cut := limit
		if i := strings.LastIndex(string(runes[:limit]), "\n"); i > 0 {
			if n := utf8.RuneCountInString(string(runes[:limit])[:i]); n > limit/2 {
				cut = n
			}
		}
		out = append(out, strings.TrimRight(string(runes[:cut]), "\n "))
		s = strings.TrimLeft(string(runes[cut:]), "\n")
	}
	return append(out, s)
}
