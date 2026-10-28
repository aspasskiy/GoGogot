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
	InputTokens  int    // input tokens billed at the plain input price
	OutputTokens int

	// Prompt-cache accounting; not included in InputTokens. Zero for models
	// without caching.
	CacheReadTokens  int
	CacheWriteTokens int
}

// Pricing holds a model's USD prices per million tokens. A zero cache price
// means the model has no separate cache rate (or it is unknown), and those
// tokens are billed at the plain input price.
type Pricing struct {
	InputPerM      float64
	OutputPerM     float64
	CacheReadPerM  float64
	CacheWritePerM float64
}

// Cost estimates the USD cost of one response.
func (p Pricing) Cost(resp *Response) float64 {
	readPerM, writePerM := p.CacheReadPerM, p.CacheWritePerM
	if readPerM == 0 {
		readPerM = p.InputPerM
	}
	if writePerM == 0 {
		writePerM = p.InputPerM
	}
	return (float64(resp.InputTokens)*p.InputPerM +
		float64(resp.CacheReadTokens)*readPerM +
		float64(resp.CacheWriteTokens)*writePerM +
		float64(resp.OutputTokens)*p.OutputPerM) / 1_000_000
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

// IsToolResultTurn reports whether blocks carry only tool results — a
// user-role turn the agent produced, not a message the user typed.
func IsToolResultTurn(blocks []ContentBlock) bool {
	if len(blocks) == 0 {
		return false
	}
	for _, b := range blocks {
		if b.Type != BlockToolResult {
			return false
		}
	}
	return true
}

func NewUserMessage(blocks ...ContentBlock) Message {
	return Message{Role: RoleUser, Content: blocks}
}
