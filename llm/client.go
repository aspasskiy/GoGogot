package llm

import (
	"context"

	anthpkg "gogogot/llm/anthropic"
	oaipkg "gogogot/llm/openai"
	"gogogot/llm/types"
)

type (
	Message      = types.Message
	Response     = types.Response
	ToolDef      = types.ToolDef
	ContentBlock = types.ContentBlock
)

type Adapter interface {
	Call(ctx context.Context, model string, systemPrompt string,
		messages []types.Message, tools []types.ToolDef, maxTokens int,
	) (*types.Response, error)
}

type LLM interface {
	Call(ctx context.Context, messages []Message, opts CallOptions) (*Response, error)
	ModelID() string
	ModelLabel() string
	ContextWindow() int
	InputPricePerM() float64
	OutputPricePerM() float64
}

type CallOptions struct {
	System       string
	SystemAppend string
	Memory       string
	NoTools      bool
	ExtraTools   []ToolDef
}

type Client struct {
	adapter  Adapter
	model    string
	tools    []ToolDef
	provider Provider
}

func NewClient(p Provider, toolDefs []ToolDef) *Client {
	var adapter Adapter
	switch p.Format {
	case "openai":
		adapter = oaipkg.NewAdapter(p.BaseURL, p.APIKey, p.SupportsVision)
	default:
		adapter = anthpkg.NewAdapter(p.APIKey, p.BaseURL)
	}

	return &Client{
		adapter:  adapter,
		model:    p.Model,
		tools:    toolDefs,
		provider: p,
	}
}

func (c *Client) ModelID() string {
	return c.model
}

func (c *Client) ModelLabel() string {
	return c.provider.Label
}

func (c *Client) ContextWindow() int {
	return c.provider.ContextWindow
}

func (c *Client) InputPricePerM() float64 {
	return c.provider.InputPricePerM
}

func (c *Client) OutputPricePerM() float64 {
	return c.provider.OutputPricePerM
}

func (c *Client) Call(ctx context.Context, messages []Message, opts CallOptions) (*Response, error) {
	sys := opts.System
	if opts.SystemAppend != "" {
		sys += "\n\n" + opts.SystemAppend
	}
	if opts.Memory != "" {
		sys += "\n\n## Current Memory\n" + opts.Memory
	}

	var tools []ToolDef
	if !opts.NoTools {
		tools = c.tools
		if len(opts.ExtraTools) > 0 {
			tools = append(append([]ToolDef{}, c.tools...), opts.ExtraTools...)
		}
	}

	return c.adapter.Call(ctx, c.model, sys, messages, tools, 4096)
}


