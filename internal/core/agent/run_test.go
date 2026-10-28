package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/aspasskiy/gogogot/internal/domain"
	"github.com/aspasskiy/gogogot/internal/llm"
	"github.com/aspasskiy/gogogot/internal/transport"
)

// fakeLLM returns canned responses in order, with no network or provider.
type fakeLLM struct {
	responses []*domain.Response
	idx       int
	calls     int
}

func (f *fakeLLM) Call(_ context.Context, _ []domain.Message, _ llm.CallOptions) (*domain.Response, error) {
	f.calls++
	r := f.responses[f.idx]
	f.idx++
	return r, nil
}

func (f *fakeLLM) ModelID() string          { return "fake" }
func (f *fakeLLM) ModelLabel() string       { return "Fake" }
func (f *fakeLLM) ContextWindow() int       { return 100_000 }
func (f *fakeLLM) InputPricePerM() float64  { return 0 }
func (f *fakeLLM) OutputPricePerM() float64 { return 0 }

// captureSink records every event. The agent emits from a single goroutine
// (the one calling Run), so no synchronization is needed in the test.
type captureSink struct{ events []transport.Event }

func (c *captureSink) Emit(ev transport.Event) { c.events = append(c.events, ev) }

// TestRunEmitsEventSequenceWithoutStore proves the decoupling: a real agent
// run, driven by a fake LLM and a local tool, produces a fully typed event
// stream into a plain sink — with no store, no disk, and no channel involved.
func TestRunEmitsEventSequenceWithoutStore(t *testing.T) {
	fake := &fakeLLM{responses: []*domain.Response{
		{ // iteration 1: call the local report_status tool
			Content: []domain.ContentBlock{
				domain.ToolUseBlock("call_1", "report_status", json.RawMessage(`{"text":"working"}`)),
			},
			StopReason:   "tool_use",
			InputTokens:  10,
			OutputTokens: 5,
		},
		{ // iteration 2: final text, no tools -> loop ends
			Content:      []domain.ContentBlock{domain.TextBlock("done")},
			StopReason:   "end_turn",
			InputTokens:  8,
			OutputTokens: 3,
		},
	}}

	// nil registry is fine: the run only uses the built-in local tool, so the
	// registry is never consulted.
	ag := New(fake, func() string { return "system" }, nil)
	sink := &captureSink{}

	input := RunInput{
		UserBlocks: []domain.ContentBlock{domain.TextBlock("hello")},
	}
	if err := ag.Run(context.Background(), input, sink); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	if fake.calls != 2 {
		t.Fatalf("expected 2 LLM calls, got %d", fake.calls)
	}

	want := []string{
		"transport.MessageEvent",   // user turn
		"transport.LLMStartEvent",  // iteration 1
		"transport.MessageEvent",   // assistant turn (tool_use)
		"transport.ToolStartEvent", // report_status begins
		"transport.ProgressEvent",  // emitted by report_status via the sink-in-context
		"transport.ToolEndEvent",   // report_status ends
		"transport.MessageEvent",   // tool-result turn
		"transport.LLMStartEvent",  // iteration 2
		"transport.MessageEvent",   // assistant turn (text)
		"transport.LLMStreamEvent", // final text
		"transport.DoneEvent",      // terminal
	}

	got := make([]string, len(sink.events))
	for i, ev := range sink.events {
		got[i] = fmt.Sprintf("%T", ev)
	}
	if len(got) != len(want) {
		t.Fatalf("event count = %d, want %d\n got: %v\nwant: %v", len(got), len(want), got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("event[%d] = %s, want %s", i, got[i], want[i])
		}
	}

	// The first event is the user turn, persisted verbatim from UserBlocks.
	userMsg, ok := sink.events[0].(transport.MessageEvent)
	if !ok {
		t.Fatalf("event[0] is %T, want MessageEvent", sink.events[0])
	}
	if userMsg.Turn.Role != string(domain.RoleUser) {
		t.Errorf("user turn role = %q, want %q", userMsg.Turn.Role, domain.RoleUser)
	}
	if got := domain.ExtractText(userMsg.Turn.Content); got != "hello" {
		t.Errorf("user turn text = %q, want %q", got, "hello")
	}
	if !userMsg.Persistent() {
		t.Error("MessageEvent.Persistent() = false, want true")
	}

	// The tool-result turn carries the report_status output.
	toolMsg := sink.events[6].(transport.MessageEvent)
	if toolMsg.Turn.Role != string(domain.RoleUser) {
		t.Errorf("tool-result turn role = %q, want %q", toolMsg.Turn.Role, domain.RoleUser)
	}

	// The terminal event is Done and is non-persistent.
	done, ok := sink.events[len(sink.events)-1].(transport.DoneEvent)
	if !ok {
		t.Fatalf("last event is %T, want DoneEvent", sink.events[len(sink.events)-1])
	}
	if done.Persistent() {
		t.Error("DoneEvent.Persistent() = true, want false")
	}
	if done.Text != "done" {
		t.Errorf("done text = %q, want %q", done.Text, "done")
	}
	if done.Usage.LLMCalls != 2 {
		t.Errorf("done usage LLMCalls = %d, want 2", done.Usage.LLMCalls)
	}
}
