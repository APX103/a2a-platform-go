package model

// BuiltinTool represents a built-in tool definition.
type BuiltinTool struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Parameters  []ToolParameter         `json:"parameters"`
	Execute     func(args map[string]any) (string, error)
	IsReadOnly  bool                   `json:"is_read_only,omitempty"` // true = safe for concurrent execution
}

// ToolParameter represents a tool parameter.
type ToolParameter struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	Description string `json:"description"`
	Required    bool   `json:"required"`
}

// BuiltinToolRequest represents a request to execute a builtin tool.
type BuiltinToolRequest struct {
	Name      string                 `json:"name"`
	Arguments map[string]any        `json:"arguments"`
}

// BuiltinToolResponse represents a tool execution result.
type BuiltinToolResponse struct {
	Name      string `json:"name"`
	Result    string `json:"result"`
	Error     string `json:"error,omitempty"`
	Status    string `json:"status"`
	StartTime string `json:"start_time"`
	EndTime   string `json:"end_time"`
}