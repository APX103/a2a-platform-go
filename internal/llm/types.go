package llm

import "context"

type Provider interface {
	ChatStream(ctx context.Context, req *ChatRequest) (<-chan StreamEvent, error)
}

type ChatRequest struct {
	Model        string
	SystemPrompt string
	Messages     []ChatMessage
	Tools        []ToolDef
	MaxTokens    int
}

type ChatMessage struct {
	Role       string     `json:"role"`
	Content    string     `json:"content,omitempty"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
}

type ToolDef struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	InputSchema map[string]interface{} `json:"input_schema"`
}

type ToolCall struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type StreamEvent struct {
	Type     string // "text", "reasoning", "tool_call", "done", "error"
	Text     string
	Reasoning string // reasoning / thinking content (e.g. from DeepSeek/R1-style models)
	ToolCall *ToolCall
	Error    error
}
