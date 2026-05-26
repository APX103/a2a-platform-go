# A2A Platform Go — 审计复核与修复状态报告

**初始审计日期**: 2026-05-25
**复核日期**: 2026-05-26
**复核范围**: `codex/production-hardening-audit-remediation` 分支，覆盖服务生命周期、权限边界、外部调用、流式读取、Store 一致性和文档准确性。
**规模口径**: 初始审计草稿的源文件数量偏低。2026-05-25 复核基线为 53 个 Go 文件、14 个 Go 测试文件；当前修复分支加入 bridge 测试后为 54 个 Go 文件、16 个 Go 测试文件。

---

## 1. 状态模型

| Status | Meaning |
|--------|---------|
| Confirmed fixed | Verified against current code and fixed in this remediation. |
| Confirmed deferred | Verified but intentionally outside this remediation, with reason. |
| Corrected | Original finding was directionally useful but evidence, severity, or wording was wrong. |
| Rejected | Current code does not match the finding. |

---

## 2. 复核结论

原始审计方向总体有价值，但部分证据、严重性和修复建议需要修正。尤其是 Human 裸 handle 登录：它不是本轮要移除的漏洞，而是产品上有意保留的便利身份入口；它应被文档化为可信协作场景下的 passwordless convenience identity，而不是强账户认证。

本轮已完成的 P0/P1 修复集中在：

- HTTP panic recovery、health 真实状态、stats 管理边界和优雅关闭。
- Group 成员删除授权、invite 原子消费与 token 返回一致性。
- Store SQL 白名单、RowsAffected/rows.Err、核心迁移失败处理和 registry DB/内存顺序。
- LLM stream 超时、panic/error 事件、截断检测和 SSE data 解析。
- Engine/tool/subagent context 传播、panic recover 和 nil guard。
- Bridge HTTP URL 校验、专用 client timeout、CLI stdout 上限和 CLI args 安全传递。

---

## 3. 必须更正的原始发现

| 原始主题 | Status | 复核后结论 |
|----------|--------|------------|
| 项目规模统计 | Corrected | 原始统计偏低。2026-05-25 复核基线为 53 个 Go 文件、14 个 Go 测试文件；2026-05-26 当前分支为 54 个 Go 文件、16 个 Go 测试文件。 |
| Human 裸 handle 登录 | Corrected | 裸 handle 登录保留是明确产品要求。它是可信协作场景的便利身份，不是强账户认证；文档和 readiness matrix 应按此口径表达。 |
| Group 成员删除 | Corrected | 问题不是任意伪造 header 删除任意对象，而是有效的同 group member token 曾可删除同 group 其他成员。本轮已修为 member 只能删自己，Admin 可删任意成员。 |
| Panic recovery 范围 | Corrected | 风险应限定到生产 HTTP/goroutine 路径。测试代码中出现 `recover` 不应被算作生产保护。当前生产 HTTP 链和相关 goroutine 已补保护。 |
| Auxiliary tools 绑定 | Corrected | 风险应限定为 DB-loaded builtin agents 的 auxiliary tool 配置路径，不应泛化为所有 builtin agent。 |
| TaskItem claim | Corrected | 原始说法过度简化。真实问题是 claim 未检查 `RowsAffected`，且依赖检查与 claim 不是同一原子事务。本轮已补 `RowsAffected`，事务化依赖检查仍作为后续一致性增强。 |

---

## 4. 修复状态清单

### `cmd/server`

| Issue | Status | Evidence / Current State |
|-------|--------|--------------------------|
| Handler panic can crash process | Confirmed fixed | Added recovery middleware and regression test `TestRecoverMiddlewareReturnsJSON500`. |
| Graceful shutdown does not close service resources | Confirmed fixed | Shutdown now waits for HTTP shutdown and calls `ServiceContext.Close()` to stop registry health checks and close DB resources. |
| `/health` returns healthy when DB ping fails | Confirmed fixed | `/health` now returns HTTP 503 with degraded DB status on ping failure. |
| `/api/stats` lacks admin boundary | Confirmed fixed | Stats is protected by the admin route matrix. |
| Response logging lacks status/size | Confirmed fixed | Logging wrapper records status and size while preserving optional flush behavior. |
| Rate limiter has no TTL eviction | Confirmed deferred | Still valid, but outside this remediation because it is lower risk than auth/lifecycle/data consistency fixes. |
| IPv6 rate-limit key parsing | Confirmed deferred | Still valid; defer with rate limiter cleanup. |
| Large `main.go` routing/bootstrap responsibility | Confirmed deferred | Maintainability concern only; no behavior change in this remediation. |

### `internal/handler`

| Issue | Status | Evidence / Current State |
|-------|--------|--------------------------|
| Same-group member token can remove another member | Confirmed fixed | Member tokens can remove only their own bound actor; admin token can remove any member. |
| Group invite consume is non-atomic | Confirmed fixed | Invite consume, member upsert, token creation, and response member reads happen in one transaction. |
| Human passwordless handle login | Corrected | Preserved by requirement. It is documented as convenience identity, not strong authentication. |
| Client disconnect handling for external proxy SSE | Confirmed deferred | Not in this remediation; still a valid reliability follow-up. |
| Group event orchestration resource limits | Confirmed deferred | Valid follow-up for per-request timeout and fan-out caps. |
| SSE events heartbeat/connection timeout | Confirmed deferred | Valid follow-up; current priority was panic and cancellation safety. |
| Context pagination bounds | Confirmed deferred | Valid follow-up. |
| Group handler size/refactor | Confirmed deferred | Maintainability-only follow-up. |
| Mixed error response formats | Confirmed deferred | Valid API polish item, not required for this production-hardening pass. |

### `internal/svc`

| Issue | Status | Evidence / Current State |
|-------|--------|--------------------------|
| `TaskStore.Update` dynamic column names | Confirmed fixed | Update keys are whitelisted and sorted before SQL construction. |
| `RegisterAgent` updates memory before DB | Confirmed fixed | DB write succeeds before memory/event state is updated. |
| `DisconnectAgent` clears memory before DB | Confirmed fixed | DB state update happens before memory state change. |
| Registry health check cannot stop and lacks panic protection | Confirmed fixed | Health checks are cancelable/stoppable and recover from panic. |
| Core migration errors are warnings only | Confirmed fixed | Core schema migration errors are fatal; compatibility ALTER/backfill remains best-effort. |
| Legacy lineage repair uses MySQL-incompatible date SQL | Confirmed fixed | Repair SQL now uses MySQL-compatible expressions. |
| Store list methods miss `rows.Err()` | Confirmed fixed | Audited list/scan loops now check `rows.Err()`. |
| Task item claim success is not checked | Confirmed fixed | Claim checks affected rows before reporting success. |
| Task item dependency check and claim are separate steps | Confirmed deferred | Still a possible TOCTOU follow-up; current remediation fixed false-success claim behavior. |
| Human token hashing uses unsalted SHA-256 | Confirmed deferred | Valid credential-hardening follow-up. |
| Builtin API key storage/return masking | Confirmed deferred | Valid secret-management follow-up outside this pass. |
| Store interface abstraction | Confirmed deferred | Testability/refactor follow-up. |
| Versioned migrations | Confirmed deferred | Architecture follow-up; current code remains inline migrations with fail-fast core schema. |

### `internal/llm`

| Issue | Status | Evidence / Current State |
|-------|--------|--------------------------|
| Provider clients have no HTTP timeout | Confirmed fixed | OpenAI and Anthropic clients use bounded timeouts. |
| Stream reader panic can crash or leave channels open | Confirmed fixed | Stream goroutines recover and emit error events. |
| Malformed stream JSON is silently skipped | Confirmed fixed | Non-empty malformed JSON now produces stream error events. |
| Scanner errors are ignored | Confirmed fixed | Scanner errors are reported. |
| EOF before provider terminal marker looks successful | Confirmed fixed | Truncated streams produce errors unless provider terminal markers are observed. |
| Compact SSE `data:` frames are skipped | Confirmed fixed | Compact frames are parsed. |
| Retry/backoff for transient LLM errors | Confirmed deferred | Valid resilience follow-up. |
| Provider registry instead of switch | Confirmed deferred | Maintainability follow-up. |

### `internal/engine` and `internal/tools`

| Issue | Status | Evidence / Current State |
|-------|--------|--------------------------|
| `RecordTrace` nil callback panic | Confirmed fixed | Engine guards nil callbacks. |
| Tool timeout leaves goroutine uncanceled | Confirmed fixed | Tool calls derive cancellable child contexts and call `ExecuteContext` where implemented. |
| Tool goroutine panic can crash process | Confirmed fixed | Tool execution paths recover and return errors. |
| Tool definitions with nil executors panic | Confirmed fixed | Nil executor paths return explicit errors. |
| A2A/fetch/subagent helpers ignore parent context | Confirmed fixed | HTTP helper tools and subagent spawning propagate request context. |
| Subagent nested timeout ignores parent cancellation | Confirmed fixed | Nested subagent execution uses parent context. |
| Subagent prompt duplication | Confirmed deferred | Valid behavior review follow-up; not required for safety pass. |
| Store-level DB calls lack context parameters | Confirmed deferred | Valid larger API refactor. |

### `internal/bridge`

| Issue | Status | Evidence / Current State |
|-------|--------|--------------------------|
| HTTP bridge accepts unsupported URL schemes or missing hosts | Confirmed fixed | `validateBridgeURL` allows only `http`/`https` and requires `Host`. |
| HTTP bridge uses default client | Confirmed fixed | Each invocation uses a dedicated `http.Client{Timeout: ...}`. |
| CLI bridge returned stdout without a size cap | Confirmed fixed | Returned stdout is capped at 1MiB before trimming. |
| CLI bridge args are shell-concatenated | Confirmed fixed | CLI args are passed as shell position parameters; regression test keeps `$()` literal. |
| CLI command strings are operator-trusted shell snippets | Corrected | Shell mode remains intentionally supported for trusted operator config; strict user-input safety depends on using `args`, not interpolating user text into `command`. |
| `writeSSE` ignores `json.Marshal` error | Confirmed deferred | Valid follow-up. |
| Skill routing by first token is fragile | Confirmed deferred | Valid product/API follow-up. |

### Other Modules

| Issue | Status | Evidence / Current State |
|-------|--------|--------------------------|
| `internal/events` duplicate subscriber leak/drop metrics | Confirmed deferred | Valid operational follow-up. Race-sensitive tests remain covered. |
| Config validation for unresolved env/user/password/db | Confirmed deferred | Valid deploy-safety follow-up. |
| Redaction provider key coverage | Confirmed deferred | Valid future hardening item. |
| `internal/llm` tests absent | Rejected | LLM stream regression tests now exist. |
| `internal/bridge` tests absent | Rejected | Bridge safety regression tests now exist. |

---

## 5. Current Residual Risks

| Risk | Status | Rationale |
|------|--------|-----------|
| Passwordless Human handle login is not strong authentication | Confirmed deferred | Intentional convenience identity for trusted collaboration flows. Keep it documented and do not market it as account security. |
| Bridge CLI shell snippets can execute operator-provided shell code | Confirmed deferred | This is an intentional operator config capability. User-controlled values must be passed through `args`; configs that interpolate user text into `command` remain trusted-shell snippets. |
| Inline migrations remain non-versioned | Confirmed deferred | Core schema now fails fast, but migration versioning is a larger architecture task. |
| Store DB APIs still mostly lack context parameters | Confirmed deferred | External calls now propagate cancellation; DB context propagation needs a broader store API change. |
| Rate limiter TTL and IPv6 parsing remain unaddressed | Confirmed deferred | Operational hardening follow-up. |

---

## 6. Verification To Run Before Merge

```bash
go test ./...
go test -race ./internal/svc ./internal/events
git status --short
```

Expected:

- The two `go test` commands pass.
- The stale-text check from the remediation plan prints no output.
- Working tree is clean after commits.
