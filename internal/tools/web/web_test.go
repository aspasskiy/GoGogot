package web

import (
	"context"
	"strings"
	"testing"
)

func TestWebSearch_NoAPIKeyDisablesSearch(t *testing.T) {
	r := webSearch(context.Background(), map[string]any{"query": "anything"}, "")
	if !r.IsErr {
		t.Errorf("expected IsErr=true when API key is empty")
	}
	if !strings.Contains(r.Output, "BRAVE_API_KEY") {
		t.Errorf("output should mention BRAVE_API_KEY; got %q", r.Output)
	}
}

func TestWebSearch_MissingQueryErrors(t *testing.T) {
	r := webSearch(context.Background(), map[string]any{}, "fake-key")
	if !r.IsErr {
		t.Errorf("expected IsErr=true for missing query")
	}
}

func TestWebSearchTool_ShapeAndDetail(t *testing.T) {
	tool := WebSearchTool("k")
	if tool.Name != "web_search" {
		t.Errorf("Name = %q, want web_search", tool.Name)
	}
	if len(tool.Required) != 1 || tool.Required[0] != "query" {
		t.Errorf("Required = %v, want [query]", tool.Required)
	}
	if got := tool.DetailFunc(map[string]any{"query": "hello world"}); got != "hello world" {
		t.Errorf("DetailFunc = %q, want 'hello world'", got)
	}
	if got := tool.DetailFunc(map[string]any{}); got != "" {
		t.Errorf("DetailFunc with no query = %q, want empty", got)
	}
}
