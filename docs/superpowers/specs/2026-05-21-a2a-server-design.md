# A2A Server — 生产版代理平台设计

> 新项目：`~/work/a2a-server`，从零构建纯 A2A 代理平台。
> 参考：`a2a-platform-go` 原型中可复用的代码。

## 1. 项目定位

平台是一个**透明的消息代理**——不参与 Agent 内部逻辑，只负责"谁在线"和"消息怎么到"。所有 A2A 协议消息经过平台转发，但不解析消息内容的语义。

### 1.1 V1 范围

| 功能 | 说明 |
|------|------|
| Agent 注册与发现 | REST API 注册，AgentCard 抓取，内存注册表 + 数据库持久化 |
| 保活机制 | SSE 长连接 + Poll 心跳双模式，超时自动下线 |
| 消息代理 | JSON-RPC 请求解析 → 目标查找 → HTTP/SSE 透传转发 |
| API + MCP 服务 | REST API 管理 + MCP SSE 端点暴露平台能力 |

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
| 语言 | Go 1.25 | 高并发、单二进制部署、标准库 net/http |
| 服务发现 | 内存 map + 数据库 | V1 简化方案，接口预留后续 etcd 切换 |
| 持久存储 | SQLite（开发）/ MySQL（生产） | 双方言兼容，SQLite 零配置，MySQL 适合生产 |
| 通信协议 | A2A Protocol（JSON-RPC over HTTP/SSE） | 遵循 Google A2A 规范 |
| 工具协议 | MCP（Model Context Protocol） | 标准化工具暴露，兼容 MCP 客户端 |

## 3. 项目结构

```
a2a-server/
├── cmd/server/main.go          # 入口，依赖注入
├── internal/
│   ├── config/                 # 配置加载（YAML + 环境变量）
│   ├── model/                  # 数据模型（Agent、Skill、API 请求/响应类型）
│   ├── store/                  # 存储层接口 + SQLite/MySQL 实现
│   ├── registry/               # Agent 注册表（内存 map + 接口抽象）
│   ├── proxy/                  # 消息代理核心（JSON-RPC 路由、SSE 透传）
│   ├── handler/                # HTTP handler（REST API + MCP SSE）
│   ├── events/                 # SSE 事件广播器
│   └── middleware/             # CORS、认证、限流、日志
├── sql/                        # DDL 文件（SQLite + MySQL）
├── config.example.yaml         # 配置示例
├── Makefile
├── go.mod
└── Dockerfile
```

## 4. 核心组件设计

### 4.1 Registry（注册表）

```go
// registry/registry.go
type AgentEvent struct {
    Type     string  // "register", "deregister", "heartbeat", "offline"
    AgentName string
    Agent    *model.Agent
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
- `Watch()` 返回事件 channel，基于 `events.Broadcaster`

**AgentCard 抓取：**
- 注册成功后，异步 HTTP GET `{AgentURL}/.well-known/agent.json`
- 解析 JSON 存入 `agent.agent_card_json` 字段
- 抓取失败不影响注册，AgentCard 字段留空，状态标记为 `unreachable`

### 4.2 Store（存储层）

```go
// store/store.go
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
- 通过配置文件切换，接口统一

**数据库表（agents）：**

| 字段 | 类型 | 说明 |
|------|------|------|
| id | int64, PK | 自增主键 |
| name | string, UNIQUE | Agent 唯一名称 |
| type | string | Agent 类型（bridge / external） |
| url | string | Agent 服务地址 |
| status | string | 在线状态（connected / offline / unreachable） |
| agent_card_json | text | AgentCard JSON |
| skills_json | text | 技能列表 JSON |
| error_message | string, nullable | 最近错误信息 |
| secret | string | 幂等重注册密钥 |
| connected_at | timestamp | 最近连接时间 |
| created_at | timestamp | 创建时间 |
| updated_at | timestamp | 更新时间 |

**不做 messages 表**——V1 不持久化消息。

### 4.3 Proxy（消息代理）

```go
// proxy/proxy.go
type Proxy struct {
    registry registry.Registry
    client   *http.Client
    timeout  time.Duration  // 默认 180s
}

func (p *Proxy) Forward(ctx context.Context, targetName string, body io.Reader, w http.ResponseWriter) error
```

**Forward 流程：**
1. 从 body 读取 JSON-RPC 请求
2. `registry.Get()` 查找目标 Agent
3. 不存在或 offline → 返回 404
4. 构造 HTTP POST 到 `{targetAgent.URL}/agent`，带 headers：
   - `Accept: text/event-stream`
   - `A2A-Version: 1.0`
   - `Content-Type: application/json`
5. 读取目标 Agent 响应：
   - `Content-Type: text/event-stream` → 逐行透传 SSE chunk
   - 其他 Content-Type → 直接透传响应体
6. 超时保护：180s context timeout

**设计原则：**
- 纯透传，不解析消息语义
- 保留目标 Agent 的原始 Content-Type 和 headers
- 转发错误（目标不可达、超时）返回 JSON 错误响应

### 4.4 Handler（HTTP 层）

**REST API：**

| 方法 | 路径 | 认证 | 说明 |
|------|------|------|------|
| GET | `/health` | - | 平台健康检查（DB 状态 + Agent 数量） |
| POST | `/api/agents` | token | 注册 Agent |
| GET | `/api/agents` | - | 列出所有 Agent（含状态） |
| GET | `/api/agents/{name}` | - | 获取指定 Agent 详情 |
| DELETE | `/api/agents/{name}` | token | 注销 Agent |
| POST | `/api/agents/{name}/heartbeat` | - | 心跳续约 |
| POST | `/agent/{name}` | - | 发送 A2A 消息（透传转发） |
| GET | `/api/events` | - | SSE 事件流 |
| GET | `/.well-known/agent-card/{name}` | - | 获取指定 Agent 的 AgentCard |

**注册请求体：**
```json
{
  "name": "my-agent",
  "type": "bridge",
  "url": "http://localhost:8080",
  "port": 8080,
  "skills": [{"id": "search", "name": "Search", "description": "Web search"}],
  "secret": "optional-secret-for-idempotent-re-registration"
}
```

**认证：**
- 需要 token 的接口：注册、注销
- Header: `X-Admin-Token` 或 `Authorization: Bearer <token>`
- Token 从配置文件读取

### 4.5 MCP 服务

复用原型的 MCP 实现，精简为纯 A2A 平台工具。

**连接流程：**
1. `GET /mcp/sse` → 建立 SSE 连接，返回 `endpoint` 事件（含 session_id）
2. `POST /mcp/messages?session_id=xxx` → 发送 `initialize` 请求
3. 协商能力：协议版本 `2024-11-05`，支持 tools + resources
4. 正常交互：`tools/list`、`tools/call`、`resources/list`、`resources/read`

**暴露的工具：**

| 工具 | 参数 | 说明 |
|------|------|------|
| `list_agents` | - | 列出所有已注册 Agent |
| `send_to_agent` | agent_name, message | 向 Agent 发消息并等待回复 |
| `get_agent_info` | agent_name | 获取 Agent 详情 |

**暴露的资源：**

| URI | 类型 | 说明 |
|-----|------|------|
| `a2a://agents` | application/json | 所有 Agent 列表 |
| `a2a://agents/{name}` | application/json | 单个 Agent 详情 |

**会话模式：**
- 有状态：SSE channel 推送响应
- 无状态：直接 HTTP POST `/mcp/messages`，响应作为 HTTP body 返回

### 4.6 Events（SSE 广播器）

复用原型的 `events/broadcaster.go` 实现。

**事件类型：**
- `agent.online` — Agent 注册成功或心跳恢复
- `agent.offline` — Agent 心跳超时或主动注销
- `agent.card_updated` — AgentCard 更新

### 4.7 Middleware

| 中间件 | 说明 |
|--------|------|
| CORS | 允许跨域请求 |
| Auth | Token 认证（注册/注销接口） |
| RateLimit | 全局限流 100 req/s + 每 IP 20 req/s |
| Logger | 请求日志（method、path、status、duration） |

## 5. 配置

```yaml
server:
  port: 18090
  admin_token: "${ADMIN_TOKEN}"

database:
  driver: sqlite  # sqlite | mysql
  dsn: "./data/a2a.db"  # SQLite: 文件路径; MySQL: "user:pass@tcp(host:3306)/dbname"

heartbeat:
  ttl: 30s        # 心跳超时
  check_interval: 10s  # 扫描间隔

proxy:
  timeout: 180s   # 转发超时

log:
  level: info     # debug | info | warn | error
```

## 6. 从原型复用的代码

| 模块 | 原型路径 | 处理方式 |
|------|----------|----------|
| MCP SSE handler | `internal/handler/mcp_sse.go` | 复制并精简 |
| SSE 广播器 | `internal/events/broadcaster.go` | 复制 |
| 数据模型 | `internal/model/types.go` | 复制并精简（只保留 Agent、Skill、RegisterAgentReq 等） |
| Store 双方言 | `internal/svc/store.go` | 复制并精简（只保留 Agent 相关操作） |
| 配置解析 | `internal/config/config.go` | 复制 |

## 7. 与原型的关系

| 操作 | 模块 |
|------|------|
| **复用** | MCP 服务端、SSE 广播器、数据模型、Store 双方言、配置解析 |
| **重写** | 注册/发现层（简化为内存 map）、消息代理层（纯转发）、API 层（精简接口）、中间件 |
| **移除** | 内建 Agent 引擎、Bridge Agent、LLM Provider、聊天前端、会话管理、追踪系统、子代理 |

## 8. 性能预期

| 指标 | 预期 |
|------|------|
| 消息转发延迟 | < 50ms（同机房） |
| 并发 SSE 连接 | 1000+ |
| Agent 注册数 | 100+ |
| 消息吞吐量 | 1000 msg/s |
