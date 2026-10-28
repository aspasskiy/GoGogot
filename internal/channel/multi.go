package channel

import (
	"context"
	"errors"
	"strings"
	"sync"

	"github.com/aspasskiy/gogogot/internal/transport"
)

// Multi runs several channels side by side as one. Messages from any of them
// reach the same handler; owner notifications (scheduled tasks, startup
// errors) go to all of them. When one channel's Run returns — the owner quit
// the terminal, say — the others are stopped too.
type Multi struct {
	chans []Channel
}

func NewMulti(chans ...Channel) *Multi {
	return &Multi{chans: chans}
}

func (m *Multi) Name() string {
	var names []string
	for _, ch := range m.chans {
		if n := ch.Name(); n != "" {
			names = append(names, n)
		}
	}
	return strings.Join(names, " and ")
}

func (m *Multi) OwnerReplier() transport.Replier {
	rs := make([]transport.Replier, len(m.chans))
	for i, ch := range m.chans {
		rs[i] = ch.OwnerReplier()
	}
	return FanOut(rs...)
}

func (m *Multi) Run(ctx context.Context, handler Handler) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	errs := make(chan error, len(m.chans))
	var wg sync.WaitGroup
	for _, ch := range m.chans {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs <- ch.Run(ctx, handler)
		}()
	}

	first := <-errs
	cancel()
	wg.Wait()
	if errors.Is(first, context.Canceled) {
		return nil
	}
	return first
}

// FanOut returns a Replier that sends to every given replier. ConsumeEvents
// can render a run in one place only, so it goes to the first replier.
func FanOut(rs ...transport.Replier) transport.Replier {
	if len(rs) == 1 {
		return rs[0]
	}
	return fanOut(rs)
}

type fanOut []transport.Replier

func (f fanOut) SendText(ctx context.Context, text string) error {
	var errs []error
	for _, r := range f {
		errs = append(errs, r.SendText(ctx, text))
	}
	return errors.Join(errs...)
}

func (f fanOut) SendFile(ctx context.Context, path, caption string) error {
	var errs []error
	for _, r := range f {
		errs = append(errs, r.SendFile(ctx, path, caption))
	}
	return errors.Join(errs...)
}

func (f fanOut) SendTyping(ctx context.Context) error {
	for _, r := range f {
		_ = r.SendTyping(ctx)
	}
	return nil
}

func (f fanOut) SendAsk(ctx context.Context, prompt string, kind transport.AskKind, options []transport.AskOption) error {
	var errs []error
	for _, r := range f {
		errs = append(errs, r.SendAsk(ctx, prompt, kind, options))
	}
	return errors.Join(errs...)
}

func (f fanOut) ConsumeEvents(ctx context.Context, events <-chan transport.Event, replyInbox <-chan string) string {
	if len(f) == 0 {
		for range events {
		}
		return ""
	}
	return f[0].ConsumeEvents(ctx, events, replyInbox)
}
