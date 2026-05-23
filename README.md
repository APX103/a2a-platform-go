# A2A Platform (Go)

Go 实现的 [Agent-to-Agent (A2A)](https://github.com/google/A2A) 协议平台。负责 Agent 注册、发现、消息路由、任务追踪，并内置 LLM Agent 引擎。

## 架构概览

```
┌─────────────────────────────────────────────────────────────┐
│                  A2A Platform (:18090)                       │
│                                                             │
│  ┌──────────┐  ┌──────────┐  ┌──────────┐                │
│  │ Admin UI │  │ REST API │  │ A2A Proxy│                │
│  │ /        │  │ /api/*   │  │ /agent/* │                │
│  └──────────┘  └────┬─────┘  └────┬─────┘                │
│                      │              │                     │
│  ┌───────────────────┴──────────────┴─────────────────┐   │
│  │              Agent Registry (内存 + DB)               │   │
│  └────────────────────┬─────────────────────────────────┘   │
│                       │                                     │
│  ┌────────────────────┴────────────────────┐                │
│  │   SQLite (默认) / MySQL 8.0 (可选)       │                │
│  │   agents | tasks | messages | traces    │                │
│  └─────────────────────────────────────────┘                │
│                                                             │
│  ┌──────────────────────────────────────────┐               │
│  │         Builtin Agent Engine             │               │
│  │   OpenAI / Anthropic + Platform Tools    │               │
│  └──────────────────────────────────────────┘               │
└─────────────────────────────────────────────────────────────┘
              │ 代理转发（外部 Agent）
   ┌──────────┼──────────┐
   ▼          ▼          ▼
┌────────┐ ┌────────┐ ┌────────┐
│Bridge A│ │Bridge B│ │Bridge C│
└───┬────┘ └───┬────┘ └───┬────┘
    ▼          ▼          ▼
 LLM API   LLM API   LLM API
```

## 功能

| 功能 | 说明 |
|------|------|
| **内建 LLM Agent** | 直接配置 OpenAI/Anthropic API，无需外部 Bridge，支持多轮对话 + 平台工具调用 |
| **Bridge Agent** | 配置式 HTTP/CLI 桥接，将任意 API 包装为 A2A Agent，无需编写代码 |
| **Agent 注册/发现** | 支持发现式注册（自动抓取 AgentCard）和静态注册（提交 AgentCard 由平台托管） |
| **A2A 消息代理** | `POST /agent/{name}` 透明转发 JSON-RPC，支持 SSE 流式 |
| **任务追踪** | 每条消息自动创建 Task，记录状态流转 |
| **调用链追踪** | traces 表记录完整的 send → stream → response 调用链 |
| **Admin Web UI** | 内嵌 React 管理界面，单二进制部署 |
| **双数据库** | 默认 SQLite（零配置），可选 MySQL 8.0 |
| **自动恢复** | 重启后从 DB 恢复已注册的 Agent 连接 |
| **聊天界面** | 内置对话界面，支持流式响应、思维可视化、工具调用展示 |

## 快速开始

### 方式一：单二进制（SQLite，推荐本地开发）

```bash
# 构建（需要 Node.js + Go 1.25+）
make build

# 启动
./server -f etc/config-sqlite.yaml

# 访问 Admin UI
open http://localhost:18090
```

无需安装 MySQL，数据存储在 `./data/a2a.db`。

### 方式二：Docker Compose（MySQL，推荐生产部署）

```bash
docker compose up -d
curl http://localhost:18090/health
# → {"status":"ok","db":"ok","agents_connected":0,"agents_total":0}
```

### 创建内建 Agent

通过 Admin UI（`http://localhost:18090/builtin-agents`）或 API：

```bash
curl -X POST http://localhost:18090/api/builtin-agents \
  -H "Content-Type: application/json" \
  -H "X-Admin-Token: a2a-admin-token" \
  -d '{
    "name": "assistant",
    "provider": "openai",
    "base_url": "https://api.openai.com",
    "api_key": "sk-...",
    "model": "gpt-4o",
    "description": "General assistant",
    "system_prompt": "You are a helpful assistant.",
    "max_tokens": 4096
  }'
```

### 发送消息

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
        "parts": [{"text": "Hello!"}]
      }
    }
  }'
```

返回 SSE 流式事件（text.delta、tool.call、task.status）。

## 聊天界面

平台包含内置聊天界面，可直接与 Agent 对话：

- **时间轴布局**: 消息以垂直时间轴展示，区分用户和 AI 消息
- **流式响应**: 实时打字机效果的 AI 回复
- **思维可视化**: 可折叠的思维块展示 Agent 推理过程
- **工具调用展示**: 查看工具调用详情（参数、结果、状态）
- **会话管理**: 每个 Agent 支持多个会话，可查看历史、继续对话
- **Markdown 渲染**: 支持格式化文本、代码高亮、表格、引用块
- **内置工具**: 文件操作、HTTP 请求等工具

**访问**: 导航至 `/chat/<agent_name>` 或从 Agents 页面点击 Agent。

**聊天 API**: 使用 Server-Sent Events (SSE) 实时流式：
- `POST /agent/<name>` - 发送消息并接收流式响应
- SSE 事件: `text.delta`、`tool.call_start`、`tool.result`、`task.status` 等

**内置工具**:
| 工具 | 说明 |
|------|------|
| `fetch_url` | 发起 HTTP 请求（GET/POST/PUT/DELETE） |
| `read_file` | 读取文件内容（支持分页） |
| `write_file` | 写入文件（支持追加模式） |
| `list_directory` | 列出目录内容 |
| `tool_search` | 搜索可用工具 |

### 配置 Bridge Agent（API 桥接）

在 config 中直接桥接任意 OpenAI 兼容 API：

```yaml
# etc/config.yaml
bridge_agents:
  - name: my-llm
    description: "Local LLM"
    target:
      http:
        baseUrl: "http://localhost:8642"
        headers:
          Authorization: "Bearer ${LLM_API_KEY}"
    skills:
      - id: chat
        name: Chat
        invoke:
          type: http
          method: POST
          path: /v1/chat/completions
          body:
            model: "qwen-72b"
            messages:
              - role: user
                content: "{{inputText}}"
          response:
            text: "{{output.choices.0.message.content}}"
```

### 注册外部 Agent（独立进程）

外部 Agent 有两种注册方式：

- **发现式注册**：外部 Agent 暴露 `GET /.well-known/agent.json`，平台注册时自动抓取 AgentCard，后续健康检查也会访问该端点。
- **静态注册**：注册请求直接提交 `agent_card`，平台把 AgentCard 存入数据库并对外提供查询；外部 Agent 只需要保留可被平台代理调用的消息入口。若需要健康状态，请在 `agent_card.health_url` 中提供健康检查地址。

外部 Agent 还可以通过 `context_mode` 声明会话模式：

- `context`（默认）：平台会为未带 `contextId` 的请求自动生成 context，并把同一个 context 下的 task/message/trace 串起来，适合多轮 agent 交互。
- `stateless`：平台不生成、不转发 `contextId`，每条消息都是一次无上下文 proxy 调用；平台只记录无上下文 task/trace，适合 completion API 或每次都新开会话的目标。

发现式注册：

```bash
curl -X POST http://localhost:18090/api/agents \
  -H "Content-Type: application/json" \
  -H "X-Admin-Token: a2a-admin-token" \
  -d '{"name": "my-agent", "type": "external", "url": "http://10.1.52.70:10004", "context_mode": "context"}'
```

静态注册：

```bash
curl -X POST http://localhost:18090/api/agents \
  -H "Content-Type: application/json" \
  -H "X-Admin-Token: a2a-admin-token" \
  -d '{
    "name": "my-agent",
    "type": "external",
    "url": "http://10.1.52.70:10004/run",
    "context_mode": "stateless",
    "agent_card": {
      "description": "Existing agent behind a custom endpoint",
      "version": "1.0.0",
      "health_url": "http://10.1.52.70:10004/health",
      "skills": [
        {"id": "chat", "name": "Chat", "description": "General conversation"}
      ]
    }
  }'
```

更完整的 Bridge 设计与实现约定见 [Bridge 实现指导手册](docs/BRIDGE_GUIDE.md)。

## API 参考

### REST API

| 方法 | 路径 | 认证 | 说明 |
|------|------|------|------|
| `GET` | `/health` | - | 健康检查（含 DB 状态） |
| `GET` | `/api/stats` | - | 统计信息 |
| `GET` | `/api/agents` | - | 列出所有 Agent |
| `POST` | `/api/agents` | token | 注册外部 Agent |
| `GET` | `/api/agents/{name}` | - | Agent 详情 |
| `DELETE` | `/api/agents/{name}` | token | 删除 Agent |
| `GET` | `/api/builtin-agents` | - | 列出内建 Agent |
| `POST` | `/api/builtin-agents` | token | 创建内建 Agent |
| `DELETE` | `/api/builtin-agents/{name}` | token | 删除内建 Agent |
| `GET` | `/api/contexts/{agent_name}` | - | 列出 Agent 的会话 |
| `POST` | `/api/contexts` | - | 创建新会话 |
| `GET` | `/api/contexts/{id}` | - | 获取会话详情 |
| `PATCH` | `/api/contexts/{id}` | - | 更新会话标题 |
| `DELETE` | `/api/contexts/{id}` | - | 删除会话 |
| `GET` | `/api/subagents/{context_id}` | - | 列出子代理 |
| `GET` | `/api/subagents/{id}` | - | 获取子代理详情 |
| `POST` | `/agent/{name}` | - | A2A 消息代理（JSON-RPC） |
| `GET` | `/api/tasks` | - | 任务列表 |
| `GET` | `/api/tasks/{id}` | - | 任务详情 |
| `GET` | `/api/traces` | - | 最近追踪 |
| `GET` | `/api/traces/contexts` | - | 按 Context 聚合 |
| `GET` | `/api/traces/task/{id}` | - | 按 Task 查追踪 |
| `GET` | `/api/traces/context/{id}` | - | 按 Context 查追踪 |
| `GET` | `/api/events` | - | SSE 实时事件流 |

Task 会记录 `source_agent -> target_agent`；旧字段 `agent_name` 仍保留，语义等价于 `target_agent`。Message 会保留 `role` 作为协议角色，同时用 `sender_agent` / `recipient_agent` 表达真实通信方向。Bridge 或 Agent 代发平台消息时可设置 `X-A2A-Source-Agent` header 来标记真实发起方。

认证：需要 `X-Admin-Token` header 或 `Authorization: Bearer <token>`。

### Platform Tools

| 工具 | 说明 |
|------|------|
| `list_groups` | 列出当前 agent 可见的协作群 |
| `list_agents` | 按 group_id 列出群内可见 Agent |
| `send_to_agent` | 在群边界内向 Agent 发消息并等待回复 |
| `get_agent_info` | 获取群内可见 Agent 详情 |

## 项目结构

```
.
├── cmd/server/main.go           # 入口：路由 + 中间件 + SPA 服务
├── internal/
│   ├── bridge/                  # Bridge Agent 引擎（HTTP/CLI 桥接）
│   │   ├── bridge.go            # 核心：BridgeAgent + Registry + SSE 流式
│   │   ├── http.go              # HTTP Skill 调用
│   │   ├── cli.go               # CLI Skill 调用
│   │   └── template.go          # {{var}} 模板渲染 + 响应提取
│   ├── config/config.go         # YAML 配置（双数据库自动检测）
│   ├── engine/engine.go         # 内建 Agent 引擎（LLM + 平台工具循环）
│   ├── llm/                     # LLM Provider（OpenAI、Anthropic）
│   ├── handler/
│   │   ├── handler.go           # REST handlers
│   │   ├── builtin_agent.go     # 内建 Agent CRUD
│   │   └── stats.go             # 统计
│   ├── model/types.go           # 数据模型
│   └── svc/
│       ├── servicecontext.go    # DB 初始化（SQLite/MySQL） + 自动迁移
│       ├── store.go             # CRUD（双方言兼容）
│       └── registry.go          # Agent 注册表 + 心跳
├── web/
│   ├── admin/                   # React 前端源码
│   ├── dist/                    # Vite 构建输出（git-ignored）
│   └── embed*.go                # Admin UI embed（默认占位，WITH_FRONTEND=1 嵌 dist）
├── docs/USAGE.md                # 完整使用指南
├── docs/PROJECT_MAP.md          # 项目结构与整理建议
├── tests/e2e/e2e_test.go        # E2E 测试（63 cases）
├── etc/
│   ├── config.yaml              # MySQL 模式配置
│   └── config-sqlite.yaml       # SQLite 模式配置
├── Dockerfile                   # 多阶段构建（Node + Go → Alpine）
├── docker-compose.yml           # MySQL + Platform
├── Makefile                     # build / dev / clean
└── go.mod
```

## 前后端分离构建

默认 `make build` 会编译 React 前端并通过 Go build tag `frontend` 嵌入二进制。只构建后端 API：

```bash
make build WITH_FRONTEND=0
```

显式构建嵌入版：

```bash
make build WITH_FRONTEND=1
```

Docker 同样支持这个开关：

```bash
docker build --build-arg WITH_FRONTEND=0 -t a2a-platform:api .
docker build --build-arg WITH_FRONTEND=1 -t a2a-platform:embedded .
```

本地前端连接远端后端时可设置：

```bash
cd web/admin
VITE_DEV_API_PROXY=https://api.example.com npm run dev
# 或者直连跨域 API，此时后端 cors_origins 需要允许本地前端 origin
VITE_API_BASE_URL=https://api.example.com npm run dev
```

## 配置

### SQLite 模式（默认）

```yaml
name: a2a-platform
host: 0.0.0.0
port: 18090
admin_token: "your-secret-token"
```

数据自动存储在 `./data/a2a.db`（WAL 模式）。

### MySQL 模式

```yaml
name: a2a-platform
host: 0.0.0.0
port: 18090
admin_token: "your-secret-token"

mysql:
  host: 127.0.0.1
  port: 13306
  user: a2a
  password: your_password
  database: a2a_platform
```

配置中有 `mysql:` 块且 `host` 非空时自动使用 MySQL。

### 内建 Agent 配置（可选）

```yaml
builtin_agents:
  - name: assistant
    provider: openai          # openai 或 anthropic
    base_url: https://api.openai.com
    api_key: ${OPENAI_API_KEY}
    model: gpt-4o
    description: "General assistant"
    system_prompt: "You are a helpful assistant."
    max_tokens: 4096
    max_tool_rounds: 10
```

支持 `${ENV_VAR}` 环境变量展开。

## 本地开发

```bash
# 前端开发（热重载）
make dev    # 启动 Vite dev server on :3001，自动代理 /api 到 :18090

# 后端开发
go run ./cmd/server -f etc/config-sqlite.yaml

# 后端单元测试
make test

# 完整构建
make build  # 前端 build + Go build

# 运行测试（需要 Docker 中的平台运行在 :18090）
go test -v ./tests/e2e/
```

## 技术栈

- **Go 1.25** — 标准库 `net/http`，无框架
- **SQLite** (modernc.org/sqlite) — 默认存储，纯 Go 无 CGO
- **MySQL 8.0** — 可选生产存储
- **React 19 + Vite 8 + Tailwind CSS 4** — Admin UI
- **embed.FS** — 前端嵌入 Go 二进制
- **Docker Compose** — 一键部署

## License

MIT
