package types

import "encoding/json"

type Role string

const (
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
)

type Message struct {
	Role    Role
	Content []ContentBlock
}

type ContentBlock struct {
	Type       string          // "text", "image", "tool_use", "tool_result"
	Text       string          `json:"text,omitempty"`
	ToolUseID  string          `json:"tool_use_id,omitempty"`
	ToolName   string          `json:"tool_name,omitempty"`
	ToolInput  json.RawMessage `json:"tool_input,omitempty"`
	ToolOutput string          `json:"tool_output,omitempty"`
	ToolIsErr  bool            `json:"tool_is_err,omitempty"`
	ImageData  string          `json:"image_data,omitempty"`
	MimeType   string          `json:"mime_type,omitempty"`
}

type ToolDef struct {
	Name        string
	Description string
	Parameters  map[string]any
	Required    []string
}

type Response struct {
	ID           string
	Content      []ContentBlock
	StopReason   string // "end_turn", "tool_use", "max_tokens"
	InputTokens  int
	OutputTokens int
}

func TextBlock(text string) ContentBlock {
	return ContentBlock{Type: "text", Text: text}
}

func ImageBlock(mimeType, base64Data string) ContentBlock {
	return ContentBlock{Type: "image", MimeType: mimeType, ImageData: base64Data}
}

func ToolUseBlock(id, name string, input json.RawMessage) ContentBlock {
	return ContentBlock{Type: "tool_use", ToolUseID: id, ToolName: name, ToolInput: input}
}

func ToolResultBlock(id, output string, isErr bool) ContentBlock {
	return ContentBlock{Type: "tool_result", ToolUseID: id, ToolOutput: output, ToolIsErr: isErr}
}

func ExtractText(blocks []ContentBlock) string {
	var s string
	for _, b := range blocks {
		if b.Type == "text" {
			s += b.Text
		}
	}
	return s
}

func NewUserMessage(blocks ...ContentBlock) Message {
	return Message{Role: RoleUser, Content: blocks}
}

func NewAssistantMessage(blocks ...ContentBlock) Message {
	return Message{Role: RoleAssistant, Content: blocks}
}
