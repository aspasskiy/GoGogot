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
	Pricing() domain.Pricing
}

type CallOptions struct {
	System     string
	NoTools    bool
	ExtraTools []domain.ToolDef
	MaxTokens  int // overrides the provider default when > 0
}

const defaultMaxTokens = 4096

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
		adapter = oaipkg.NewAdapter(p.BaseURL, p.APIKey, p.SupportsVision, p.CacheControl)
	default:
		adapter = anthpkg.NewAdapter(p.APIKey, p.BaseURL, p.CacheControl)
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

func (c *Client) Pricing() domain.Pricing {
	return domain.Pricing{
		InputPerM:      c.provider.InputPricePerM,
		OutputPerM:     c.provider.OutputPricePerM,
		CacheReadPerM:  c.provider.CacheReadPricePerM,
		CacheWritePerM: c.provider.CacheWritePricePerM,
	}
}

func (c *Client) Call(ctx context.Context, messages []domain.Message, opts CallOptions) (*domain.Response, error) {
	var tools []domain.ToolDef
	if !opts.NoTools {
		tools = c.tools
		if len(opts.ExtraTools) > 0 {
			tools = append(append([]domain.ToolDef{}, c.tools...), opts.ExtraTools...)
		}
	}

	maxTokens := opts.MaxTokens
	if maxTokens <= 0 {
		maxTokens = c.provider.MaxTokens
	}
	if maxTokens <= 0 {
		maxTokens = defaultMaxTokens
	}

	callCtx, cancel := c.callCtx(ctx)
	defer cancel()
	return c.adapter.Call(callCtx, c.model, opts.System, messages, tools, maxTokens)
}
