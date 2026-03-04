package openai

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"gogogot/infra/llm/types"

	"github.com/openai/openai-go"
	oaioption "github.com/openai/openai-go/option"
	"github.com/openai/openai-go/shared"
)

type Backend struct {
	client *openai.Client
}

func NewBackend(baseURL, apiKey string) *Backend {
	c := openai.NewClient(
		oaioption.WithBaseURL(baseURL),
		oaioption.WithAPIKey(apiKey),
	)
	return &Backend{client: &c}
}

func (b *Backend) Call(
	ctx context.Context,
	model string,
	systemPrompt string,
	messages []types.Message,
	tools []types.ToolDef,
	maxTokens int,
) (*types.Response, error) {
	oaiMsgs := messagesToOpenAI(model, systemPrompt, messages)
	oaiTools := toolDefsToOpenAI(tools)

	params := openai.ChatCompletionNewParams{
		Model:     shared.ChatModel(model),
		Messages:  oaiMsgs,
		MaxTokens: openai.Int(int64(maxTokens)),
	}
	if len(oaiTools) > 0 {
		params.Tools = oaiTools
	}

	reqJSON, _ := json.Marshal(params)
	slog.Debug("openai call start", "model", model, "messages", len(oaiMsgs), "req", string(reqJSON))
	start := time.Now()

	resp, err := b.client.Chat.Completions.New(ctx, params)
	elapsed := time.Since(start)
	if err != nil {
		slog.Error("openai call failed", "error", err, "elapsed", elapsed)
		return nil, err
	}

	slog.Info("openai call completed",
		"elapsed", elapsed,
		"prompt_tokens", resp.Usage.PromptTokens,
		"completion_tokens", resp.Usage.CompletionTokens,
	)

	return openaiToResponse(resp), nil
}

func messagesToOpenAI(model, system string, msgs []types.Message) []openai.ChatCompletionMessageParamUnion {
	var out []openai.ChatCompletionMessageParamUnion

	if system != "" {
		out = append(out, openai.SystemMessage(system))
	}

	supportsVision := !strings.Contains(model, "minimax")

	for _, msg := range msgs {
		switch msg.Role {
		case types.RoleUser:
			var parts []openai.ChatCompletionContentPartUnionParam
			var toolResults []openai.ChatCompletionMessageParamUnion
			hasImage := false
			var textOnly string

			for _, b := range msg.Content {
				switch b.Type {
				case "text":
					parts = append(parts, openai.TextContentPart(b.Text))
					textOnly += b.Text
				case "image":
					if supportsVision {
						hasImage = true
						dataURL := fmt.Sprintf("data:%s;base64,%s", b.MimeType, b.ImageData)
						parts = append(parts, openai.ImageContentPart(openai.ChatCompletionContentPartImageImageURLParam{
							URL: dataURL,
						}))
					} else {
						placeholder := "[Image omitted: model does not support vision]"
						parts = append(parts, openai.TextContentPart(placeholder))
						textOnly += placeholder + "\n"
					}
				case "tool_result":
					toolResults = append(toolResults, openai.ToolMessage(b.ToolOutput, b.ToolUseID))
				}
			}

			if len(parts) > 0 {
				if hasImage {
					out = append(out, openai.UserMessage(parts))
				} else {
					out = append(out, openai.UserMessage(textOnly))
				}
			}
			out = append(out, toolResults...)

		case types.RoleAssistant:
			var text string
			var toolCalls []openai.ChatCompletionMessageToolCallParam

			for _, b := range msg.Content {
				switch b.Type {
				case "text":
					text += b.Text
				case "tool_use":
					toolCalls = append(toolCalls, openai.ChatCompletionMessageToolCallParam{
						ID: b.ToolUseID,
						Function: openai.ChatCompletionMessageToolCallFunctionParam{
							Name:      b.ToolName,
							Arguments: string(b.ToolInput),
						},
					})
				}
			}

			asst := openai.ChatCompletionAssistantMessageParam{}
			if text != "" {
				asst.Content.OfString = openai.String(text)
			}
			if len(toolCalls) > 0 {
				asst.ToolCalls = toolCalls
			}
			out = append(out, openai.ChatCompletionMessageParamUnion{OfAssistant: &asst})
		}
	}

	return out
}

func toolDefsToOpenAI(defs []types.ToolDef) []openai.ChatCompletionToolParam {
	var out []openai.ChatCompletionToolParam
	for _, d := range defs {
		params := shared.FunctionParameters{
			"type": "object",
		}
		if d.Parameters != nil {
			params["properties"] = d.Parameters
		}
		if len(d.Required) > 0 {
			params["required"] = d.Required
		}

		fd := shared.FunctionDefinitionParam{
			Name:        d.Name,
			Description: openai.String(d.Description),
			Parameters:  params,
		}
		out = append(out, openai.ChatCompletionToolParam{Function: fd})
	}
	return out
}

func openaiToResponse(resp *openai.ChatCompletion) *types.Response {
	r := &types.Response{
		ID:           resp.ID,
		InputTokens:  int(resp.Usage.PromptTokens),
		OutputTokens: int(resp.Usage.CompletionTokens),
	}

	if len(resp.Choices) == 0 {
		r.StopReason = "end_turn"
		return r
	}

	choice := resp.Choices[0]

	switch choice.FinishReason {
	case "stop":
		r.StopReason = "end_turn"
	case "tool_calls":
		r.StopReason = "tool_use"
	case "length":
		r.StopReason = "max_tokens"
	default:
		r.StopReason = "end_turn"
	}

	if choice.Message.Content != "" {
		r.Content = append(r.Content, types.TextBlock(choice.Message.Content))
	}

	for _, tc := range choice.Message.ToolCalls {
		r.Content = append(r.Content, types.ToolUseBlock(
			tc.ID,
			tc.Function.Name,
			json.RawMessage(tc.Function.Arguments),
		))
	}

	return r
}
