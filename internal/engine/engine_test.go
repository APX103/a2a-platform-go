package engine

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"a2a-platform/internal/config"
	"a2a-platform/internal/llm"
	"a2a-platform/internal/model"
)

// mockProvider is a test double for llm.Provider.
type mockProvider struct {
	events    []llm.StreamEvent
	eventIdx  int
	callCount int
	requests  []*llm.ChatRequest
}

func (m *mockProvider) ChatStream(ctx context.Context, req *llm.ChatRequest) (<-chan llm.StreamEvent, error) {
	m.callCount++
	m.requests = append(m.requests, req)
	ch := make(chan llm.StreamEvent, len(m.events))
	for m.eventIdx < len(m.events) {
		evt := m.events[m.eventIdx]
		m.eventIdx++
		if evt.Type == "done" {
			break
		}
		ch <- evt
	}
	close(ch)
	return ch, nil
}

func TestBuildChatHistory_RestoresToolCallIDs(t *testing.T) {
	ctxId := "ctx-1"
	toolCallId := "call-1"
	history := buildChatHistory([]*model.Message{
		{
			TaskId:    "task-1",
			ContextId: &ctxId,
			Role:      "agent",
			Content:   "I'll call a tool.",
			ToolCalls: `[{"id":"call-1","name":"test_tool","arguments":"{}"}]`,
		},
		{
			TaskId:     "task-1",
			ContextId:  &ctxId,
			Role:       "tool",
			Content:    "tool result",
			ToolCallId: &toolCallId,
		},
	})

	if len(history) != 2 {
		t.Fatalf("history len = %d, want 2", len(history))
	}
	if history[0].Role != "assistant" {
		t.Fatalf("first role = %q, want assistant", history[0].Role)
	}
	if len(history[0].ToolCalls) != 1 {
		t.Fatalf("assistant tool calls len = %d, want 1", len(history[0].ToolCalls))
	}
	if history[0].ToolCalls[0].ID != toolCallId {
		t.Fatalf("tool call id = %q, want %q", history[0].ToolCalls[0].ID, toolCallId)
	}
	if history[1].Role != "tool" {
		t.Fatalf("second role = %q, want tool", history[1].Role)
	}
	if history[1].ToolCallID != toolCallId {
		t.Fatalf("tool message tool_call_id = %q, want %q", history[1].ToolCallID, toolCallId)
	}
}

func TestBuildChatHistory_SkipsInvalidToolMessageWithoutID(t *testing.T) {
	history := buildChatHistory([]*model.Message{
		{TaskId: "task-1", Role: "tool", Content: "orphan result"},
		{TaskId: "task-1", Role: "user", Content: "next question"},
	})

	if len(history) != 1 {
		t.Fatalf("history len = %d, want 1", len(history))
	}
	if history[0].Role != "user" {
		t.Fatalf("role = %q, want user", history[0].Role)
	}
}

func TestHandleRequest_LoadsToolHistoryForNextTurn(t *testing.T) {
	eng := New()
	provider := &mockProvider{
		events: []llm.StreamEvent{
			{Type: "text", Text: "next answer"},
			{Type: "done"},
		},
	}
	agent := &BuiltinAgent{
		Config: config.BuiltinAgent{
			Name:          "test-agent",
			Provider:      "openai",
			Model:         "gpt-4",
			MaxToolRounds: 5,
		},
		Provider: provider,
	}
	eng.agents["test-agent"] = agent

	ctxId := "ctx-1"
	toolCallId := "call-1"
	deps := &Deps{
		LoadHistory: func(cid string) ([]*model.Message, error) {
			return []*model.Message{
				{
					TaskId:    "task-old",
					ContextId: &ctxId,
					Role:      "agent",
					Content:   "checking",
					ToolCalls: `[{"id":"call-1","name":"test_tool","arguments":"{}"}]`,
				},
				{
					TaskId:     "task-old",
					ContextId:  &ctxId,
					Role:       "tool",
					Content:    "tool result",
					ToolCallId: &toolCallId,
				},
			}, nil
		},
		RecordTrace: func(e *model.TraceEvent) error { return nil },
		SaveMessage: func(m *model.Message) error { return nil },
	}

	rec := httptest.NewRecorder()
	eng.HandleRequest(context.Background(), rec, "test-agent", "next question", ctxId, ctxId, "task-2", "", deps)

	if len(provider.requests) != 1 {
		t.Fatalf("provider requests len = %d, want 1", len(provider.requests))
	}
	messages := provider.requests[0].Messages
	if len(messages) != 3 {
		t.Fatalf("request messages len = %d, want 3", len(messages))
	}
	if len(messages[0].ToolCalls) != 1 {
		t.Fatalf("assistant history tool calls len = %d, want 1", len(messages[0].ToolCalls))
	}
	if messages[1].Role != "tool" || messages[1].ToolCallID != toolCallId {
		t.Fatalf("tool history = role %q id %q, want tool %q", messages[1].Role, messages[1].ToolCallID, toolCallId)
	}
}

// TestRunLoop_MaxRoundsExceeded_ToolExecuted is a regression test for the bug
// where tools are executed even when maxRounds is exceeded.
func TestRunLoop_MaxRoundsExceeded_ToolExecuted(t *testing.T) {
	eng := New()

	// Create a mock agent with maxToolRounds = 1
	agent := &BuiltinAgent{
		Config: config.BuiltinAgent{
			Name:          "test-agent",
			Provider:      "openai",
			Model:         "gpt-4",
			MaxToolRounds: 1,
			SystemPrompt:  "You are a test agent.",
		},
		Provider: &mockProvider{
			events: []llm.StreamEvent{
				// First round: text + tool call
				{Type: "text", Text: "Let me help you."},
				{Type: "tool_call", ToolCall: &llm.ToolCall{ID: "tc-1", Name: "test_tool", Arguments: `{}`}},
				{Type: "done"},
				// Second round: also returns tool call (should trigger maxRounds error)
				{Type: "text", Text: "Still need help."},
				{Type: "tool_call", ToolCall: &llm.ToolCall{ID: "tc-2", Name: "test_tool", Arguments: `{}`}},
				{Type: "done"},
			},
		},
		Tools: []llm.ToolDef{
			{Name: "test_tool", Description: "A test tool", InputSchema: map[string]interface{}{"type": "object"}},
		},
	}

	var callCount int
	var deps = &Deps{
		LoadHistory: func(cid string) ([]*model.Message, error) { return nil, nil },
		RecordTrace: func(e *model.TraceEvent) error { return nil },
		SaveMessage: func(m *model.Message) error { return nil },
	}

	// Override callTool to count executions
	originalCallTool := eng.callTool
	eng.callTool = func(a *BuiltinAgent, name string, arguments string, execCtx ToolExecutionContext) (string, error) {
		callCount++
		return "tool result", nil
	}
	defer func() { eng.callTool = originalCallTool }()

	rec := httptest.NewRecorder()
	flusher := &mockFlusher{recorder: rec}

	_, err := eng.runLoop(context.Background(), agent, []llm.ChatMessage{{Role: "user", Content: "hello"}}, rec, flusher, "task-1", "ctx-1", "ctx-1", "", deps)

	// With maxRounds=1:
	// - round=0: LLM returns tool call -> tool IS executed (callCount becomes 1)
	// - round=1: LLM returns tool call -> maxRounds exceeded, should error BEFORE executing
	//
	// The bug was that round=1's tool was ALSO executed (callCount would be 2).
	if err == nil {
		t.Errorf("Expected error when maxRounds exceeded, got nil")
	}

	if callCount != 1 {
		t.Errorf("Expected 1 tool execution (round=0 only), got %d", callCount)
	}
}

// TestRunLoop_NoToolCalls_ReturnsText verifies normal operation without tool calls.
func TestRunLoop_NoToolCalls_ReturnsText(t *testing.T) {
	eng := New()

	agent := &BuiltinAgent{
		Config: config.BuiltinAgent{
			Name:          "test-agent",
			Provider:      "openai",
			Model:         "gpt-4",
			MaxToolRounds: 5,
		},
		Provider: &mockProvider{
			events: []llm.StreamEvent{
				{Type: "text", Text: "Hello, user!"},
				{Type: "done"},
			},
		},
	}

	deps := &Deps{
		LoadHistory: func(cid string) ([]*model.Message, error) { return nil, nil },
		RecordTrace: func(e *model.TraceEvent) error { return nil },
		SaveMessage: func(m *model.Message) error { return nil },
	}

	rec := httptest.NewRecorder()
	flusher := &mockFlusher{recorder: rec}

	result, err := eng.runLoop(context.Background(), agent, []llm.ChatMessage{{Role: "user", Content: "hi"}}, rec, flusher, "task-1", "ctx-1", "ctx-1", "", deps)
	if err != nil {
		t.Fatalf("runLoop failed: %v", err)
	}
	if result != "Hello, user!" {
		t.Errorf("Expected 'Hello, user!', got %q", result)
	}
}

// TestRunLoop_ToolCallsWithinLimit verifies tool calls work when within maxRounds.
func TestRunLoop_ToolCallsWithinLimit(t *testing.T) {
	eng := New()

	agent := &BuiltinAgent{
		Config: config.BuiltinAgent{
			Name:          "test-agent",
			Provider:      "openai",
			Model:         "gpt-4",
			MaxToolRounds: 5,
		},
		Provider: &mockProvider{
			events: []llm.StreamEvent{
				// Round 0: tool call
				{Type: "text", Text: "Let me check."},
				{Type: "tool_call", ToolCall: &llm.ToolCall{ID: "tc-1", Name: "test_tool", Arguments: `{}`}},
				{Type: "done"},
				// Round 1: final text
				{Type: "text", Text: "Here is the answer."},
				{Type: "done"},
			},
		},
		Tools: []llm.ToolDef{
			{Name: "test_tool", Description: "A test tool", InputSchema: map[string]interface{}{"type": "object"}},
		},
	}

	var toolExecuted bool
	eng.callTool = func(a *BuiltinAgent, name string, arguments string, execCtx ToolExecutionContext) (string, error) {
		toolExecuted = true
		return "tool result", nil
	}

	deps := &Deps{
		LoadHistory: func(cid string) ([]*model.Message, error) { return nil, nil },
		RecordTrace: func(e *model.TraceEvent) error { return nil },
		SaveMessage: func(m *model.Message) error { return nil },
	}

	rec := httptest.NewRecorder()
	flusher := &mockFlusher{recorder: rec}

	result, err := eng.runLoop(context.Background(), agent, []llm.ChatMessage{{Role: "user", Content: "hi"}}, rec, flusher, "task-1", "ctx-1", "ctx-1", "", deps)
	if err != nil {
		t.Fatalf("runLoop failed: %v", err)
	}

	if !toolExecuted {
		t.Error("Tool should have been executed")
	}
	if result != "Here is the answer." {
		t.Errorf("Expected 'Here is the answer.', got %q", result)
	}
}

func TestRunLoop_PassesRootAndParentToTool(t *testing.T) {
	eng := New()
	agent := &BuiltinAgent{
		Config: config.BuiltinAgent{
			Name:          "mi-1",
			Provider:      "openai",
			Model:         "gpt-4",
			MaxToolRounds: 5,
		},
		Provider: &mockProvider{
			events: []llm.StreamEvent{
				{Type: "tool_call", ToolCall: &llm.ToolCall{ID: "tool-1", Name: "send_to_agent", Arguments: `{}`}},
				{Type: "done"},
				{Type: "text", Text: "done"},
				{Type: "done"},
			},
		},
		Tools: []llm.ToolDef{
			{Name: "send_to_agent", Description: "Send", InputSchema: map[string]interface{}{"type": "object"}},
		},
	}

	var got ToolExecutionContext
	eng.callTool = func(a *BuiltinAgent, name string, arguments string, execCtx ToolExecutionContext) (string, error) {
		got = execCtx
		return "ok", nil
	}

	deps := &Deps{
		LoadHistory: func(cid string) ([]*model.Message, error) { return nil, nil },
		RecordTrace: func(e *model.TraceEvent) error { return nil },
		SaveMessage: func(m *model.Message) error { return nil },
	}
	rec := httptest.NewRecorder()
	flusher := &mockFlusher{recorder: rec}

	if _, err := eng.runLoop(context.Background(), agent, []llm.ChatMessage{{Role: "user", Content: "hi"}}, rec, flusher, "task-1", "ctx-1", "root-1", "group-1", deps); err != nil {
		t.Fatalf("runLoop failed: %v", err)
	}
	if got.SourceAgent != "mi-1" || got.RootContextId != "root-1" || got.ParentTaskId != "task-1" || got.ParentToolCallId != "tool-1" || got.GroupId != "group-1" {
		t.Fatalf("tool exec context = %#v", got)
	}
}

func TestRunLoop_EmitsToolProgressDuringLongToolCall(t *testing.T) {
	eng := New()
	originalInterval := toolProgressInterval
	toolProgressInterval = 10 * time.Millisecond
	defer func() { toolProgressInterval = originalInterval }()

	agent := &BuiltinAgent{
		Config: config.BuiltinAgent{
			Name:          "mi-1",
			Provider:      "openai",
			Model:         "gpt-4",
			MaxToolRounds: 5,
		},
		Provider: &mockProvider{
			events: []llm.StreamEvent{
				{Type: "tool_call", ToolCall: &llm.ToolCall{ID: "tool-1", Name: "send_to_agent", Arguments: `{"agent":"mi-2","message":"hi"}`}},
				{Type: "done"},
				{Type: "text", Text: "done"},
				{Type: "done"},
			},
		},
		Tools: []llm.ToolDef{
			{Name: "send_to_agent", Description: "Send", InputSchema: map[string]interface{}{"type": "object"}},
		},
	}

	eng.callTool = func(a *BuiltinAgent, name string, arguments string, execCtx ToolExecutionContext) (string, error) {
		time.Sleep(35 * time.Millisecond)
		return "ok", nil
	}

	deps := &Deps{
		LoadHistory: func(cid string) ([]*model.Message, error) { return nil, nil },
		RecordTrace: func(e *model.TraceEvent) error { return nil },
		SaveMessage: func(m *model.Message) error { return nil },
	}
	rec := httptest.NewRecorder()
	flusher := &mockFlusher{recorder: rec}

	result, err := eng.runLoop(context.Background(), agent, []llm.ChatMessage{{Role: "user", Content: "hi"}}, rec, flusher, "task-1", "ctx-1", "root-1", "group-1", deps)
	if err != nil {
		t.Fatalf("runLoop failed: %v", err)
	}
	if result != "done" {
		t.Fatalf("result = %q, want done", result)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "event: tool.progress") {
		t.Fatalf("expected tool.progress in SSE, got: %s", body)
	}
	if !strings.Contains(body, "event: tool.result") {
		t.Fatalf("expected tool.result in SSE, got: %s", body)
	}
}

func TestRunLoop_AppendsA2AToolGuidance(t *testing.T) {
	provider := &mockProvider{
		events: []llm.StreamEvent{
			{Type: "text", Text: "ok"},
			{Type: "done"},
		},
	}
	eng := New()
	agent := &BuiltinAgent{
		Config: config.BuiltinAgent{
			Name:          "mi-1",
			Provider:      "openai",
			Model:         "gpt-4",
			SystemPrompt:  "You are helpful.",
			MaxToolRounds: 5,
		},
		Provider: provider,
	}
	deps := &Deps{
		LoadHistory: func(cid string) ([]*model.Message, error) { return nil, nil },
		RecordTrace: func(e *model.TraceEvent) error { return nil },
		SaveMessage: func(m *model.Message) error { return nil },
	}
	rec := httptest.NewRecorder()
	flusher := &mockFlusher{recorder: rec}

	if _, err := eng.runLoop(context.Background(), agent, []llm.ChatMessage{{Role: "user", Content: "hi"}}, rec, flusher, "task-1", "ctx-1", "root-1", "", deps); err != nil {
		t.Fatalf("runLoop failed: %v", err)
	}
	if len(provider.requests) != 1 {
		t.Fatalf("requests len = %d, want 1", len(provider.requests))
	}
	prompt := provider.requests[0].SystemPrompt
	if !strings.Contains(prompt, "A2A collaboration tool policy") || !strings.Contains(prompt, "call list_groups") {
		t.Fatalf("system prompt missing guidance: %q", prompt)
	}
}

// TestHandleRequest_RecordsAgentResponse verifies that agent responses are persisted.
func TestHandleRequest_RecordsAgentResponse(t *testing.T) {
	eng := New()

	agent := &BuiltinAgent{
		Config: config.BuiltinAgent{
			Name:          "test-agent",
			Provider:      "openai",
			Model:         "gpt-4",
			MaxToolRounds: 5,
		},
		Provider: &mockProvider{
			events: []llm.StreamEvent{
				{Type: "text", Text: "This is the response."},
				{Type: "done"},
			},
		},
	}
	eng.agents["test-agent"] = agent

	var recordedMessages []*model.Message
	deps := &Deps{
		LoadHistory: func(cid string) ([]*model.Message, error) { return nil, nil },
		RecordTrace: func(e *model.TraceEvent) error { return nil },
		SaveMessage: func(m *model.Message) error {
			recordedMessages = append(recordedMessages, m)
			return nil
		},
	}

	rec := httptest.NewRecorder()

	eng.HandleRequest(context.Background(), rec, "test-agent", "hello", "ctx-1", "ctx-1", "task-1", "", deps)

	// Verify that the assistant message was persisted
	if len(recordedMessages) != 1 {
		t.Errorf("Expected 1 recorded message, got %d", len(recordedMessages))
	} else {
		m := recordedMessages[0]
		if m.Role != "agent" {
			t.Errorf("Expected role 'agent', got %q", m.Role)
		}
		if m.Content != "This is the response." {
			t.Errorf("Expected content 'This is the response.', got %q", m.Content)
		}
		if m.TaskId != "task-1" {
			t.Errorf("Expected taskId 'task-1', got %q", m.TaskId)
		}
		if m.ContextId == nil || *m.ContextId != "ctx-1" {
			t.Errorf("Expected contextId 'ctx-1', got %v", m.ContextId)
		}
	}

	// Check that response was written to SSE
	body := rec.Body.String()
	if !strings.Contains(body, "This is the response.") {
		t.Errorf("Expected response text in SSE, got: %s", body)
	}
}

// mockFlusher implements http.Flusher for testing.
type mockFlusher struct {
	recorder *httptest.ResponseRecorder
}

func (m *mockFlusher) Flush() {
	// No-op for test; httptest.ResponseRecorder handles buffering.
}

// TestWriteSSE_JSONSerialization verifies SSE JSON serialization.
func TestWriteSSE_JSONSerialization(t *testing.T) {
	rec := httptest.NewRecorder()
	flusher := &mockFlusher{recorder: rec}

	writeSSE(rec, flusher, "test.event", map[string]string{"key": "value"})

	body := rec.Body.String()
	if !strings.Contains(body, "event: test.event") {
		t.Errorf("Expected event header, got: %s", body)
	}
	if !strings.Contains(body, `"key":"value"`) {
		t.Errorf("Expected JSON data, got: %s", body)
	}
}

// TestWriteSSE_InvalidData verifies behavior with unserializable data.
func TestWriteSSE_InvalidData(t *testing.T) {
	rec := httptest.NewRecorder()
	flusher := &mockFlusher{recorder: rec}

	// This should not panic even with invalid data
	// Currently json.Marshal ignores errors, which is a bug
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("writeSSE panicked with invalid data: %v", r)
		}
	}()

	// Channels cannot be JSON serialized
	writeSSE(rec, flusher, "bad.event", map[string]interface{}{"ch": make(chan int)})
}

// TestRunLoop_ReasoningEvents_EmitsThinkingDelta verifies that reasoning events
// from the LLM are forwarded as thinking.delta SSE events and persisted.
func TestRunLoop_ReasoningEvents_EmitsThinkingDelta(t *testing.T) {
	eng := New()

	agent := &BuiltinAgent{
		Config: config.BuiltinAgent{
			Name:          "test-agent",
			Provider:      "openai",
			Model:         "gpt-4",
			MaxToolRounds: 5,
		},
		Provider: &mockProvider{
			events: []llm.StreamEvent{
				{Type: "reasoning", Reasoning: "Let me think..."},
				{Type: "text", Text: "The answer is 42."},
				{Type: "done"},
			},
		},
	}

	var savedMsg *model.Message
	deps := &Deps{
		LoadHistory: func(cid string) ([]*model.Message, error) { return nil, nil },
		RecordTrace: func(e *model.TraceEvent) error { return nil },
		SaveMessage: func(m *model.Message) error {
			savedMsg = m
			return nil
		},
	}

	rec := httptest.NewRecorder()
	flusher := &mockFlusher{recorder: rec}

	result, err := eng.runLoop(context.Background(), agent, []llm.ChatMessage{{Role: "user", Content: "hi"}}, rec, flusher, "task-1", "ctx-1", "ctx-1", "", deps)
	if err != nil {
		t.Fatalf("runLoop failed: %v", err)
	}

	if result != "The answer is 42." {
		t.Errorf("Expected 'The answer is 42.', got %q", result)
	}

	// Verify SSE contains thinking.delta event
	body := rec.Body.String()
	if !strings.Contains(body, "event: thinking.delta") {
		t.Errorf("Expected thinking.delta event in SSE, got: %s", body)
	}
	if !strings.Contains(body, "Let me think...") {
		t.Errorf("Expected reasoning content in SSE, got: %s", body)
	}

	// Verify saved message includes reasoning content
	if savedMsg == nil {
		t.Fatal("Expected message to be saved")
	}
	if savedMsg.ReasoningContent == nil || *savedMsg.ReasoningContent != "Let me think..." {
		t.Errorf("Expected reasoning content 'Let me think...', got %v", savedMsg.ReasoningContent)
	}
}
