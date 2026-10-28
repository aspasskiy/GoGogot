// Package terminal is the interactive console channel: the owner types into
// stdin and the agent answers on stdout, with a live status line at the bottom.
package terminal

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"mime"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/aspasskiy/gogogot/internal/channel"
	"github.com/aspasskiy/gogogot/internal/channel/render"
	"github.com/aspasskiy/gogogot/internal/transport"

	"github.com/mattn/go-isatty"
)

const (
	chatID = "terminal"

	dim   = "\033[2m"
	bold  = "\033[1m"
	reset = "\033[0m"

	// A second Ctrl+C within this window exits; the first one stops the task.
	exitWindow = 2 * time.Second
)

// Interactive reports whether stdin and stdout are both a terminal — the
// condition for the terminal channel to make sense.
func Interactive() bool {
	return isatty.IsTerminal(os.Stdin.Fd()) && isatty.IsTerminal(os.Stdout.Fd())
}

// Channel reads the owner's lines from in and prints the agent's replies to
// out. It handles SIGINT itself: the first Ctrl+C stops the running task, a
// second one in quick succession exits.
type Channel struct {
	in  io.Reader
	out io.Writer
	tty bool // out is a terminal: colors and an in-place status line
	// LogPath is shown in the banner so the owner knows where logs went.
	LogPath string

	mu     sync.Mutex
	nextID int
	live   *liveMsg // status message currently drawn at the bottom
	ask    []transport.AskOption
}

type liveMsg struct {
	id    string
	text  string
	lines int
}

func New() *Channel {
	return &Channel{in: os.Stdin, out: os.Stdout, tty: isatty.IsTerminal(os.Stdout.Fd())}
}

func (c *Channel) Name() string { return "terminal" }

func (c *Channel) OwnerReplier() transport.Replier {
	return render.NewReplier(c, chatID)
}

func (c *Channel) Run(ctx context.Context, handler channel.Handler) error {
	lines := make(chan string)
	readErr := make(chan error, 1)
	go func() {
		r := bufio.NewReader(c.in)
		for {
			line, err := r.ReadString('\n')
			if line != "" || err == nil {
				lines <- strings.TrimRight(line, "\r\n")
			}
			if err != nil {
				readErr <- err
				return
			}
		}
	}()

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, os.Interrupt)
	defer signal.Stop(sigs)

	c.banner()

	var lastInterrupt time.Time
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()

		case <-readErr:
			// Ctrl+D or closed stdin.
			c.println("")
			return nil

		case <-sigs:
			if time.Since(lastInterrupt) < exitWindow {
				c.println("")
				return nil
			}
			lastInterrupt = time.Now()
			c.println("")
			render.RunCommand(ctx, handler, c.OwnerReplier(), "/stop")
			c.printDim("(press Ctrl+C again to exit)")

		case line := <-lines:
			if c.handleLine(ctx, handler, line) {
				return nil
			}
		}
	}
}

// handleLine processes one typed line; it returns true when the owner asked
// to exit.
func (c *Channel) handleLine(ctx context.Context, handler channel.Handler, line string) bool {
	text := strings.TrimSpace(line)
	fields := strings.Fields(text)
	if len(fields) == 0 {
		return false
	}

	switch fields[0] {
	case "/exit", "/quit":
		return true
	case "/help":
		_ = c.OwnerReplier().SendText(ctx, render.HelpText()+
			"/attach <path> [message] — send a file with an optional message\n"+
			"/exit — quit (or Ctrl+D)")
		return false
	case "/attach":
		c.attach(ctx, handler, text)
		return false
	}

	// A bare number answers the pending choice question with its value.
	c.mu.Lock()
	options := c.ask
	c.ask = nil
	c.mu.Unlock()
	if n, err := strconv.Atoi(text); err == nil && n >= 1 && n <= len(options) {
		text = options[n-1].Value
	}

	render.Dispatch(ctx, handler, c, render.Inbound{Chat: chatID, Text: text})
	return false
}

func (c *Channel) attach(ctx context.Context, handler channel.Handler, text string) {
	rest := strings.TrimSpace(strings.TrimPrefix(text, "/attach"))
	path, msg, _ := strings.Cut(rest, " ")
	if path == "" {
		c.printDim("usage: /attach <path> [message]")
		return
	}
	if strings.HasPrefix(path, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			path = filepath.Join(home, path[2:])
		}
	}
	data, err := os.ReadFile(path)
	if err != nil {
		c.printDim("cannot attach: " + err.Error())
		return
	}
	mimeType := mime.TypeByExtension(filepath.Ext(path))
	if mimeType == "" {
		mimeType = "application/octet-stream"
	}
	if i := strings.IndexByte(mimeType, ';'); i > 0 {
		mimeType = mimeType[:i]
	}
	render.Dispatch(ctx, handler, c, render.Inbound{
		Chat:        chatID,
		Text:        strings.TrimSpace(msg),
		Attachments: []transport.Attachment{{Filename: filepath.Base(path), MimeType: mimeType, Data: data}},
	})
}

func (c *Channel) banner() {
	c.mu.Lock()
	defer c.mu.Unlock()
	head := "GoGogot"
	if c.tty {
		head = bold + head + reset
	}
	fmt.Fprintf(c.out, "%s — type a message and press Enter. /help for commands, Ctrl+C stops a task, Ctrl+D exits.\n", head)
	if c.LogPath != "" {
		c.writeDim("logs: " + c.LogPath)
	}
	fmt.Fprintln(c.out)
}

func (c *Channel) println(s string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.withLiveHidden(func() { fmt.Fprintln(c.out, s) })
}

func (c *Channel) printDim(s string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.withLiveHidden(func() { c.writeDim(s) })
}

func (c *Channel) writeDim(s string) {
	if c.tty {
		fmt.Fprintln(c.out, dim+s+reset)
	} else {
		fmt.Fprintln(c.out, s)
	}
}
