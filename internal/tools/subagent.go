package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"a2a-platform/internal/llm"
)

const (
	maxSubagentRounds = 10
	subagentTimeout   = 5 * time.Minute
)

// SubagentStore is the interface for subagent session persistence
// Must match svc.SubagentStore method signatures
type SubagentStore interface {
	Create(parentContextId, parentToolCallId, task, context string) (*SubagentSession, error)
	Complete(id, result string) error
	Fail(id, errorMsg string) error
	UpdateMessages(id, messagesJSON string) error
}

// SubagentSession represents a spawned subagent execution
type SubagentSession struct {
	ID               string    `json:"id"`
	ParentContextId  string    `json:"parent_context_id"`
	ParentToolCallId string    `json:"parent_tool_call_id"`
	Task             string    `json:"task"`
	Context          string    `json:"context"`
	Status           string    `json:"status"`
	Messages         string    `json:"messages"`
	Result           string    `json:"result"`
	Error            string    `json:"error"`
	CreatedAt        time.Time `json:"created_at"`
	CompletedAt      *time.Time `json:"completed_at"`
}

// SubagentEngine handles spawning and managing subagents for isolated task execution
type SubagentEngine struct {
	store        SubagentStore
	llmProvider  llm.Provider
	agentName    string
	agentConfig  llm.ChatRequest
	mu           sync.Mutex
	activeCount  int
}

// NewSubagentEngine creates a new subagent engine
func NewSubagentEngine(store SubagentStore, llmProvider llm.Provider, agentName string, config llm.ChatRequest) *SubagentEngine {
	return &SubagentEngine{
		store:       store,
		llmProvider: llmProvider,
		agentName:   agentName,
		agentConfig: config,
	}
}

// Run executes a subagent task and returns the result
func (e *SubagentEngine) Run(
	ctx context.Context,
	task string,
	contextStr string,
	parentContextId string,
	parentToolCallId string,
) (string, error) {
	// Limit concurrent subagents
	e.mu.Lock()
	if e.activeCount >= 3 {
		e.mu.Unlock()
		return "", fmt.Errorf("too many concurrent subagents (max 3)")
	}
	e.activeCount++
	e.mu.Unlock()
	defer func() {
		e.mu.Lock()
		e.activeCount--
		e.mu.Unlock()
	}()

	// Create subagent session
	subId, err := e.store.Create(parentContextId, parentToolCallId, task, contextStr)
	if err != nil {
		return "", fmt.Errorf("failed to create subagent session: %w", err)
	}

	slog.Info("Subagent started", "id", subId, "task", task, "parent", parentContextId)

	// Prepare system prompt for subagent
	toolNames := ""
	for _, t := range GetBuiltinTools() {
		toolNames += fmt.Sprintf("- %s: %s\n", t.Name, t.Description)
	}

	contextSection := ""
	if contextStr != "" {
		contextSection = fmt.Sprintf("\n\n【父对话提供的上下文】\n%s", contextStr)
	}

	systemPrompt := fmt.Sprintf(`你是一个子代理（Subagent），被父代理调用来完成特定任务。

你的任务：%s

可用工具列表：
%s%s

当需要使用工具时，通过 function call 调用。专注于完成任务，不要发散到无关主题。
如果任务无法完成，请说明原因和已尝试的步骤。`, task, toolNames, contextSection)

	// Prepare initial messages
	messages := []llm.ChatMessage{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: task},
	}

	startTime := time.Now()

	// Run LLM loop with timeout
	timeoutCtx, cancel := context.WithTimeout(ctx, subagentTimeout)
	defer cancel()

	result, err := e.runLLMLoop(timeoutCtx, messages, subId.ID)

	if err != nil {
		// Store error result
		slog.Error("Subagent failed", "id", subId.ID, "error", err)
		e.store.Fail(subId.ID, err.Error())
		return "", err
	}

	// Save final messages
	messagesJSON, _ := json.Marshal(messages)
	e.store.UpdateMessages(subId.ID, string(messagesJSON))

	// Complete session
	e.store.Complete(subId.ID, result)

	slog.Info("Subagent completed", "id", subId.ID, "duration", time.Since(startTime))
	return result, nil
}

func (e *SubagentEngine) runLLMLoop(
	ctx context.Context,
	messages []llm.ChatMessage,
	sessionId string,
) (string, error) {
	for round := 0; round < maxSubagentRounds; round++ {
		// Build tools for LLM
		var toolDefs []llm.ToolDef
		for _, tool := range GetBuiltinTools() {
			properties := make(map[string]interface{})
			required := make([]string, 0)
			for _, param := range tool.Parameters {
				paramType := param.Type
				if paramType == "number" {
					properties[param.Name] = map[string]interface{}{
						"type": "number",
					}
				} else if paramType == "boolean" {
					properties[param.Name] = map[string]interface{}{
						"type": "boolean",
					}
				} else {
					properties[param.Name] = map[string]interface{}{
						"type": "string",
					}
				}
				if param.Required {
					required = append(required, param.Name)
				}
			}
			inputSchema := map[string]interface{}{
				"type":       "object",
				"properties": properties,
			}
			if len(required) > 0 {
				inputSchema["required"] = required
			}
			toolDefs = append(toolDefs, llm.ToolDef{
				Name:        tool.Name,
				Description: tool.Description,
				InputSchema: inputSchema,
			})
		}

		// Create LLM request
		req := &llm.ChatRequest{
			Model:        e.agentConfig.Model,
			SystemPrompt: e.agentConfig.SystemPrompt,
			Messages:     messages,
			Tools:        toolDefs,
			MaxTokens:    e.agentConfig.MaxTokens,
		}

		stream, err := e.llmProvider.ChatStream(ctx, req)
		if err != nil {
			return "", fmt.Errorf("LLM call failed: %w", err)
		}

		var textBuf strings.Builder
		var toolCalls []llm.ToolCall

		for evt := range stream {
			switch evt.Type {
			case "text":
				textBuf.WriteString(evt.Text)
			case "tool_call":
				if evt.ToolCall != nil {
					toolCalls = append(toolCalls, *evt.ToolCall)
				}
			case "error":
				return "", evt.Error
			}
		}

		// If no tool calls, we're done
		if len(toolCalls) == 0 {
			return textBuf.String(), nil
		}

		// Record assistant message with tool calls
		messages = append(messages, llm.ChatMessage{
			Role:      "assistant",
			Content:   textBuf.String(),
			ToolCalls: toolCalls,
		})

		// Execute tool calls
		for _, tc := range toolCalls {
			// Parse arguments from JSON string
			var args map[string]any
			if tc.Arguments != "" {
				if err := json.Unmarshal([]byte(tc.Arguments), &args); err != nil {
					args = make(map[string]any)
				}
			}

			result, err := ExecuteTool(tc.Name, args)
			if err != nil {
				result = fmt.Sprintf("Error: %s", err)
			}

			messages = append(messages, llm.ChatMessage{
				Role:       "tool",
				Content:    result,
				ToolCallID: tc.ID,
			})
		}
	}

	return "", fmt.Errorf("max subagent rounds (%d) exceeded", maxSubagentRounds)
}

// SpawnAgent creates a subagent execution function that can be used as a tool
// This is a factory function - you need to pass the engine when calling it
func SpawnAgent(engine *SubagentEngine) func(args map[string]any) (string, error) {
	return func(args map[string]any) (string, error) {
		task, _ := args["task"].(string)
		if task == "" {
			return "", fmt.Errorf("task is required for spawn_agent")
		}

		contextStr, _ := args["context"].(string)

		// Get parent context from the calling context if available
		parentContextId := ""
		parentToolCallId := ""

		// Execute subagent
		ctx, cancel := context.WithTimeout(context.Background(), subagentTimeout)
		defer cancel()

		result, err := engine.Run(ctx, task, contextStr, parentContextId, parentToolCallId)
		if err != nil {
			return "", fmt.Errorf("subagent execution failed: %w", err)
		}

		return result, nil
	}
}