# A2A 代理平台 — 调研与设计总结

> 更新日期：2026-05-21
> 状态：调研阶段

---

## 1. 项目概述

### 1.1 目标

构建一套生产级别的纯 A2A（Agent-to-Agent）代理平台。平台作为 Agent 间通信的中间层，提供注册、发现、保活和消息转发能力。Agent 通过自研 Bridge 对接平台 API 或 MCP 服务来接入。

### 1.2 第一版范围（必须实现）

| 能力 | 说明 |
|------|------|
| Agent 注册与发现 | Agent 通过 REST API 注册到平台，提交 AgentCard，平台维护可用 Agent 列表 |
| 保活机制 | 支持长连接（SSE）和定时 poll 两种保活模式 |
| 消息代理 | 所有 Agent 间消息通过平台转发，遵循 A2A 协议（JSON-RPC `SendStreamingMessage`），平台只做协议层面的转发 |
| API 接口 | REST API 提供 Agent 管理、消息发送等接口 |
| MCP 服务 | 暴露 MCP（Model Context Protocol）端点，提供 `list_agents`、`send_to_agent`、`get_agent_info` 等工具 |

### 1.3 明确不在第一版范围

- Bridge 的具体实现（Agent 自行对接平台 API 或 MCP）
- 内建 Agent（LLM Agent 引擎）
- 会话管理、多轮对话上下文
- 消息追踪（Trace）与可观测性
- 前端管理界面

### 1.4 技术选型

| 层面 | 选择 | 理由 |
|------|------|------|
| 语言 | Go | 高并发、单二进制部署、标准库 `net/http` 足够 |
| 服务发现与保活 | etcd | lease + keepalive 天然支持注册与保活；Watch 机制支持实时发现变更；生产级可靠性与一致性 |
| 存储 | SQLite（开发）/ MySQL（生产） | 双方言兼容，SQLite 零配置本地开发 |
| 通信协议 | A2A Protocol（JSON-RPC over HTTP/SSE） | 遵循 Google A2A 规范 |

---

## 2. 确定要做的 — A2A 代理平台

### 2.1 整体架构

```
┌──────────────────────────────────────────────────────┐
│                   A2A 代理平台                        │
│                                                       │
│  ┌──────────┐  ┌──────────┐  ┌────────────────────┐ │
│  │ REST API │  │ MCP 服务  │  │ etcd 服务发现/保活  │ │
│  │ /api/*   │  │ /mcp/*   │  │ lease + watch      │ │
│  └────┬─────┘  └────┬─────┘  └────────┬───────────┘ │
│       │              │                  │             │
│  ┌────┴──────────────┴──────────────────┴──────────┐ │
│  │              消息代理核心                         │ │
│  │   - A2A 协议解析与转发                           │ │
│  │   - Agent 路由表                                 │ │
│  │   - SSE 流式响应                                 │ │
│  └─────────────────────────────────────────────────┘ │
│                                                       │
│  ┌─────────────────────────────────────────────────┐ │
│  │         存储层 (SQLite / MySQL)                  │ │
│  │   - Agent 注册信息                               │ │
│  │   - 消息转发记录                                 │ │
│  └─────────────────────────────────────────────────┘ │
└──────────────────────────────────────────────────────┘
        ▲                               ▲
        │ A2A Protocol                  │ A2A Protocol
        │ (HTTP/SSE)                    │ (HTTP/SSE)
   ┌────┴────┐                     ┌────┴────┐
   │ Agent A │                     │ Agent B │
   │+ Bridge │                     │+ Bridge │
   └─────────┘                     └─────────┘
```

### 2.2 Agent 注册与发现

**注册流程**：
1. Agent 启动后调用 `POST /api/agents` 提交注册信息（名称、地址、AgentCard）
2. 平台将注册信息写入 etcd（带 TTL lease）
3. 平台抓取 `/.well-known/agent.json` 获取 AgentCard（能力、版本、健康端点）

**发现机制**：
- Agent 调用 `GET /api/agents` 查询在线 Agent 列表
- MCP 工具 `list_agents` 也提供发现能力
- etcd Watch 机制可用于内部组件监听 Agent 变更事件

**为什么用 etcd 而非纯数据库**：
- etcd 的 lease + keepalive 天然适合"注册 + 保活"模式，比自建心跳更可靠
- Watch 机制可以实时感知 Agent 上下线，无需轮询
- 支持集群部署，平台多实例间可共享状态
- 为后续水平扩展预留空间

### 2.3 保活机制

两种模式并存，Agent 可按需选择：

**长连接模式（SSE）**：
- Agent 与平台建立 SSE 长连接 `/api/events`
- 连接断开即触发下线逻辑
- 适合需要实时接收事件的场景

**定时 poll 模式**：
- Agent 定期调用 `POST /api/agents/{name}/heartbeat` 续约
- 续约间隔由 Agent 决定，平台设定超时阈值（如 3 次未续约则下线）
- etcd lease TTL 自动过期处理下线
- 适合无法保持长连接的场景（如 Serverless 环境）

### 2.4 消息代理

**协议**：A2A JSON-RPC over HTTP/SSE

**流程**：
1. 发送方 Agent 向 `POST /agent/{target_name}` 发送 JSON-RPC 请求
2. 平台根据 `{target_name}` 查找目标 Agent 地址
3. 平台将请求转发给目标 Agent
4. 目标 Agent 的响应（SSE 流式）由平台代理回传给发送方

**消息格式**：
```json
{
  "jsonrpc": "2.0",
  "id": "request-1",
  "method": "SendStreamingMessage",
  "params": {
    "message": {
      "role": "ROLE_USER",
      "parts": [{"text": "你好"}]
    }
  }
}
```

**设计原则**：
- 平台只做转发，不解析消息内容语义
- 平台记录消息转发日志（来源、目标、时间戳），用于排查问题
- 不负责会话管理、上下文保持——这些由 Agent 自行处理

### 2.5 API 接口

| 方法 | 路径 | 说明 |
|------|------|------|
| `GET` | `/health` | 平台健康检查 |
| `POST` | `/api/agents` | 注册 Agent |
| `GET` | `/api/agents` | 列出所有在线 Agent |
| `GET` | `/api/agents/{name}` | 获取指定 Agent 详情 |
| `DELETE` | `/api/agents/{name}` | 注销 Agent |
| `POST` | `/api/agents/{name}/heartbeat` | 心跳续约 |
| `POST` | `/agent/{name}` | 向指定 Agent 发送 A2A 消息（代理转发） |
| `GET` | `/api/events` | SSE 实时事件流 |

### 2.6 MCP 服务

**端点**：
- `POST /mcp/sse` — SSE 传输建立
- `POST /mcp/messages` — JSON-RPC 消息处理

**暴露的工具**：

| 工具 | 说明 |
|------|------|
| `list_agents` | 列出所有已注册 Agent |
| `send_to_agent` | 向 Agent 发消息并等待回复 |
| `get_agent_info` | 获取 Agent 详情（能力、版本等） |

**暴露的资源**：
- `a2a://agents` — 所有 Agent 列表
- `a2a://agents/{name}` — 单个 Agent 详情

**用途**：Agent 无需自研 HTTP 客户端对接平台 API，直接通过 MCP 协议（如使用 MCP SDK）即可发现和调用其他 Agent。

---

## 3. 调研内容

以下内容为平台后续演进方向的调研，尚未确定实现方案。

### 3.1 内建 Agent

#### 概念

内建 Agent 是指平台自身提供 LLM 推理能力的 Agent。它不需要外部 Bridge，平台直接调用 LLM API（如 OpenAI、Anthropic），将 LLM 包装为一个 Agent 暴露给其他 Agent 调用。

#### 已实现的调研原型

当前仓库中已有一个功能完整的内建 Agent 实现（`internal/engine/engine.go`），具备：

- 多轮对话能力：通过 `contextId` 维护会话历史
- 工具调用循环：LLM 可调用内置工具（`fetch_url`、`read_file`、`write_file`、`list_directory`）和 A2A 平台工具（`list_agents`、`send_to_agent`、`get_agent_info`）
- MCP 客户端集成：可接入外部 MCP Server 作为额外工具
- 多 Provider 支持：OpenAI 和 Anthropic 流式 API
- SSE 流式响应：`text.delta`、`thinking.delta`、`tool.call_start`、`tool.result` 等事件类型
- Agent 间调用：内建 Agent 可以通过 `send_to_agent` 工具调用平台上的其他 Agent

#### 待决策问题

1. **内建 Agent 是否属于平台核心能力？**
   - 如果是：平台需要管理 LLM API Key、Provider 配置、模型选择
   - 如果不是：内建 Agent 作为独立的 Bridge Agent 存在，平台只做转发

2. **安全边界**：内建 Agent 调用其他 Agent 时，是否需要权限控制？工具调用是否需要沙箱？

3. **资源隔离**：多个内建 Agent 实例并发时的资源限制（并发数、Token 配额、超时）

### 3.2 SubAgent vs AgentTeam

#### SubAgent（子代理）

**概念**：一个 Agent 在执行任务时，生成一个或多个子代理来完成特定的子任务。子代理有独立的上下文，执行完成后将结果返回给父代理。

**特点**：
- 父子关系明确，子代理由父代理创建和管理
- 子代理有独立的执行上下文，不共享父代理的对话历史
- 生命周期短暂：任务完成即结束
- 结果汇报：子代理将结果返回给父代理

**已实现的调研原型**（`internal/tools/subagent.go`）：
- `SubagentEngine`：管理子代理的创建、执行、完成
- `spawn_agent` 工具：Agent 可通过 function call 调用来生成子代理
- 并发限制：最多 3 个并发子代理
- 超时控制：5 分钟超时
- 执行循环：子代理有自己的 LLM 工具调用循环（最多 10 轮）

#### AgentTeam（代理团队）

**概念**：多个 Agent 组成一个团队，共同协作完成复杂任务。团队中的 Agent 角色不同，有明确的分工和协作模式。

**与 SubAgent 的区别**：

| 维度 | SubAgent | AgentTeam |
|------|----------|-----------|
| 关系 | 父子（层级） | 平等协作（扁平）或主从 |
| 生命周期 | 临时，任务完成即销毁 | 持久，可跨任务复用 |
| 上下文 | 独立，不共享 | 可能有共享上下文 |
| 调度 | 父代理直接调度 | 需要编排机制（轮转、投票、协调者） |
| 通信 | 结果单向返回 | 双向交互 |
| 典型场景 | 拆分子任务并行执行 | 多角色协作（如：研究员 + 写手 + 审核员） |

**行业参考**：
- **CrewAI**：AgentTeam 模型，预定义角色和目标，支持顺序、层级、共识等编排模式
- **AutoGen**：Multi-Agent 对话框架，Agent 间可自由对话
- **LangGraph**：基于图的工作流编排，支持循环和条件分支

#### 待调研问题

1. AgentTeam 是否需要在平台层面提供编排能力，还是仅作为 Agent 侧的协作模式？
2. 团队中 Agent 的负载均衡和故障转移如何处理？
3. 团队的生命周期管理（创建、动态加入/退出、销毁）

### 3.3 Agent 交互编排

#### Workflow（工作流）

**概念**：定义 Agent 之间的协作流程，包括执行顺序、条件分支、循环等。

**类型**：

| 类型 | 说明 | 适用场景 |
|------|------|----------|
| 顺序（Sequential） | Agent A → B → C，按顺序执行 | 流水线处理，前一步的输出是后一步的输入 |
| 并行（Parallel） | A、B、C 同时执行，结果汇总 | 独立子任务并行处理 |
| 条件分支（Conditional） | 根据 A 的输出决定走 B 还是 C | 分类、判断类任务 |
| 循环（Loop） | 重复执行某步骤直到满足条件 | 迭代优化、重试 |
| 人工审批（Human-in-the-loop） | 某个步骤需要人工确认 | 需要人类判断的决策点 |

**编排方式对比**：

| 方式 | 说明 | 优点 | 缺点 |
|------|------|------|------|
| 代码编排 | 在 Agent 或 Bridge 代码中硬编码流程 | 灵活，完全可控 | 不可复用，维护成本高 |
| 配置编排 | YAML/JSON 声明工作流定义 | 可视化、可复用 | 灵活性受限 |
| 图编排 | DAG（有向无环图）定义节点和边 | 直观，支持复杂拓扑 | 实现复杂度高 |
| 事件驱动 | Agent 发布/订阅事件，按事件触发 | 松耦合，可扩展 | 调试困难，流程不直观 |

**行业参考**：
- **LangGraph**：基于状态图的编排，节点是函数/Agent，边是条件转移
- **Dify**：可视化工作流编排，支持拖拽节点
- **Google A2A 规范**：目前不包含工作流定义，Agent 间交互是点对点的

#### 待调研问题

1. 第一版是否需要在平台层面支持工作流，还是先让 Agent 自行编排？
2. 如果支持，首选哪种编排方式？配置编排（YAML）的投入产出比最高
3. 工作流的错误处理和重试策略

### 3.4 MCP 集成

#### 已实现的调研

当前仓库已实现完整的 MCP 服务端和客户端：

**MCP 服务端**（`internal/handler/mcp_sse.go`）：
- SSE 传输，协议版本 `2024-11-05`
- 暴露 `list_agents`、`send_to_agent`、`get_agent_info` 工具
- 暴露 `a2a://agents`、`a2a://agents/{name}` 资源
- 支持有状态和无状态两种会话模式

**MCP 客户端**（`internal/mcpclient/client.go`）：
- 支持 stdio 和 SSE 两种传输
- 内建 Agent 可接入外部 MCP Server 扩展工具能力

#### 意义

MCP 集成让平台可以：
- 被任何 MCP 兼容客户端（如 Claude Desktop、Cursor）直接使用
- 让 Agent 无需实现 A2A 客户端，通过 MCP SDK 即可接入平台
- 作为工具层的标准协议，解耦工具提供和工具使用

---

## 4. 技术预期与待决策项

### 4.1 etcd 引入的影响

**优势**：
- 注册、保活、发现的原生支持，减少自建逻辑
- Watch 机制实现实时变更通知
- 支持平台多实例水平扩展
- 分布式锁可用于 Leader 选举（如：全局任务调度）

**挑战**：
- 新增基础设施依赖，部署复杂度上升
- etcd 运维（备份、监控、集群管理）
- 与现有 SQLite/MySQL 存储层的关系：etcd 管运行时状态（注册、保活），数据库管持久化数据（Agent 配置、消息日志）

**待决策**：第一版是否直接引入 etcd，还是先用数据库 + 内存注册表实现，后续再迁移到 etcd？

### 4.2 性能预期

| 指标 | 预期 |
|------|------|
| 消息转发延迟 | < 50ms（同机房） |
| 并发连接数 | 1000+ SSE 长连接 |
| Agent 注册数 | 100+ |
| 消息吞吐量 | 1000 msg/s |

### 4.3 安全性

| 方面 | 考虑 |
|------|------|
| 认证 | Agent 注册需要 token 认证，消息发送暂不强制认证 |
| 授权 | 第一版不做细粒度权限控制，后续可引入 RBAC |
| 传输加密 | 生产环境建议 TLS，平台本身不强制 |
| 消息隔离 | Agent 间消息默认不可被第三方读取 |

### 4.4 部署模式

| 模式 | 适用 | 说明 |
|------|------|------|
| 单二进制 | 开发/测试 | SQLite + 内存注册表 |
| Docker Compose | 小规模生产 | MySQL + etcd（可选） |
| Kubernetes | 大规模生产 | MySQL + etcd + 水平扩展 |

### 4.5 与现有原型的关系

当前仓库（`a2a-platform-go`）是一个功能完整的调研原型，包含了内建 Agent、Bridge、聊天界面等第一版不需要的能力。新建生产平台时需要：

- **复用**：MCP 服务端/客户端实现、A2A 协议解析、SSE 事件广播、数据模型定义
- **重写**：注册/发现层（引入 etcd）、消息代理层（简化为纯转发）、API 层（精简接口）
- **移除**：内建 Agent 引擎、Bridge Agent、聊天界面、会话管理、追踪系统

---

## 5. 开放问题

1. **etcd vs 自建心跳**：第一版是否引入 etcd？还是先用简单的内存注册表 + 数据库持久化？
2. **消息持久化粒度**：平台是否需要记录每条消息的内容？还是只记录转发元数据（来源、目标、时间、状态）？
3. **Bridge 规范**：是否需要为 Bridge 提供标准化的接入规范（如最少需要实现哪些 API）？
4. **内建 Agent 定位**：内建 Agent 是平台核心能力还是可选组件？这决定了架构上的耦合程度
5. **工作流优先级**：Agent 交互编排是否需要在平台层面支持？如果支持，与 Agent 侧自编排的边界在哪？

---

## 6. 参考资料

- [Google A2A 协议规范](https://github.com/google/A2A)
- [Model Context Protocol (MCP) 规范](https://modelcontextprotocol.io/)
- [CrewAI — Multi-Agent 框架](https://github.com/crewAIInc/crewAI)
- [AutoGen — Multi-Agent 对话框架](https://github.com/microsoft/autogen)
- [LangGraph — 基于图的工作流编排](https://github.com/langchain-ai/langgraph)
- [etcd — 分布式键值存储](https://etcd.io/)
