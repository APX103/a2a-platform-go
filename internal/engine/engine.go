package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"a2a-platform/internal/config"
	"a2a-platform/internal/llm"
	"a2a-platform/internal/mcpclient"
	"a2a-platform/internal/model"
)

const (
	sseEventTextDelta        = "text.delta"
	sseEventThinkingDelta    = "thinking.delta"
	sseEventThinkingBlock    = "thinking.block"
	sseEventToolCallStart    = "tool.call_start"
	sseEventToolCallDelta    = "tool.call_delta"
	sseEventToolCallEnd      = "tool.call_end"
	sseEventToolResult       = "tool.result"
	sseEventSubagentStarted  = "subagent.started"
	sseEventSubagentComplete = "subagent.completed"
	sseEventSubagentError    = "subagent.error"
)

type Deps struct {
	LoadHistory func(contextId string) ([]*model.Message, error)
	RecordTrace func(e *model.TraceEvent) error
}

type BuiltinAgent struct {
	Config     config.BuiltinAgent
	Provider   llm.Provider
	MCPClients []*mcpclient.Client
	Tools      []llm.ToolDef
}

type Engine struct {
	agents map[string]*BuiltinAgent
	mu     sync.RWMutex
}

func New() *Engine {
	return &Engine{agents: make(map[string]*BuiltinAgent)}
}

func (e *Engine) RegisterAgent(cfg config.BuiltinAgent) error {
	var provider llm.Provider
	switch cfg.Provider {
	case "openai":
		provider = llm.NewOpenAIProvider(cfg.BaseURL, cfg.APIKey)
	case "anthropic":
		provider = llm.NewAnthropicProvider(cfg.BaseURL, cfg.APIKey)
	default:
		return fmt.Errorf("unknown provider: %s", cfg.Provider)
	}

	agent := &BuiltinAgent{
		Config:   cfg,
		Provider: provider,
	}

	// Connect MCP servers
	var allTools []llm.ToolDef
	for _, mcp := range cfg.MCPServers {
		var client *mcpclient.Client
		var err error
		switch mcp.Transport {
		case "sse":
			client, err = mcpclient.ConnectSSE(mcp.Name, mcp.URL)
		case "stdio":
			client, err = mcpclient.ConnectStdio(mcp.Name, mcp.Command, mcp.Args)
		default:
			slog.Warn("Unknown MCP transport", "name", mcp.Name, "transport", mcp.Transport)
			continue
		}
		if err != nil {
			slog.Error("Failed to connect MCP server", "name", mcp.Name, "error", err)
			continue
		}
		agent.MCPClients = append(agent.MCPClients, client)
		allTools = append(allTools, client.Tools...)
	}
	agent.Tools = allTools

	e.mu.Lock()
	e.agents[cfg.Name] = agent
	e.mu.Unlock()

	slog.Info("Registered builtin agent", "name", cfg.Name, "provider", cfg.Provider, "model", cfg.Model, "tools", len(allTools))
	return nil
}

func (e *Engine) RemoveAgent(name string) {
	e.mu.Lock()
	agent, ok := e.agents[name]
	if ok {
		for _, c := range agent.MCPClients {
			c.Close()
		}
		delete(e.agents, name)
	}
	e.mu.Unlock()
}

func (e *Engine) GetAgent(name string) *BuiltinAgent {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.agents[name]
}

func (e *Engine) ListAgents() []config.BuiltinAgent {
	e.mu.RLock()
	defer e.mu.RUnlock()
	var result []config.BuiltinAgent
	for _, a := range e.agents {
		result = append(result, a.Config)
	}
	return result
}

// HandleRequest processes a message for a builtin agent, writing an SSE response.
func (e *Engine) HandleRequest(
	ctx context.Context,
	w http.ResponseWriter,
	agentName string,
	userText string,
	contextId string,
	taskId string,
	deps *Deps,
) {
	agent := e.GetAgent(agentName)
	if agent == nil {
		http.Error(w, `{"error":"builtin agent not found"}`, 404)
		return
	}

	// Load conversation history for multi-turn
	var history []llm.ChatMessage
	if contextId != "" {
		msgs, err := deps.LoadHistory(contextId)
		if err == nil {
			for _, m := range msgs {
				role := m.Role
				if role == "agent" {
					role = "assistant"
				}
				history = append(history, llm.ChatMessage{Role: role, Content: m.Content})
			}
		}
	}

	// Add current user message
	history = append(history, llm.ChatMessage{Role: "user", Content: userText})

	// Set up SSE response
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", 500)
		return
	}

	// Send initial task status
	writeSSE(w, flusher, "task.status", map[string]interface{}{
		"taskId":    taskId,
		"contextId": contextId,
		"status":    map[string]string{"state": "working"},
	})

	// Run the LLM + tool loop
	finalText, err := e.runLoop(ctx, agent, history, w, flusher, taskId, contextId, deps)
	if err != nil {
		slog.Error("Builtin agent error", "agent", agentName, "error", err)
		writeSSE(w, flusher, "task.status", map[string]interface{}{
			"taskId":    taskId,
			"contextId": contextId,
			"status":    map[string]string{"state": "failed", "message": err.Error()},
		})
		return
	}

	// Send final completed status
	writeSSE(w, flusher, "task.status", map[string]interface{}{
		"taskId":    taskId,
		"contextId": contextId,
		"status": map[string]interface{}{
			"state": "completed",
			"message": map[string]interface{}{
				"role":  "agent",
				"parts": []map[string]string{{"text": finalText}},
			},
		},
	})
}

func (e *Engine) runLoop(
	ctx context.Context,
	agent *BuiltinAgent,
	messages []llm.ChatMessage,
	w http.ResponseWriter,
	flusher http.Flusher,
	taskId, contextId string,
	deps *Deps,
) (string, error) {
	cfg := agent.Config
	maxRounds := cfg.MaxToolRounds

	for round := 0; round <= maxRounds; round++ {
		req := &llm.ChatRequest{
			Model:        cfg.Model,
			SystemPrompt: cfg.SystemPrompt,
			Messages:     messages,
			Tools:        agent.Tools,
			MaxTokens:    cfg.MaxTokens,
		}

		stream, err := agent.Provider.ChatStream(ctx, req)
		if err != nil {
			return "", fmt.Errorf("LLM call failed: %w", err)
		}

		var textBuf strings.Builder
		var toolCalls []llm.ToolCall

		for evt := range stream {
			switch evt.Type {
			case "text":
				textBuf.WriteString(evt.Text)
				writeSSE(w, flusher, sseEventTextDelta, map[string]interface{}{
					"taskId": taskId,
					"text":   evt.Text,
				})
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
		assistantMsg := llm.ChatMessage{
			Role:      "assistant",
			Content:   textBuf.String(),
			ToolCalls: toolCalls,
		}
		messages = append(messages, assistantMsg)

		// Execute tool calls
		for _, tc := range toolCalls {
			toolStart := time.Now()

			writeSSE(w, flusher, sseEventToolCallStart, map[string]interface{}{
				"taskId": taskId,
				"tool": map[string]interface{}{
					"id":        tc.ID,
					"name":      tc.Name,
					"arguments": tc.Arguments,
					"status":    "started",
					"start_time": toolStart.Format(time.RFC3339),
				},
			})

			traceData, _ := json.Marshal(map[string]string{"tool": tc.Name, "arguments": tc.Arguments})
			deps.RecordTrace(&model.TraceEvent{
				TaskId:    taskId,
				ContextId: &contextId,
				EventType: "tool_call",
				AgentName: agent.Config.Name,
				DataJson:  string(traceData),
			})

			result, err := e.callTool(agent, tc.Name, tc.Arguments)
			if err != nil {
				result = fmt.Sprintf("Error: %s", err)
			}

			writeSSE(w, flusher, sseEventToolCallEnd, map[string]interface{}{
				"taskId": taskId,
				"tool": map[string]interface{}{
					"id":        tc.ID,
					"name":      tc.Name,
					"arguments": tc.Arguments,
				},
			})

			writeSSE(w, flusher, sseEventToolResult, map[string]interface{}{
				"taskId": taskId,
				"tool": map[string]interface{}{
					"id":       tc.ID,
					"name":     tc.Name,
					"result":   truncate(result, 2000),
					"status":   "completed",
					"end_time": time.Now().Format(time.RFC3339),
				},
			})

			messages = append(messages, llm.ChatMessage{
				Role:       "tool",
				Content:    result,
				ToolCallID: tc.ID,
			})
		}
	}

	return "", fmt.Errorf("max tool rounds (%d) exceeded", maxRounds)
}

func (e *Engine) callTool(agent *BuiltinAgent, name string, arguments string) (string, error) {
	for _, c := range agent.MCPClients {
		for _, t := range c.Tools {
			if t.Name == name {
				return c.CallTool(name, arguments)
			}
		}
	}
	return "", fmt.Errorf("tool %q not found", name)
}

func writeSSE(w http.ResponseWriter, flusher http.Flusher, event string, data interface{}) {
	jsonData, _ := json.Marshal(data)
	fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, string(jsonData))
	flusher.Flush()
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}
