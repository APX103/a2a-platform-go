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
	"a2a-platform/internal/model"
	"a2a-platform/internal/redact"
)

const (
	maxSubagentRounds = 10
	subagentTimeout   = 5 * time.Minute
)

// SubagentStore is the interface for subagent session persistence
// Must match svc.SubagentStore method signatures
type SubagentStore interface {
	Create(parentContextId, parentToolCallId, task, context string) (*model.SubagentSession, error)
	Complete(id, result string) error
	Fail(id, errorMsg string) error
	UpdateMessages(id, messagesJSON string) error
}

// SubagentEngine handles spawning and managing subagents for isolated task execution
type SubagentEngine struct {
	store       SubagentStore
	llmProvider llm.Provider
	agentName   string
	agentConfig llm.ChatRequest
	mu          sync.Mutex
	activeCount int
}

// Store returns the subagent store for session management
func (e *SubagentEngine) Store() SubagentStore {
	return e.store
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
	// Create subagent session
	subId, err := e.store.Create(parentContextId, parentToolCallId, redact.Text(task), redact.Text(contextStr))
	if err != nil {
		return "", fmt.Errorf("failed to create subagent session: %w", err)
	}

	return e.RunExisting(ctx, subId.ID, task, contextStr)
}

// RunExisting executes a subagent task using an already-created session.
func (e *SubagentEngine) RunExisting(
	ctx context.Context,
	sessionID string,
	task string,
	contextStr string,
) (string, error) {
	// Limit concurrent subagents
	e.mu.Lock()
	if e.activeCount >= 3 {
		e.mu.Unlock()
		err := fmt.Errorf("too many concurrent subagents (max 3)")
		_ = e.store.Fail(sessionID, err.Error())
		return "", err
	}
	e.activeCount++
	e.mu.Unlock()
	defer func() {
		e.mu.Lock()
		e.activeCount--
		e.mu.Unlock()
	}()

	slog.Info("Subagent started", "id", sessionID, "task", redact.Text(task))

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

	result, err := e.runLLMLoop(timeoutCtx, messages, sessionID)

	if err != nil {
		// Store error result
		slog.Error("Subagent failed", "id", sessionID, "error", redact.Text(err.Error()))
		e.store.Fail(sessionID, redact.Text(err.Error()))
		return "", err
	}

	// Save final messages
	messagesJSON, _ := json.Marshal(redactChatMessagesForPersistence(messages))
	e.store.UpdateMessages(sessionID, string(messagesJSON))

	// Complete session
	e.store.Complete(sessionID, redact.Text(result))

	slog.Info("Subagent completed", "id", sessionID, "duration", time.Since(startTime))
	return result, nil
}

func redactChatMessagesForPersistence(messages []llm.ChatMessage) []llm.ChatMessage {
	out := make([]llm.ChatMessage, len(messages))
	copy(out, messages)
	for i := range out {
		out[i].Content = redact.Text(out[i].Content)
		out[i].ReasoningContent = redact.Text(out[i].ReasoningContent)
		if len(out[i].ToolCalls) == 0 {
			continue
		}
		out[i].ToolCalls = make([]llm.ToolCall, len(messages[i].ToolCalls))
		copy(out[i].ToolCalls, messages[i].ToolCalls)
		for j := range out[i].ToolCalls {
			out[i].ToolCalls[j].Arguments = redact.Text(out[i].ToolCalls[j].Arguments)
		}
	}
	return out
}

func (e *SubagentEngine) runLLMLoop(
	ctx context.Context,
	messages []llm.ChatMessage,
	sessionId string,
) (string, error) {
	for round := 0; round < maxSubagentRounds; round++ {
		toolDefs := ToToolDefs(GetBuiltinTools())

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
		return spawnAgent(context.Background(), engine, args)
	}
}

func SpawnAgentContext(engine *SubagentEngine) func(context.Context, map[string]any) (string, error) {
	return func(ctx context.Context, args map[string]any) (string, error) {
		return spawnAgent(ctx, engine, args)
	}
}

func spawnAgent(ctx context.Context, engine *SubagentEngine, args map[string]any) (string, error) {
	task, _ := args["task"].(string)
	if task == "" {
		return "", fmt.Errorf("task is required for spawn_agent")
	}

	contextStr, _ := args["context"].(string)

	// Get parent context from the calling context if available
	parentContextId, _ := args["_parent_context_id"].(string)
	parentToolCallId, _ := args["_parent_tool_call_id"].(string)

	result, err := engine.Run(ctx, task, contextStr, parentContextId, parentToolCallId)
	if err != nil {
		return "", fmt.Errorf("subagent execution failed: %w", err)
	}

	return result, nil
}

// NewSpawnAgentTool creates a BuiltinTool definition for spawn_agent
func NewSpawnAgentTool(engine *SubagentEngine) model.BuiltinTool {
	return model.BuiltinTool{
		Name:        "spawn_agent",
		Description: "Spawn a subagent to handle a specific task with isolated context. Use this when the task is complex, self-contained, or requires different tools than the current conversation. The subagent runs independently and returns its result.",
		Parameters: []model.ToolParameter{
			{
				Name:        "task",
				Type:        "string",
				Description: "The specific task description for the subagent to complete",
				Required:    true,
			},
			{
				Name:        "context",
				Type:        "string",
				Description: "Optional additional context or background information to pass to the subagent",
				Required:    false,
			},
		},
		Execute: func(args map[string]any) (string, error) {
			return spawnAgent(context.Background(), engine, args)
		},
		ExecuteContext: SpawnAgentContext(engine),
	}
}
