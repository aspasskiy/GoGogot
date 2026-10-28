package openai

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/aspasskiy/gogogot/internal/domain"

	"github.com/openai/openai-go"
	oaioption "github.com/openai/openai-go/option"
	"github.com/openai/openai-go/shared"
)

type Adapter struct {
	client         *openai.Client
	supportsVision bool
	cacheControl   bool // add cache_control breakpoints (Claude via OpenRouter)
}

// NewAdapter builds the OpenAI-compatible adapter. Request deadlines come from
// the caller's context (llm.Client applies the configured per-call timeout).
func NewAdapter(baseURL, apiKey string, supportsVision, cacheControl bool) *Adapter {
	c := openai.NewClient(
		oaioption.WithBaseURL(baseURL),
		oaioption.WithAPIKey(apiKey),
	)
	return &Adapter{client: &c, supportsVision: supportsVision, cacheControl: cacheControl}
}

func (a *Adapter) Call(
	ctx context.Context,
	model string,
	systemPrompt string,
	messages []domain.Message,
	tools []domain.ToolDef,
	maxTokens int,
) (*domain.Response, error) {
	reasoning := isReasoningModel(model)
	oaiMsgs := messagesToOpenAI(a.supportsVision, reasoning, systemPrompt, messages)
	oaiTools := toolDefsToOpenAI(tools)
	if a.cacheControl {
		addCacheBreakpoints(oaiMsgs)
	}

	params := openai.ChatCompletionNewParams{
		Model:    shared.ChatModel(model),
		Messages: oaiMsgs,
	}
	if reasoning {
		params.MaxCompletionTokens = openai.Int(int64(maxTokens))
	} else {
		params.MaxTokens = openai.Int(int64(maxTokens))
	}
	if len(oaiTools) > 0 {
		params.Tools = oaiTools
	}

	start := time.Now()
	resp, err := a.client.Chat.Completions.New(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("openai call (%s, elapsed %s): %w", model, time.Since(start), err)
	}

	return openaiToResponse(resp), nil
}

// cacheControlField is the OpenRouter extension that marks a prompt-cache
// breakpoint on a text content part.
var cacheControlField = map[string]any{"cache_control": map[string]any{"type": "ephemeral"}}

// addCacheBreakpoints marks the system prompt and the newest message for
// prompt caching, for models that cache only at explicit breakpoints. Their
// string contents are converted to a single text part, which is where the
// breakpoint lives.
func addCacheBreakpoints(msgs []openai.ChatCompletionMessageParamUnion) {
	if len(msgs) == 0 {
		return
	}
	if sys := msgs[0].OfSystem; sys != nil && sys.Content.OfString.Valid() {
		part := openai.ChatCompletionContentPartTextParam{Text: sys.Content.OfString.Value}
		part.SetExtraFields(cacheControlField)
		sys.Content = openai.ChatCompletionSystemMessageParamContentUnion{
			OfArrayOfContentParts: []openai.ChatCompletionContentPartTextParam{part},
		}
	}

	switch last := msgs[len(msgs)-1]; {
	case last.OfTool != nil && last.OfTool.Content.OfString.Valid():
		part := openai.ChatCompletionContentPartTextParam{Text: last.OfTool.Content.OfString.Value}
		part.SetExtraFields(cacheControlField)
		last.OfTool.Content = openai.ChatCompletionToolMessageParamContentUnion{
			OfArrayOfContentParts: []openai.ChatCompletionContentPartTextParam{part},
		}
	case last.OfUser != nil && last.OfUser.Content.OfString.Valid():
		part := openai.ChatCompletionContentPartTextParam{Text: last.OfUser.Content.OfString.Value}
		part.SetExtraFields(cacheControlField)
		last.OfUser.Content = openai.ChatCompletionUserMessageParamContentUnion{
			OfArrayOfContentParts: []openai.ChatCompletionContentPartUnionParam{{OfText: &part}},
		}
	case last.OfUser != nil:
		parts := last.OfUser.Content.OfArrayOfContentParts
		for i := len(parts) - 1; i >= 0; i-- {
			if parts[i].OfText != nil {
				parts[i].OfText.SetExtraFields(cacheControlField)
				break
			}
		}
	}
}

func isReasoningModel(model string) bool {
	// gpt-5, gpt-5-mini, gpt-5.4, gpt-5.6-luna, … all reject max_tokens.
	if strings.HasPrefix(model, "gpt-5") {
		return true
	}
	for _, prefix := range []string{"o1", "o3", "o4"} {
		if model == prefix || strings.HasPrefix(model, prefix+"-") {
			return true
		}
	}
	return false
}

func messagesToOpenAI(supportsVision, reasoning bool, system string, msgs []domain.Message) []openai.ChatCompletionMessageParamUnion {
	var out []openai.ChatCompletionMessageParamUnion

	if system != "" {
		if reasoning {
			out = append(out, openai.DeveloperMessage(system))
		} else {
			out = append(out, openai.SystemMessage(system))
		}
	}

	for _, msg := range msgs {
		switch msg.Role {
		case domain.RoleUser:
			var parts []openai.ChatCompletionContentPartUnionParam
			var toolResults []openai.ChatCompletionMessageParamUnion
			hasImage := false
			var textOnly string

			for _, b := range msg.Content {
				switch b.Type {
				case domain.BlockText:
					parts = append(parts, openai.TextContentPart(b.Text))
					textOnly += b.Text
				case domain.BlockImage:
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
				case domain.BlockToolResult:
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

		case domain.RoleAssistant:
			var text string
			var toolCalls []openai.ChatCompletionMessageToolCallParam

			for _, b := range msg.Content {
				switch b.Type {
				case domain.BlockText:
					text += b.Text
				case domain.BlockToolUse:
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

func toolDefsToOpenAI(defs []domain.ToolDef) []openai.ChatCompletionToolParam {
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

func openaiToResponse(resp *openai.ChatCompletion) *domain.Response {
	// prompt_tokens includes cached tokens; split them out so each is priced
	// at its own rate. cache_write_tokens is an OpenRouter extension.
	details := resp.Usage.PromptTokensDetails
	cacheRead := int(details.CachedTokens)
	var cacheWrite int
	if f, ok := details.JSON.ExtraFields["cache_write_tokens"]; ok {
		cacheWrite, _ = strconv.Atoi(f.Raw())
	}
	r := &domain.Response{
		ID:               resp.ID,
		InputTokens:      max(int(resp.Usage.PromptTokens)-cacheRead-cacheWrite, 0),
		OutputTokens:     int(resp.Usage.CompletionTokens),
		CacheReadTokens:  cacheRead,
		CacheWriteTokens: cacheWrite,
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
		r.Content = append(r.Content, domain.TextBlock(choice.Message.Content))
	}

	for _, tc := range choice.Message.ToolCalls {
		r.Content = append(r.Content, domain.ToolUseBlock(
			tc.ID,
			tc.Function.Name,
			json.RawMessage(tc.Function.Arguments),
		))
	}

	return r
}
