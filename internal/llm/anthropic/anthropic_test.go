package anthropic

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/aspasskiy/gogogot/internal/domain"
)

func TestMessagesToAnthropicThinking(t *testing.T) {
	msgs := []domain.Message{
		// earlier run
		domain.NewUserMessage(domain.TextBlock("hi")),
		{Role: domain.RoleAssistant, Content: []domain.ContentBlock{domain.ThinkingBlock("", "sig-old")}},
		{Role: domain.RoleAssistant, Content: []domain.ContentBlock{
			domain.ThinkingBlock("", "sig-old-2"),
			domain.TextBlock("hello"),
		}},
		// current run
		domain.NewUserMessage(domain.TextBlock("list files")),
		{Role: domain.RoleAssistant, Content: []domain.ContentBlock{
			domain.ThinkingBlock("plan", "sig-1"),
			domain.RedactedThinkingBlock("enc"),
			domain.ToolUseBlock("tu1", "bash", json.RawMessage(`{"cmd":"ls"}`)),
		}},
		domain.NewUserMessage(domain.ToolResultBlock("tu1", "a.txt", false)),
	}

	out := messagesToAnthropic(msgs)

	// The thinking-only assistant turn of the earlier run disappears entirely.
	if len(out) != len(msgs)-1 {
		t.Fatalf("got %d messages, want %d", len(out), len(msgs)-1)
	}
	if got := out[1].Content; len(got) != 1 || got[0].OfText == nil {
		t.Fatalf("earlier-run thinking not stripped: %+v", got)
	}

	cur := out[3].Content
	if len(cur) != 3 {
		t.Fatalf("current-run assistant turn: got %d blocks, want 3", len(cur))
	}
	if th := cur[0].OfThinking; th == nil || th.Signature != "sig-1" || th.Thinking != "plan" {
		t.Fatalf("thinking block not replayed unchanged: %+v", cur[0])
	}
	if rt := cur[1].OfRedactedThinking; rt == nil || rt.Data != "enc" {
		t.Fatalf("redacted thinking block not replayed: %+v", cur[1])
	}
	if cur[2].OfToolUse == nil {
		t.Fatalf("tool_use must follow thinking: %+v", cur[2])
	}
}

func TestBindsThinking(t *testing.T) {
	for model, want := range map[string]bool{
		"claude-sonnet-5-5": true,
		"claude-opus-5":     true,
		"claude-fable-5-1":  true,
		"claude-haiku-5-5":  true,
		"claude-opus-4-8":   false,
		"claude-sonnet-4-6": false,
		"claude-haiku-4-5":  false,
	} {
		if got := bindsThinking(model); got != want {
			t.Errorf("bindsThinking(%q) = %v, want %v", model, got, want)
		}
	}
}

func TestCallThinkingRequestShape(t *testing.T) {
	var body map[string]any
	var beta string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		beta = r.Header.Get("anthropic-beta")
		_ = json.NewDecoder(r.Body).Decode(&body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"id":"msg_1","type":"message","role":"assistant","model":"claude-sonnet-5-5",
			"content":[{"type":"thinking","thinking":"","signature":"sig"},{"type":"text","text":"ok"}],
			"stop_reason":"end_turn","usage":{"input_tokens":1,"output_tokens":1}}`)
	}))
	defer srv.Close()

	a := NewAdapter("test-key", srv.URL, false)
	resp, err := a.Call(context.Background(), "claude-sonnet-5-5", "sys",
		[]domain.Message{domain.NewUserMessage(domain.TextBlock("hi"))}, nil, 300)
	if err != nil {
		t.Fatal(err)
	}

	if beta != thinkingBindingBeta {
		t.Errorf("anthropic-beta = %q, want %q", beta, thinkingBindingBeta)
	}
	thinking, _ := body["thinking"].(map[string]any)
	if thinking["type"] != "adaptive" {
		t.Errorf("thinking.type = %v, want adaptive", thinking["type"])
	}
	binding, _ := thinking["block_binding"].(map[string]any)
	if binding["prefix_mismatch_behavior"] != "drop_block" {
		t.Errorf("block_binding = %v", thinking["block_binding"])
	}
	if body["max_tokens"] != float64(thinkingMaxTokens) {
		t.Errorf("max_tokens = %v, want %d", body["max_tokens"], thinkingMaxTokens)
	}
	if len(resp.Content) != 2 || resp.Content[0].Type != domain.BlockThinking || resp.Content[0].Signature != "sig" {
		t.Errorf("response content = %+v", resp.Content)
	}
}

func TestCallNoThinkingForOlderModels(t *testing.T) {
	var body map[string]any
	var beta string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		beta = r.Header.Get("anthropic-beta")
		_ = json.NewDecoder(r.Body).Decode(&body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"id":"msg_1","type":"message","role":"assistant","model":"claude-opus-4-8",
			"content":[{"type":"text","text":"ok"}],"stop_reason":"end_turn","usage":{"input_tokens":1,"output_tokens":1}}`)
	}))
	defer srv.Close()

	a := NewAdapter("test-key", srv.URL, false)
	if _, err := a.Call(context.Background(), "claude-opus-4-8", "sys",
		[]domain.Message{domain.NewUserMessage(domain.TextBlock("hi"))}, nil, 300); err != nil {
		t.Fatal(err)
	}
	if beta != "" || body["thinking"] != nil || body["max_tokens"] != float64(300) {
		t.Errorf("older model request changed: beta=%q thinking=%v max_tokens=%v", beta, body["thinking"], body["max_tokens"])
	}
}
