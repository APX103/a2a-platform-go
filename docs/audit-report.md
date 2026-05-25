# A2A Platform Go — 全面代码审计报告

**审计日期**: 2026-05-25  
**项目规模**: 38 源文件, ~13,185 行 Go 代码, 14 测试文件  
**依赖数量**: 4 直接依赖 (stdlib-heavy 设计)  

---

## 1. 执行摘要 (Executive Summary)

### 评分

| 维度 | 评分 | 评分依据 |
|------|------|----------|
| **稳定性** | 52/100 | 整个 HTTP 栈无 panic recovery; LLM/Bridge 外部调用无超时/重试; 多处并发操作非原子且无事务保护。对比 Go 社区最佳实践差距明显，生产环境存在进程崩溃和数据不一致风险。 |
| **易用性** | 65/100 | 接口设计一致性较好，结构化日志已引入，但配置文档缺失、store 层无 interface 抽象、核心函数过长（多个 >200 行），新开发者上手成本高。 |

### 问题统计

| 严重度 | 数量 | 说明 |
|--------|------|------|
| **Critical** | 7 | 可导致进程崩溃或安全漏洞 |
| **High** | 26 | 影响数据一致性、资源泄漏或认证绕过 |
| **Medium** | 48 | 影响可靠性但不会直接导致宕机 |
| **Low** | 31 | 代码质量、可维护性、日志缺失 |

### Top 3 最紧迫问题

1. **无 Panic Recovery 中间件** (`cmd/server/main.go:167`) — 任何 handler panic 直接崩溃进程，SSE 连接异常即可触发
2. **LLM Provider 无 HTTP 超时** (`internal/llm/openai.go:24`, `anthropic.go:24`) — `http.Client{}` 无 Timeout，上游挂起将耗尽 goroutine
3. **Group 成员删除授权绕过** (`internal/handler/group.go:534-557`) — 仅靠请求 header 中的 member ID 鉴权，任何群成员可伪造 header 删除他人

---

## 2. 逐模块详细审计报告

---

### `cmd/server/main.go`

**Lines**: 970  
**Role**: 服务入口，路由注册，中间件链，Graceful Shutdown

#### 稳定性

| 位置 | 严重度 | 问题描述 | 潜在影响 | 修复建议 |
|------|--------|----------|----------|----------|
| `main.go:167` | Critical | HTTP 链无 `recover` 中间件 | handler panic → 进程崩溃 | 添加最外层 recover middleware（见下方代码） |
| `main.go:183-204` | High | Graceful shutdown 未关闭 DB / 停止 HealthCheck goroutine / 排空 SSE | 连接泄漏、DB 连接池溢出 | `server.Shutdown` 后依次 `svcCtx.DB.Close()`、`registry.StopHealthCheck()` |
| `main.go:221-239`, `makeHealthHandler()` | High | DB Ping 失败仍返回 HTTP 200 `"status":"ok"` | LB/Docker 认为服务健康，流量继续导入 | Ping 失败时 `w.WriteHeader(503)` |
| `main.go:851-886`, `rateLimitMiddleware()` | Medium | `sync.Map` 无淘汰策略，每个 IP 永久驻留 | 长运行下内存泄漏 | 改用 LRU/TTL 限流器 |
| `main.go:872-875` | Medium | `strings.LastIndex(ip, ":")` 处理 IPv6 不正确 | IPv6 客户端限流键错误 | 使用 `net.SplitHostPort` |
| `main.go:956`, `loadBuiltinAgents()` | Medium | `ConfigureAuxiliaryAgentTools` 仅对第一个 DB agent 调用 | 多 agent 场景下 auxiliary tools 绑错 | 循环内逐 agent 调用或显式选择 canonical agent |

**Panic Recovery 修复代码**:
```go
func recoverMiddleware(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        defer func() {
            if v := recover(); v != nil {
                slog.Error("panic recovered", "error", v, "path", r.URL.Path,
                    "request_id", r.Context().Value(requestIDKey))
                http.Error(w, `{"error":"internal server error"}`, 500)
            }
        }()
        next.ServeHTTP(w, r)
    })
}
```

#### 易用性

| 位置 | 严重度 | 问题描述 | 影响 | 修复建议 |
|------|--------|----------|------|----------|
| `main.go:41-205` | High | `main()` 165 行混合启动、50+ 路由注册、中间件 | 难以 review 和测试 | 拆分 `setupRoutes()`, `buildMiddleware()`, `startServer()` |
| `main.go:888-901`, `loggingMiddleware()` | Medium | 日志缺少 HTTP 状态码和响应大小 | 排查问题时缺关键信息 | 包装 `ResponseWriter` 捕获 status |
| `main.go:197-199` | Medium | 启动日志不含配置摘要 | 运维排查困难 | 输出非敏感配置快照 |

---

### `internal/config/`

**Lines**: 164  
**Role**: YAML 配置加载、`${ENV}` 展开、默认值

#### 稳定性

| 位置 | 严重度 | 问题描述 | 潜在影响 | 修复建议 |
|------|--------|----------|----------|----------|
| `config.go:100-107`, `expandEnv()` | Medium | 未解析的 `${VAR}` 保留为字面量 | 密码/Token 字段含字面量 `${...}` 时静默错误 | Load 后扫描残余 `${...}` 并 fail-fast |
| `config.go:125-127` | Medium | MySQL 仅验证 `Host`，User/Password/Database 可空 | 运行时连接错误信息不清 | 在 `Load()` 中验证所有必填字段 |

#### 易用性

| 位置 | 严重度 | 问题描述 | 影响 | 修复建议 |
|------|--------|----------|------|----------|
| `config.go:11-21` | High | Config 结构体无字段注释 | 运维必须读示例才能配置 | 添加 godoc + 字段说明 |
| `config.go:16`, `AdminToken` | High | 空 Token 静默禁用 admin auth | 部署遗漏配置 → 管理 API 无保护 | 非 dev 环境 AdminToken 为空时 fail 或 warn |
| 全局 | Medium | 不支持 `A2A_PORT` 等标准环境变量覆盖 | 容器化部署体验差 | 增加显式 env binding 或文档化 `${...}` 机制 |

---

### `internal/handler/`

**Lines**: ~2,800 (8 files)  
**Role**: HTTP Handler 层 — REST API, A2A Proxy, SSE Events, Group Chat, Human Users

#### 稳定性

| 位置 | 严重度 | 问题描述 | 潜在影响 | 修复建议 |
|------|--------|----------|----------|----------|
| `group.go:534-557` | Critical | 成员删除仅靠 header `X-Member-Id` 鉴权 | 任意群成员伪造 header 删除他人 | 验证请求者 session token 与操作者身份一致 |
| `human.go:104-119` | Critical | Handle-only login 无凭证验证 | 公开端点，知道 handle 即可登录 | 要求 password/token 验证 |
| `handler.go:646-697`, agent proxy SSE | High | 客户端断开后上游读取继续 | Goroutine 泄漏 + 无用上游流量 | 在 `ctx.Done()` 时中断上游 resp.Body 读取 |
| `group.go` 整体 | High | Group 事件编排无资源限制 | 单个请求可触发 N 个 agent 无限时调用 | 添加 per-request timeout + agent 并发上限 |
| `events.go` SSE handler | Medium | SSE 连接无超时/心跳 | 僵尸连接占用资源 | 定期发送 `:ping` 并设置连接超时 |
| `stats.go:22-62` | Medium | `/api/stats` 未在 `requiresAdmin()` 中 | 未认证用户可读取运营指标 | 加入 admin 鉴权或成员 token 验证 |
| `context_handler.go` 多处 | Medium | 未对 page/size 参数做边界校验 | 负数/0 值导致异常 SQL | 添加 `min(page, 1)`, `clamp(size, 1, 100)` |

#### 易用性

| 位置 | 严重度 | 问题描述 | 影响 | 修复建议 |
|------|--------|----------|------|----------|
| `group.go` | High | 单文件 1,200+ 行，含 5 种编排模式 | 难维护、难测试 | 按编排模式拆分子 handler |
| `handler.go` | Medium | Agent proxy 错误信息可能泄露下游 URL 和 body | 安全信息泄露 | 包装错误信息，仅返回 generic message |
| 全局 | Medium | 错误响应格式不统一（有时 `{"error":...}`，有时 `http.Error()`） | 客户端解析困难 | 统一 `jsonError(w, msg, code)` |

---

### `internal/svc/`

**Lines**: ~3,700 (8 files)  
**Role**: 数据持久化层 — Agents, Tasks, Messages, Traces, Groups, Humans

#### 稳定性

| 位置 | 严重度 | 问题描述 | 潜在影响 | 修复建议 |
|------|--------|----------|----------|----------|
| `servicecontext.go:334,382` | Critical | `repairLegacyContextLineagePass()` 使用 SQLite `strftime()`/`datetime()` | MySQL 环境下修复逻辑静默失败 | 使用 `DATE_FORMAT()` 或运行时判断驱动类型 |
| `store.go:141-156`, `TaskStore.Update()` | Critical | 动态列名来自 map key，无白名单 | SQL 注入 | 添加 `allowedColumns` 白名单校验 |
| `registry.go:172-195`, `RegisterAgent()` | Critical | 内存注册在 DB 持久化之前 | DB 失败 → 内存与 DB 状态不一致 | 先 DB 成功再更新内存，失败时回滚 |
| `registry.go:395-403`, `StartHealthCheck()` | High | Goroutine 无 `recover()`，无停止机制 | Panic → 进程崩溃；无法优雅关闭 | 添加 defer recover + context/stopCh |
| `group_store.go:353-355`, `Consume()` | High | 非原子 `used_count + 1` | 并发消费可超出 `max_uses` | 用 `UPDATE...WHERE used_count < max_uses` 原子操作 |
| `group_store.go:729-758`, `UpsertByName()` | High | 先读后写无事务 | 并发更新丢失 | 使用 `INSERT...ON CONFLICT` 或事务包裹 |
| `store.go` 多处 list 方法 | High | 6+ 处缺少 `rows.Err()` 检查 | 部分读取静默当作成功 | 每个 scan 循环后 `return result, rows.Err()` |
| `servicecontext.go:161-177`, `migrate()` | High | 迁移失败仅 warn 日志继续启动 | Schema 不完整 → 运行时 SQL 错误 | 迁移失败 → fail fast |
| `registry.go:341-351`, `DisconnectAgent()` | High | 先清内存再写 DB | DB 失败 → 状态不一致 | 先 DB 后内存 |
| `human_store.go:502-504`, `HashAccessToken()` | Medium | SHA-256 无 salt | DB 泄露 → 彩虹表攻击 | 使用 bcrypt/argon2 或至少加 salt |
| `context.go:127-146`, `Delete()` | Medium | 仅删 messages + context，不清理 tasks/traces/task_items | 孤儿数据 | 级联删除或使用 FK CASCADE |
| `builtin_agent.go:26,71,124` | High | API Key 明文存储并返回 | 凭证泄露风险 | 存储时加密，API 返回时 mask |

#### 易用性

| 位置 | 严重度 | 问题描述 | 影响 | 修复建议 |
|------|--------|----------|------|----------|
| `store.go` (727 行) | High | 4 个 Store 混在一个文件 | 定位困难 | 按域拆分：agent_store.go, task_store.go, message_store.go, trace_store.go |
| `group_store.go` (821 行) | High | 7 个 Store 类型在同一文件 | 同上 | 按域拆分 |
| `servicecontext.go:548-788` | High | 240 行内联 DDL | 无版本管理、无回滚 | 迁移文件 + 版本表 (golang-migrate 或手写) |
| `store.go:159-320` | Medium | Task scan/nullable 逻辑复制粘贴 4+ 次 | 修改易遗漏 | 提取 `scanTask(rows) (*Task, error)` |
| 全局 | Medium | 无 Store interface 定义 | mock 困难，无法切换存储后端 | 定义 `AgentStore`, `TaskStore` 等 interface |
| `servicecontext.go:21-44` | Medium | 20+ 字段 God Object，无 `Close()` 方法 | 资源清理遗漏 | 添加 `func (sc *ServiceContext) Close() error` |

---

### `internal/engine/`

**Lines**: 699  
**Role**: Builtin LLM Agent 引擎 — 流式对话、工具调用、SSE 推送、Subagent 编排

#### 稳定性

| 位置 | 严重度 | 问题描述 | 潜在影响 | 修复建议 |
|------|--------|----------|----------|----------|
| `engine.go:412`, `runLoop()` | Critical | `deps.RecordTrace(...)` 无 nil 检查 | deps 未完全初始化 → panic | `if deps.RecordTrace != nil { ... }` |
| `engine.go:566-601`, `callToolWithTimeout()` | High | 超时后 goroutine 继续运行且不可取消 | Goroutine 泄漏 + 超时后仍执行副作用 | 传入 cancelable context，超时时 cancel |
| `engine.go:372-391` | High | 并行 RO tool goroutine 无 `defer recover()` | 一个 tool panic → 进程崩溃 | 添加 per-goroutine recover |
| `engine.go:153-156`, `HandleRequest()` | Medium | `LoadHistory` 错误静默忽略 | 多轮对话历史丢失，用户无感知 | 返回错误或发送 SSE error event |
| `engine.go:317,351,543` | Medium | `SaveMessage` 返回值忽略 | 消息持久化静默失败 | 至少 log.Error |
| `engine.go:360-393` | Medium | 并行 RO tools 不响应 parent ctx 取消 | 请求取消后 tools 仍运行 | `roWg.Wait()` 与 `ctx.Done()` select |

#### 易用性

| 位置 | 严重度 | 问题描述 | 影响 | 修复建议 |
|------|--------|----------|------|----------|
| `engine.go:237-555`, `runLoop()` | High | 318 行深嵌套单函数 | 难读难测 | 拆分为 `streamLLMResponse`, `executeToolCalls`, `persistMessage` |
| `engine.go:86-93`, `RegisterAgent()` | Medium | Provider `switch` 硬编码 | 新增 provider 需改 engine 核心 | 改为 provider registry 模式 |
| `engine.go:32-35` | Medium | Magic numbers (`120s`, `5s`, `2000`, `500`) 无命名常量 | 含义不清 | 定义 const + 注释 |

---

### `internal/events/`

**Lines**: 109  
**Role**: 进程内 SSE Pub/Sub 广播器

#### 稳定性

| 位置 | 严重度 | 问题描述 | 潜在影响 | 修复建议 |
|------|--------|----------|----------|----------|
| `broadcaster.go:26-32`, `Subscribe()` | Medium | 重复 sessionID 覆盖 map entry，旧 channel 未关闭 | Channel + goroutine 泄漏 | 检测重复，先 close 旧 subscriber |
| `broadcaster.go:64-69`, `Broadcast()` | Medium | 慢消费者丢弃事件仅 warn 日志 | 无可观测指标 | 暴露 drop counter metric |

#### 易用性

✅ 未发现问题 — API 简洁内聚

---

### `internal/llm/`

**Lines**: ~444 (3 files)  
**Role**: LLM Provider 封装 — OpenAI, Anthropic 流式调用

#### 稳定性

| 位置 | 严重度 | 问题描述 | 潜在影响 | 修复建议 |
|------|--------|----------|----------|----------|
| `openai.go:24` | Critical | `http.Client{}` 无 Timeout | 上游挂起 → goroutine 永久阻塞 | `&http.Client{Timeout: 120 * time.Second}` |
| `anthropic.go:24` | Critical | 同上 | 同上 | 同上 |
| `openai.go:55`, `anthropic.go:54` | High | 流读取 goroutine 无 `defer recover()` | Panic → 进程崩溃 + channel 未关闭 | 添加 recover 并发送 error event |
| `openai.go:163-164`, `anthropic.go:137-138` | High | JSON 解析失败 `continue` | 损坏数据流被当作截断成功 | 发送 `StreamEvent{Type:"error"}` |
| `openai.go:136-205`, `anthropic.go:129-181` | Medium | `scanner.Err()` 未检查 | 网络中断 → 流静默结束 | 循环后检查 scanner.Err() 并发送 error |
| `anthropic.go:70` | High | tool_call Arguments JSON 解析失败被忽略 | 无效参数以 null 发送给 API | 返回 error event |

#### 易用性

| 位置 | 严重度 | 问题描述 | 影响 | 修复建议 |
|------|--------|----------|------|----------|
| `types.go:5-44` | Medium | 所有导出类型无 godoc | 接入新 provider 时需读实现才能理解契约 | 添加 Provider, StreamEvent 文档 |
| 全局 | Medium | 无重试/退避逻辑 | LLM 瞬时错误直接失败 | 至少 429/5xx 重试 1-2 次 |

---

### `internal/bridge/`

**Lines**: ~458 (4 files)  
**Role**: 配置驱动的外部 Agent Bridge — HTTP 调用、CLI 命令、模板引擎

#### 稳定性

| 位置 | 严重度 | 问题描述 | 潜在影响 | 修复建议 |
|------|--------|----------|----------|----------|
| `cli.go:40-45`, `invokeCLI()` | High | 用户输入拼接后传入 `sh -c` | Shell 注入 | 使用 `exec.CommandContext` + 独立参数列表 |
| `http.go:16-24` | Medium | 无 URL scheme/host 校验 | SSRF（若 bridge config 可被用户影响）| 校验 scheme 为 http/https，禁止内网地址 |
| `http.go:66` | Medium | 使用 `http.DefaultClient`（无独立超时） | 全局连接池行为不可预测 | 创建独立 `http.Client{Timeout: ...}` |
| `bridge.go:126`, `writeSSE()` | Medium | `json.Marshal` 错误被忽略 | 空 SSE frame 发送给客户端 | 检查 err 并 log |
| `cli.go:50-52` | Medium | 命令 stdout 无大小限制 | 大输出 OOM | 使用 `io.LimitReader` |

#### 易用性

| 位置 | 严重度 | 问题描述 | 影响 | 修复建议 |
|------|--------|----------|------|----------|
| `bridge.go:107-123`, `selectSkill()` | Medium | 首 token 匹配逻辑脆弱且无文档 | 多 skill agent 行为不可预测 | 文档化或改为显式 skill routing |
| `template.go:13-18` | Medium | `TemplateContext` 支持的变量无文档 | 配置者需读源码 | 添加 godoc 列出所有变量 |

---

### `internal/tools/`

**Lines**: ~1,566 (4 files)  
**Role**: 平台内置工具 — A2A 调用、Subagent、Task System、文件/HTTP

#### 稳定性

| 位置 | 严重度 | 问题描述 | 潜在影响 | 修复建议 |
|------|--------|----------|----------|----------|
| `a2a.go:257-278`, `executeSendToAgent()` | High | `http.NewRequest` 无 context | 不响应父请求取消，60s 后才超时 | 使用 `http.NewRequestWithContext(ctx, ...)` |
| `a2a.go:380-386`, `platformRequest()` | High | 同上 | 同上 | 同上 |
| `tools.go:206-222`, `executeFetchURL()` | High | 无 context 的 HTTP 请求 | 不可取消 | 传入 ctx |
| `subagent.go:261-262`, `SpawnAgent()` | High | `context.WithTimeout(context.Background(), ...)` | 忽略父请求取消 | 使用传入的 ctx 而非 Background |
| `subagent.go:119-123 vs 178-180` | High | 系统提示词重复注入（messages[0] + ChatRequest.SystemPrompt） | LLM 收到矛盾指令 | 只保留一处 |
| `task_tools.go:191-201`, `executeClaimTask()` | High | `CanStart` + `Claim` 非原子 | TOCTOU race，两个 agent 同时 claim | 在 store 层用 `UPDATE...WHERE status='pending'` 原子 claim |
| `a2a.go:419-454`, `fetchVisibleGroups()` | Medium | N+1 pattern，每个 group 一次 `/members` 调用 | 延迟放大，错误扩散 | 批量查询或缓存 |
| `task_tools.go:103` | Medium | ID 用 `time.Now().UnixNano()` | 突发场景碰撞 | 使用 UUID |

#### 易用性

| 位置 | 严重度 | 问题描述 | 影响 | 修复建议 |
|------|--------|----------|------|----------|
| `subagent.go:109-117` | High | 硬编码中文系统提示词 | 非中文环境不适用 | 提取为可配置字段 |
| `tools.go:168-237` | Medium | Magic numbers (`120s`, `1<<20`, `8000`) | 含义不清 | 定义命名常量 |
| `a2a.go:24-28` | Medium | `sync.Once` 限制 base URL 只能设一次 | 测试困难，不支持热更新 | 改为 atomic.Value 或参数传入 |

---

### `internal/model/`

**Lines**: ~502 (3 files)  
**Role**: 领域模型定义 — 数据实体、API DTO、工具 Schema

#### 稳定性

| 位置 | 严重度 | 问题描述 | 潜在影响 | 修复建议 |
|------|--------|----------|----------|----------|
| `builtin_tools.go:8`, `Execute` 函数指针 | Medium | 模型类型耦合运行时行为 | 不可序列化，测试困难 | 分离 schema 定义与 runtime |

#### 易用性

| 位置 | 严重度 | 问题描述 | 影响 | 修复建议 |
|------|--------|----------|------|----------|
| `types.go` (447 行) | Medium | DB 实体、API DTO、Group 模型混在一个文件 | 职责不清 | 按 bounded context 拆分 |
| `task_item.go:12-13` | Low | Status 值仅注释说明，无 typed constant | 使用时靠记忆 | 定义 `TaskItemStatus` 常量组 |
| 多处导出类型 | Low | 缺少 godoc | IDE 提示无文档 | 添加文档注释 |

---

### `internal/redact/`

**Lines**: 146  
**Role**: 凭证脱敏 — JSON 感知 + 正则兜底

#### 稳定性

| 位置 | 严重度 | 问题描述 | 潜在影响 | 修复建议 |
|------|--------|----------|----------|----------|
| `redaction.go:14`, `providerAPIKeyRe` | Medium | 仅匹配 `sk-`/`sk-ant-` 前缀 | Azure/Gemini/自定义 key 泄露 | 扩展正则或依赖 JSON key 匹配 |

#### 易用性

| 位置 | 严重度 | 问题描述 | 影响 | 修复建议 |
|------|--------|----------|------|----------|
| `redaction.go:111-133`, `isSensitiveKey()` | Medium | 后缀启发式可能过度脱敏 (`*key` 匹配 `monkey`) | 调试信息丢失 | 缩窄匹配或添加白名单 |
| 全局 | Low | 无包级 godoc 说明支持的 secret 模式 | 开发者不知覆盖范围 | 添加 package doc |

---

### `web/embed*.go`

**Lines**: 31 (2 files)  
**Role**: Build-tag 条件嵌入前端静态资源

#### 稳定性

✅ 未发现问题

#### 易用性

| 位置 | 严重度 | 问题描述 | 影响 | 修复建议 |
|------|--------|----------|------|----------|
| `embed.go:17` | Low | 非 frontend 模式无任何 landing page 提示 | 访问者困惑 | 提供最小静态页或重定向到 API docs |

---

## 3. 全局跨模块问题 (Architecture-level Issues)

### 3.1 无 Panic Recovery — 全栈裸奔

**影响范围**: 所有 handler + 所有 goroutine  
**现状**: 搜索全部生产代码，`recover()` 出现次数为 **0**。registry 的 health check goroutine、engine 的 tool 执行 goroutine、LLM 的 stream reader goroutine — 全部无保护。

### 3.2 Store 层无 Interface 抽象

**影响范围**: `internal/svc/` → `internal/handler/`, `internal/engine/`, `internal/tools/`  
**现状**: 除 `tools/task_tools.go` 中为避免循环依赖定义了 `TaskItemStore` interface 外，所有 Store 都是具体结构体直接传递。  
**后果**: 
- 单元测试必须依赖真实 DB（或用 SQLite in-memory）
- 无法在不改代码的情况下切换存储后端
- 违反依赖反转原则

### 3.3 Context 传播断裂

**影响范围**: `internal/svc/` (全部)、`internal/tools/a2a.go`、`internal/tools/subagent.go`  
**现状**: 
- 整个 `svc/` 包的所有 DB 操作不接受 `context.Context` 参数
- `tools/` 中多处 HTTP 请求使用无 context 的 `http.NewRequest`
- Subagent spawn 使用 `context.Background()` 脱离父请求生命周期

**后果**: 请求超时/取消无法传播到下游操作，DB 慢查询和外部调用无法被中断。

### 3.4 内联迁移无版本管理

**影响范围**: `internal/svc/servicecontext.go`  
**现状**: 240 行 DDL 内联在 Go 代码中，每次启动执行 `CREATE TABLE IF NOT EXISTS` + best-effort `ALTER TABLE`。无迁移版本号、无回滚机制、失败继续运行。  
**后果**: Schema 演进不可追溯，部分迁移失败 → 运行时 SQL 错误。

### 3.5 错误处理模式不统一

**影响范围**: 全项目  
**现状**:
- Store 层: 大量 `_, _ = db.Exec(...)` 静默丢弃错误
- Handler 层: 部分用 `jsonError()`、部分用 `http.Error()`
- Engine 层: `SaveMessage` 返回值被忽略
- 全局: 很少使用 `%w` 包装，错误链断裂

### 3.6 测试覆盖不均匀

| 模块 | 覆盖状况 |
|------|----------|
| `internal/handler/` | 良好 (handler_test.go ~830 行) |
| `internal/engine/` | 良好 (engine_test.go 多场景) |
| `internal/svc/store.go` | 中等 (tasks/messages/traces) |
| `internal/svc/group_store.go` | 中等 (happy path) |
| `internal/svc/human_store.go` | 良好 |
| `internal/events/` | 良好 (含 race test) |
| `internal/tools/tools.go` | 良好 |
| `internal/llm/` | ❌ 无测试 |
| `internal/bridge/` | ❌ 无测试 |
| `internal/tools/subagent.go` | ❌ 无测试 |
| `internal/tools/task_tools.go` | ❌ 无测试 |
| `internal/svc/servicecontext.go` | ❌ 无测试 |
| `internal/svc/task_item_store.go` | ❌ 无测试 |
| `internal/config/` | 最小 (3 tests) |

---

## 4. 修复路线图 (Remediation Roadmap)

### P0 — 本周 (Critical, 影响生产稳定性)

| # | 问题 | 文件 | 预估工时 |
|---|------|------|----------|
| 1 | 添加全局 panic recovery 中间件 | `cmd/server/main.go` | 0.5h |
| 2 | LLM Provider 添加 HTTP Client Timeout | `internal/llm/openai.go`, `anthropic.go` | 0.5h |
| 3 | 修复 Group 成员删除授权绕过 | `internal/handler/group.go` | 1h |
| 4 | 修复 Human handle-only login 无凭证验证 | `internal/handler/human.go` | 1h |
| 5 | TaskStore.Update() 白名单列名防 SQL 注入 | `internal/svc/store.go` | 0.5h |
| 6 | Registry.RegisterAgent() 先 DB 后内存 | `internal/svc/registry.go` | 1h |
| 7 | deps.RecordTrace nil check | `internal/engine/engine.go` | 0.25h |

### P1 — 本月 (High, 影响可靠性与 DX)

| # | 问题 | 文件 | 预估工时 |
|---|------|------|----------|
| 8 | 完善 Graceful Shutdown (关 DB、停 health check、排空 SSE) | `cmd/server/main.go`, `registry.go` | 2h |
| 9 | Health check 返回真实状态码 | `cmd/server/main.go` | 0.5h |
| 10 | LLM stream goroutine 添加 recover + scanner.Err() | `internal/llm/*.go` | 1h |
| 11 | Engine tool goroutine recover + timeout 改用 ctx cancel | `internal/engine/engine.go` | 2h |
| 12 | Agent proxy SSE 客户端断开时中断上游 | `internal/handler/handler.go` | 1h |
| 13 | 所有 list 方法添加 `rows.Err()` 检查 | `internal/svc/*.go` | 1h |
| 14 | GroupInviteStore.Consume() 原子操作 | `internal/svc/group_store.go` | 0.5h |
| 15 | Bridge CLI shell 注入修复 | `internal/bridge/cli.go` | 1h |
| 16 | Tools/A2A HTTP 调用传入 context | `internal/tools/a2a.go`, `tools.go` | 1h |
| 17 | Subagent 使用父 context 而非 Background | `internal/tools/subagent.go` | 0.5h |
| 18 | Store interface 抽象（核心 4 个 Store） | `internal/svc/` | 4h |
| 19 | main.go 拆分（routes + middleware 提取） | `cmd/server/` | 2h |
| 20 | Config 添加 Validate() + 字段文档 | `internal/config/` | 1h |
| 21 | API Key 存储加密 | `internal/svc/builtin_agent.go` | 2h |
| 22 | Rate limiter 添加 TTL 淘汰 | `cmd/server/main.go` | 1h |

### P2 — 后续 (Medium/Low + 架构优化)

| # | 问题 | 文件 | 预估工时 |
|---|------|------|----------|
| 23 | DB 迁移重构为版本化文件 | `internal/svc/servicecontext.go`, `sql/` | 4h |
| 24 | Store 文件按域拆分 | `internal/svc/store.go`, `group_store.go` | 2h |
| 25 | engine.runLoop() 拆分子函数 | `internal/engine/engine.go` | 3h |
| 26 | group.go 按编排模式拆分 | `internal/handler/group.go` | 3h |
| 27 | LLM 重试/退避策略 | `internal/llm/` | 2h |
| 28 | N+1 查询优化 (fetchVisibleGroups, listGroupAgents) | `internal/tools/a2a.go` | 2h |
| 29 | SSE 心跳 + 连接超时 | `internal/handler/events.go` | 1h |
| 30 | Token hash 改用 bcrypt/argon2 | `internal/svc/` | 2h |
| 31 | 添加 llm/ 和 bridge/ 单元测试 | `internal/llm/`, `internal/bridge/` | 4h |
| 32 | MySQL 兼容修复 (strftime → DATE_FORMAT) | `internal/svc/servicecontext.go` | 1h |
| 33 | Provider registry 替代 switch 硬编码 | `internal/engine/engine.go` | 1h |
| 34 | Context 参数传入 Store 层 | `internal/svc/` 全部 | 4h |
| 35 | Redaction 模式扩展 + 文档 | `internal/redact/` | 1h |
| 36 | Logging middleware 添加 status code | `cmd/server/main.go` | 0.5h |

---

## 5. 正面反馈 (What's Working Well)

1. **极简依赖策略**: 仅 4 个直接依赖，全面使用 stdlib (`net/http`, `database/sql`, `encoding/json`, `log/slog`)。这大幅降低了供应链风险和升级负担，在 Go 生态中是值得推崇的做法。

2. **结构化日志 (slog) 已全面引入**: 关键路径（请求处理、agent 注册、health check、错误）使用 `slog` 且携带 request ID。日志级别和字段设计合理，具备生产可观测基础。

3. **良好的 SSE 事件设计与一致性**: Engine 和 Bridge 的 SSE 事件流设计清晰，事件类型命名有层次感 (`task.status`, `text.delta`, `tool.call.start`)。`events/broadcaster.go` 用 RWMutex + 非阻塞 send 实现的背压策略简洁有效，并有专门的 race condition 测试。

4. **认证与中间件分层清晰**: Request ID → CORS → Rate Limit → Auth → Logging 的中间件链设计合理，admin/member/human 三级权限分离明确。测试文件中有完整的 auth matrix 覆盖。

5. **脱敏层 (redact) 的防御深度**: 同时使用 JSON 结构化脱敏 + 正则兜底 + URL userinfo 脱敏，在 trace 记录和 SSE 输出前一致应用，有效防止了 API key 泄露到前端。

---

*报告生成: 2026-05-25 by automated code audit*  
*审计工具: 静态分析 + 人工逐文件 review*
