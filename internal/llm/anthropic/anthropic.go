package anthropic

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/aspasskiy/gogogot/internal/llm/types"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
)

type Adapter struct {
	client *anthropic.Client
}

func NewAdapter(apiKey, baseURL string) *Adapter {
	var opts []option.RequestOption
	if apiKey != "" {
		opts = append(opts, option.WithAPIKey(apiKey))
	}
	if baseURL != "" {
		opts = append(opts, option.WithBaseURL(baseURL))
	}
	api := anthropic.NewClient(opts...)
	return &Adapter{client: &api}
}

func (a *Adapter) Call(
	ctx context.Context,
	model string,
	systemPrompt string,
	messages []types.Message,
	tools []types.ToolDef,
	maxTokens int,
) (*types.Response, error) {
	anthMsgs := messagesToAnthropic(messages)
	anthTools := toolDefsToAnthropic(tools)

	params := anthropic.MessageNewParams{
		Model:     anthropic.Model(model),
		MaxTokens: int64(maxTokens),
		System:    []anthropic.TextBlockParam{{Text: systemPrompt}},
		Messages:  anthMsgs,
	}
	if len(anthTools) > 0 {
		params.Tools = anthTools
	}

	start := time.Now()
	msg, err := a.client.Messages.New(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("anthropic call (%s, elapsed %s): %w", model, time.Since(start), err)
	}

	return anthropicToResponse(msg), nil
}

func messagesToAnthropic(msgs []types.Message) []anthropic.MessageParam {
	out := make([]anthropic.MessageParam, 0, len(msgs))
	for _, m := range msgs {
		var blocks []anthropic.ContentBlockParamUnion
		for _, b := range m.Content {
			switch b.Type {
			case "text":
				blocks = append(blocks, anthropic.NewTextBlock(b.Text))
			case "image":
				blocks = append(blocks, anthropic.NewImageBlockBase64(b.MimeType, b.ImageData))
			case "tool_use":
				blocks = append(blocks, anthropic.NewToolUseBlock(b.ToolUseID, b.ToolInput, b.ToolName))
			case "tool_result":
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

		role := anthropic.MessageParamRoleUser
		if m.Role == types.RoleAssistant {
			role = anthropic.MessageParamRoleAssistant
		}
		out = append(out, anthropic.MessageParam{Role: role, Content: blocks})
	}
	return out
}

func toolDefsToAnthropic(defs []types.ToolDef) []anthropic.ToolUnionParam {
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

func anthropicToResponse(msg *anthropic.Message) *types.Response {
	resp := &types.Response{
		ID:           msg.ID,
		StopReason:   string(msg.StopReason),
		InputTokens:  int(msg.Usage.InputTokens),
		OutputTokens: int(msg.Usage.OutputTokens),
	}
	for _, block := range msg.Content {
		switch block.Type {
		case "text":
			resp.Content = append(resp.Content, types.TextBlock(block.AsText().Text))
		case "tool_use":
			tu := block.AsToolUse()
			raw, _ := json.Marshal(tu.Input)
			resp.Content = append(resp.Content, types.ToolUseBlock(tu.ID, tu.Name, raw))
		}
	}
	return resp
}
