package llm

import (
	"context"
	"time"

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

type Backend interface {
	Call(ctx context.Context, model string, systemPrompt string,
		messages []types.Message, tools []types.ToolDef, maxTokens int,
	) (*types.Response, error)
}

type LLM interface {
	Call(ctx context.Context, messages []Message, opts CallOptions) (*Response, error)
	ModelID() string
	ModelLabel() string
	ContextWindow() int
}

type CallOptions struct {
	System       string
	SystemAppend string
	Memory       string
	NoTools      bool
	ExtraTools   []ToolDef
}

type Client struct {
	backend  Backend
	model    string
	tools    []ToolDef
	provider Provider
}

func NewClient(p Provider, toolDefs []ToolDef) *Client {
	var backend Backend
	switch p.Format {
	case "openai":
		backend = oaipkg.NewBackend(p.BaseURL, p.APIKey, p.SupportsVision)
	default:
		backend = anthpkg.NewBackend(p.APIKey, p.BaseURL)
	}

	return &Client{
		backend:  backend,
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

	logCallStart(c.model, sys, messages, tools, c.provider.Format)

	start := time.Now()
	resp, err := c.backend.Call(ctx, c.model, sys, messages, tools, 4096)
	if err != nil {
		return nil, err
	}
	logCallDone(c.model, resp, time.Since(start))
	return resp, nil
}

