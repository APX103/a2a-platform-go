# A2A Platform 生产完备性验收与加固闭环设计

> 目标环境：内网可信环境上线。
> 设计目标：让现有功能有可验证的完备性证据，并让不可信 Agent、异常输入和错误运行状态不能击穿平台边界。

## 1. 管理摘要

本设计不把“生产化”理解为部署脚本或 UI 包装，而是建立一个闭环：

1. 从 README、USAGE、当前架构文档和代码入口抽取平台已经承诺的功能、协议和权限边界。
2. 为每条承诺定义可验证的 contract、风险等级、证据和状态。
3. 用测试和最小必要加固把 P0/P1 风险推进到 `verified`。
4. 对尚未完整实现的能力诚实标注 `doc-partial` 或 `planned`，避免文档比系统能力跑得更快。

第一阶段的核心判断标准是：接入平台的 Agent 即使返回坏 JSON、乱序 SSE、超大响应、慢响应、断连、错误状态码，或者试图跨 group 探测和调用其他 Agent，平台也不能崩溃、挂死、泄露 token/secret、破坏业务数据一致性。

## 2. 目标与非目标

### 2.1 目标

- 建立 canonical 验收账本，覆盖当前文档和代码承诺的核心能力。
- 补齐 P0/P1 场景的自动化证据，优先是单元测试、handler/contract 测试和恶意 Agent e2e 测试。
- 修复验证暴露出的崩溃、挂死、越权、敏感数据泄露和核心数据不一致问题。
- 同步更新 `docs/architecture/current-architecture.html`，让架构文档描述当前实现，并明确 planned/partial 兼容工作。
- 保持改造局部、可回滚、可逐步验收，不重写整体系统。

### 2.2 非目标

- 不在第一阶段引入公网账号体系、OAuth、多租户、计费或 Kubernetes 运维平台。
- 不重写 Go 单体、Registry、Engine、Group 或 Admin UI 架构。
- 不追求一次性实现完整 A2A 标准所有方法；只把当前承诺和核心兼容缺口纳入验收。
- 不做无关 UI 美化、文档重排或大规模目录整理。

## 3. 验收矩阵

新增 `docs/production-readiness/acceptance-matrix.md` 作为生产完备性总账本。矩阵不是泛泛 checklist，而是功能承诺、风险和证据的索引。

每一行至少包含：

| 字段 | 含义 |
|------|------|
| Capability | 功能、协议或边界，例如外部 Agent 注册、静态 AgentCard 托管、member token group scope。 |
| Source | 承诺来源，指向 README、USAGE、架构文档、API handler 或测试文件。 |
| Contract | 可验证行为，用 HTTP/API/数据状态描述，而不是口号。 |
| Risk | `P0`、`P1`、`P2`。 |
| Evidence | 已有测试、新增测试、人工检查或文档标注。 |
| Status | `verified`、`missing-test`、`implementation-gap`、`doc-partial`、`planned`。 |
| Owner module | 主要代码边界，例如 `cmd/server`、`internal/handler`、`internal/engine`、`internal/tools`。 |

### 3.1 第一阶段分类

矩阵第一阶段覆盖 8 类能力：

1. Agent 注册与发现：发现式注册、静态注册、AgentCard 代理路径、健康检查、simple-mode。
2. A2A 消息代理：JSON-RPC 解析、context mode、SSE、外部 Agent 异常、Bridge 异常、Builtin Agent 异常。
3. 任务与审计链路：task、message、trace、root context、parent task 的一致性。
4. Group 权限边界：成员可见性、跨 group 禁止、P2P 限制、invite/member token。
5. Human Client 身份：human session、last_seen、默认 group、token 签发和撤销。
6. 敏感数据保护：admin token、member token、human token、agent secret、builtin API key、日志和响应脱敏。
7. 内建工具安全：`fetch_url`、文件读写、A2A tools、subagent/task tools 的边界和资源限制。
8. 运行可靠性：超时、限流、body size、SSE 断连、DB 失败、迁移失败、启动和关闭。

### 3.2 完备定义

一项能力只有满足以下条件之一，才算在账本中闭环：

- `verified`：实现存在，且有自动化测试或明确人工验证证据。
- `doc-partial`：实现仅覆盖部分能力，文档明确列出当前支持范围和未支持范围。
- `planned`：不是第一阶段承诺，文档明确它是后续工作，不作为当前生产能力销售或依赖。

P0 项不能以 `missing-test`、`implementation-gap` 或未说明状态结束第一阶段。

## 4. 风险分级

### 4.1 P0

P0 是会让内网生产不可相信的问题，第一阶段必须处理到 `verified`。

- 进程崩溃或挂死：外部 Agent、Bridge 或 Builtin LLM 返回坏 JSON、坏 SSE、超长响应、半开连接、慢响应、非 2xx 或错误 content-type 时，平台不能 panic、无限阻塞或泄露 goroutine。
- 认证绕过或越权：member token 不能读写其他 group；Agent 不能通过 `/agent/{name}`、AgentCard、A2A tools 或 group events 越界发现或调用非成员 Agent；Admin-only API 必须统一覆盖。
- 敏感数据泄露：API 响应、trace、SSE、日志、Admin 页面和错误消息不能泄露 admin token、member token、human token、builtin API key、agent secret，除明确的一次性签发或 Admin-only credential 读取外。
- 业务数据损坏：失败调用不能留下互相矛盾的 task、message、trace 或 group event 状态；超时和取消要有可解释的失败记录。
- 核心文档承诺失真：README、USAGE 或架构文档明确承诺的核心能力如果不可用，要么修复，要么标注 partial/planned。

### 4.2 P1

P1 应在第一阶段尽量处理；如果规模较大，必须进入矩阵并有后续计划。

- body size、SSE event size、tool result size 和响应体大小限制不统一。
- Bridge CLI/HTTP 的超时、退出码、stderr、输出截断和错误分类不清楚。
- 启动迁移失败只记录 warn/debug 后继续，可能导致半可用状态。
- 默认配置中的 token/password 容易被误用为生产秘密。
- `fetch_url` 存在 SSRF 风险，文件工具和内建工具启用策略需要更明确。
- contract/e2e 测试入口不够标准化，生产验收容易依赖手工验证。

### 4.3 P2

P2 是可维护性或体验问题，不阻塞第一阶段上线判断，但应进入后续路线图。

- `cmd/server/main.go`、`internal/svc/servicecontext.go`、`internal/handler/handler.go`、`internal/handler/group.go` 等文件较大，后续应按职责拆分。
- Admin 和 Human Client 的 token 存储可在更高安全等级下迁移到 httpOnly cookie 或 BFF 模式。
- 更完整的公网安全、租户隔离、配额和审计导出能力。

## 5. 验证策略

### 5.1 单元测试

覆盖纯逻辑和边界函数：

- auth 判定、token 解析、group scope、Admin-only endpoint 判定。
- 路径解析、JSON-RPC method dispatch、AgentCard URL 重写。
- 脱敏函数、响应截断、body size 限制、迁移 runner 行为。

单元测试必须纳入 `make test`。

### 5.2 Handler 与 contract 测试

使用 `httptest` 和本地 fake service 验证 HTTP contract：

- public、Admin-only、member-only endpoint 权限矩阵。
- `/agent/{name}` 对坏 JSON-RPC、未知 method、缺字段、超大 body 的响应。
- AgentCard 只暴露平台代理 URL，不泄露上游 URL、secret 或内部配置。
- task/message/trace 在成功、失败、超时、取消时保持一致。
- 错误响应和日志输出不包含敏感字段。

### 5.3 恶意 Agent e2e 测试

新增或扩展 `tests/e2e`，启动一组本地 fake agents：

- 返回无效 JSON。
- 发送格式错乱的 SSE。
- 中途断连。
- 一直不返回直到超时。
- 返回超大事件或超大 body。
- 在文本、metadata、error 中回显 token/secret。
- 声称支持文档中没有的能力。

平台期望结果：

- 不崩溃、不挂死、不越权、不泄露敏感数据。
- 调用方收到可解释的错误。
- task/message/trace/group event 形成一致的失败记录。

### 5.4 文档-实现一致性审计

从 README、docs/USAGE.md、docs/architecture/current-architecture.html 抽取承诺，并在 acceptance matrix 中绑定证据。

如果某个能力当前只部分实现，例如完整 A2A JSON-RPC 方法集，文档必须明确当前支持的方法和后续 planned 方法。

## 6. 第一阶段交付物

第一阶段完成后，应有以下产物：

- `docs/production-readiness/acceptance-matrix.md`：生产完备性总账本。
- P0/P1 contract 测试与恶意 Agent 测试。
- 必要的安全和可靠性修复。
- 更新后的 `docs/architecture/current-architecture.html`。
- README/USAGE 中被验收发现不准确的部分修正。
- 一个标准验证命令集合，至少包括：
  - `make test`
  - Admin 前端 build
  - Human Client build
  - P0 contract/e2e 测试入口

## 7. 后续 implementation plan 结构

后续计划应按闭环拆任务，而不是按“安全”“测试”“文档”孤立拆分。建议任务顺序：

1. 建立 acceptance matrix 初版，抽取当前文档和代码承诺。
2. 建立 P0 contract 测试框架和 fake malicious agents。
3. 扫描敏感数据出口，设计统一脱敏和一次性 token/secret 展示规则。
4. 加固 `/agent/{name}`、AgentCard、Bridge、Builtin Engine 的异常路径和超时路径。
5. 加固 group/member/Admin 权限矩阵。
6. 修复 task/message/trace/group event 失败状态一致性。
7. 更新 README、USAGE 和 architecture，让 partial/planned 能力被清楚标注。
8. 运行完整验证并把 P0 项推进到 `verified`。

## 8. 成功标准

第一阶段成功不是“所有理想生产能力都完成”，而是满足：

- 所有 P0 项在 acceptance matrix 中为 `verified`。
- P1 项至少有明确状态、证据或后续计划。
- 平台在恶意 Agent、坏输入、断流、超时和越权尝试下不崩溃、不泄露、不损坏核心数据。
- 文档不再把 partial/planned 能力写成当前已完整支持。
- 后续开发可以围绕矩阵持续推进，而不是靠记忆和感觉判断生产完备性。
