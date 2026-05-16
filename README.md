# A2A Platform (Go)

Go 实现的 [Agent-to-Agent (A2A)](https://github.com/google/A2A) 协议平台。负责 Agent 注册、发现、消息路由、任务追踪，并暴露 MCP (Model Context Protocol) 端点，让任意 LLM Agent 通过工具调用的方式与其他 Agent 通信。

## 架构概览

```
┌─────────────────────────────────────────────────────┐
│                   A2A Platform (:18090)              │
│                                                     │
│  ┌──────────┐  ┌──────────┐  ┌───────────┐         │
│  │ REST API │  │ A2A Proxy│  │ MCP Server│         │
│  │ /api/*   │  │ /agent/* │  │ /mcp/*    │         │
│  └────┬─────┘  └────┬─────┘  └─────┬─────┘         │
│       │              │              │                │
│  ┌────┴──────────────┴──────────────┴──────┐        │
│  │            Agent Registry (内存 + DB)    │        │
│  └────────────────────┬────────────────────┘        │
│                       │                             │
│  ┌────────────────────┴────────────────────┐        │
│  │           MySQL (a2a_platform)          │        │
│  │  agents | tasks | messages | traces     │        │
│  └─────────────────────────────────────────┘        │
└─────────────────────────────────────────────────────┘
                        │ 代理转发
         ┌──────────────┼──────────────┐
         ▼              ▼              ▼
   ┌──────────┐   ┌──────────┐   ┌──────────┐
   │ Bridge A │   │ Bridge B │   │ Bridge C │
   │ :10004   │   │ :10005   │   │ :10006   │
   └────┬─────┘   └────┬─────┘   └────┬─────┘
        │              │              │
        ▼              ▼              ▼
   ┌─────────┐   ┌─────────┐   ┌──────────┐
   │LLM API A│   │LLM API B│   │LLM API C │
   └─────────┘   └─────────┘   └──────────┘
```

每个 Bridge 是一个 [a2a-bridge](https://github.com/APX103/a2a-bridge) 进程（Node.js），负责将 OpenAI 兼容 API 桥接为 A2A 协议。

## 功能

| 功能 | 说明 |
|------|------|
| Agent 注册/发现 | 注册 Agent → 自动抓取 AgentCard → 心跳检测 → 持久化到 MySQL |
| A2A 消息代理 | `POST /agent/{name}` 透明转发 JSON-RPC，支持 SSE 流式 |
| MCP 端点 | `POST /mcp/messages` 暴露 `list_agents` / `send_to_agent` / `get_agent_info` 三个工具 |
| 任务追踪 | 每条消息自动创建 Task，记录状态流转 |
| 消息记录 | 用户消息和 Agent 回复都存入 messages 表 |
| 调用链追踪 | traces 表记录完整的 send → response 调用链 |
| 自动恢复 | 平台重启后从 DB 恢复已注册的 Agent 连接 |

## 快速开始

### 前置条件

- Docker & Docker Compose
- Node.js（用于运行 a2a-bridge，如果需要桥接 LLM Agent）
- 有一个 OpenAI 兼容的 LLM API（如 Hermes Agent、vLLM、Ollama 等）

### 1. 启动平台

```bash
# 克隆仓库
git clone <repo-url> && cd a2a-platform-go

# 构建并启动（MySQL + Platform）
docker compose up -d

# 等待 MySQL healthy，平台会自动启动
# 查看状态
docker compose ps
curl http://localhost:18090/health
# → {"status":"ok"}
```

平台启动后默认监听 `:18090`，MySQL 在 `:13306`。

### 2. 配置并启动 Bridge

Bridge 负责把你的 LLM API 包装成 A2A Agent。

```bash
# 安装 a2a-bridge
npm install -g @anthropic-ai/a2a-bridge
# 或者用本地 clone
git clone https://github.com/APX103/a2a-bridge && cd a2a-bridge && npm install && npm link

# 复制示例配置
cp bridges/bridge.example.yaml bridges/my-agent.yaml

# 编辑配置，填入你的 LLM API 地址和 Key
vim bridges/my-agent.yaml

# 启动 bridge
a2a-bridge --config bridges/my-agent.yaml
```

配置文件关键字段说明：

```yaml
name: "my-agent"              # Agent 名称（全局唯一）
description: "我的 Agent"       # 描述
server:
  port: 10004                  # Bridge 监听端口
target:
  http:
    baseUrl: "http://10.1.52.70:8642"   # LLM API 地址
    headers:
      Authorization: "Bearer sk-xxx"    # API Key
skills:
  - id: "chat"
    invoke:
      path: "/v1/chat/completions"      # OpenAI 兼容端点
      body:
        model: "glm-5.1"               # 模型名
```

### 3. 注册 Agent 到平台

```bash
curl -X POST http://localhost:18090/api/agents \
  -H "Content-Type: application/json" \
  -d '{
    "name": "my-agent",
    "type": "bridge",
    "url": "http://10.1.52.70:10004",
    "port": 10004
  }'
# → {"ok":true,"name":"my-agent","url":"http://10.1.52.70:10004","status":"connected"}
```

平台会自动：
1. 访问 `http://10.1.52.70:10004/.well-known/agent.json` 获取 AgentCard
2. 做轻量 ping 检测
3. 持久化到 MySQL
4. 加入内存注册表

### 4. 发送消息

#### 方式一：A2A 协议直发

```bash
curl -X POST http://localhost:18090/agent/my-agent \
  -H "Content-Type: application/json" \
  -H "A2A-Version: 1.0" \
  -d '{
    "jsonrpc": "2.0",
    "id": "test-001",
    "method": "SendStreamingMessage",
    "params": {
      "message": {
        "role": "ROLE_USER",
        "parts": [{"text": "你好，介绍下你自己"}],
        "message_id": "msg-001"
      }
    }
  }'
```

返回 SSE 流式事件，最终包含 Agent 回复。

#### 方式二：MCP 工具调用

```bash
# 列出所有 Agent
curl -X POST http://localhost:18090/mcp/messages \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"list_agents","arguments":{}}}'

# 发消息给指定 Agent
curl -X POST http://localhost:18090/mcp/messages \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"send_to_agent","arguments":{"agent_name":"my-agent","message":"你好"}}}'
```

## API 参考

### REST API

| 方法 | 路径 | 说明 |
|------|------|------|
| `GET` | `/health` | 健康检查 |
| `GET` | `/api/agents` | 列出所有 Agent |
| `POST` | `/api/agents` | 注册 Agent |
| `GET` | `/api/agents/{name}` | 获取单个 Agent 信息 |
| `DELETE` | `/api/agents/{name}` | 删除 Agent |
| `POST` | `/agent/{name}` | A2A 消息代理（JSON-RPC） |
| `GET` | `/api/tasks` | 任务列表（`?agent_name=&state=&page=1&size=20`） |
| `GET` | `/api/tasks/{id}` | 任务详情（含消息和追踪） |
| `GET` | `/api/traces/task/{id}` | 按 Task 查追踪 |
| `GET` | `/api/traces/context/{id}` | 按 Context 查追踪 |

### MCP Tools

| 工具 | 参数 | 说明 |
|------|------|------|
| `list_agents` | — | 列出所有已注册 Agent 及状态 |
| `send_to_agent` | `agent_name`, `message` | 向指定 Agent 发消息并等待回复 |
| `get_agent_info` | `agent_name` | 获取 Agent 详细信息 |

### MCP SSE

标准 MCP 客户端（如 Hermes Agent、Claude Desktop）可通过 SSE 连接：

```
MCP SSE URL: http://localhost:18090/mcp/sse
```

也可以直接 `POST /mcp/messages`（无 session 模式，适合脚本测试）。

## 项目结构

```
.
├── cmd/server/main.go           # 入口：路由注册 + 中间件 + 优雅关闭
├── internal/
│   ├── config/config.go         # YAML 配置加载
│   ├── handler/
│   │   ├── handler.go           # REST handlers：注册/发现/代理/任务/追踪
│   │   └── mcp_sse.go           # MCP SSE server：tools + resources
│   ├── model/types.go           # 数据模型
│   └── svc/
│       ├── servicecontext.go    # 全局 ServiceContext + DB 自动迁移
│       ├── store.go             # MySQL CRUD（agents/tasks/messages/traces）
│       └── registry.go          # 内存 Agent 注册表（并发安全 + 自动恢复）
├── etc/config.yaml              # 运行配置
├── sql/init.sql                 # MySQL 建表语句
├── bridges/
│   ├── bridge.example.yaml      # Bridge 配置模板
│   └── *.yaml                   # 实际配置（被 .gitignore 忽略）
├── Dockerfile                   # 多阶段构建（golang:1.21 → alpine）
├── docker-compose.yml           # MySQL + Platform
└── go.mod                       # 依赖：mysql driver + uuid + yaml
```

## 配置说明

**etc/config.yaml**

```yaml
name: a2a-platform
host: 0.0.0.0
port: 18090

mysql:
  host: mysql          # Docker 内用 service 名
  port: 3306
  user: a2a
  password: a2a_secret_2024
  database: a2a_platform
  max_idle: 10
  max_open: 100
```

如果要在宿主机直接运行（不用 Docker），把 `mysql.host` 改为 `127.0.0.1`，`mysql.port` 改为 `13306`。

## 本地开发

```bash
# 确保 MySQL 在跑
docker compose up -d mysql

# 修改 etc/config.yaml 指向本地 MySQL
# mysql.host: "127.0.0.1"
# mysql.port: 13306

# 运行
go run ./cmd/server -f etc/config.yaml
```

## 常见问题

**Q: Agent 注册失败 "Cannot fetch agent card"**
A: 确认 Bridge 进程已启动并且端口可达。在 Docker 中注册时，URL 需要用宿主机可达的 IP（如 `10.1.52.70:10004`），不要用 `localhost`。

**Q: MCP `send_to_agent` 返回 "(empty response)"**
A: Bridge 的 LLM API 调用失败或超时。检查 `target.http.baseUrl` 和 API Key 是否正确。

**Q: 平台重启后 Agent 状态变成 disconnected**
A: 平台会自动尝试恢复连接。如果 Bridge 进程没启动或 IP 变了，恢复会失败。重新启动 Bridge 后手动注册或重启平台即可。

**Q: Docker 里平台访问不到宿主机的 Bridge**
A: `docker-compose.yml` 已配置 `extra_hosts: host.docker.internal:host-gateway`，也可以直接用内网 IP。

## 技术栈

- **Go 1.21** — 标准库 `net/http`，无框架依赖
- **MySQL 8.0** — 数据持久化
- **a2a-bridge** — Node.js 桥接器，LLM API ↔ A2A 协议
- **Docker Compose** — 一键部署

## License

MIT
