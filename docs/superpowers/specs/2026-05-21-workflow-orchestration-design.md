# Workflow Orchestration Design

## Overview

Add a visual workflow orchestration module to the A2A platform. Users define multi-agent workflows as DAGs in YAML, edit them visually with a React Flow canvas, and execute them via a built-in Go engine. The system supports four trigger types (chat, API, schedule, event) and four node types (agent, tool, control flow, human).

## Core Decisions

| Decision | Choice | Rationale |
|----------|--------|-----------|
| Architecture | Built-in Go engine (monolith) | Consistent with current single-binary deployment |
| Editor interaction | Auto-layout (dagre) + draggable fine-tuning | Clean defaults with user flexibility |
| Storage | Database (SQLite/MySQL) + YAML import/export | Consistent with existing agent storage; files for version control |
| Data flow between nodes | Shared state (global key-value) | Flexible, supports branching and parallel patterns |
| Agent context model | Each agent node runs in its own contextId | No cross-contamination of system prompts; structured data transfer only |

## YAML Schema

```yaml
workflows:
  customer-support:
    description: "Customer support workflow"

    input_schema:
      type: object
      properties:
        user_message: { type: string }
        priority: { type: string, enum: [low, medium, high] }

    shared_state:
      classification: null
      response: null
      approved: false

    nodes:
      classify:
        type: agent
        agent_name: classifier
        instruction: "Classify the user message"
        inputs: ["{{.input.user_message}}"]
        outputs:
          classification: "{{.result}}"
        timeout: 30s
        retry: { max: 2, backoff: exponential }

      route:
        type: conditional
        expression: "{{.state.classification}}"
        routes:
          BUG: handle_bug
          SUPPORT: handle_support
          default: general_response

      handle_bug:
        type: agent
        agent_name: bug-agent
        instruction: "Handle bug report classified as: {{.state.classification}}"
        inputs: ["{{.state.classification}}"]
        outputs:
          response: "{{.result}}"

      handle_support:
        type: parallel
        nodes: [lookup_order, check_faq]
        timeout: 60s

      lookup_order:
        type: tool
        tool_name: http_request
        config:
          url: "https://api.example.com/orders/{{.input.order_id}}"
          method: GET
        outputs:
          order_info: "{{.result.body}}"

      check_faq:
        type: agent
        agent_name: faq-agent
        outputs:
          faq_answer: "{{.result}}"

      human_review:
        type: human
        prompt: "Review this response: {{.state.response}}"
        timeout: 30m
        outputs:
          approved: "{{.result.approved}}"

      send_response:
        type: agent
        agent_name: responder
        instruction: "Send final response based on review result"

      general_response:
        type: agent
        agent_name: general-agent
        outputs:
          response: "{{.result}}"

    edges:
      - [START, classify]
      - [classify, route]
      - [route, handle_bug, { condition: BUG }]
      - [route, handle_support, { condition: SUPPORT }]
      - [route, general_response, { condition: default }]
      - [handle_bug, human_review]
      - [handle_support, send_response]
      - [human_review, send_response, { condition: approved }]
      - [send_response, END]
      - [general_response, END]

    triggers:
      - type: api
      - type: chat
        agent_name: support-wf
      - type: schedule
        cron: "0 9 * * 1-5"
        input:
          report_type: "daily_summary"
      - type: event
        event_type: "task.completed"
        filter: "agent_name == 'bug-agent'"
```

### Node Types

| Type | Required Fields | Description |
|------|----------------|-------------|
| `agent` | `agent_name`, `instruction` | Calls a builtin agent via `Engine.HandleRequest()` |
| `tool` | `tool_name`, `config` | Executes a deterministic tool (HTTP request, function, etc.) |
| `conditional` | `expression`, `routes` | Evaluates expression, routes to matching branch |
| `parallel` | `nodes` | Runs child nodes concurrently, waits for all |
| `loop` | `nodes`, `condition`, `max_iterations` | Iterates child nodes until condition met |
| `human` | `prompt`, `timeout` | Pauses execution, waits for human input/approval |

### Template Expressions

All `inputs`, `instruction`, and `prompt` fields support Go template syntax:

- `{{.input.xxx}}` — workflow run input
- `{{.state.xxx}}` — shared state value
- `{{.result}}` — current node's raw execution result
- `{{.state}}` — entire shared state object (for pass_full_state: true)

## Database Schema

```sql
CREATE TABLE workflows (
    id          VARCHAR(64) PRIMARY KEY,
    name        VARCHAR(255) NOT NULL,
    description TEXT,
    yaml_def    TEXT NOT NULL,
    version     INT DEFAULT 1,
    enabled     BOOLEAN DEFAULT TRUE,
    created_at  DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at  DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE workflow_runs (
    id           VARCHAR(36) PRIMARY KEY,
    workflow_id  VARCHAR(64) NOT NULL,
    status       VARCHAR(20) DEFAULT 'pending',
    input_json   TEXT,
    state_json   TEXT,
    context_id   VARCHAR(36),
    trigger_type VARCHAR(20),
    started_at   DATETIME,
    completed_at DATETIME,
    error        TEXT,
    FOREIGN KEY (workflow_id) REFERENCES workflows(id) ON DELETE CASCADE
);

CREATE TABLE workflow_node_runs (
    id           VARCHAR(36) PRIMARY KEY,
    run_id       VARCHAR(36) NOT NULL,
    node_id      VARCHAR(64) NOT NULL,
    node_type    VARCHAR(20) NOT NULL,
    status       VARCHAR(20) DEFAULT 'pending',
    input_json   TEXT,
    output_json  TEXT,
    started_at   DATETIME,
    completed_at DATETIME,
    error        TEXT,
    FOREIGN KEY (run_id) REFERENCES workflow_runs(id) ON DELETE CASCADE
);

CREATE INDEX idx_workflow_runs_workflow ON workflow_runs(workflow_id);
CREATE INDEX idx_workflow_node_runs_run ON workflow_node_runs(run_id);
```

## Execution Engine

### Architecture

```
internal/workflow/
  engine.go        — WorkflowEngine: parse, validate, execute
  dag.go           — DAG construction and topological sort
  executor.go      — Node executor dispatch
  nodes/
    agent.go       — Agent node: calls Engine.HandleRequest()
    tool.go        — Tool node: executes HTTP/function calls
    conditional.go — Conditional node: evaluates expression, routes
    parallel.go    — Parallel node: concurrent goroutine execution
    loop.go        — Loop node: iterates until condition
    human.go       — Human node: pauses, waits for resume
  state.go         — Shared state management (read/write/render templates)
  store.go         — Database persistence for workflows, runs, node_runs
```

### Execution Flow

1. **Parse**: YAML -> DAG (nodes + edges). Validate acyclic, all edges reference existing nodes, exactly one START and one END.
2. **Initialize**: Create `workflow_run` record. Initialize shared state from `shared_state` defaults merged with run `input`. Compute in-degree for each node. Queue nodes with zero in-degree.
3. **Execute loop**:
   - Dequeue ready nodes.
   - For each node: render templates against current shared state, dispatch to executor by type.
   - On completion: merge node outputs into shared state, update `workflow_runs.state_json`, decrement in-degree of downstream nodes, queue newly ready nodes.
   - Broadcast node status via `events.Broadcaster` for real-time frontend updates.
   - Repeat until END node reached or error.
4. **Complete**: Update run status to `completed` or `failed`. Record final state and duration.

### Shared State Model

- Stored as JSON in `workflow_runs.state_json`.
- Initialized from YAML `shared_state` defaults + run `input`.
- Each node's `outputs` are merged in after execution.
- Nodes read state via Go template rendering: `{{.state.classification}}`.
- Each agent node runs in its own `contextId` — agents do not share conversation history, only structured data flows through the workflow state.

### Error Handling

Each node supports optional configuration:

```yaml
retry:
  max: 2
  backoff: exponential    # fixed | exponential
  interval: 5s
timeout: 30s
on_error: fail            # fail | skip | fallback
fallback: error_handler_node_id
```

- **fail**: Mark the entire workflow run as failed.
- **skip**: Mark node as skipped, proceed to downstream nodes (empty output).
- **fallback**: Route to an alternative node instead.

Human nodes that exceed `timeout` are marked `timeout` and the workflow is paused (can be resumed or cancelled).

## Frontend

### Pages

| Route | Page | Description |
|-------|------|-------------|
| `/workflows` | WorkflowList | Card/table list of all workflows |
| `/workflows/:id` | WorkflowEditor | Visual editor + YAML dual mode |
| `/workflows/:id/runs` | RunList | Execution history |
| `/workflows/:id/runs/:runId` | RunDetail | Real-time execution monitor |

### Tech Stack

| Component | Library | Notes |
|-----------|---------|-------|
| Canvas | `@xyflow/react` (React Flow) | Same as Hector |
| Auto-layout | `@dagrejs/dagre` | Top-to-bottom DAG layout |
| YAML editor | `@monaco-editor/react` + `monaco-yaml` | Schema validation |
| YAML parse | `js-yaml` | Bidirectional conversion |
| State | Zustand (existing) | New workflow slice |

### Editor Layout

```
┌──────────────────────────────────────────────────────┐
│ Toolbar: [Save] [Run] [Import] [Export] [Code/Canvas]│
├──────────┬─────────────────────────┬─────────────────┤
│ Node     │                         │ Properties      │
│ Palette  │   React Flow Canvas     │ Panel           │
│          │                         │                 │
│ · Agent  │   ┌──────┐  ┌──────┐   │ Name: classify  │
│ · Tool   │   │classify│→│ route │   │ Type: agent    │
│ · Cond.  │   └──────┘  └──────┘   │ Agent: xxx     │
│ · Par.   │        ↓       ↓       │ Instruction:   │
│ · Loop   │   ┌──────┐ ┌──────┐   │ [textarea]     │
│ · Human  │   │handle │ │handle│   │                │
│          │   │_bug   │ │_sup  │   │ Inputs:        │
│ Drag to  │   └──────┘ └──────┘   │ · state.xxx    │
│ canvas   │                         │                │
├──────────┴─────────────────────────┴─────────────────┤
│ YAML Editor (Monaco, collapsible or full-code mode)   │
└──────────────────────────────────────────────────────┘
```

### Node Visual Design

| Type | Color | Icon | Details |
|------|-------|------|---------|
| Agent | Blue | Bot | Agent name, pulse animation when running |
| Tool | Orange | Wrench | Tool name, config preview |
| Conditional | Purple | GitBranch | Condition text, branch labels on edges |
| Parallel | Teal | Layers | Container, children stacked vertically |
| Loop | Emerald | Repeat | Container, max iterations badge |
| Human | Pink | UserCheck | Prompt preview, timeout badge |
| START/END | Gray | Play/Flag | Fixed position nodes |

### Bidirectional Sync (Canvas <-> YAML)

- Canvas edits: serialize nodes/edges to YAML via `graphToYaml()`.
- YAML edits: parse and regenerate nodes/edges via `yamlToGraph()`.
- Dagre auto-layout by default; user can drag nodes to adjust positions.
- Store node positions in node data; on YAML reload, restore saved positions.
- Debounce (300ms) to prevent render loops. Use a `syncedYamlRef` to break circular updates.

### Run Monitor

- Run detail page renders the workflow canvas with per-node status overlays.
- Nodes show: pending (gray), running (blue spinner), completed (green check), failed (red X), skipped (gray dash).
- Click a node to inspect its input/output/error in a side panel.
- Human nodes in `paused` state show an approval dialog inline.
- Real-time updates via SSE events from `events.Broadcaster`.

## API

### Workflow CRUD

```
GET    /api/workflows              — List all workflows
GET    /api/workflows/:id          — Get workflow (with YAML)
POST   /api/workflows              — Create workflow
PUT    /api/workflows/:id          — Update workflow
DELETE /api/workflows/:id          — Delete workflow
POST   /api/workflows/import       — Import YAML file
GET    /api/workflows/:id/export   — Export as YAML file
```

### Run Management

```
POST   /api/workflows/:id/run                    — Trigger execution
GET    /api/workflows/:id/runs                   — Run history (paginated)
GET    /api/workflows/:id/runs/:runId            — Run detail (with node statuses)
POST   /api/workflows/:id/runs/:runId/cancel     — Cancel running workflow
POST   /api/workflows/:id/runs/:runId/resume     — Resume paused workflow (e.g., after human approval)
```

### Chat Trigger

Workflows with `triggers: [{ type: chat, agent_name: "support-wf" }]` register as virtual agents. When the existing `/agent/:name` proxy receives a request for that name:

1. Handler checks WorkflowStore before Builtin/Bridge/External registries.
2. Extracts user message from the JSON-RPC body.
3. Creates a workflow run with the message as input.
4. Streams the final output back as SSE text events.

### Schedule Trigger

On startup, `WorkflowEngine` parses all enabled workflows' schedule triggers and registers them with a cron scheduler (e.g., `github.com/robfig/cron`). Each tick creates a workflow run with the configured `input`.

### Event Trigger

The engine subscribes to `events.Broadcaster`, matches incoming events against configured `event_type` and `filter` expressions, and triggers a run when matched.

## Integration Points

### Existing Infrastructure Reuse

| Component | Reuse |
|-----------|-------|
| `Engine.HandleRequest()` | Agent nodes call this directly for builtin agents |
| `send_to_agent` tool logic | For invoking external/bridge agents as workflow nodes |
| `events.Broadcaster` | Real-time workflow/node status updates to frontend |
| `svc/` store pattern | Database persistence following existing patterns |
| `cmd/server/main.go` routing | New routes following existing `pathTail()` pattern |
| `model/types.go` | New model types following existing patterns |

### New Go Packages

```
internal/workflow/
  engine.go
  dag.go
  executor.go
  state.go
  store.go
  nodes/
    agent.go
    tool.go
    conditional.go
    parallel.go
    loop.go
    human.go

internal/handler/workflow_handler.go
```

### New Frontend Modules

```
web/admin/src/
  pages/WorkflowList.tsx
  pages/WorkflowEditor.tsx
  pages/WorkflowRunDetail.tsx
  components/WorkflowCanvas/
    CanvasMode.tsx
    nodes/
      AgentNode.tsx
      ToolNode.tsx
      ConditionalNode.tsx
      ParallelGroupNode.tsx
      LoopGroupNode.tsx
      HumanNode.tsx
    yaml-converter.ts
  stores/workflowStore.ts
  api/workflowClient.ts
```

### New Dependencies

**Go:**
- `github.com/robfig/cron/v3` — Schedule triggers

**Frontend:**
- `@xyflow/react` — React Flow canvas
- `@dagrejs/dagre` — Auto-layout
- `@monaco-editor/react` — YAML editor
- `monaco-yaml` — YAML schema validation
- `js-yaml` — YAML parse/serialize
