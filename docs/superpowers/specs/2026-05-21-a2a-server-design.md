# A2A Server — 生产版代理平台设计

> 新项目：`~/work/a2a-server`，从零构建纯 A2A 代理平台。
> 参考：`a2a-platform-go` 原型中可复用的代码。

## 1. 项目定位

平台是一个**透明的消息代理**——不参与 Agent 内部逻辑，只负责"谁在线"和"消息怎么到"。所有 A2A 协议消息经过平台转发，但不解析消息内容的语义。

**核心架构：** 平台即 Proxy——所有 Agent 的 A2A endpoint 都是平台上的路由（`POST /agent/{name}`）。Agent 不需要有自己的 URL，通过 MCP 收件箱模型拉取消息。

### 1.1 V1 范围

| 功能 | 说明 |
|------|------|
| Agent 注册与发现 | REST API + MCP tool 注册，AgentCard 抓取，内存注册表 + 数据库持久化 |
| 保活机制 | MCP heartbeat tool + SSE 长连接 + HTTP poll 三种模式，超时自动下线 |
| 消息代理 | 收件箱队列模型：消息入队 → Agent 通过 MCP 拉取 → 回复后推送给发送方 |
| API + MCP 服务 | REST API 管理 + MCP 端点暴露平台能力（含消息收发） |

### 1.2 明确不做

- 内建 Agent / LLM 引擎
- Bridge Agent
- 消息持久化（不记录消息内容或元数据）
- 会话管理 / 多轮对话上下文
- 消息追踪（Trace）与可观测性
- 前端管理界面
- etcd 服务发现（V1 用内存+数据库）

## 2. 技术选型

| 层面 | 选择 | 理由 |
|------|------|------|
| 语言 | Go | 高并发、单二进制部署 |
| 框架 | go-zero | 全框架：HTTP server、路由、中间件、依赖注入 |
| 消息队列 | go-queue（go-zero 组件） | Agent 收件箱实现，支持 beanstalkd/kafka |
| 持久存储 | SQLite（开发）/ MySQL（生产） | 双方言兼容 |
| 通信协议 | A2A Protocol（JSON-RPC over HTTP/SSE） | 遵循 Google A2A 规范 |
| 工具协议 | MCP（Model Context Protocol） | 标准化工具暴露，Agent 通过 MCP 收发消息和保活 |

## 3. 项目结构（go-zero 风格）

```
a2a-server/
├── etc/
│   └── a2a-server.yaml          # 配置文件
├── internal/
│   ├── config/                  # 配置结构体 + 加载
│   ├── handler/                 # HTTP handler（go-zero 风格）
│   │   ├── agenthandler.go      # Agent CRUD + 心跳
│   │   ├── proxyhandler.go      # 消息转发入口
│   │   ├── eventshandler.go     # SSE 事件流
│   │   ├── healthhandler.go     # 健康检查
│   │   └── mcphandler.go        # MCP SSE 端点
│   ├── logic/                   # 业务逻辑层
│   │   ├── agentlogic.go        # Agent 注册/发现/保活
│   │   ├── proxylogic.go        # 消息代理逻辑
│   │   ├── inboxlogic.go        # 收件箱队列管理
│   │   └── mcplogic.go          # MCP 工具调用逻辑
│   ├── model/                   # 数据模型 + Store 接口
│   ├── registry/                # Agent 注册表（内存 map + 接口抽象）
│   ├── inbox/                   # 收件箱队列（go-queue 封装）
│   ├── events/                  # SSE 事件广播器
│   ├── mcp/                     # MCP 协议实现
│   ├── middleware/              # CORS、认证、限流
│   └── svc/                     # ServiceContext（依赖注入容器）
├── cmd/
│   └── server/
│       └── main.go              # 入口
├── sql/                         # DDL 文件
├── a2a-server.go                # go-zero 生成的 server 定义
├── go.mod
├── Makefile
└── Dockerfile
```

## 4. 核心架构：消息流转

### 4.1 整体流程

```
发送方 Agent                    平台 (go-zero)              接收方 Agent
     │                              │                           │
     │ POST /agent/{target}         │                           │
     │ ──────────────────────────►  │                           │
     │         SSE 连接等待响应      │  消息入队 (go-queue)       │
     │◄──────────────────────────   │  ──────► inbox queue      │
     │                              │                           │
     │                              │    MCP: receive_message   │
     │                              │  ◄─────────────────────── │
     │                              │                           │
     │                              │    MCP: reply_message     │
     │                              │  ◄─────────────────────── │
     │                              │                           │
     │     SSE 事件流推送响应        │                           │
     │◄──────────────────────────   │                           │
```

**关键规则：**
- 收一条必须回一条，Agent 在回复当前消息前无法拉取下一条
- 首字超时 1min（Agent 开始处理的时间）
- 不设总超时，超时由平台保障
- 超时时平台回复发送方："对方无法响应"（A2A JSON-RPC error）

### 4.2 双模式 Agent 支持

**模式 A：MCP Agent（主要，无 URL）**
- 通过 MCP tools 收发消息和保活
- 消息通过收件箱队列中转
- 不需要运行自己的 HTTP 服务

**模式 B：HTTP Agent（次要，有 URL）**
- 注册时提供 URL
- 平台通过 HTTP POST 主动转发消息到 Agent 的 URL
- Agent 通过 HTTP 响应（SSE 或 JSON）返回结果
- SSE 长连接存续 = 在线

## 5. 核心组件设计

### 5.1 Registry（注册表）

```go
type AgentEvent struct {
    Type      string       // "register", "deregister", "heartbeat", "offline"
    AgentName string
    Agent     *model.Agent
}

type Registry interface {
    Register(ctx context.Context, agent *model.Agent) error
    Deregister(ctx context.Context, name string) error
    Get(ctx context.Context, name string) (*model.Agent, error)
    List(ctx context.Context) ([]*model.Agent, error)
    Heartbeat(ctx context.Context, name string) error
    Watch(ctx context.Context) (<-chan AgentEvent, error)
}
```

**MemoryRegistry 实现：**
- `sync.RWMutex` 保护的 `map[string]*model.Agent`
- 启动时从数据库恢复已注册 Agent（状态标记为 offline）
- `Heartbeat()` 更新 `lastHeartbeat` 时间戳
- 后台 goroutine 每 10s 扫描，超过 TTL（30s）的标记为 offline 并广播事件

### 5.2 Inbox（收件箱队列）

```go
type InboxMessage struct {
    MessageID   string    // UUID
    FromAgent   string    // 发送方名称
    TargetAgent string    // 接收方名称
    Content     string    // JSON-RPC 请求体
    ReplyChan   chan *Reply // 回复通道（用于同步等待回复）
    CreatedAt   time.Time
}

type Reply struct {
    MessageID string
    Content   string    // 响应内容
    IsSSE     bool      // 是否 SSE 流式响应
    Error     string    // 错误信息（如果有）
}

type Inbox interface {
    Push(ctx context.Context, agentName string, msg *InboxMessage) error
    Pop(ctx context.Context, agentName string) (*InboxMessage, error)  // 阻塞等待
    Reply(ctx context.Context, messageID string, reply *Reply) error
    Close() error
}
```

**实现：**
- 基于 go-queue 组件（beanstalkd 或 kafka）
- 每个 Agent 注册后自动创建一个命名队列
- `Pop` 阻塞等待直到有消息或上下文取消
- `ReplyChan` 用于将回复同步传回等待中的发送方
- 首字超时 1min：Pop 返回后启动计时器，1min 内未收到 reply → 超时

### 5.3 Store（存储层）

```go
type Store interface {
    CreateAgent(ctx context.Context, agent *model.Agent) error
    UpdateAgent(ctx context.Context, agent *model.Agent) error
    DeleteAgent(ctx context.Context, name string) error
    GetAgent(ctx context.Context, name string) (*model.Agent, error)
    ListAgents(ctx context.Context) ([]*model.Agent, error)
    Close() error
}
```

**双方言实现：**
- `SQLiteStore` — 使用 `modernc.org/sqlite`（纯 Go，无 CGO）
- `MySQLStore` — 使用 `go-sql-driver/mysql`
- 通过配置文件切换

**数据库表（agents）：**

| 字段 | 类型 | 说明 |
|------|------|------|
| id | int64, PK | 自增主键 |
| name | string, UNIQUE | Agent 唯一名称 |
| type | string | Agent 类型（mcp / http） |
| url | string, nullable | Agent 服务地址（MCP Agent 可为空） |
| status | string | 在线状态（connected / offline / unreachable） |
| agent_card_json | text | AgentCard JSON |
| skills_json | text | 技能列表 JSON |
| error_message | string, nullable | 最近错误信息 |
| secret | string | 幂等重注册密钥 |
| connected_at | timestamp | 最近连接时间 |
| created_at | timestamp | 创建时间 |
| updated_at | timestamp | 更新时间 |

## 6. REST API

| 方法 | 路径 | 认证 | 说明 |
|------|------|------|------|
| GET | `/health` | - | 平台健康检查 |
| POST | `/api/agents` | token | 注册 Agent |
| GET | `/api/agents` | - | 列出所有 Agent |
| GET | `/api/agents/{name}` | - | 获取 Agent 详情 |
| DELETE | `/api/agents/{name}` | token | 注销 Agent |
| POST | `/api/agents/{name}/heartbeat` | - | 心跳续约 |
| POST | `/agent/{name}` | - | 发送 A2A 消息（入队到收件箱，SSE 流式返回响应） |
| GET | `/api/events` | - | SSE 事件流（Agent 上下线通知） |
| GET | `/.well-known/agent-card/{name}` | - | 获取 AgentCard |

**注册请求体：**
```json
{
  "name": "my-agent",
  "type": "mcp",
  "url": "",
  "port": 0,
  "skills": [{"id": "search", "name": "Search", "description": "Web search"}],
  "secret": "optional-secret"
}
```

- `type: "mcp"` — 无 URL，通过 MCP 收消息
- `type: "http"` — 有 URL，平台 HTTP 转发

**认证：**
- 需要 token 的接口：注册、注销
- Header: `X-Admin-Token` 或 `Authorization: Bearer <token>`

## 7. MCP 服务

### 7.1 连接流程

1. `GET /mcp/sse` → 建立 SSE 连接，返回 `endpoint` 事件（含 session_id）
2. `POST /mcp/messages?session_id=xxx` → 发送 `initialize` 请求
3. 协商能力：协议版本 `2024-11-05`，支持 tools + resources
4. 正常交互

### 7.2 MCP 工具

| 工具 | 参数 | 说明 |
|------|------|------|
| `list_agents` | - | 列出所有已注册 Agent |
| `receive_message` | - | 从收件箱拉取一条消息，阻塞等待直到有消息。返回 message_id、from_agent、content |
| `reply_message` | message_id, response | 回复指定消息。平台将响应推送给等待中的发送方 |
| `get_agent_info` | agent_name | 获取 Agent 详情 |
| `heartbeat` | - | 保活续约，防止被标记 offline |
| `send_to_agent` | agent_name, message | 向指定 Agent 发消息（内部调用 `POST /agent/{name}` 逻辑） |

### 7.3 MCP 资源

| URI | 类型 | 说明 |
|-----|------|------|
| `a2a://agents` | application/json | 所有 Agent 列表 |
| `a2a://agents/{name}` | application/json | 单个 Agent 详情 |

### 7.4 会话模式

- **有状态**：SSE channel 推送响应（适合 MCP 客户端如 Claude Desktop）
- **无状态**：直接 HTTP POST `/mcp/messages`，响应作为 HTTP body 返回（适合脚本测试）

## 8. Events（SSE 广播器）

事件类型：
- `agent.online` — Agent 注册成功或心跳恢复
- `agent.offline` — Agent 心跳超时或主动注销
- `agent.card_updated` — AgentCard 更新
- `message.timeout` — 消息响应超时

## 9. Middleware

| 中间件 | 说明 |
|--------|------|
| CORS | 允许跨域请求 |
| Auth | Token 认证（注册/注销接口） |
| RateLimit | 全局限流 100 req/s + 每 IP 20 req/s |
| Logger | 请求日志（method、path、status、duration） |

## 10. 配置

```yaml
Name: a2a-server
Host: 0.0.0.0
Port: 18090
AdminToken: "${ADMIN_TOKEN}"

DB:
  Driver: sqlite          # sqlite | mysql
  DataSource: "./data/a2a.db"

Queue:
  Type: beanstalkd        # beanstalkd | kafka
  Beanstalkd:
    Endpoint: "localhost:11300"

Heartbeat:
  TTL: 30s
  CheckInterval: 10s

Proxy:
  FirstTokenTimeout: 60s  # 首字超时
  MaxTimeout: 0s          # 总超时（0 = 不限制）

Log:
  Mode: console
  Level: info
```

## 11. 从原型复用的代码

| 模块 | 原型路径 | 处理方式 |
|------|----------|----------|
| MCP SSE handler | `internal/handler/mcp_sse.go` | 复制并改为 go-zero handler 风格 |
| SSE 广播器 | `internal/events/broadcaster.go` | 复制 |
| 数据模型 | `internal/model/types.go` | 复制并精简 |
| Store 双方言 | `internal/svc/store.go` | 复制并精简 |

## 12. 与原型的关系

| 操作 | 模块 |
|------|------|
| **复用** | MCP 协议实现、SSE 广播器、数据模型、Store 双方言 |
| **重写** | 注册表（go-zero 风格）、消息代理（收件箱队列模型）、API 层（go-zero handler）、中间件 |
| **新增** | Inbox 收件箱（go-queue）、MCP receive_message/reply_message/heartbeat 工具 |
| **移除** | 内建 Agent 引擎、Bridge Agent、LLM Provider、聊天前端、会话管理、追踪系统、子代理 |
