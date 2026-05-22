# ContextId Refactor 方案

## 背景

当前平台里的 `contextId` 同时承担了两个职责：

1. 作为用户和某个 agent 的聊天会话 id，用来加载多轮历史。
2. 作为 task、message、trace 的聚合 id，用来观察一次调用链。

这两个职责在简单场景下看起来可以共用，但在多 agent 协作时会冲突。例如用户在同一个 UI 会话里向 `mi-1` 发出请求：

> 你让 mi-2 给 mi-3 发消息让 mi-3 讲个笑话给 mi-2，让 mi-2 评价一下，你告诉我它的评价

现状会产生：

- host -> mi-1：沿用 UI 当前 `contextId`，同一个会话下可能已有多轮 task。
- mi-1 -> mi-2：`send_to_agent` 没有传 `contextId`，平台代理层自动生成新的 `contextId`。
- mi-2 -> mi-3：同样自动生成另一个新的 `contextId`。

从 trace 视角看，一次用户协作被拆成多个 context；但如果简单地把 host -> mi-1 的 `contextId` 原样传给 mi-2/mi-3，又会让下游 agent 加载到不属于自己的历史消息，造成提示上下文污染。

## 目标

本次 refactor 的目标是引入“整次协作的根上下文”，同时保留每个 agent 自己的会话能力。

核心问题拆成三个独立概念：

| 概念 | 字段 | 回答的问题 | 用途 |
| --- | --- | --- | --- |
| 根上下文 | `root_context_id` | 这件事属于哪次用户协作？ | trace/task 聚合、调用链观察、全链路 UI |
| 目标会话 | `context_id` | 当前目标 agent 应该恢复哪段自己的对话？ | agent 多轮历史、下游请求 session |
| 调用父子关系 | `parent_task_id` | 这个 task 是由哪个 task 触发的？ | task tree、trace tree、调试调用路径 |

## 非目标

- 不在第一阶段重写 A2A 协议字段命名。
- 不强制所有外部 agent 理解 `root_context_id`。
- 不把所有 agent 的消息历史混入同一个 LLM prompt。
- 不立即删除现有 `context_id` 查询能力。

## 当前逻辑梳理

### 请求入口

`/agent/{name}` 在 `AgentProxyHandler` 中解析请求：

- 如果请求里有 `params.contextId`，则使用它。
- 如果目标 agent 是 `context` 模式且请求没有 `contextId`，则自动生成一个新的 id 并写回请求。
- 如果目标 agent 是 `stateless` 模式，则移除 `contextId`。

这意味着任何没有显式携带 `contextId` 的 agent-to-agent 调用都会形成新的 context。

### agent-to-agent 工具

`send_to_agent` 当前只传：

- target agent
- message
- `X-A2A-Source-Agent`

它不会传当前 task id、root context，也不会传父 task 关系。

### 历史加载

builtin agent 加载历史时按 `context_id` 全量读取 messages。当前没有按 `source_agent`、`recipient_agent` 或 target agent 过滤。

因此，共用同一个 `context_id` 会导致多个 agent 的消息和 tool result 被一起塞进目标 agent 的 prompt。

## 推荐设计

### 字段语义

#### `root_context_id`

表示一次用户协作或一次顶层工作流的根上下文。

规则：

- 顶层 host -> agent 请求：
  - 如果请求带 `X-A2A-Root-Context-Id`，使用它。
  - 否则如果请求带 `params.rootContextId`，使用它。
  - 否则如果请求有 `params.contextId`，`root_context_id = context_id`。
  - 否则生成新的 `context_id` 后，`root_context_id = context_id`。
- agent -> agent 请求：
  - 必须由平台内部工具通过 header 传播 `X-A2A-Root-Context-Id`。
  - 下游 task、trace 都写入同一个 `root_context_id`。
- stateless target：
  - 仍然不向目标 agent 转发 `contextId`。
  - 但平台自己的 task/trace 仍可记录 `root_context_id`。

#### `context_id`

表示目标 agent 的会话上下文。

规则：

- 继续兼容现有 `params.contextId`。
- 它只用于当前 target agent 的历史加载和下游协议会话。
- 不再承担全链路聚合职责。
- 第一阶段不要把父 agent 的 `context_id` 自动传给子 agent。

后续可以增加 `agent_context_policy`：

| 策略 | 行为 |
| --- | --- |
| `new_per_call` | 每次 agent-to-agent 调用都给目标 agent 新建 `context_id` |
| `reuse_by_edge` | 同一个 `root_context_id + source_agent + target_agent` 复用一个目标会话 |
| `explicit` | 只有工具参数显式传 `contextId` 时才复用 |

第一阶段建议使用 `new_per_call`，因为它最不容易污染下游 agent prompt。

#### `parent_task_id`

表示 task 调用树。

规则：

- 顶层 host 请求：`parent_task_id = NULL`。
- agent 调用 `send_to_agent`：下游 task 的 `parent_task_id` 等于当前 agent task id。
- 如果 task 是由 tool call 触发，可额外记录 `parent_tool_call_id`。

## 数据库变更

### `tasks`

新增字段：

```sql
ALTER TABLE tasks ADD COLUMN root_context_id VARCHAR(64);
ALTER TABLE tasks ADD COLUMN parent_task_id VARCHAR(64);
ALTER TABLE tasks ADD COLUMN parent_tool_call_id VARCHAR(128);
CREATE INDEX idx_tasks_root_context_id ON tasks(root_context_id);
CREATE INDEX idx_tasks_parent_task_id ON tasks(parent_task_id);
```

写入规则：

- 新 task 创建时写入 `root_context_id`。
- 如果未提供 root，则 fallback 到当前 `context_id`。
- agent-to-agent 调用写入 `parent_task_id`。

兼容回填：

```sql
UPDATE tasks
SET root_context_id = context_id
WHERE root_context_id IS NULL;
```

### `traces`

新增字段：

```sql
ALTER TABLE traces ADD COLUMN root_context_id VARCHAR(64);
ALTER TABLE traces ADD COLUMN parent_task_id VARCHAR(64);
CREATE INDEX idx_traces_root_context_id ON traces(root_context_id);
```

写入规则：

- 每条 trace 写当前 task 的 `root_context_id`。
- `parent_task_id` 可冗余写入，便于 trace 页面直接构树。

兼容回填：

```sql
UPDATE traces
SET root_context_id = context_id
WHERE root_context_id IS NULL;
```

### `messages`

第一阶段可以不加字段，因为 messages 仍服务于 target agent 的 `context_id` 历史加载。

如果后续需要做“全链路 transcript”，可以新增：

```sql
ALTER TABLE messages ADD COLUMN root_context_id VARCHAR(64);
CREATE INDEX idx_messages_root_context_id ON messages(root_context_id);
```

但要注意：按 `root_context_id` 展示 transcript 和按 `context_id` 注入 LLM prompt 是两个不同查询，不能混用。

## 请求传播设计

### 平台内部 header

新增内部 header：

| Header | 含义 |
| --- | --- |
| `X-A2A-Root-Context-Id` | 整次协作根上下文 |
| `X-A2A-Parent-Task-Id` | 触发当前请求的父 task |
| `X-A2A-Parent-Tool-Call-Id` | 触发当前请求的 tool call |
| `X-A2A-Source-Agent` | 当前真实发起 agent，已有 |

这些 header 主要由平台内部工具和 bridge 使用。外部用户无需感知。

### JSON-RPC params 兼容

可以支持但不强依赖：

```json
{
  "params": {
    "contextId": "target-agent-session",
    "rootContextId": "root-collaboration-id"
  }
}
```

对外推荐仍以 `contextId` 作为 agent session 字段；`rootContextId` 是平台增强字段。

## 后端改造步骤

### 1. model 增加字段

`model.Task` 增加：

```go
RootContextId    *string `db:"root_context_id" json:"root_context_id,omitempty"`
ParentTaskId     *string `db:"parent_task_id" json:"parent_task_id,omitempty"`
ParentToolCallId *string `db:"parent_tool_call_id" json:"parent_tool_call_id,omitempty"`
```

`model.TraceEvent` 增加：

```go
RootContextId *string `db:"root_context_id" json:"root_context_id,omitempty"`
ParentTaskId  *string `db:"parent_task_id" json:"parent_task_id,omitempty"`
```

### 2. AgentProxyHandler 解析 root 和 parent

新增辅助函数：

```go
func resolveRootContextId(r *http.Request, rpcReq map[string]interface{}, contextId *string) *string
func resolveParentTaskId(r *http.Request) *string
func resolveParentToolCallId(r *http.Request) *string
```

解析优先级：

1. Header。
2. JSON-RPC params。
3. 当前 `contextId`。
4. 空。

注意：`applyContextModeToRPC` 仍只处理目标 agent 的 `contextId`，不要把 root 逻辑塞进去。

### 3. TaskStore 和 TraceStore 写入新字段

更新：

- `TaskStore.Create`
- `TaskStore.Get`
- `TaskStore.ListByFilter`
- `TraceStore.Append`
- `TraceStore.GetByTask`
- `TraceStore.GetByContext`
- `TraceStore.ListRecent`
- `TraceStore.ListContexts`

新增查询：

```go
func (s *TaskStore) ListByRootContext(rootContextId string) ([]*model.Task, error)
func (s *TraceStore) GetByRootContext(rootContextId string) ([]*model.TraceEvent, error)
```

### 4. Engine Deps 传递当前执行上下文

当前 `defaultCallTool` 只给工具传 `_source_agent`。需要继续传：

```go
args["_source_agent"] = agent.Config.Name
args["_root_context_id"] = currentRootContextId
args["_parent_task_id"] = currentTaskId
args["_parent_tool_call_id"] = currentToolCallId
```

为了避免把 task 运行态藏在全局变量里，建议把 `callTool` 签名从：

```go
func(agent *BuiltinAgent, name string, arguments string) (string, error)
```

调整为：

```go
type ToolExecutionContext struct {
    SourceAgent      string
    RootContextId    string
    ParentTaskId     string
    ParentToolCallId string
}

func(agent *BuiltinAgent, name string, arguments string, execCtx ToolExecutionContext) (string, error)
```

测试里 mock `callTool` 也要同步更新。

### 5. `send_to_agent` 传播 root 和 parent

`executeSendToAgent` 读取隐藏参数：

```go
rootContextId, _ := args["_root_context_id"].(string)
parentTaskId, _ := args["_parent_task_id"].(string)
parentToolCallId, _ := args["_parent_tool_call_id"].(string)
```

发请求时设置 header：

```go
if rootContextId != "" {
    req.Header.Set("X-A2A-Root-Context-Id", rootContextId)
}
if parentTaskId != "" {
    req.Header.Set("X-A2A-Parent-Task-Id", parentTaskId)
}
if parentToolCallId != "" {
    req.Header.Set("X-A2A-Parent-Tool-Call-Id", parentToolCallId)
}
```

第一阶段不要默认设置下游 `params.contextId`。

### 6. API 增加 root context 查询

新增接口：

| Method | Path | 用途 |
| --- | --- | --- |
| `GET` | `/api/traces/root/{root_context_id}` | 查询整次协作 trace |
| `GET` | `/api/tasks/root/{root_context_id}` | 查询整次协作 task |
| `GET` | `/api/contexts/{context_id}/collaboration` | 从 UI context 进入完整协作视图 |

`/api/contexts/{context_id}/collaboration` 可以内部等价于按 `root_context_id = context_id` 查询，适合 UI 从当前聊天跳到整链路。

### 7. 前端展示

#### Chat TaskPanel

当前 TaskPanel 主要展示 subagent sessions。建议改成：

- 默认以当前 UI `contextId` 作为 `root_context_id` 查询 task tree。
- 按 `parent_task_id` 构树展示：
  - host -> mi-1
  - mi-1 -> mi-2
  - mi-2 -> mi-3
- 每个节点展示：
  - source agent
  - target agent
  - state
  - task id
  - context id
  - root context id

#### Trace 页面

保留现有按 `context_id` 的视图，但新增“Root Contexts”视图：

- 聚合字段改用 `root_context_id`。
- 点击后展示完整调用树和按时间排序的 trace events。

## 历史加载策略

第一阶段保持简单：

- builtin agent 仍按当前 `context_id` 加载历史。
- agent-to-agent 调用不自动复用父 `context_id`。
- 因此下游 agent 不会读到父 agent 的完整聊天历史。

第二阶段可以引入 edge session：

```text
edge_context_id = hash(root_context_id, source_agent, target_agent)
```

或落表：

```sql
CREATE TABLE agent_context_edges (
    id VARCHAR(64) PRIMARY KEY,
    root_context_id VARCHAR(64) NOT NULL,
    source_agent VARCHAR(128) NOT NULL,
    target_agent VARCHAR(128) NOT NULL,
    context_id VARCHAR(64) NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(root_context_id, source_agent, target_agent)
);
```

这样同一次协作中，mi-1 多次调用 mi-2 时可以复用 mi-1->mi-2 的独立 session，但仍不会混入 host->mi-1 的历史。

## 迁移和兼容策略

### 第一阶段：双写新字段

- 新请求写 `root_context_id`。
- 老接口继续按 `context_id` 工作。
- 老数据回填 `root_context_id = context_id`。
- UI 默认仍可用，不阻断现有聊天。

### 第二阶段：新增 root 视图

- Trace contexts 页面增加 root context 聚合。
- TaskPanel 切到 root task tree。
- 保留按 target `context_id` 的调试入口。

### 第三阶段：优化 agent session 策略

- 增加 `agent_context_edges` 或配置项。
- 支持同一 root 下的 source-target 复用会话。
- 支持工具参数显式指定下游 `contextId`。

## 测试计划

### 单元测试

- `resolveRootContextId`：
  - header 优先。
  - params 次之。
  - fallback 到 `contextId`。
  - stateless target 仍能记录 root。
- `send_to_agent`：
  - 带 root header。
  - 带 parent task header。
  - 不默认带下游 `params.contextId`。
- `TaskStore`：
  - 创建和读取 root/parent 字段。
  - 按 root 查询。
- `TraceStore`：
  - 写入和按 root 查询。

### 集成测试

构造链路：

```text
host -> mi-1 -> mi-2 -> mi-3
```

断言：

- 三个 task 的 `root_context_id` 相同。
- mi-1 task 的 `parent_task_id` 为空。
- mi-2 task 的 `parent_task_id` 等于 mi-1 task id。
- mi-3 task 的 `parent_task_id` 等于 mi-2 task id。
- mi-2 和 mi-3 的 `context_id` 可以不同于 root。
- `/api/traces/root/{root_context_id}` 能返回完整 trace。

### 回归测试

- 普通单 agent 多轮聊天不变。
- `context_mode=stateless` 仍不向目标 agent 转发 `contextId`。
- 老的 `/api/traces/context/{context_id}` 仍可用。
- 老的 `/api/tasks?context_id=...` 仍可用。

## 风险

### 字段语义混淆

风险：开发时继续把 `context_id` 当成全链路聚合字段。

缓解：

- 新增注释和 README 文档。
- 新 API 使用 `root` 命名。
- 前端文案区分 "Session Context" 和 "Root Context"。

### 历史上下文污染

风险：为了聚合 trace，把 root 误传成下游 `contextId`。

缓解：

- `send_to_agent` 第一阶段只传 root header，不传下游 `params.contextId`。
- builtin agent 历史加载只读 `context_id`。

### 老数据展示不完整

风险：历史数据没有 parent 关系，只能按时间线展示。

缓解：

- 回填 root。
- parent 为空时 UI 退化为时间线。

## 建议实施顺序

1. DB schema + model/store 双写。
2. handler 解析 root/parent 并写 task/trace。
3. engine 工具执行上下文改造。
4. `send_to_agent` 传播 root/parent header。
5. 新增 root task/trace API。
6. 前端 TaskPanel/Trace 页面改用 root 聚合。
7. 文档和测试补齐。

## 最终效果

对于一次用户请求：

```text
host -> mi-1 -> mi-2 -> mi-3
```

数据应类似：

| task | source | target | context_id | root_context_id | parent_task_id |
| --- | --- | --- | --- | --- | --- |
| t1 | host | mi-1 | ctx-user-mi1 | ctx-user-mi1 | NULL |
| t2 | mi-1 | mi-2 | ctx-mi1-mi2-call1 | ctx-user-mi1 | t1 |
| t3 | mi-2 | mi-3 | ctx-mi2-mi3-call1 | ctx-user-mi1 | t2 |

这样：

- 看 mi-1 的多轮聊天：查 `context_id = ctx-user-mi1`。
- 看整次协作链路：查 `root_context_id = ctx-user-mi1`。
- 看调用树：按 `parent_task_id` 构树。

这个模型能同时满足“整次协作的根上下文”和“各 agent 自己的 session 隔离”。
