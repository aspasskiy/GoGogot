package terminal

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/aspasskiy/gogogot/internal/channel/render"
	"github.com/aspasskiy/gogogot/internal/transport"
)

// The terminal is a render.Surface. Only the status message is editable: it
// stays at the bottom of the output, is redrawn in place, and other messages
// are printed above it. Without a TTY there is no status message at all.

func (c *Channel) Caps() render.Caps {
	return render.Caps{
		Format:  render.FormatMarkdown,
		MaxLen:  1 << 20, // no reason to split in a terminal
		CanEdit: c.tty,
		Buttons: true,
	}
}

func (c *Channel) newID() string {
	c.nextID++
	return strconv.Itoa(c.nextID)
}

func (c *Channel) Send(_ context.Context, _ string, t render.Text) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	id := c.newID()
	if t.Status && c.tty {
		c.eraseLive()
		c.live = &liveMsg{id: id, text: t.Body}
		c.drawLive()
		return id, nil
	}
	c.withLiveHidden(func() { c.writeMessage(t.Body) })
	return id, nil
}

func (c *Channel) Edit(_ context.Context, _, msgID string, t render.Text) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.live == nil || c.live.id != msgID {
		// Only the live status is editable; anything else is printed anew.
		c.withLiveHidden(func() { c.writeMessage(t.Body) })
		return nil
	}
	c.eraseLive()
	if t.Status {
		c.live.text = t.Body
		c.drawLive()
		return nil
	}
	// The status turns into the final answer.
	c.live = nil
	c.writeMessage(t.Body)
	return nil
}

func (c *Channel) Delete(_ context.Context, _, msgID string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.live != nil && c.live.id == msgID {
		c.eraseLive()
		c.live = nil
	}
	return nil
}

func (c *Channel) Typing(context.Context, string) error { return nil }

func (c *Channel) SendFile(_ context.Context, _, path, caption string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	line := "📎 " + path
	if caption != "" {
		line += " — " + caption
	}
	c.withLiveHidden(func() { c.writeMessage(line) })
	return nil
}

func (c *Channel) Ask(_ context.Context, _ string, t render.Text, options []transport.AskOption) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	var b strings.Builder
	b.WriteString(t.Body)
	for i, opt := range options {
		label := opt.Label
		if label == "" {
			label = opt.Value
		}
		fmt.Fprintf(&b, "\n  %d. %s", i+1, label)
	}
	b.WriteString("\nType a number or your answer.")
	c.ask = options
	c.withLiveHidden(func() { c.writeMessage(b.String()) })
	return nil
}

// writeMessage prints one agent message followed by a blank line.
func (c *Channel) writeMessage(body string) {
	fmt.Fprintln(c.out, strings.TrimRight(body, "\n"))
	fmt.Fprintln(c.out)
}

// withLiveHidden runs print with the live status erased, then redraws the
// status below whatever was printed. Callers hold c.mu.
func (c *Channel) withLiveHidden(print func()) {
	c.eraseLive()
	print()
	c.drawLive()
}

func (c *Channel) drawLive() {
	if c.live == nil || !c.tty {
		return
	}
	width := termWidth()
	lines := strings.Split(c.live.text, "\n")
	for i, l := range lines {
		lines[i] = dim + truncate(l, width-1) + reset
	}
	fmt.Fprintln(c.out, strings.Join(lines, "\n"))
	c.live.lines = len(lines)
}

func (c *Channel) eraseLive() {
	if c.live == nil || !c.tty || c.live.lines == 0 {
		return
	}
	// Each status line is truncated to the width, so it is exactly one row.
	fmt.Fprint(c.out, strings.Repeat("\033[1A\033[2K", c.live.lines), "\r")
	c.live.lines = 0
}

// truncate cuts s to at most width display columns, counting emoji and other
// wide characters as two.
func truncate(s string, width int) string {
	cols := 0
	for i, r := range s {
		w := 1
		if isWide(r) {
			w = 2
		}
		if cols+w > width {
			return s[:i] + "…"
		}
		cols += w
	}
	return s
}

func isWide(r rune) bool {
	return r >= 0x1100 && (r <= 0x115f || // Hangul Jamo
		(r >= 0x2e80 && r <= 0xa4cf) || // CJK
		(r >= 0xac00 && r <= 0xd7a3) || // Hangul syllables
		(r >= 0xf900 && r <= 0xfaff) ||
		(r >= 0xfe30 && r <= 0xfe4f) ||
		(r >= 0xff00 && r <= 0xff60) ||
		(r >= 0x1f300 && r <= 0x1faff) || // emoji
		(r >= 0x2600 && r <= 0x27bf) || // misc symbols, dingbats
		(r >= 0x20000 && r <= 0x3fffd))
}
