package llm

import (
	"context"
	"time"

	"github.com/aspasskiy/gogogot/internal/domain"
	anthpkg "github.com/aspasskiy/gogogot/internal/llm/anthropic"
	oaipkg "github.com/aspasskiy/gogogot/internal/llm/openai"
)

type Adapter interface {
	Call(ctx context.Context, model string, systemPrompt string,
		messages []domain.Message, tools []domain.ToolDef, maxTokens int,
	) (*domain.Response, error)
}

type LLM interface {
	Call(ctx context.Context, messages []domain.Message, opts CallOptions) (*domain.Response, error)
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
	ExtraTools   []domain.ToolDef
	MaxTokens    int
}

type Client struct {
	adapter  Adapter
	model    string
	tools    []domain.ToolDef
	provider Provider
	timeout  time.Duration
}

func NewClient(p Provider, toolDefs []domain.ToolDef) *Client {
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
		timeout:  p.Timeout,
	}
}

// callCtx wraps the parent context with the per-call timeout when configured.
func (c *Client) callCtx(parent context.Context) (context.Context, context.CancelFunc) {
	if c.timeout > 0 {
		return context.WithTimeout(parent, c.timeout)
	}
	return parent, func() {}
}

func (c *Client) SetTools(defs []domain.ToolDef) {
	c.tools = defs
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

func (c *Client) Call(ctx context.Context, messages []domain.Message, opts CallOptions) (*domain.Response, error) {
	sys := opts.System
	if opts.SystemAppend != "" {
		sys += "\n\n" + opts.SystemAppend
	}
	if opts.Memory != "" {
		sys += "\n\n## Current Memory\n" + opts.Memory
	}

	var tools []domain.ToolDef
	if !opts.NoTools {
		tools = c.tools
		if len(opts.ExtraTools) > 0 {
			tools = append(append([]domain.ToolDef{}, c.tools...), opts.ExtraTools...)
		}
	}

	maxTokens := opts.MaxTokens
	if maxTokens <= 0 {
		maxTokens = 4096
	}

	callCtx, cancel := c.callCtx(ctx)
	defer cancel()
	return c.adapter.Call(callCtx, c.model, sys, messages, tools, maxTokens)
}
