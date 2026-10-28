package render

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/aspasskiy/gogogot/internal/channel"
	"github.com/aspasskiy/gogogot/internal/transport"
)

type op struct {
	kind, msgID, body string
	plain             bool
}

type fakeSurface struct {
	caps       Caps
	rejectHTML bool

	mu   sync.Mutex
	ops  []op
	next int
}

func (f *fakeSurface) record(o op) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.ops = append(f.ops, o)
}

func (f *fakeSurface) Caps() Caps { return f.caps }

func (f *fakeSurface) Send(_ context.Context, _ string, t Text) (string, error) {
	if f.rejectHTML && !t.Plain {
		return "", errors.New("bad html")
	}
	f.mu.Lock()
	f.next++
	id := strconv.Itoa(f.next)
	f.mu.Unlock()
	f.record(op{kind: "send", msgID: id, body: t.Body, plain: t.Plain})
	return id, nil
}

func (f *fakeSurface) Edit(_ context.Context, _, msgID string, t Text) error {
	if f.rejectHTML && !t.Plain {
		return errors.New("bad html")
	}
	f.record(op{kind: "edit", msgID: msgID, body: t.Body, plain: t.Plain})
	return nil
}

func (f *fakeSurface) Delete(_ context.Context, _, msgID string) error {
	f.record(op{kind: "delete", msgID: msgID})
	return nil
}

func (f *fakeSurface) Typing(context.Context, string) error { return nil }

func (f *fakeSurface) SendFile(_ context.Context, _, path, _ string) error {
	f.record(op{kind: "file", body: path})
	return nil
}

func (f *fakeSurface) Ask(_ context.Context, _ string, t Text, options []transport.AskOption) error {
	var vals []string
	for _, o := range options {
		vals = append(vals, o.Value)
	}
	f.record(op{kind: "ask", body: t.Body + " [" + strings.Join(vals, ",") + "]"})
	return nil
}

func runEvents(r transport.Replier, evs ...transport.Event) {
	ch := make(chan transport.Event, len(evs))
	for _, ev := range evs {
		ch <- ev
	}
	close(ch)
	r.ConsumeEvents(context.Background(), ch, nil)
}

func TestConsumeEventsEditsStatusIntoFinal(t *testing.T) {
	s := &fakeSurface{caps: Caps{Format: FormatTelegramHTML, CanEdit: true}}
	runEvents(NewReplier(s, "1"),
		transport.ToolStartEvent{Name: "bash", Label: "Running", Detail: "ls"},
		transport.DoneEvent{Text: "**done**"},
	)

	if len(s.ops) < 2 {
		t.Fatalf("ops = %+v", s.ops)
	}
	first, last := s.ops[0], s.ops[len(s.ops)-1]
	if first.kind != "send" || !first.plain || !strings.Contains(first.body, "Thinking") {
		t.Errorf("first op = %+v, want plain thinking status", first)
	}
	if last.kind != "edit" || last.msgID != first.msgID || last.body != "<b>done</b>" || last.plain {
		t.Errorf("last op = %+v, want HTML edit of the status message", last)
	}
}

func TestConsumeEventsWithoutEditSendsFinalOnly(t *testing.T) {
	s := &fakeSurface{caps: Caps{Format: FormatMarkdown}}
	runEvents(NewReplier(s, "1"),
		transport.ToolStartEvent{Name: "bash"},
		transport.DoneEvent{Text: "**done**"},
	)

	if len(s.ops) != 1 || s.ops[0].kind != "send" || s.ops[0].body != "**done**" {
		t.Fatalf("ops = %+v, want a single markdown send", s.ops)
	}
}

func TestSendFallsBackToPlain(t *testing.T) {
	s := &fakeSurface{caps: Caps{Format: FormatTelegramHTML}, rejectHTML: true}
	_ = NewReplier(s, "1").SendText(context.Background(), "**hi**")

	if len(s.ops) != 1 || !s.ops[0].plain || s.ops[0].body != "hi" {
		t.Fatalf("ops = %+v, want plain fallback", s.ops)
	}
}

func TestAskWithoutButtonsListsOptions(t *testing.T) {
	s := &fakeSurface{caps: Caps{Format: FormatPlain}}
	_ = NewReplier(s, "1").SendAsk(context.Background(), "Deploy?", transport.AskConfirm, nil)

	if len(s.ops) != 1 || !strings.Contains(s.ops[0].body, "• yes") || !strings.Contains(s.ops[0].body, "• no") {
		t.Fatalf("ops = %+v, want options listed as text", s.ops)
	}

	b := &fakeSurface{caps: Caps{Format: FormatPlain, Buttons: true}}
	_ = NewReplier(b, "1").SendAsk(context.Background(), "Deploy?", transport.AskConfirm, nil)
	if len(b.ops) != 1 || b.ops[0].kind != "ask" || !strings.HasSuffix(b.ops[0].body, "[yes,no]") {
		t.Fatalf("ops = %+v, want buttons", b.ops)
	}
}

func TestSplitText(t *testing.T) {
	long := strings.Repeat("a", 30) + "\n" + strings.Repeat("b", 30)
	got := splitText(long, 40)
	if len(got) != 2 || got[0] != strings.Repeat("a", 30) || got[1] != strings.Repeat("b", 30) {
		t.Fatalf("splitText = %q", got)
	}
	if got := splitText(strings.Repeat("я", 25), 10); len(got) != 3 || got[2] != "яяяяя" {
		t.Fatalf("rune split = %q", got)
	}
}

func TestDispatchRoutesCommandsAndMessages(t *testing.T) {
	s := &fakeSurface{caps: Caps{Format: FormatMarkdown}}
	var got []channel.Message
	h := func(_ context.Context, msg channel.Message) {
		if msg.Command != nil {
			msg.Command.Result.Data = map[string]string{"text": "cmd:" + msg.Command.Name}
		}
		got = append(got, msg)
	}

	Dispatch(context.Background(), h, s, Inbound{Chat: "1", Text: "/new@my_bot"})
	Dispatch(context.Background(), h, s, Inbound{Chat: "1", Text: "/nope"})
	Dispatch(context.Background(), h, s, Inbound{Chat: "1", Attachments: []transport.Attachment{
		{Filename: "a.txt", MimeType: "text/plain", Data: []byte("hello")},
	}})

	if len(got) != 2 || got[0].Command == nil || got[0].Command.Name != channel.CmdNewChat {
		t.Fatalf("handler got %+v", got)
	}
	if !strings.Contains(got[1].Text, "[File: a.txt]") || !strings.Contains(got[1].Text, "hello") {
		t.Errorf("message text = %q, want the inlined file", got[1].Text)
	}
	if len(s.ops) != 2 || s.ops[0].body != "cmd:new_chat" || !strings.HasPrefix(s.ops[1].body, "Unknown command") {
		t.Errorf("ops = %+v", s.ops)
	}
}
