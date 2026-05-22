# Bridge 实现指导手册

这份手册说明如何把已有 Agent 接入 A2A Platform。这里的 Bridge 指一个适配层：它把某种已有 Agent 的调用方式转换成平台可代理的 HTTP/A2A 接口。

平台里有两类容易混淆的 Bridge：

- **配置式 Bridge Agent**：写在 `bridge_agents` 配置里的轻量 HTTP/CLI 映射，由平台进程内执行，适合简单 API 或简单命令。
- **外部 Bridge 进程**：一个独立 HTTP 服务，注册为 External Agent。它负责接收平台转发来的消息，再调用 Claude Code、Kimi Code、Hermes、OpenClaw 等真实 Agent。

如果已有 Agent 有复杂 session、权限处理、流式输出、工作目录、进程生命周期，优先做外部 Bridge 进程。

## 总体模型

平台负责：

- Agent 注册、发现、状态展示
- 消息代理：客户端调用 `/agent/{name}`，平台转发到注册的 `url`
- task/message/trace 记录
- 可选的 `contextId` 生成和转发
- 健康检查和自动恢复连接

Bridge 负责：

- 将平台传来的 A2A JSON-RPC 请求解析成目标 Agent 能理解的输入
- 管理目标 Agent 的 session 或无状态调用
- 把目标 Agent 的输出转换成平台可消费的 JSON 或 SSE
- 处理超时、取消、错误、权限确认、进程清理

目标 Agent 负责：

- 实际推理、工具调用、代码执行或对话
- 维护自己的内部状态，如果它支持状态

## 注册方式

外部 Bridge 进程注册到平台时有两种方式。

### 发现式注册

Bridge 暴露 `GET /.well-known/agent.json`，平台注册时自动抓取 AgentCard。

```bash
curl -X POST http://localhost:18090/api/agents \
  -H "Content-Type: application/json" \
  -H "X-Admin-Token: a2a-admin-token" \
  -d '{
    "name": "claude-code",
    "type": "external",
    "url": "http://127.0.0.1:10004",
    "context_mode": "context"
  }'
```

适合：你愿意让 Bridge 按 A2A 风格暴露 AgentCard，并希望平台重启后重新发现能力。

### 静态注册

注册时直接提交 `agent_card`。外部 Bridge 不需要暴露 `/.well-known/agent.json`。

```bash
curl -X POST http://localhost:18090/api/agents \
  -H "Content-Type: application/json" \
  -H "X-Admin-Token: a2a-admin-token" \
  -d '{
    "name": "completion-agent",
    "type": "external",
    "url": "http://127.0.0.1:10005/run",
    "context_mode": "stateless",
    "agent_card": {
      "description": "OpenAI-compatible completion bridge",
      "version": "1.0.0",
      "health_url": "http://127.0.0.1:10005/health",
      "skills": [
        {"id": "chat", "name": "Chat", "description": "Single-turn completion"}
      ]
    }
  }'
```

适合：已有 Agent 接口固定，不想额外实现 discovery endpoint。

## 会话模式

Bridge 设计前必须先决定 `context_mode`。

### `stateless`

纯代理模式。平台不生成、不转发 `contextId`，每条消息都是目标侧的新请求。

适合：

- OpenAI-compatible `/v1/chat/completions`
- Hermes/OpenClaw 这类本身以 completion API 暴露的 Agent
- 目标端没有 session/resume 能力
- 每次调用都应该独立，不能继承上一次上下文

平台行为：

- 即使客户端传了 `contextId`，平台转发前也会移除
- task/message/trace 的 `context_id` 为空
- 仍然记录每次调用的 task 和 trace

Bridge 行为：

- 不需要保存 `contextId -> targetSessionId`
- 每次请求构造完整 prompt 或 messages
- 可以直接调用目标 completion API
- 超时后直接返回失败，不需要恢复 session

### `context`

带平台上下文的多轮模式。平台会读取请求中的 `contextId`；如果没有，则自动生成并注入转发请求。

适合：

- Claude Code、Kimi Code、OpenCode 这类交互式 CLI 或 headless server
- 一个 Agent 和另一个 Agent 多轮协作
- 目标 Agent 有自己的 session/resume 机制
- 需要把同一段协作中的 task/message/trace 串起来

平台行为：

- 没有 `contextId` 时自动生成
- 转发请求中包含 `params.contextId`
- task/message/trace 都挂到该 `contextId`

Bridge 行为：

- 必须维护 `platform contextId -> target session id`
- 映射要持久化，至少写入本地 SQLite/JSON 文件/Redis
- 同一个 `contextId` 的后续请求必须 resume 到同一个目标 session
- bridge 重启后应该能从映射恢复

## 消息入口协议

外部 Bridge 至少需要处理平台注册的 `url` 上的 `POST` 请求。

请求体是 JSON-RPC：

```json
{
  "jsonrpc": "2.0",
  "id": "1",
  "method": "SendStreamingMessage",
  "params": {
    "contextId": "optional-context-id",
    "message": {
      "role": "ROLE_USER",
      "parts": [{"text": "hello"}]
    }
  }
}
```

Bridge 应该提取：

- `params.contextId`：仅 `context` 模式需要
- `params.message.parts[].text`：用户输入文本
- `id`：可原样透传或用于日志关联
- `method`：当前主路径是 `SendStreamingMessage`，也要兼容 `message/send`

## 响应格式

推荐返回 SSE：

```http
Content-Type: text/event-stream
Cache-Control: no-cache

data: {"type":"task.status","taskId":"...","contextId":"...","status":{"state":"working"}}

data: {"type":"text.delta","taskId":"...","contextId":"...","text":"hello"}

data: {"type":"task.status","taskId":"...","contextId":"...","status":{"state":"completed","message":{"role":"agent","parts":[{"text":"hello"}]}}}
```

也可以返回非流式 JSON：

```json
{
  "jsonrpc": "2.0",
  "result": {
    "message": {
      "role": "agent",
      "parts": [{"text": "hello"}]
    }
  }
}
```

优先建议 SSE，因为平台 Chat UI 和任务追踪更适合流式响应。

## Bridge 类型

### Stateless HTTP Bridge

用于 completion API。

流程：

1. 接收平台 JSON-RPC
2. 提取用户文本
3. 构造目标 API 请求
4. 等待目标返回
5. 将返回文本转换成 SSE 或 JSON

伪代码：

```text
on POST /run:
  input = extractText(request.params.message.parts)
  response = POST target /v1/chat/completions with input
  text = response.choices[0].message.content
  return completed SSE or JSON
```

设计要点：

- 不保存 session
- 请求超时应短于平台代理超时
- 目标错误要返回 `task.status failed`
- 如果目标支持流式，逐块转成 `text.delta`
- 如果目标不支持流式，也可以一次性输出一个 `text.delta`

### Stateful CLI Bridge

用于 Claude Code、Kimi Code、OpenCode 等交互式工具。

流程：

1. 接收平台 JSON-RPC
2. 读取 `params.contextId`
3. 查找或创建目标 Agent session
4. 把用户文本发送给目标 session
5. 监听目标输出
6. 转换成 SSE
7. 保存 `contextId -> targetSessionId`

伪代码：

```text
on POST /:
  contextId = request.params.contextId
  targetSession = sessionStore.get(contextId)
  if targetSession missing:
    targetSession = createTargetSession()
    sessionStore.put(contextId, targetSession.id)

  sendMessage(targetSession, input)
  stream target output as text.delta
  return completed
```

设计要点：

- `contextId` 是平台会话，不等于目标 Agent session id
- Bridge 必须保存映射，不能只放内存
- 同一 `contextId` 下的请求应串行处理，避免一个 CLI session 被并发写入
- 支持取消时，应把 HTTP client disconnect 映射为目标命令中断
- 进程退出后，如果目标支持 resume，就恢复；不支持 resume 时要新开 session 并在日志里标记 session lost

## Claude Code 类工具建议

Claude Code 这类 CLI 的自然模型是：

- 启动后进入一个交互 session
- 多轮输入复用该 session
- 可以另起新 session

Bridge 不应该为每条消息都启动一个全新的 Claude Code 进程，除非注册为 `stateless`。更推荐：

```text
platform contextId -> claude code session id/process id
```

最低要求：

- `context_mode = "context"`
- 每个 platform context 对应一个 Claude Code session
- session 映射持久化
- 同一 session 内的消息按顺序执行
- 超时后标记该 session 状态，下一轮决定 resume 或重建

如果 Claude Code 提供 resume/session id：

- 保存 resume id
- bridge 重启后优先 resume
- resume 失败再新建 session，并在 trace/error 中说明

如果 Claude Code 只提供交互式 TTY：

- Bridge 需要管理 PTY
- 输出解析要区分最终回答、工具日志、权限提示
- 权限提示要么自动拒绝/批准，要么转换为平台事件等待人工处理
- 进程崩溃要清理映射，下一轮新建 session

## OpenCode/Headless 类工具建议

仓库里的 `bridges/opencode-headless-client.py` 是一个单次调用 wrapper：它每次创建 session，发送 prompt，轮询结果。

这类脚本适合两种用法：

- 当作配置式 CLI Bridge：简单，但通常是 `stateless`
- 改造成外部 Stateful Bridge：把 session 创建和 session id 保存挪到常驻服务里

如果目标 headless server 已经有 session API，优先做常驻外部 Bridge，而不是每次用脚本重建 session。

## 并发和锁

Bridge 需要明确并发策略。

推荐规则：

- 不同 `contextId` 可以并发
- 同一 `contextId` 默认串行
- 如果目标 Agent 明确支持并发分支，再增加高级能力

建议实现：

```text
lockKey = contextId
acquire lock
try:
  send to target session
finally:
  release lock
```

如果请求在等待锁时超时，应返回失败事件，而不是把消息悄悄丢掉。

## 超时、取消、重试

平台代理外部 Agent 的超时目前是 180 秒。Bridge 自己应设置更细的超时：

- 连接目标 Agent：5-10 秒
- 等待首 token/首输出：30-60 秒
- 总执行时间：小于平台代理超时

取消处理：

- HTTP client 断开时停止读取目标输出
- 如果目标支持 cancel，发送 cancel
- 如果目标是 CLI，必要时中断当前命令

重试建议：

- 注册失败可以重试
- 健康检查失败由平台标记状态并恢复
- 消息执行失败不要盲目重试有副作用的操作

## 健康检查

如果是静态注册，建议提供 `agent_card.health_url`。

健康检查应该验证：

- Bridge 进程可用
- 目标 Agent 依赖可用
- 对 stateful bridge，可选检查 session store 可用

健康检查不应该：

- 启动昂贵模型推理
- 修改工作目录
- 创建新目标 session

返回 200 表示可服务；非 200 表示平台可以标记为 `unreachable`。

## 错误映射

Bridge 应把错误转换成明确的失败事件：

```json
{
  "type": "task.status",
  "status": {
    "state": "failed",
    "message": {
      "role": "agent",
      "parts": [{"text": "target session timed out"}]
    }
  }
}
```

常见错误分类：

- `invalid_request`：平台请求格式不对
- `target_unavailable`：目标 Agent 不可用
- `session_lost`：目标 session 丢失
- `timeout`：执行超时
- `permission_required`：需要人工授权
- `internal_error`：Bridge 自身错误

错误文本可以给用户看，但不要暴露 API key、完整环境变量或敏感路径。

## 日志和可观测性

每个 Bridge 日志至少包含：

- platform agent name
- platform contextId
- platform task/id 或 JSON-RPC id
- target session id
- target request id
- start/end time
- status
- error class

不要只记目标 Agent 输出。Bridge 最重要的日志是“平台请求如何映射到目标 session”。

## 安全边界

CLI/code agent bridge 尤其要小心：

- 限制工作目录
- 明确权限策略
- 不要默认自动批准危险操作
- 不要把平台所有 agent 的消息无过滤地喂给代码 agent
- API key 只放在 bridge 环境变量或 secret store，不写入 AgentCard
- 对外暴露 bridge 时加内网限制或认证

## 实现清单

### Stateless Bridge

- [ ] `POST /run` 或 `POST /` 接收平台消息
- [ ] 提取 `parts[].text`
- [ ] 调用目标 API
- [ ] 返回 SSE 或 JSON
- [ ] 注册时设置 `context_mode: "stateless"`
- [ ] 提供可选 `/health`
- [ ] 错误转换成 failed event

### Stateful Bridge

- [ ] `POST /` 接收平台消息
- [ ] 要求并读取 `params.contextId`
- [ ] 持久化 `contextId -> targetSessionId`
- [ ] 同一 context 串行处理
- [ ] 支持 resume 或 session 重建策略
- [ ] 返回 SSE
- [ ] 注册时设置 `context_mode: "context"`
- [ ] 提供 `/health`
- [ ] 记录 session 映射日志

## 推荐落地顺序

1. 先做一个最小 Stateless HTTP Bridge，验证注册、转发、trace。
2. 再做一个 Stateful Bridge，把 `contextId -> session id` 映射跑通。
3. 给 Stateful Bridge 增加持久化和锁。
4. 增加健康检查和错误分类。
5. 做 e2e：分别验证 `stateless` 不转发 context、`context` 复用同一 session。

