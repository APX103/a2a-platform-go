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
	"a2a-platform/internal/tools"
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
	toolCallTimeout          = 120 * time.Second
)

type Deps struct {
	LoadHistory func(contextId string) ([]*model.Message, error)
	RecordTrace func(e *model.TraceEvent) error
	SaveMessage func(m *model.Message) error
}

type BuiltinAgent struct {
	Config     config.BuiltinAgent
	Provider   llm.Provider
	MCPClients []*mcpclient.Client
	Tools      []llm.ToolDef
}

type Engine struct {
	agents         map[string]*BuiltinAgent
	mu             sync.RWMutex
	callTool       func(agent *BuiltinAgent, name string, arguments string) (string, error)
	subagentEngine *tools.SubagentEngine
}

func New() *Engine {
	e := &Engine{agents: make(map[string]*BuiltinAgent)}
	e.callTool = e.defaultCallTool
	return e
}

func (e *Engine) SetSubagentEngine(se *tools.SubagentEngine) {
	e.subagentEngine = se
}

func (e *Engine) RegisterAgent(cfg config.BuiltinAgent) error {
	// Normalize defaults
	if cfg.MaxToolRounds == 0 {
		cfg.MaxToolRounds = 10
	}
	if cfg.MaxTurns == 0 {
		cfg.MaxTurns = 20
	}
	if cfg.MaxToolResultSize == 0 {
		cfg.MaxToolResultSize = 10000
	}

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

	// Add builtin tools (including A2A platform tools)
	for _, builtinTool := range tools.GetAllTools() {
		// Convert ToolParameter to InputSchema
		properties := make(map[string]interface{})
		required := make([]string, 0)
		for _, param := range builtinTool.Parameters {
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

		allTools = append(allTools, llm.ToolDef{
			Name:        builtinTool.Name,
			Description: builtinTool.Description,
			InputSchema: inputSchema,
			IsReadOnly:  builtinTool.IsReadOnly,
		})
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
		writeJSONError(w, "builtin agent not found", http.StatusNotFound)
		return
	}

	// Load conversation history for multi-turn
	var history []llm.ChatMessage
	if contextId != "" {
		msgs, err := deps.LoadHistory(contextId)
		if err == nil {
			history = buildChatHistory(msgs)
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
		writeJSONError(w, "streaming not supported", http.StatusInternalServerError)
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

func buildChatHistory(messages []*model.Message) []llm.ChatMessage {
	history := make([]llm.ChatMessage, 0, len(messages))
	for _, m := range messages {
		role := m.Role
		if role == "agent" {
			role = "assistant"
		}

		msg := llm.ChatMessage{Role: role, Content: m.Content}
		if m.ReasoningContent != nil && *m.ReasoningContent != "" {
			msg.ReasoningContent = *m.ReasoningContent
		}
		if m.ToolCalls != "" {
			var calls []llm.ToolCall
			if err := json.Unmarshal([]byte(m.ToolCalls), &calls); err == nil {
				msg.ToolCalls = calls
			}
		}
		if m.ToolCallId != nil && *m.ToolCallId != "" {
			msg.ToolCallID = *m.ToolCallId
		}

		// A tool-role message without tool_call_id is invalid for OpenAI-style
		// chat history and will break the next LLM call.
		if msg.Role == "tool" && msg.ToolCallID == "" {
			continue
		}
		history = append(history, msg)
	}
	return history
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
	if maxRounds == 0 {
		maxRounds = 10
	}
	maxTurns := cfg.MaxTurns
	if maxTurns == 0 {
		maxTurns = 20
	}
	turnCount := 0

	for round := 0; round <= maxRounds; round++ {
		turnCount++
		if turnCount > maxTurns {
			return "", fmt.Errorf("max turns (%d) exceeded", maxTurns)
		}

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
		var reasoningBuf strings.Builder
		var toolCalls []llm.ToolCall

		for evt := range stream {
			switch evt.Type {
			case "text":
				textBuf.WriteString(evt.Text)
				writeSSE(w, flusher, sseEventTextDelta, map[string]interface{}{
					"taskId": taskId,
					"text":   evt.Text,
				})
			case "reasoning":
				reasoningBuf.WriteString(evt.Reasoning)
				writeSSE(w, flusher, sseEventThinkingDelta, map[string]interface{}{
					"taskId":   taskId,
					"thinking": evt.Reasoning,
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
			if deps.SaveMessage != nil {
				msg := &model.Message{
					TaskId:    taskId,
					ContextId: &contextId,
					Role:      "agent",
					Content:   textBuf.String(),
				}
				if reasoningBuf.Len() > 0 {
					rc := reasoningBuf.String()
					msg.ReasoningContent = &rc
				}
				deps.SaveMessage(msg)
			}
			return textBuf.String(), nil
		}

		// If this is the last allowed round, don't execute tools.
		// Return an error before producing side effects.
		if round == maxRounds {
			return "", fmt.Errorf("max tool rounds (%d) exceeded", maxRounds)
		}

		// Record assistant message with tool calls
		assistantMsg := llm.ChatMessage{
			Role:             "assistant",
			Content:          textBuf.String(),
			ReasoningContent: reasoningBuf.String(),
			ToolCalls:        toolCalls,
		}
		messages = append(messages, assistantMsg)

		// Persist assistant message with tool calls
		if deps.SaveMessage != nil {
			tcJSON, _ := json.Marshal(toolCalls)
			msg := &model.Message{
				TaskId:    taskId,
				ContextId: &contextId,
				Role:      "agent",
				Content:   textBuf.String(),
				ToolCalls: string(tcJSON),
			}
			if reasoningBuf.Len() > 0 {
				rc := reasoningBuf.String()
				msg.ReasoningContent = &rc
			}
			deps.SaveMessage(msg)
		}

		// Build read-only lookup from registered tools
		toolRO := make(map[string]bool, len(agent.Tools))
		for _, t := range agent.Tools {
			toolRO[t.Name] = t.IsReadOnly
		}

		// Pre-execute read-only tools in parallel (no side effects)
		roResults := make(map[string]struct {
			result string
			err    error
		})
		var roWg sync.WaitGroup
		var roMu sync.Mutex
		for _, tc := range toolCalls {
			if tc.Name == "spawn_agent" || !toolRO[tc.Name] {
				continue
			}
			roWg.Add(1)
			go func(tcall llm.ToolCall) {
				defer roWg.Done()
				res, err := e.callToolWithTimeout(ctx, agent, tcall.Name, tcall.Arguments)
				if err != nil {
					res = fmt.Sprintf("Error: %s", err)
				}
				roMu.Lock()
				roResults[tcall.ID] = struct {
					result string
					err    error
				}{result: res, err: err}
				roMu.Unlock()
			}(tc)
		}
		roWg.Wait()

		// Process all tool calls serially (SSE + persist must be thread-safe)
		for _, tc := range toolCalls {
			toolStart := time.Now()

			writeSSE(w, flusher, sseEventToolCallStart, map[string]interface{}{
				"taskId": taskId,
				"tool": map[string]interface{}{
					"id":         tc.ID,
					"name":       tc.Name,
					"arguments":  tc.Arguments,
					"status":     "started",
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

			var result string
			var err error

			// Inline handling for spawn_agent to emit subagent SSE events
			if tc.Name == "spawn_agent" && e.subagentEngine != nil {
				var args map[string]any
				if tc.Arguments != "" {
					_ = json.Unmarshal([]byte(tc.Arguments), &args)
				}
				task, _ := args["task"].(string)
				contextStr, _ := args["context"].(string)

				// Create subagent session first to get the ID
				subSession, createErr := e.subagentEngine.Store().Create(contextId, tc.ID, task, contextStr)
				if createErr == nil && subSession != nil {
					writeSSE(w, flusher, sseEventSubagentStarted, map[string]interface{}{
						"taskId":        taskId,
						"subagent_id":   subSession.ID,
						"subagent_task": task,
						"tool_call_id":  tc.ID,
					})

					result, err = e.subagentEngine.Run(ctx, task, contextStr, contextId, tc.ID)
					if err != nil {
						result = fmt.Sprintf("Error: %s", err)
						writeSSE(w, flusher, sseEventSubagentError, map[string]interface{}{
							"taskId":       taskId,
							"subagent_id":  subSession.ID,
							"error":        err.Error(),
							"tool_call_id": tc.ID,
						})
					} else {
						writeSSE(w, flusher, sseEventSubagentComplete, map[string]interface{}{
							"taskId":       taskId,
							"subagent_id":  subSession.ID,
							"result":       truncate(result, 500),
							"tool_call_id": tc.ID,
						})
					}
				} else {
					result, err = e.callToolWithTimeout(ctx, agent, tc.Name, tc.Arguments)
					if err != nil {
						result = fmt.Sprintf("Error: %s", err)
					}
				}
			} else if res, ok := roResults[tc.ID]; ok {
				// Read-only result from parallel execution
				result = res.result
				err = res.err
			} else {
				// Write tool: execute serially
				result, err = e.callToolWithTimeout(ctx, agent, tc.Name, tc.Arguments)
				if err != nil {
					result = fmt.Sprintf("Error: %s", err)
				}
			}

			// Truncate large tool results to prevent context explosion
			maxToolResultSize := cfg.MaxToolResultSize
			if maxToolResultSize == 0 {
				maxToolResultSize = 10000
			}
			truncatedResult := result
			if len(result) > maxToolResultSize {
				truncatedResult = result[:maxToolResultSize] + fmt.Sprintf("\n... (truncated, %d chars omitted)", len(result)-maxToolResultSize)
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
					"result":   truncate(truncatedResult, 2000),
					"status":   "completed",
					"end_time": time.Now().Format(time.RFC3339),
				},
			})

			messages = append(messages, llm.ChatMessage{
				Role:       "tool",
				Content:    truncatedResult,
				ToolCallID: tc.ID,
			})

			// Persist tool result message (store truncated to protect DB)
			if deps.SaveMessage != nil {
				deps.SaveMessage(&model.Message{
					TaskId:     taskId,
					ContextId:  &contextId,
					Role:       "tool",
					Content:    truncatedResult,
					ToolCallId: &tc.ID,
				})
			}
		}
	}

	return "", fmt.Errorf("max tool rounds (%d) exceeded", maxRounds)
}

func (e *Engine) callToolWithTimeout(ctx context.Context, agent *BuiltinAgent, name string, arguments string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, toolCallTimeout)
	defer cancel()

	type result struct {
		text string
		err  error
	}
	done := make(chan result, 1)
	go func() {
		text, err := e.callTool(agent, name, arguments)
		done <- result{text: text, err: err}
	}()

	select {
	case res := <-done:
		return res.text, res.err
	case <-ctx.Done():
		return "", fmt.Errorf("tool %q timed out after %s", name, toolCallTimeout)
	}
}

func (e *Engine) defaultCallTool(agent *BuiltinAgent, name string, arguments string) (string, error) {
	// Check MCP tools first
	for _, c := range agent.MCPClients {
		for _, t := range c.Tools {
			if t.Name == name {
				result, err := c.CallTool(name, arguments)
				if err != nil {
					return fmt.Sprintf("Error: %v", err), err
				}
				return result, nil
			}
		}
	}

	// Check builtin tools
	for _, tool := range tools.GetAllTools() {
		if tool.Name == name {
			var args map[string]any
			if arguments != "" {
				if err := json.Unmarshal([]byte(arguments), &args); err != nil {
					return "", fmt.Errorf("error parsing arguments: %w", err)
				}
			}
			if args == nil {
				args = map[string]any{}
			}
			args["_source_agent"] = agent.Config.Name
			result, err := tool.Execute(args)
			if err != nil {
				return fmt.Sprintf("Error: %v", err), err
			}
			return result, nil
		}
	}

	return "", fmt.Errorf("tool %q not found", name)
}

func writeSSE(w http.ResponseWriter, flusher http.Flusher, event string, data interface{}) {
	// Inject event type into the data payload so the frontend can use data.type
	// to determine the event type, matching the frontend SSEEvent interface.
	payload := data
	if m, ok := data.(map[string]interface{}); ok {
		// Make a shallow copy to avoid mutating the caller's map
		copyMap := make(map[string]interface{}, len(m)+1)
		for k, v := range m {
			copyMap[k] = v
		}
		copyMap["type"] = event
		payload = copyMap
	}
	jsonData, err := json.Marshal(payload)
	if err != nil {
		slog.Error("Failed to marshal SSE data", "event", event, "error", err)
		return
	}
	fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, string(jsonData))
	flusher.Flush()
}

func writeJSONError(w http.ResponseWriter, message string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": message})
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}
