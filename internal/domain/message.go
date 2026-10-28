package domain

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

// BlockType identifies the kind of a ContentBlock. It is a plain string type so
// the persisted JSON format stays unchanged.
type BlockType = string

const (
	BlockText       BlockType = "text"
	BlockImage      BlockType = "image"
	BlockToolUse    BlockType = "tool_use"
	BlockToolResult BlockType = "tool_result"
)

type ContentBlock struct {
	Type       BlockType       `json:"type"`
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
	return ContentBlock{Type: BlockText, Text: text}
}

func ImageBlock(mimeType, base64Data string) ContentBlock {
	return ContentBlock{Type: BlockImage, MimeType: mimeType, ImageData: base64Data}
}

func ToolUseBlock(id, name string, input json.RawMessage) ContentBlock {
	return ContentBlock{Type: BlockToolUse, ToolUseID: id, ToolName: name, ToolInput: input}
}

func ToolResultBlock(id, output string, isErr bool) ContentBlock {
	return ContentBlock{Type: BlockToolResult, ToolUseID: id, ToolOutput: output, ToolIsErr: isErr}
}

func ExtractText(blocks []ContentBlock) string {
	var s string
	for _, b := range blocks {
		if b.Type == BlockText {
			s += b.Text
		}
	}
	return s
}

func NewUserMessage(blocks ...ContentBlock) Message {
	return Message{Role: RoleUser, Content: blocks}
}
