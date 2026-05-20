# Chat Interface and Context Management Design

**Date:** 2026-05-20
**Status:** Design Complete
**Author:** Claude Code

---

## 1. Overview

Add an in-platform chat interface for interacting with agents directly, with support for:
- Streaming responses with typewriter effect
- Markdown rendering
- Thinking process visualization (collapsible, time-blocked)
- Tool call display with chain relationships
- Context/session management (list, view, delete, continue)
- Subagent isolation and spawning

---

## 2. Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                         Frontend (React)                         │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐           │
│  │ ChatPage     │  │ ContextPanel │  │ SSEClient    │           │
│  └──────┬───────┘  └──────┬───────┘  └──────┬───────┘           │
│         └──────────────────┴──────────────────┘                   │
└────────────────────────────┼───────────────────────────────────────┘
                             │
                    ┌────────▼────────┐
                    │  REST API + SSE │
                    └────────┬────────┘
                             │
        ┌────────────────────┼────────────────────┐
        │                    │                    │
┌───────▼────────┐  ┌────────▼────────┐  ┌──────▼──────────────┐
│  BuiltinAgent  │  │  ContextStore   │  │  SubagentEngine     │
│  Engine        │  │  (DB: contexts) │  │  (DB: subagent_     │
└────────────────┘  └─────────────────┘  │   sessions)         │
                                             └─────────────────────┘
```

---

## 3. Database Schema

### 3.1 `messages` Table (Extended)

```sql
ALTER TABLE messages
  ADD COLUMN reasoning_content TEXT,
  ADD COLUMN tool_calls JSON,
  ADD COLUMN tool_call_id VARCHAR(64),
  ADD COLUMN thinking_blocks JSON;
```

**thinking_blocks format:**
```json
[
  {"id": "t1", "timestamp": 1747788800, "content": "Analyzing..."},
  {"id": "t2", "timestamp": 1747788860, "content": "Decision made"}
]
```

### 3.2 `contexts` Table (New)

```sql
CREATE TABLE contexts (
  id VARCHAR(36) PRIMARY KEY,
  agent_name VARCHAR(128) NOT NULL,
  title VARCHAR(256),
  message_count INT DEFAULT 0,
  created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
  INDEX idx_agent (agent_name)
);
```

### 3.3 `subagent_sessions` Table (New)

```sql
CREATE TABLE subagent_sessions (
  id VARCHAR(36) PRIMARY KEY,
  parent_context_id VARCHAR(36),
  parent_tool_call_id VARCHAR(64),
  task TEXT,
  context TEXT,
  status VARCHAR(16),  -- running, completed, failed
  messages JSON,
  result TEXT,
  error TEXT,
  created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
  completed_at TIMESTAMP,
  INDEX idx_parent (parent_context_id)
);
```

---

## 4. Frontend Components

### 4.1 ChatPage

**Route:** `/chat/:agentName?contextId=:id`

**Features:**
- Timeline layout for messages
- Input box with send button
- Agent info header
- Context switcher (dropdown or sidebar)
- SSE connection for streaming

### 4.2 Message Timeline

**Structure:**
```
│ ─── (vertical timeline line)
│
├─ User Message (blue, left-aligned)
│   └─ content
│
├─ Thinking Block (yellow, collapsible)
│   └─ [T1 +0.2s] thinking content
│   └─ [T2 +0.6s] decision made
│
├─ Tool Call (purple, card)
│   └─ tool_name + parameters
│   └─ result
│
├─ Assistant Message (green, right-aligned)
│   └─ markdown content
```

### 4.3 Components List

| Component | Description |
|-----------|-------------|
| `ChatPage.tsx` | Main chat page container |
| `MessageTimeline.tsx` | Vertical timeline renderer |
| `ThinkingBlock.tsx` | Collapsible thinking content |
| `ToolCallCard.tsx` | Tool invocation display |
| `MarkdownRenderer.tsx` | Markdown + code highlighting |
| `SSEClient.tsx` | SSE event handler |
| `ContextPanel.tsx` | Session list sidebar |
| `InputBox.tsx` | Message input + send |

---

## 5. Backend API (New)

| Method | Path | Description | Auth |
|--------|------|-------------|------|
| GET | `/api/contexts/:agentName` | List agent contexts | - |
| GET | `/api/contexts/:id` | Get context details | - |
| DELETE | `/api/contexts/:id` | Delete context | token |
| GET | `/api/messages/by-context/:contextId` | Get messages by context | - |
| GET | `/api/subagents/:contextId` | List subagents | - |
| GET | `/api/subagents/:id` | Get subagent details | - |
| POST | `/agent/:name` | Send message (SSE) | - |

---

## 6. SSE Events (Extended)

| Event | Description |
|-------|-------------|
| `text.delta` | Text streaming (existing) |
| `thinking.delta` | Thinking content streaming |
| `thinking.block` | Thinking time block |
| `tool.call_start` | Tool call start (with params) |
| `tool.call_delta` | Tool parameter streaming |
| `tool.call_end` | Tool call end |
| `tool.result` | Tool execution result |
| `subagent.started` | Subagent spawned |
| `subagent.completed` | Subagent finished |
| `subagent.error` | Subagent error |

---

## 7. Builtin Tools

Based on `mcp-conversation-engine/backend/src/tools.ts`:

| Tool | Description |
|------|-------------|
| `tool_search` | Search MCP tools by name |
| `read_file` | Read file content |
| `write_file` | Write/create file |
| `edit_file` | Replace string in file |
| `list_directory` | List directory contents |
| `fetch_url` | Make HTTP request |
| `spawn_agent` | Create subagent (new) |

**Note:** `web_search` to be implemented as an abstract interface supporting multiple search APIs.

---

## 8. Subagent System

Based on Kimi CLI architecture + `mcp-conversation-engine/subagent`:

**Features:**
- Isolated context per subagent
- Foreground (sync) and Background (async) modes
- 1-level depth only (no nesting)
- 5-minute timeout
- Session persistence in `subagent_sessions` table

**Workflow:**
1. LLM calls `spawn_agent` with task description
2. System creates subagent with isolated context
3. Subagent executes and returns result
4. Result is fed back to parent agent

---

## 9. Tech Stack

### Frontend
- **Markdown:** `react-markdown` + `remark-gfm`
- **Code highlighting:** `shiki` (faster, tree-shakeable)
- **SSE:** `@microsoft/fetch-event-source` (auto-reconnect)
- **State:** Zustand (lightweight, swappable)

### Backend
- Keep existing OpenAI/Anthropic providers
- Standard `net/http` for HTTP
- Abstract interface for search APIs

---

## 10. Implementation Phases

| Phase | Tasks | Duration |
|-------|-------|----------|
| **Phase 1** | ChatPage + Timeline + SSE + Markdown | 1-2 days |
| **Phase 2** | Thinking collapsible + Tool call display | 1-2 days |
| **Phase 3** | Context table + CRUD API + Session UI | 1-2 days |
| **Phase 4** | Builtin tools + Subagent engine | 2-3 days |

**Total:** ~5-9 days

---

## 11. Data Flow

```
User sends message
    ↓
POST /agent/:name (with contextId if existing)
    ↓
Engine runs LLM loop
    ↓
Stream events to frontend (SSE)
    ↓
Frontend renders:
    - User message
    - Thinking blocks (collapsible)
    - Tool calls
    - Assistant response (markdown)
    ↓
Store to DB:
    - messages (with reasoning, tool_calls)
    - contexts (update message_count, updated_at)
    - traces (existing)
```

---

## 12. Open Questions

1. **Context title generation:** Auto-generate from first message or user input?
2. **Web search provider:** Which search API to integrate? (Google, Bing, etc.)
3. **Subagent UI:** Should subagent activities be visible in main timeline?
4. **Max message history:** Context window limit per session?