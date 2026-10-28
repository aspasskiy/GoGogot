package anthropic

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/aspasskiy/gogogot/internal/domain"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
)

type Adapter struct {
	client       *anthropic.Client
	cacheControl bool // add prompt-cache breakpoints to each request
}

// NewAdapter builds the Anthropic adapter. Request deadlines come from the
// caller's context (llm.Client applies the configured per-call timeout).
func NewAdapter(apiKey, baseURL string, cacheControl bool) *Adapter {
	var opts []option.RequestOption
	if apiKey != "" {
		opts = append(opts, option.WithAPIKey(apiKey))
	}
	if baseURL != "" {
		opts = append(opts, option.WithBaseURL(baseURL))
	}
	api := anthropic.NewClient(opts...)
	return &Adapter{client: &api, cacheControl: cacheControl}
}

func (a *Adapter) Call(
	ctx context.Context,
	model string,
	systemPrompt string,
	messages []domain.Message,
	tools []domain.ToolDef,
	maxTokens int,
) (*domain.Response, error) {
	anthMsgs := messagesToAnthropic(messages)
	anthTools := toolDefsToAnthropic(tools)

	// Thinking shares max_tokens with the answer; callers size the limit for
	// the visible answer only, so a small limit would be spent on reasoning.
	if bindsThinking(model) && maxTokens < thinkingMaxTokens {
		maxTokens = thinkingMaxTokens
	}

	params := anthropic.MessageNewParams{
		Model:     anthropic.Model(model),
		MaxTokens: int64(maxTokens),
		Messages:  anthMsgs,
	}
	if systemPrompt != "" {
		params.System = []anthropic.TextBlockParam{{Text: systemPrompt}}
	}
	if len(anthTools) > 0 {
		params.Tools = anthTools
	}
	if a.cacheControl {
		addCacheBreakpoints(&params)
	}

	var reqOpts []option.RequestOption
	if bindsThinking(model) {
		// Thinking is always on for these models and its blocks are bound to
		// the conversation prefix. If history was rewritten (compaction), drop
		// the stale blocks instead of failing the request with a 400.
		params.Thinking = anthropic.ThinkingConfigParamUnion{OfAdaptive: &anthropic.ThinkingConfigAdaptiveParam{}}
		reqOpts = append(reqOpts,
			option.WithHeaderAdd("anthropic-beta", thinkingBindingBeta),
			option.WithJSONSet("thinking.block_binding.prefix_mismatch_behavior", "drop_block"),
		)
	}

	start := time.Now()
	msg, err := a.client.Messages.New(ctx, params, reqOpts...)
	if err != nil {
		return nil, fmt.Errorf("anthropic call (%s, elapsed %s): %w", model, time.Since(start), err)
	}

	return anthropicToResponse(msg), nil
}

const (
	thinkingBindingBeta = "thinking-binding-controls-2026-08-01"
	// Floor for max_tokens on always-thinking models. Stays under the SDK's
	// ~21K non-streaming limit.
	thinkingMaxTokens = 16000
)

// bindsThinking reports whether the model runs thinking by default and binds
// thinking blocks to the conversation (Claude 5 family).
func bindsThinking(model string) bool {
	for _, prefix := range []string{"claude-fable-5", "claude-mythos-5", "claude-opus-5", "claude-sonnet-5", "claude-haiku-5"} {
		if strings.HasPrefix(model, prefix) {
			return true
		}
	}
	return false
}

// addCacheBreakpoints marks the request prefix for prompt caching: the tool
// list, the system prompt and the newest message. Each agent iteration
// re-sends the previous iteration's prefix, so it is read from the cache at a
// fraction of the input price. The API allows up to 4 breakpoints.
func addCacheBreakpoints(params *anthropic.MessageNewParams) {
	if n := len(params.Tools); n > 0 {
		if cc := params.Tools[n-1].GetCacheControl(); cc != nil {
			*cc = anthropic.NewCacheControlEphemeralParam()
		}
	}
	if n := len(params.System); n > 0 {
		params.System[n-1].CacheControl = anthropic.NewCacheControlEphemeralParam()
	}
	if n := len(params.Messages); n > 0 {
		blocks := params.Messages[n-1].Content
		if m := len(blocks); m > 0 {
			if cc := blocks[m-1].GetCacheControl(); cc != nil {
				*cc = anthropic.NewCacheControlEphemeralParam()
			}
		}
	}
}

func messagesToAnthropic(msgs []domain.Message) []anthropic.MessageParam {
	runStart := currentRunStart(msgs)
	out := make([]anthropic.MessageParam, 0, len(msgs))
	for i, m := range msgs {
		var blocks []anthropic.ContentBlockParamUnion
		for _, b := range m.Content {
			switch b.Type {
			case domain.BlockThinking, domain.BlockRedactedThinking:
				// Earlier runs were produced under a different system prompt
				// (it carries the current time), so their thinking no longer
				// matches the prefix. Dropping blocks from the front of the
				// history is allowed; the current run's blocks go back as-is.
				if i < runStart {
					continue
				}
				if b.Type == domain.BlockThinking {
					blocks = append(blocks, anthropic.NewThinkingBlock(b.Signature, b.Text))
				} else {
					blocks = append(blocks, anthropic.NewRedactedThinkingBlock(b.Signature))
				}
			case domain.BlockText:
				blocks = append(blocks, anthropic.NewTextBlock(b.Text))
			case domain.BlockImage:
				blocks = append(blocks, anthropic.NewImageBlockBase64(b.MimeType, b.ImageData))
			case domain.BlockToolUse:
				blocks = append(blocks, anthropic.NewToolUseBlock(b.ToolUseID, b.ToolInput, b.ToolName))
			case domain.BlockToolResult:
				blocks = append(blocks, anthropic.ContentBlockParamUnion{
					OfToolResult: &anthropic.ToolResultBlockParam{
						ToolUseID: b.ToolUseID,
						IsError:   anthropic.Bool(b.ToolIsErr),
						Content: []anthropic.ToolResultBlockParamContentUnion{
							{OfText: &anthropic.TextBlockParam{Text: b.ToolOutput}},
						},
					},
				})
			}
		}

		if len(blocks) == 0 {
			continue // e.g. a thinking-only turn from an earlier run
		}
		role := anthropic.MessageParamRoleUser
		if m.Role == domain.RoleAssistant {
			role = anthropic.MessageParamRoleAssistant
		}
		out = append(out, anthropic.MessageParam{Role: role, Content: blocks})
	}
	return out
}

// currentRunStart returns the index of the last message the user typed (a
// user turn that is not just tool results) — where the current agent run began.
func currentRunStart(msgs []domain.Message) int {
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Role == domain.RoleUser && !domain.IsToolResultTurn(msgs[i].Content) {
			return i
		}
	}
	return 0
}

func toolDefsToAnthropic(defs []domain.ToolDef) []anthropic.ToolUnionParam {
	out := make([]anthropic.ToolUnionParam, 0, len(defs))
	for _, d := range defs {
		out = append(out, anthropic.ToolUnionParam{
			OfTool: &anthropic.ToolParam{
				Name:        d.Name,
				Description: anthropic.String(d.Description),
				InputSchema: anthropic.ToolInputSchemaParam{
					Properties: d.Parameters,
					Required:   d.Required,
				},
			},
		})
	}
	return out
}

func anthropicToResponse(msg *anthropic.Message) *domain.Response {
	resp := &domain.Response{
		ID:           msg.ID,
		StopReason:   string(msg.StopReason),
		InputTokens:  int(msg.Usage.InputTokens),
		OutputTokens: int(msg.Usage.OutputTokens),

		CacheReadTokens:  int(msg.Usage.CacheReadInputTokens),
		CacheWriteTokens: int(msg.Usage.CacheCreationInputTokens),
	}
	for _, block := range msg.Content {
		switch block.Type {
		case "text":
			resp.Content = append(resp.Content, domain.TextBlock(block.AsText().Text))
		case "tool_use":
			tu := block.AsToolUse()
			raw, _ := json.Marshal(tu.Input)
			resp.Content = append(resp.Content, domain.ToolUseBlock(tu.ID, tu.Name, raw))
		case "thinking":
			th := block.AsThinking()
			resp.Content = append(resp.Content, domain.ThinkingBlock(th.Thinking, th.Signature))
		case "redacted_thinking":
			resp.Content = append(resp.Content, domain.RedactedThinkingBlock(block.AsRedactedThinking().Data))
		}
	}
	return resp
}
