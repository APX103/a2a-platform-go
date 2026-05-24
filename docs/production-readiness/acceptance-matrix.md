# Production Readiness Acceptance Matrix

> Scope: first-stage readiness for an internal trusted-network deployment.
> Status values: `verified`, `missing-test`, `implementation-gap`, `doc-partial`, `planned`.
> Risk values: `P0`, `P1`, `P2`.

## P0 Capabilities

| Capability | Source | Contract | Risk | Evidence | Status | Owner module |
|---|---|---|---|---|---|---|
| External Agent invalid JSON response does not crash platform | README "A2A message proxy"; `internal/handler/handler.go` `AgentProxyHandler` | A registered external Agent that returns invalid JSON with HTTP 200 produces an upstream response to the caller, records a bounded response trace, and leaves the task in `ERROR` or `RESPONDED` according to response handling rules without panic. | P0 | `tests/e2e/e2e_test.go::TestMaliciousExternalAgentInvalidJSONDoesNotCrash` | missing-test | `internal/handler` |
| External Agent broken SSE stream does not hang platform | README "SSE streaming"; architecture "A2A message proxy" | A registered external Agent that sends malformed SSE and closes the connection returns promptly, records stream/error traces with redacted bounded data, and updates task state. | P0 | `tests/e2e/e2e_test.go::TestMaliciousExternalAgentBrokenSSEDoesNotHang` | missing-test | `internal/handler` |
| Member token cannot access another group | architecture "Group permission boundary"; `cmd/server/main.go` `authMiddleware` | A member token bound to group A receives 403 for scoped group B endpoints, `/agent/{name}`, and AgentCard proxy paths when target agent is not in group A. | P0 | `cmd/server/main_test.go::TestAuthMiddlewareRestrictsAgentProxyToSameGroup`; `cmd/server/main_test.go::TestAuthMiddlewareRestrictsAgentCardProxyToSameGroup` | missing-test | `cmd/server` |
| Admin-only endpoints reject missing token | README REST API auth column; `cmd/server/main.go` `requiresAdmin` | POST/PUT/DELETE mutations for Agents, Builtin Agents, Humans, Tasks/Traces/Contexts/Subagents, and protected Group routes return 401 without Admin token. | P0 | `cmd/server/main_test.go::TestRequiresAdminProductionEndpointMatrix` | missing-test | `cmd/server` |
| Sensitive values are not written to traces or ordinary errors | architecture "Sensitive data protection"; config models | Admin token, human token, group member token, agent secret, builtin API key, Authorization bearer values, and `X-Admin-Token` values are redacted before trace/error persistence unless the endpoint is an explicit one-time credential issuance response. | P0 | `internal/handler/redaction_test.go` | missing-test | `internal/handler` |
| Failed proxy calls leave coherent task/trace state | architecture "task/message/trace audit chain" | Connection refusal, timeout, upstream 5xx, malformed SSE, and empty response each produce a task terminal state plus a response/error trace. | P0 | `internal/handler/handler_test.go` proxy failure tests | missing-test | `internal/handler` |
| Current A2A compatibility is honestly documented | architecture "A2A compatibility gaps" | Current supported AgentCard paths and message proxy behavior are documented as current; unsupported JSON-RPC methods are marked partial/planned. | P0 | architecture doc review | missing-test | `docs/architecture` |

## P1 Capabilities

| Capability | Source | Contract | Risk | Evidence | Status | Owner module |
|---|---|---|---|---|---|---|
| Request and response size limits are explicit | `internal/handler/handler.go`; `internal/tools/tools.go`; `internal/bridge/http.go` | Request bodies, proxied non-streaming responses, SSE frame trace data, tool results, and bridge responses have named limits and tests for truncation behavior. | P1 | handler/tool/bridge tests | missing-test | `internal/handler`, `internal/tools`, `internal/bridge` |
| Bridge HTTP/CLI failures are classified | `internal/bridge/http.go`; `internal/bridge/cli.go` | Timeout, non-2xx HTTP, command timeout, non-zero exit, and large output return bounded errors without leaking secret headers. | P1 | bridge package tests | missing-test | `internal/bridge` |
| Default secrets are documented as examples only | `etc/config.yaml`; `docker-compose.yml`; README | Example tokens/passwords are labeled non-production, and production configuration uses environment-variable examples. | P1 | doc review | missing-test | `README.md`, `docs/USAGE.md` |
| Builtin file and fetch tools have clear boundaries | `internal/tools/tools.go` | File tools cannot leave process working directory; fetch tool behavior and SSRF risk are documented or guarded for internal deployment. | P1 | existing `internal/tools/tools_test.go` plus added SSRF policy test if code changes | missing-test | `internal/tools` |

## Completion Rule

First-stage readiness is complete only when every P0 row is `verified` or explicitly moved to `doc-partial` with the current implementation described in `docs/architecture/current-architecture.html`.
