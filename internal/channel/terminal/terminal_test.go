package terminal

import (
	"bytes"
	"context"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/aspasskiy/gogogot/internal/channel"
	"github.com/aspasskiy/gogogot/internal/channel/render"
	"github.com/aspasskiy/gogogot/internal/transport"
)

type syncBuf struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuf) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *syncBuf) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}

func TestTerminalRoutesLinesAndExits(t *testing.T) {
	inR, inW := io.Pipe()
	out := &syncBuf{}
	c := &Channel{in: inR, out: out}

	got := make(chan channel.Message, 4)
	h := func(_ context.Context, msg channel.Message) {
		if msg.Command != nil {
			msg.Command.Result.Data = map[string]string{"text": "stopped"}
			return
		}
		got <- msg
	}

	done := make(chan error, 1)
	go func() { done <- c.Run(context.Background(), h) }()

	// A choice question, answered with its number.
	_ = c.OwnerReplier().SendAsk(context.Background(), "Pick", transport.AskChoice,
		[]transport.AskOption{{Value: "red", Label: "Red"}, {Value: "blue", Label: "Blue"}})
	_, _ = io.WriteString(inW, "2\n")
	select {
	case msg := <-got:
		if msg.Text != "blue" {
			t.Errorf("answer = %q, want blue", msg.Text)
		}
		_ = msg.Reply.SendText(context.Background(), "**done**")
	case <-time.After(2 * time.Second):
		t.Fatal("no message")
	}

	_, _ = io.WriteString(inW, "/stop\n/exit\n")
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run = %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("/exit did not stop Run")
	}

	text := out.String()
	for _, want := range []string{"2. Blue", "**done**", "stopped"} {
		if !strings.Contains(text, want) {
			t.Errorf("output lacks %q:\n%s", want, text)
		}
	}
	if strings.Contains(text, "\033[") {
		t.Error("escape codes written to a non-terminal")
	}
}

func TestTruncateCountsWideRunes(t *testing.T) {
	if got := truncate("🔧 Running build", 6); got != "🔧 Run…" {
		t.Errorf("truncate = %q", got)
	}
}

func TestStatusRedrawsInPlace(t *testing.T) {
	out := &syncBuf{}
	c := &Channel{out: out, tty: true}
	ctx := context.Background()

	id, _ := c.Send(ctx, chatID, render.Text{Body: "📋 1/2\n🔧 Running...", Plain: true, Status: true})
	_, _ = c.Send(ctx, chatID, render.Text{Body: "a note"})
	_ = c.Edit(ctx, chatID, id, render.Text{Body: "⚡ Working...", Plain: true, Status: true})
	_ = c.Edit(ctx, chatID, id, render.Text{Body: "the answer"})

	erase2 := strings.Repeat("\033[1A\033[2K", 2) + "\r"
	erase1 := "\033[1A\033[2K\r"
	text := out.String()
	if strings.Count(text, erase2) != 2 || strings.Count(text, erase1) != 3 {
		t.Errorf("erase sequences: %d×2-line, %d×1-line in %q", strings.Count(text, erase2), strings.Count(text, erase1), text)
	}
	if !strings.HasSuffix(text, "the answer\n\n") || c.live != nil {
		t.Errorf("final answer not printed last or status still live: %q", text)
	}
	if i, j := strings.Index(text, "a note"), strings.LastIndex(text, "Running"); i > j {
		t.Error("the status was not redrawn below the note")
	}
}
