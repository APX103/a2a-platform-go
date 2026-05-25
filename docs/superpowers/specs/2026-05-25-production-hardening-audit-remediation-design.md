# Production Hardening Audit Remediation Design

Date: 2026-05-25

## Purpose

`docs/audit-report.md` contains a broad audit of the current A2A Platform Go codebase. The report is directionally useful, but several claims are stale, imprecise, or mix product choices with bugs. This work will verify the report against the current code, correct the report, and implement the confirmed security, reliability, and data-consistency fixes as one production-hardening effort.

The goal is not a cosmetic refactor. The goal is to make the current implementation safer to operate, harder to misuse, and easier to reason about under failure.

## Confirmed Scope

This remediation includes:

- HTTP lifecycle hardening: panic recovery, health status correctness, request logging fields, graceful shutdown, and resource cleanup.
- Authorization and identity boundaries: stats admin protection, group member deletion rules, group member token scope, and invite consumption correctness.
- Human identity documentation: preserve passwordless handle login because it is an intentional convenience feature, while documenting its security model honestly.
- External call safety: LLM, bridge, tool, subagent, and A2A helper timeout/cancel behavior.
- Goroutine and stream safety: recover guards where goroutines execute untrusted or tool-controlled code, scanner error handling, stream parse error reporting, and parent context propagation.
- Data layer correctness: dynamic SQL field whitelisting, DB-before-memory registry state changes, rows.Err checks, MySQL-compatible lineage repair SQL, and migration failure handling.
- Focused architecture cleanup that supports the fixes: `ServiceContext.Close()`, stoppable registry health checks, narrow store interfaces where they remove test coupling, and a lightweight migration runner/version record for migration steps that need state tracking.
- Documentation updates: a corrected audit report and architecture updates for changed auth, lifecycle, schema, migration, and orchestration behavior.

This remediation excludes broad file shuffling and whole-project interface conversion unless a specific bug fix requires it.

## Product Decision: Human Handle Login

Human login must continue to support bare handle login. This is a product requirement for low-friction human-client use.

The audit report currently treats handle-only login as an unconditional Critical vulnerability. The corrected position is:

- Passwordless handle login is an intentional identity model, not an accidental endpoint exposure.
- It is not strong authentication. Anyone who knows a handle can assume that human identity.
- The architecture and audit documents must label it as a convenience identity suitable for trusted or low-risk collaboration flows.
- Tests must preserve handle login behavior so a security hardening pass does not silently remove it.
- Optional future hardening can add a config switch or deployment warning, but this remediation must not force token/password login by default.

## Architecture

### HTTP Lifecycle

Add an outer recovery middleware around the existing middleware chain. It should convert panics into a JSON 500 response, log request ID, method, path, and panic value, and avoid leaking stack or internal data to callers.

Change `/health` so DB ping failure returns HTTP 503 and an explicit degraded status. Keep the response JSON simple and compatible with existing callers.

Move lifecycle ownership into `ServiceContext.Close()` and `AgentRegistry.StopHealthCheck()`. Shutdown should call `server.Shutdown(ctx)`, stop the registry health loop, and close DB resources. Startup and shutdown logs should include enough non-sensitive config and lifecycle information for operators.

Update logging middleware to capture response status and response size by wrapping `http.ResponseWriter`.

### Authorization

Protect `/api/stats` with admin auth.

Group member deletion should use the already-authenticated principal:

- Admin may remove any group member.
- A member token may only remove the same actor represented by that token from the same group.
- Deleting another member with a member token returns 403.
- The route-derived actor type/id remains the deletion target, but it is no longer treated as proof of caller identity.

Group invite consumption should be atomic. It should only increment `used_count` when status, expiry, and max-use constraints still permit consumption, and it should check affected rows to detect races.

### External Calls And Streams

LLM providers should use HTTP clients with explicit timeouts. Stream readers should:

- Recover inside goroutines.
- Close response bodies and channels exactly once.
- Emit an error stream event for scanner errors or malformed upstream JSON that invalidates the stream.
- Continue to emit `done` only for a clean terminal condition.

Bridge HTTP invocation should use a dedicated timeout client and validate URLs enough to reject missing/unsupported schemes. Bridge CLI invocation should avoid shell string concatenation for templated arguments where possible, or narrowly preserve shell mode only for explicitly configured shell snippets. Output should be bounded.

Tools and A2A helper requests should use `http.NewRequestWithContext` with a parent context. Subagent spawning should derive from the caller context, not `context.Background()`.

Engine tool execution should propagate cancellation into tool calls where the tool API can accept it. Goroutines launched for read-only tools and timeout wrappers should recover and report errors without crashing the process.

### Data Layer

`TaskStore.Update()` must whitelist allowed task columns before constructing SQL.

Registry state changes should persist first, then update memory and broadcast. If DB persistence fails, in-memory state and event bus state should remain unchanged.

List and scan methods should check `rows.Err()` after iteration. Existing methods that already do this should remain unchanged.

Legacy lineage repair SQL must use MySQL-compatible timestamp functions or avoid driver-specific date functions. Migration failures for core schema should fail startup instead of logging a warning and continuing with a broken schema.

Introduce a small migration boundary for non-trivial schema/backfill steps: named migration steps, a schema version table, and clear distinction between idempotent compatibility notes and fatal core migration failures. Do not introduce a large external migration dependency unless the existing code cannot support the fix cleanly.

## Components

Expected touched areas:

- `cmd/server`: middleware chain, auth route classification, health handler, logging wrapper, shutdown flow.
- `internal/svc`: service context cleanup, registry lifecycle, store update/query safety, invite consume, migration SQL.
- `internal/handler`: group member deletion behavior and any affected tests.
- `internal/engine`: tool goroutine recovery, trace nil checks, timeout/cancel behavior.
- `internal/llm`: HTTP client timeout and stream error handling.
- `internal/bridge`: HTTP client, URL validation, CLI invocation safety, output bounds.
- `internal/tools`: request context propagation, subagent parent context propagation, task claim correctness if rows affected are currently ignored.
- `docs/audit-report.md`: verified/corrected/deferred/rejected findings.
- `docs/architecture/current-architecture.html`: current implementation changes that affect auth, health, lifecycle, schema/migration, external calls, and human identity.

## Data Flow

Requests enter through request ID, recovery, CORS, rate limit, auth, and logging middleware before reaching handlers. Auth middleware derives admin/member principals from trusted tokens and clears any caller-supplied principal headers. Handlers use those derived principals for authorization decisions.

Builtin and bridge agent calls receive the request context. Tool and subagent calls derive child contexts from it. If the caller disconnects or the request times out, downstream HTTP requests and tool waits should stop promptly where the underlying operation supports cancellation.

Registry health checking runs under an explicit stop channel or context. Shutdown stops the health loop and closes DB resources through `ServiceContext.Close()`.

Schema migration runs before stores are used. Core schema failures fail startup. Compatibility/backfill steps may be best-effort only when skipping them cannot break current runtime SQL.

## Error Handling

The public API should prefer JSON error responses. Panic recovery returns a generic 500. Auth failures return 401 for missing/invalid credentials and 403 for valid credentials lacking permission.

Stream errors should be visible as stream error events and persistent task/trace state where applicable. Malformed upstream chunks should not be silently transformed into success.

Database operations that depend on conditional updates must inspect affected rows. A conditional update that matches no rows should return a domain error instead of looking successful.

## Testing

Add or adjust tests for:

- Panic recovery returns JSON 500 and keeps the process alive.
- `/health` returns 503 when DB ping fails.
- `/api/stats` requires admin credentials.
- Bare human handle login still works and is documented as passwordless convenience identity.
- Group member token can delete self but cannot delete another group member; admin can delete any member.
- Invite consume cannot exceed max uses under concurrent attempts.
- `TaskStore.Update()` rejects unknown columns.
- Registry register/disconnect does not change memory state when DB persistence fails.
- LLM stream readers report scanner or malformed JSON errors.
- Tools and bridge calls use cancellation-aware requests.
- Migration SQL is MySQL-compatible and fatal core migration failures stop startup.

Run `go test ./...` as the baseline verification. Use targeted race/concurrency tests where the bug is specifically concurrent, especially invite consumption.

## Documentation

Update `docs/audit-report.md` into a verified report. Each finding should be marked as one of:

- Confirmed and fixed.
- Confirmed and intentionally deferred with reason.
- Corrected because the original evidence or severity was wrong.
- Rejected because the current code does not match the claim.

Update `docs/architecture/current-architecture.html` whenever behavior changes affect routes, API contracts, auth boundaries, Agent/A2A compatibility, schema/migration, orchestration, frontend flows, deployment, or module responsibilities.

## Acceptance Criteria

- All confirmed security and production-stability issues in this design are fixed or explicitly deferred with justification.
- Passwordless human handle login remains supported.
- New tests cover the changed security and failure behavior.
- `go test ./...` passes.
- The corrected audit report no longer contains known stale facts such as the Go file count, overbroad panic-recover claim, or incorrect human-login classification.
- Architecture documentation matches the new implementation.
