package render

import (
	"context"
	"time"

	"github.com/aspasskiy/gogogot/internal/transport"
)

const (
	minEditInterval = 800 * time.Millisecond
	thinkingDelay   = 10 * time.Second
)

func buildToolStatus(e transport.ToolStartEvent, plan []transport.PlanTask) transport.AgentStatus {
	label := e.Label
	if label == "" {
		label = e.Name
	}
	if e.Detail != "" {
		label = label + ": " + e.Detail
	}

	phase := transport.Phase(e.Phase)
	if phase == "" {
		phase = transport.PhaseTool
	}

	return transport.AgentStatus{Phase: phase, Tool: e.Name, Detail: label, Plan: plan}
}

func formatMessageWithLevel(text string, level transport.MessageLevel) string {
	switch level {
	case transport.LevelSuccess:
		return "✅ " + text
	case transport.LevelWarning:
		return "⚠️ " + text
	default:
		return "💡 " + text
	}
}

func (r *replier) ConsumeEvents(ctx context.Context, events <-chan transport.Event, replyInbox <-chan string) string {
	_ = r.SendTyping(ctx)
	statusID := r.sendStatus(ctx, transport.AgentStatus{Phase: transport.PhaseThinking})

	var (
		currentPlan []transport.PlanTask

		pending      *transport.AgentStatus
		lastText     string
		lastEditTime time.Time

		flushTimer *time.Timer
		flushCh    <-chan time.Time

		hadTool    bool
		thinkTimer *time.Timer
		thinkCh    <-chan time.Time
	)

	lastText = formatStatus(transport.AgentStatus{Phase: transport.PhaseThinking})
	lastEditTime = time.Now()

	defer func() {
		if flushTimer != nil {
			flushTimer.Stop()
		}
		if thinkTimer != nil {
			thinkTimer.Stop()
		}
	}()

	cancelThinking := func() {
		if thinkTimer != nil {
			thinkTimer.Stop()
			thinkTimer = nil
			thinkCh = nil
		}
	}

	flush := func() {
		if flushTimer != nil {
			flushTimer.Stop()
			flushTimer = nil
			flushCh = nil
		}
		if pending == nil {
			return
		}
		if statusID == "" {
			// No live status message: keep the typing indicator alive instead.
			_ = r.SendTyping(ctx)
			pending = nil
			return
		}
		text := formatStatus(*pending)
		if text != lastText {
			_ = r.SendTyping(ctx)
			r.updateStatus(ctx, statusID, *pending)
			lastText = text
			lastEditTime = time.Now()
		}
		pending = nil
	}

	schedule := func(s transport.AgentStatus) {
		pending = &s
		elapsed := time.Since(lastEditTime)
		if elapsed >= minEditInterval {
			flush()
			return
		}
		if flushTimer == nil {
			flushTimer = time.NewTimer(minEditInterval - elapsed)
			flushCh = flushTimer.C
		}
	}

	restoreStatus := func() string {
		return r.sendStatus(ctx, transport.AgentStatus{
			Phase: transport.PhaseWorking,
			Plan:  currentPlan,
		})
	}

	for {
		select {
		case ev, ok := <-events:
			if !ok {
				// Channel closed without a terminal Done/Error — the run was
				// cancelled. Clean up the dangling status message.
				r.deleteStatus(context.Background(), statusID)
				return ""
			}

			switch e := ev.(type) {
			case transport.LLMStartEvent:
				if hadTool {
					cancelThinking()
					thinkTimer = time.NewTimer(thinkingDelay)
					thinkCh = thinkTimer.C
				}

			case transport.LLMStreamEvent:
				// Reserved for live token streaming. The authoritative final
				// text arrives in DoneEvent.Text, so nothing to render here yet.

			case transport.ToolStartEvent:
				hadTool = true
				cancelThinking()
				schedule(buildToolStatus(e, currentPlan))

			case transport.ProgressEvent:
				cancelThinking()
				if e.Tasks != nil {
					currentPlan = e.Tasks
				}
				schedule(transport.AgentStatus{
					Phase:   transport.PhaseWorking,
					Plan:    currentPlan,
					Detail:  e.Status,
					Percent: e.Percent,
				})

			case transport.MidMessageEvent:
				cancelThinking()
				flush()
				_ = r.SendText(ctx, formatMessageWithLevel(e.Text, e.Level))

			case transport.AskEvent:
				flush()
				r.deleteStatus(ctx, statusID)
				_ = r.SendAsk(ctx, e.Prompt, e.Kind, e.Options)

				if replyInbox != nil {
					select {
					case resp := <-replyInbox:
						if e.ReplyCh != nil {
							e.ReplyCh <- resp
						}
					case <-ctx.Done():
						if e.ReplyCh != nil {
							close(e.ReplyCh)
						}
						return ""
					}
				} else {
					if e.ReplyCh != nil {
						e.ReplyCh <- "(no interactive input available)"
					}
				}
				statusID = restoreStatus()
				lastText = ""
				lastEditTime = time.Time{}

			case transport.ErrorEvent:
				if ctx.Err() != nil {
					return ""
				}
				r.deleteStatus(ctx, statusID)
				_ = r.SendText(ctx, "Error: "+e.Error)
				return ""

			case transport.DoneEvent:
				if ctx.Err() != nil {
					r.deleteStatus(context.Background(), statusID)
					return ""
				}
				if e.Text != "" {
					r.editToFinal(context.Background(), statusID, e.Text)
				} else {
					r.deleteStatus(context.Background(), statusID)
				}
				return ""
			}

		case <-flushCh:
			flush()

		case <-thinkCh:
			thinkTimer = nil
			thinkCh = nil
			schedule(transport.AgentStatus{Phase: transport.PhasePlanning, Detail: "Planning next moves", Plan: currentPlan})
		}
	}
}
