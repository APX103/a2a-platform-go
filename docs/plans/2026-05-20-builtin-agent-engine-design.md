# Built-in Agent Engine Design

Date: 2026-05-20

## Problem

The platform currently only acts as a router — agents must exist as external HTTP services (via a2a-bridge or custom A2A implementations). This forces users to deploy and maintain separate bridge processes just to connect an LLM.

## Solution

Add a Built-in Agent Engine that runs inside the platform process. It handles LLM communication, multi-turn context, and MCP tool calling — making the platform capable of hosting agents directly.

## Architecture

```
                     ┌─────────────────────────────┐
                     │      AgentProxyHandler       │
                     │    POST /agent/{name}        │
                     └──────────┬──────────────────┘
                                │
                    ┌───────────┴───────────┐
                    ▼                       ▼
            type="external"           type="builtin"
           (existing HTTP proxy)     (new in-process engine)
                    │                       │
                    ▼                       ▼
          HTTP → Bridge/A2A      BuiltinAgentEngine
                                    │
                          ┌─────────┼─────────┐
                          ▼         ▼         ▼
                      LLM Call   History   MCP Client
                    (OpenAI/     (context    (stdio/SSE
                    Anthropic)   memory)     servers)
```

Key: `AgentProxyHandler` checks agent type — external goes HTTP, builtin goes in-process. No changes to external agent flow.

## LLM Provider Interface

```go
type Provider interface {
    ChatStream(ctx context.Context, req *ChatRequest) (<-chan StreamEvent, error)
}

type ChatRequest struct {
    Model, SystemPrompt string
    Messages            []ChatMessage
    Tools               []ToolDefinition
    MaxTokens           int
}

type StreamEvent struct {
    Type     string  // "text" | "tool_call" | "done" | "error"
    Text     string
    ToolCall *ToolCall
}
```

Two implementations:
- `openai.Provider` — `/v1/chat/completions` (stream), covers DeepSeek/vLLM/Ollama
- `anthropic.Provider` — `/v1/messages` (stream), native Anthropic protocol

## MCP Client

Each builtin agent can have multiple MCP servers. On startup:
1. Connect via stdio (spawn subprocess) or SSE (remote URL)
2. Call `tools/list` to collect tool definitions
3. Convert to LLM tool format and include in ChatRequest

## Tool Execution Loop

```
User message → [system prompt + history + tools] → LLM
    ↓
LLM response:
  ├─ text → return to user, done
  └─ tool_call → execute MCP tools/call
                    ↓
               tool result → append to messages → call LLM again
                    ↓
               loop until text response (max 10 rounds)
```

## Configuration

```yaml
builtin_agents:
  - name: gpt-4o
    provider: openai
    base_url: https://api.openai.com/v1
    api_key: ${OPENAI_API_KEY}
    model: gpt-4o
    description: "GPT-4o general assistant"
    system_prompt: "You are a helpful assistant."
    max_tokens: 4096
    max_tool_rounds: 10
    mcp_servers:
      - name: web-search
        transport: sse
        url: http://localhost:3100/sse
      - name: sqlite
        transport: stdio
        command: npx
        args: ["-y", "@mcp/sqlite", "./data.db"]
```

Supports `${ENV_VAR}` expansion for API keys.

## API

| Method | Path | Description |
|--------|------|-------------|
| GET | /api/builtin-agents | List all (api_key hidden) |
| POST | /api/builtin-agents | Create (admin token required) |
| PUT | /api/builtin-agents/{name} | Update config |
| DELETE | /api/builtin-agents/{name} | Remove |

Builtin agents auto-register in AgentRegistry (type=builtin) and appear in `/api/agents`.

## Multi-turn Memory

Uses existing `context_id` mechanism. Same context_id → load message history from DB → prepend to LLM request. No new tables needed.

## Implementation Order

1. Config structure + builtin agent model
2. LLM Provider interface + OpenAI implementation
3. Anthropic provider implementation
4. BuiltinAgentEngine (orchestration + tool loop)
5. MCP Client (stdio + SSE transport)
6. AgentProxyHandler routing (type check)
7. Builtin agent CRUD API
8. Admin UI integration
