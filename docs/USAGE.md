# A2A Platform 使用指南

本文档覆盖 A2A Platform 的所有功能和使用方式。

---

## 目录

- [快速开始](#快速开始)
- [Agent 类型总览](#agent-类型总览)
- [内建 LLM Agent](#内建-llm-agent)
- [Bridge Agent（API 桥接）](#bridge-agentapi-桥接)
- [外部 Agent（独立进程）](#外部-agent独立进程)
- [Admin Web UI](#admin-web-ui)
- [MCP 集成](#mcp-集成)
- [配置参考](#配置参考)
- [API 参考](#api-参考)
- [部署方式](#部署方式)
- [常见问题](#常见问题)

---

## 快速开始

### 最简启动（单二进制 + SQLite）

```bash
# 构建
make build

# 启动（无需 MySQL）
./server -f etc/config-sqlite.yaml

# 打开浏览器
open http://localhost:18090
```

30 秒内即可使用：Admin UI 在根路径，API 在 `/api/*`。

### Docker 一键启动（MySQL）

```bash
docker compose up -d
# 等待 MySQL healthy，平台自动启动
curl http://localhost:18090/health
```

---

## Agent 类型总览

平台支持三种 Agent 接入方式，可以混合使用：

| 类型 | 说明 | 适用场景 |
|------|------|----------|
| **Builtin LLM Agent** | 内置 LLM 调用（OpenAI/Anthropic），支持多轮对话 + MCP 工具 | 通用 AI Agent，需要工具调用能力 |
| **Bridge Agent** | 配置式 HTTP/CLI 桥接，将任意 API 包装为 A2A Agent | 接入现有 API（如 vLLM、Ollama、自定义服务） |
| **External Agent** | 独立运行的 A2A 兼容进程，通过注册 API 接入 | 已有 A2A 实现，或需要独立部署 |

---

## 内建 LLM Agent

直接在平台内运行 LLM Agent，支持 OpenAI 和 Anthropic API。

### 通过配置文件创建

```yaml
# etc/config.yaml
builtin_agents:
  - name: assistant
    provider: openai          # openai 或 anthropic
    base_url: https://api.openai.com
    api_key: ${OPENAI_API_KEY}
    model: gpt-4o
    description: "通用助手"
    system_prompt: "你是一个有帮助的 AI 助手。"
    max_tokens: 4096
    max_tool_rounds: 10
```

### 通过 API 动态创建

```bash
curl -X POST http://localhost:18090/api/builtin-agents \
  -H "Content-Type: application/json" \
  -H "X-Admin-Token: your-token" \
  -d '{
    "name": "coder",
    "provider": "anthropic",
    "base_url": "https://api.anthropic.com",
    "api_key": "sk-ant-...",
    "model": "claude-sonnet-4-20250514",
    "description": "Coding assistant",
    "system_prompt": "You are an expert programmer.",
    "max_tokens": 8192
  }'
```

### 通过 Admin UI 创建

1. 打开 `http://localhost:18090/builtin-agents`
2. 点击 "Create"
3. 填写 Provider、Model、API Key 等
4. 点击 "Create Agent"

### 使用（发送消息）

```bash
curl -X POST http://localhost:18090/agent/assistant \
  -H "Content-Type: application/json" \
  -d '{
    "jsonrpc": "2.0",
    "id": "1",
    "method": "SendStreamingMessage",
    "params": {
      "message": {
        "role": "ROLE_USER",
        "parts": [{"text": "写一个快排算法"}]
      }
    }
  }'
```

返回 SSE 流：
```
event: task.status
data: {"taskId":"...","status":{"state":"working"}}

event: text.delta
data: {"taskId":"...","text":"def "}

event: text.delta
data: {"taskId":"...","text":"quicksort(arr):"}

event: task.status
data: {"taskId":"...","status":{"state":"completed","message":{...}}}
```

### 多轮对话

同一个 `contextId` 的消息会自动加载历史记录：

```bash
# 第一轮
curl -X POST http://localhost:18090/agent/assistant \
  -d '{"jsonrpc":"2.0","id":"1","method":"SendStreamingMessage","params":{"contextId":"session-1","message":{"parts":[{"text":"我叫小明"}]}}}'

# 第二轮（自动记住上下文）
curl -X POST http://localhost:18090/agent/assistant \
  -d '{"jsonrpc":"2.0","id":"2","method":"SendStreamingMessage","params":{"contextId":"session-1","message":{"parts":[{"text":"我叫什么？"}]}}}'
```

### MCP 工具调用

Builtin Agent 支持接入 MCP Server 作为工具：

```yaml
builtin_agents:
  - name: file-agent
    provider: openai
    base_url: https://api.openai.com
    api_key: ${OPENAI_API_KEY}
    model: gpt-4o
    system_prompt: "You can read and write files using tools."
    max_tool_rounds: 5
    mcp_servers:
      - name: filesystem
        transport: stdio
        command: npx
        args: ["-y", "@modelcontextprotocol/server-filesystem", "/tmp"]
      - name: web-search
        transport: sse
        url: http://localhost:3100/sse
```

Agent 会自动使用 MCP 工具并进行多轮推理。

---

## Bridge Agent（API 桥接）

将任意 HTTP API 或 CLI 命令桥接为 A2A Agent，无需编写代码。

### HTTP API 桥接

最常见用法：桥接 OpenAI 兼容 API。

```yaml
# etc/config.yaml
bridge_agents:
  - name: my-llm
    description: "本地 LLM 服务"
    target:
      http:
        baseUrl: "http://10.1.52.70:8642"
        headers:
          Authorization: "Bearer ${LLM_API_KEY}"
    skills:
      - id: chat
        name: Chat
        description: "对话"
        invoke:
          type: http
          method: POST
          path: /v1/chat/completions
          headers:
            Content-Type: application/json
          body:
            model: "qwen-72b"
            messages:
              - role: user
                content: "{{inputText}}"
            stream: false
          response:
            text: "{{output.choices.0.message.content}}"
```

### CLI 命令桥接

将本地命令行工具包装为 Agent：

```yaml
bridge_agents:
  - name: shell-agent
    description: "执行 shell 命令"
    target:
      cli:
        shell: bash
        timeout: 10000
    skills:
      - id: exec
        name: Execute
        description: "执行命令并返回结果"
        invoke:
          type: cli
          command: "{{inputText}}"
          timeout: 10000
```

使用：
```bash
curl -X POST http://localhost:18090/agent/shell-agent \
  -d '{"jsonrpc":"2.0","id":"1","method":"SendStreamingMessage","params":{"message":{"parts":[{"text":"date +%Y-%m-%d"}]}}}'
```

### 多技能 Agent

一个 Bridge Agent 可以有多个 Skill，通过输入首词匹配：

```yaml
bridge_agents:
  - name: toolkit
    description: "多功能工具箱"
    target:
      http:
        baseUrl: "http://api.example.com"
      cli:
        shell: bash
    skills:
      - id: translate
        name: Translate
        invoke:
          type: http
          method: POST
          path: /translate
          body:
            text: "{{inputText}}"
          response:
            text: "{{output.result}}"
      - id: calc
        name: Calculator
        invoke:
          type: cli
          command: "python3 -c \"print({{inputText}})\""
      - id: weather
        name: Weather
        invoke:
          type: http
          method: GET
          url: "https://wttr.in/{{inputText}}?format=3"
          response:
            raw: true
```

使用：
```bash
# 匹配 "translate" skill
curl ... -d '{"params":{"message":{"parts":[{"text":"translate Hello World"}]}}}'

# 匹配 "calc" skill
curl ... -d '{"params":{"message":{"parts":[{"text":"calc 2+2*3"}]}}}'

# 匹配 "weather" skill
curl ... -d '{"params":{"message":{"parts":[{"text":"weather Beijing"}]}}}'

# 未匹配到则使用第一个 skill
curl ... -d '{"params":{"message":{"parts":[{"text":"随便说点什么"}]}}}'
```

### 模板变量

在 `body`、`args`、`url`、`headers` 中可以使用模板变量：

| 变量 | 说明 |
|------|------|
| `{{inputText}}` | 用户输入的完整文本 |
| `{{taskId}}` | 当前任务 UUID |
| `{{contextId}}` | 当前会话 UUID |
| `{{skillId}}` | 匹配到的 Skill ID |

### 响应提取

`response.text` 使用 `{{output.path}}` 从 JSON 响应中提取字段：

```yaml
response:
  text: "{{output.choices.0.message.content}}"
```

支持：
- 对象路径：`{{output.data.result}}`
- 数组索引：`{{output.items.0.name}}`
- 嵌套组合：`{{output.response.candidates.0.content.parts.0.text}}`

如果设置 `raw: true`，则返回完整响应体（不做提取）。

---

## 外部 Agent（独立进程）

如果你已经有 A2A 兼容的 Agent 进程（如 a2a-bridge Node.js 版），可以通过 API 注册：

### 注册

```bash
curl -X POST http://localhost:18090/api/agents \
  -H "Content-Type: application/json" \
  -H "X-Admin-Token: your-token" \
  -d '{
    "name": "my-external-agent",
    "type": "bridge",
    "url": "http://10.1.52.70:10004"
  }'
```

平台会自动：
1. 访问 `url/.well-known/agent.json` 获取 AgentCard
2. 做健康检测
3. 持久化到数据库
4. 每 30 秒心跳检查

### 发送消息

注册后即可通过平台代理发送消息：

```bash
curl -X POST http://localhost:18090/agent/my-external-agent \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","id":"1","method":"SendStreamingMessage","params":{"message":{"parts":[{"text":"你好"}]}}}'
```

### 删除

```bash
curl -X DELETE http://localhost:18090/api/agents/my-external-agent \
  -H "X-Admin-Token: your-token"
```

---

## Admin Web UI

平台内嵌 React 管理界面，单二进制部署无需额外 Web 服务器。

访问：`http://localhost:18090/`

### 页面说明

| 页面 | 路径 | 功能 |
|------|------|------|
| Dashboard | `/` | 概览：Agent 数量、今日任务、连接状态 |
| Agents | `/agents` | 列出所有 Agent，注册/删除外部 Agent |
| Builtin Agents | `/builtin-agents` | 创建/删除内建 LLM Agent |
| Tasks | `/tasks` | 任务列表，支持按 Agent/状态/关键词筛选 |
| Task Detail | `/tasks/:id` | 任务详情：消息记录 + 调用追踪 |
| Traces | `/traces` | 调用链追踪聚合视图 |

### 前端开发

如果需要修改前端：

```bash
cd web/admin
npm install
npm run dev    # 启动 Vite dev server on :3001

# 后端需要同时运行在 :18090
# Vite 会自动代理 /api 和 /health 请求
```

---

## MCP 集成

平台暴露标准 MCP (Model Context Protocol) 端点，任何 MCP 客户端（如 Claude Desktop、Cursor）都可以连接。

### SSE 连接

```
MCP SSE URL: http://localhost:18090/mcp/sse
```

### 可用工具

| 工具 | 参数 | 说明 |
|------|------|------|
| `list_agents` | 无 | 列出所有已注册 Agent |
| `send_to_agent` | `agent_name`, `message` | 向 Agent 发消息并等待回复 |
| `get_agent_info` | `agent_name` | 获取 Agent 详情 |

### 可用资源

| URI | 说明 |
|-----|------|
| `a2a://agents` | 所有 Agent 列表（JSON） |
| `a2a://agents/{name}` | 单个 Agent 详情 |

### Claude Desktop 配置示例

```json
{
  "mcpServers": {
    "a2a-platform": {
      "url": "http://localhost:18090/mcp/sse"
    }
  }
}
```

### 无 Session 模式（脚本测试）

```bash
# 初始化
curl -X POST http://localhost:18090/mcp/messages \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","clientInfo":{"name":"test"}}}'

# 列出工具
curl -X POST http://localhost:18090/mcp/messages \
  -d '{"jsonrpc":"2.0","id":2,"method":"tools/list"}'

# 调用工具
curl -X POST http://localhost:18090/mcp/messages \
  -d '{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"list_agents","arguments":{}}}'

# 发消息给 Agent
curl -X POST http://localhost:18090/mcp/messages \
  -d '{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"send_to_agent","arguments":{"agent_name":"assistant","message":"Hello"}}}'
```

---

## 配置参考

完整配置文件格式（`etc/config.yaml`）：

```yaml
# 基础
name: a2a-platform         # 平台名称
host: 0.0.0.0              # 监听地址
port: 18090                # 监听端口
admin_token: "secret"      # 管理 API 认证 token

# CORS（默认允许所有）
cors_origins:
  - "*"

# 速率限制（默认 100 req/s 全局，20 req/s 每 IP）
rate_limit_rps: 100

# 数据库（可选，不配则用 SQLite）
mysql:
  host: 127.0.0.1
  port: 3306
  user: a2a
  password: secret
  database: a2a_platform
  max_idle: 10
  max_open: 100

# 内建 LLM Agent
builtin_agents:
  - name: assistant
    provider: openai         # openai | anthropic
    base_url: https://api.openai.com
    api_key: ${OPENAI_API_KEY}   # 支持环境变量
    model: gpt-4o
    description: "通用助手"
    system_prompt: "You are helpful."
    max_tokens: 4096
    max_tool_rounds: 10
    mcp_servers:             # 可选 MCP 工具
      - name: fs
        transport: stdio     # stdio | sse
        command: npx
        args: ["-y", "@modelcontextprotocol/server-filesystem", "/tmp"]

# Bridge Agent（API/CLI 桥接）
bridge_agents:
  - name: my-api
    description: "桥接到外部 API"
    version: "1.0.0"
    target:
      http:
        baseUrl: "http://api.example.com"
        headers:
          Authorization: "Bearer ${API_KEY}"
      cli:
        shell: bash
        workingDir: "."
        timeout: 30000
    skills:
      - id: chat
        name: Chat
        description: "对话"
        tags: ["general"]
        examples: ["Hello"]
        invoke:
          type: http           # http | cli
          method: POST
          path: /v1/chat/completions
          url: ""              # 完整 URL（优先于 baseUrl+path）
          headers:
            Content-Type: application/json
          body:
            model: "gpt-4o"
            messages:
              - role: user
                content: "{{inputText}}"
          response:
            text: "{{output.choices.0.message.content}}"
            raw: false         # true = 返回原始响应
          timeout: 60000       # 毫秒
```

### 环境变量

配置中支持 `${VAR_NAME}` 语法引用环境变量。未设置的变量保留原样。

---

## API 参考

### 认证

需要认证的接口使用 `X-Admin-Token` header 或 `Authorization: Bearer <token>`。

需要认证的操作：
- `POST /api/agents` — 注册 Agent
- `DELETE /api/agents/{name}` — 删除 Agent
- `POST /api/builtin-agents` — 创建内建 Agent
- `DELETE /api/builtin-agents/{name}` — 删除内建 Agent

### 端点列表

#### 健康 & 统计

```bash
# 健康检查
GET /health
# → {"status":"ok","db":"ok","agents_connected":3,"agents_total":4}

# 统计
GET /api/stats
# → {"status":"ok","agents_connected":3,"agents_total":4,"tasks_today":12,"tasks_pending":0,"uptime_seconds":3600}
```

#### Agent 管理

```bash
# 列出所有 Agent
GET /api/agents
# → [{"name":"assistant","url":"/agent/assistant","status":"connected","type":"builtin"}]

# Agent 详情
GET /api/agents/{name}

# 注册外部 Agent
POST /api/agents
# Body: {"name":"x","type":"bridge","url":"http://...","port":10004}

# 删除 Agent
DELETE /api/agents/{name}
```

#### 内建 Agent 管理

```bash
# 列出
GET /api/builtin-agents
# → [{"name":"assistant","provider":"openai","model":"gpt-4o",...}]

# 创建
POST /api/builtin-agents
# Body: {"name":"x","provider":"openai","base_url":"...","api_key":"...","model":"..."}

# 删除
DELETE /api/builtin-agents/{name}
```

#### A2A 消息代理

```bash
# 向任意 Agent 发送消息（SSE 流式响应）
POST /agent/{name}
Content-Type: application/json

{
  "jsonrpc": "2.0",
  "id": "request-1",
  "method": "SendStreamingMessage",
  "params": {
    "contextId": "optional-session-id",
    "message": {
      "role": "ROLE_USER",
      "parts": [{"text": "你好"}]
    }
  }
}
```

#### 任务管理

```bash
# 任务列表（支持分页和筛选）
GET /api/tasks?page=1&size=20&agent_name=assistant&state=RESPONDED&search=hello

# 任务详情（含消息和追踪）
GET /api/tasks/{local_task_id}
# → {"task":{...},"messages":[...],"traces":[...]}
```

#### 追踪

```bash
# 最近追踪
GET /api/traces

# 按 Context 聚合
GET /api/traces/contexts

# 按 Task 查
GET /api/traces/task/{task_id}

# 按 Context 查
GET /api/traces/context/{context_id}
```

#### SSE 实时事件

```bash
# 订阅实时事件（Agent 注册/断开、任务状态变更）
GET /api/events
# Content-Type: text/event-stream
```

---

## 部署方式

### 方式一：单二进制（推荐开发/小规模）

```bash
make build
./server -f etc/config-sqlite.yaml
```

特点：零依赖，数据在 `./data/a2a.db`，含 Admin UI。

### 方式二：Docker Compose（推荐生产）

```bash
docker compose up -d
```

特点：MySQL 持久化，自动健康检查，容器隔离。

### 方式三：自定义 Docker

```dockerfile
FROM a2a-platform:latest
COPY my-config.yaml /app/etc/config.yaml
```

### 反向代理（Nginx）

```nginx
server {
    listen 80;
    server_name a2a.example.com;

    location / {
        proxy_pass http://127.0.0.1:18090;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
        proxy_set_header Host $host;
        proxy_buffering off;          # SSE 需要
        proxy_read_timeout 180s;
    }
}
```

---

## 常见问题

### Agent 注册后显示 disconnected

- 确认 Agent URL 从平台容器可达
- Docker 中使用宿主机 IP 而非 localhost
- 检查 Agent 的 `/.well-known/agent.json` 是否可访问

### Bridge Agent 返回错误

- 检查 `target.http.baseUrl` 是否可达
- 确认响应格式匹配 `response.text` 模板
- 设置 `response.raw: true` 看原始响应排查

### 内建 Agent 无响应

- 确认 API Key 正确（检查环境变量是否加载）
- 确认 `base_url` 正确（OpenAI: `https://api.openai.com`，Anthropic: `https://api.anthropic.com`）
- 查看服务端日志（`docker logs a2a-platform`）

### SQLite "database is locked"

- SQLite 模式已启用 WAL 和 busy_timeout
- 如果并发极高，建议切换到 MySQL

### 前端 404

- 确认执行了 `make build`（前端需要先编译）
- Docker 镜像自动包含前端构建

### MCP 连接超时

- 确认 `/mcp/sse` 端点可达
- SSE 连接需要长连接支持（检查反向代理配置）
