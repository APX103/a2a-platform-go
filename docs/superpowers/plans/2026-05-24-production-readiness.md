# Production Readiness Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the first production-readiness loop: an acceptance matrix, P0 contract tests, and focused hardening so the platform does not crash, hang, leak secrets, or violate group boundaries when agents misbehave.

**Architecture:** Keep the current Go monolith and React clients. Add a canonical Markdown acceptance matrix, small focused backend helpers for redaction and proxy failure handling, contract tests around existing handlers, and targeted docs updates when implementation status is partial. Do not restructure large files unless a task explicitly extracts a small helper needed by tests.

**Tech Stack:** Go `net/http`, `httptest`, MySQL-backed stores through existing `svc` package, current `make test`, Vite/TypeScript builds for Admin and Human clients, Markdown docs.

---

## File Structure

- Create: `docs/production-readiness/acceptance-matrix.md`
  - Canonical production readiness ledger with capability, source, contract, risk, evidence, status, and owner module.
- Modify: `internal/handler/handler_test.go`
  - Add handler-level P0 contract tests for proxy errors, response truncation, and sensitive data exposure.
- Modify: `internal/handler/handler.go`
  - Add or adjust small helper functions for safe proxy error handling, response status handling, and trace redaction.
- Create: `internal/handler/redaction_test.go`
  - Unit tests for sensitive value redaction.
- Create: `internal/handler/redaction.go`
  - Handler-local redaction helper used before writing trace data and external-facing error messages.
- Modify: `cmd/server/main_test.go`
  - Add route/middleware-level auth contract tests for Admin-only and member-scoped paths.
- Modify: `tests/e2e/e2e_test.go`
  - Add build-tagged malicious external Agent tests using local fake servers and the existing e2e helpers.
- Modify: `docs/architecture/current-architecture.html`
  - Update only if code changes alter current API contracts, secret handling, A2A compatibility status, or orchestration behavior.
- Modify: `README.md` and `docs/USAGE.md`
  - Update only where the acceptance matrix finds current docs overstating implemented behavior.

---

## Task 1: Create the Acceptance Matrix

**Files:**
- Create: `docs/production-readiness/acceptance-matrix.md`
- Reference: `docs/superpowers/specs/2026-05-24-production-readiness-design.md`
- Reference: `README.md`
- Reference: `docs/USAGE.md`
- Reference: `docs/architecture/current-architecture.html`

- [ ] **Step 1: Create the matrix with P0/P1 rows**

Add `docs/production-readiness/acceptance-matrix.md` with this initial content:

```markdown
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
```

- [ ] **Step 2: Verify the new document has no placeholders**

Run:

```bash
rg -n -e 'TB''D' -e 'TO''DO' -e 'FIX''ME' docs/production-readiness/acceptance-matrix.md
```

Expected: exit code `1` and no matches.

- [ ] **Step 3: Commit the matrix**

Run:

```bash
git add docs/production-readiness/acceptance-matrix.md
git commit -m "docs: add production readiness acceptance matrix"
```

Expected: commit succeeds with one new file.

---

## Task 2: Add Redaction Helpers Before Expanding Proxy Traces

**Files:**
- Create: `internal/handler/redaction_test.go`
- Create: `internal/handler/redaction.go`
- Modify later tasks: `internal/handler/handler.go`

- [ ] **Step 1: Write failing redaction tests**

Create `internal/handler/redaction_test.go`:

```go
package handler

import (
	"strings"
	"testing"
)

func TestRedactSensitiveTextMasksKnownSecretShapes(t *testing.T) {
	input := `{
		"admin_token":"a2a-admin-token",
		"api_key":"sk-live-secret",
		"secret":"agent-secret",
		"token":"human-token",
		"Authorization":"Bearer member-token",
		"X-Admin-Token":"root-token",
		"normal":"keep-me"
	}`

	got := redactSensitiveText(input)

	for _, leaked := range []string{
		"a2a-admin-token",
		"sk-live-secret",
		"agent-secret",
		"human-token",
		"member-token",
		"root-token",
	} {
		if strings.Contains(got, leaked) {
			t.Fatalf("redacted text leaked %q: %s", leaked, got)
		}
	}
	if !strings.Contains(got, "keep-me") {
		t.Fatalf("redacted text removed non-sensitive value: %s", got)
	}
	if !strings.Contains(got, "[REDACTED]") {
		t.Fatalf("redacted text does not contain marker: %s", got)
	}
}

func TestRedactSensitiveTextMasksBearerInPlainError(t *testing.T) {
	got := redactSensitiveText("upstream said Authorization: Bearer abc.def.ghi and X-Admin-Token: secret")
	if strings.Contains(got, "abc.def.ghi") || strings.Contains(got, "secret") {
		t.Fatalf("plain error leaked token: %s", got)
	}
}

func TestSafeTraceDataRedactsThenTruncates(t *testing.T) {
	input := strings.Repeat("x", 20) + `"api_key":"sk-secret"` + strings.Repeat("y", 20)
	got := safeTraceData(input, 32)
	if len(got) > 32 {
		t.Fatalf("len = %d, want <= 32: %q", len(got), got)
	}
	if strings.Contains(got, "sk-secret") {
		t.Fatalf("safe trace data leaked secret: %s", got)
	}
}
```

- [ ] **Step 2: Run tests to verify failure**

Run:

```bash
go test ./internal/handler -run 'TestRedactSensitiveText|TestSafeTraceData' -count=1
```

Expected: FAIL with `undefined: redactSensitiveText` and `undefined: safeTraceData`.

- [ ] **Step 3: Implement redaction helpers**

Create `internal/handler/redaction.go`:

```go
package handler

import (
	"regexp"
	"strings"
)

var sensitiveJSONFieldRe = regexp.MustCompile(`(?i)("?(?:admin_token|api_key|secret|token|authorization|x-admin-token)"?\s*:\s*"?)([^",}\s]+)`)
var bearerTokenRe = regexp.MustCompile(`(?i)(Bearer\s+)[A-Za-z0-9._~+/\-=]+`)
var namedHeaderTokenRe = regexp.MustCompile(`(?i)((?:X-Admin-Token|X-Group-Member-Token|Authorization)\s*:\s*)([^\s,;]+)`)
var providerAPIKeyRe = regexp.MustCompile(`\b(?:sk|sk-ant)-[A-Za-z0-9._~+/\-=]+`)

func redactSensitiveText(input string) string {
	if input == "" {
		return ""
	}
	out := sensitiveJSONFieldRe.ReplaceAllString(input, `${1}[REDACTED]`)
	out = bearerTokenRe.ReplaceAllString(out, `${1}[REDACTED]`)
	out = namedHeaderTokenRe.ReplaceAllString(out, `${1}[REDACTED]`)
	out = providerAPIKeyRe.ReplaceAllString(out, `[REDACTED]`)
	return out
}

func safeTraceData(input string, max int) string {
	redacted := redactSensitiveText(input)
	if max <= 0 || len(redacted) <= max {
		return redacted
	}
	if max <= len("...(truncated)") {
		return redacted[:max]
	}
	return strings.TrimSpace(redacted[:max-len("...(truncated)")]) + "...(truncated)"
}
```

- [ ] **Step 4: Run redaction tests**

Run:

```bash
go test ./internal/handler -run 'TestRedactSensitiveText|TestSafeTraceData' -count=1
```

Expected: PASS.

- [ ] **Step 5: Commit redaction helpers**

Run:

```bash
git add internal/handler/redaction.go internal/handler/redaction_test.go
git commit -m "feat(handler): add sensitive trace redaction"
```

Expected: commit succeeds.

---

## Task 3: Apply Redaction to Agent Proxy Trace and Error Data

**Files:**
- Modify: `internal/handler/handler.go`
- Modify: `internal/handler/handler_test.go`
- Test: `internal/handler/redaction_test.go`

- [ ] **Step 1: Add a failing proxy trace redaction test**

Append this test to `internal/handler/handler_test.go`:

```go
func TestAgentProxyRedactsRequestBodyInSendTrace(t *testing.T) {
	db := setupAgentProxyLineageDB(t)
	agentServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"result":{"message":{"parts":[{"text":"ok"}]}}}`))
	}))
	defer agentServer.Close()

	agentStore := svc.NewAgentStore(db)
	registry := svc.NewAgentRegistry(agentStore)
	if _, err := registry.RegisterAgent("redact-agent", "external", agentServer.URL, 0, nil, "", model.ContextModeContext, &model.AgentCard{
		Name: "redact-agent",
	}); err != nil {
		t.Fatalf("register agent: %v", err)
	}
	svcCtx := &svc.ServiceContext{
		DB:             db,
		Agents:         agentStore,
		Tasks:          svc.NewTaskStore(db),
		Messages:       svc.NewMessageStore(db),
		Traces:         svc.NewTraceStore(db),
		Registry:       registry,
		Engine:         engine.New(),
		BridgeRegistry: bridge.NewRegistry(),
	}

	body := `{"jsonrpc":"2.0","id":"1","method":"SendMessage","params":{"message":{"role":"ROLE_USER","parts":[{"text":"my api_key is sk-secret-value"}]},"metadata":{"admin_token":"root-secret"}}}`
	req := httptest.NewRequest(http.MethodPost, "/agent/redact-agent", strings.NewReader(body))
	req.Header.Set("X-Path-Param-Name", "redact-agent")
	rec := httptest.NewRecorder()

	NewAgentProxyHandler(svcCtx).ServeHTTP(rec, req)

	rows, err := db.Query(`SELECT data_json FROM traces`)
	if err != nil {
		t.Fatalf("query traces: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var data string
		if err := rows.Scan(&data); err != nil {
			t.Fatalf("scan trace: %v", err)
		}
		if strings.Contains(data, "sk-secret-value") || strings.Contains(data, "root-secret") {
			t.Fatalf("trace leaked sensitive value: %s", data)
		}
	}
}
```

- [ ] **Step 2: Run the new test to verify failure**

Run:

```bash
go test ./internal/handler -run TestAgentProxyRedactsRequestBodyInSendTrace -count=1
```

Expected: FAIL because existing `sendTrace.DataJson` stores the raw body.

- [ ] **Step 3: Use `safeTraceData` in `AgentProxyHandler`**

Modify these assignments in `internal/handler/handler.go`:

```go
DataJson: safeTraceData(string(body), 4000),
```

for the send trace, and:

```go
DataJson: safeTraceData(data, 500),
```

for SSE stream traces, and:

```go
respTrace.DataJson = safeTraceData(streamErr.Error(), 1000)
```

for stream errors, and:

```go
DataJson: safeTraceData(string(respBody), 1000),
```

for non-streaming response traces.

- [ ] **Step 4: Run focused handler tests**

Run:

```bash
go test ./internal/handler -run 'TestAgentProxyRedactsRequestBodyInSendTrace|TestAgentProxyRecordsPlatformTextDeltaSSE|TestRedactSensitiveText|TestSafeTraceData' -count=1
```

Expected: PASS.

- [ ] **Step 5: Run full backend tests**

Run:

```bash
make test
```

Expected: all packages PASS.

- [ ] **Step 6: Commit proxy redaction**

Run:

```bash
git add internal/handler/handler.go internal/handler/handler_test.go
git commit -m "fix(handler): redact sensitive proxy trace data"
```

Expected: commit succeeds.

---

## Task 4: Harden External Proxy Failure States

**Files:**
- Modify: `internal/handler/handler_test.go`
- Modify: `internal/handler/handler.go`

- [ ] **Step 1: Add a failing test for upstream connection failure traces**

Append this test to `internal/handler/handler_test.go`:

```go
func TestAgentProxyConnectionFailureRecordsErrorTrace(t *testing.T) {
	db := setupAgentProxyLineageDB(t)
	agentStore := svc.NewAgentStore(db)
	registry := svc.NewAgentRegistry(agentStore)
	if _, err := registry.RegisterAgent("down-agent", "external", "http://127.0.0.1:1", 0, nil, "", model.ContextModeContext, &model.AgentCard{
		Name: "down-agent",
	}); err != nil {
		t.Fatalf("register agent: %v", err)
	}
	svcCtx := &svc.ServiceContext{
		DB:             db,
		Agents:         agentStore,
		Tasks:          svc.NewTaskStore(db),
		Messages:       svc.NewMessageStore(db),
		Traces:         svc.NewTraceStore(db),
		Registry:       registry,
		Engine:         engine.New(),
		BridgeRegistry: bridge.NewRegistry(),
	}

	body := `{"jsonrpc":"2.0","id":"1","method":"SendMessage","params":{"message":{"role":"ROLE_USER","parts":[{"text":"hello"}]}}}`
	req := httptest.NewRequest(http.MethodPost, "/agent/down-agent", strings.NewReader(body))
	req.Header.Set("X-Path-Param-Name", "down-agent")
	rec := httptest.NewRecorder()

	NewAgentProxyHandler(svcCtx).ServeHTTP(rec, req)

	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502, body=%s", rec.Code, rec.Body.String())
	}
	var state string
	if err := db.QueryRow(`SELECT state FROM tasks WHERE target_agent='down-agent'`).Scan(&state); err != nil {
		t.Fatalf("query task state: %v", err)
	}
	if state != "ERROR" {
		t.Fatalf("state = %q, want ERROR", state)
	}
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM traces WHERE event_type='error' AND target_agent='down-agent'`).Scan(&count); err != nil {
		t.Fatalf("query error traces: %v", err)
	}
	if count != 1 {
		t.Fatalf("error trace count = %d, want 1", count)
	}
}
```

- [ ] **Step 2: Run the new test to verify failure**

Run:

```bash
go test ./internal/handler -run TestAgentProxyConnectionFailureRecordsErrorTrace -count=1
```

Expected: FAIL because the current connection failure path updates task state but does not append an error trace.

- [ ] **Step 3: Add a small error trace helper**

Add this helper near the other `AgentProxyHandler` helper functions in `internal/handler/handler.go`:

```go
func (h *AgentProxyHandler) recordProxyErrorTrace(taskId string, contextId, rootContextId, parentTaskId *string, sourceAgent, targetAgent, message string) {
	if h == nil || h.svcCtx == nil || h.svcCtx.Traces == nil {
		return
	}
	trace := &model.TraceEvent{
		TaskId:        taskId,
		ContextId:     contextId,
		RootContextId: rootContextId,
		ParentTaskId:  parentTaskId,
		EventType:     "error",
		AgentName:     sourceAgent,
		TargetAgent:   &targetAgent,
		DataJson:      safeTraceData(message, 1000),
	}
	h.svcCtx.Traces.Append(trace)
	if h.svcCtx.EventBus != nil {
		h.svcCtx.EventBus.TraceEvent(trace)
	}
}
```

- [ ] **Step 4: Call the helper in the upstream request failure path**

In `AgentProxyHandler.ServeHTTP`, replace the `client.Do(proxyReq)` error block with:

```go
if err != nil {
	h.svcCtx.Tasks.Update(taskId, map[string]interface{}{"state": "ERROR"})
	if h.svcCtx.EventBus != nil {
		h.svcCtx.EventBus.Task("update", taskId, name, "ERROR")
	}
	h.recordProxyErrorTrace(taskId, contextId, rootContextId, parentTaskId, sourceAgent, name, err.Error())
	jsonError(w, "proxy failed", http.StatusBadGateway)
	return
}
```

- [ ] **Step 5: Run focused and full tests**

Run:

```bash
go test ./internal/handler -run TestAgentProxyConnectionFailureRecordsErrorTrace -count=1
make test
```

Expected: focused test PASS and `make test` PASS.

- [ ] **Step 6: Commit connection failure hardening**

Run:

```bash
git add internal/handler/handler.go internal/handler/handler_test.go
git commit -m "fix(handler): record proxy connection failures"
```

Expected: commit succeeds.

---

## Task 5: Add Admin and Member Auth Contract Matrix Tests

**Files:**
- Modify: `cmd/server/main_test.go`

- [ ] **Step 1: Add Admin endpoint matrix test**

Append this test to `cmd/server/main_test.go`:

```go
func TestRequiresAdminProductionEndpointMatrix(t *testing.T) {
	tests := []struct {
		path   string
		method string
		want   bool
	}{
		{"/api/agents", http.MethodPost, true},
		{"/api/agents/alpha", http.MethodPut, true},
		{"/api/agents/alpha", http.MethodDelete, true},
		{"/api/builtin-agents", http.MethodPost, true},
		{"/api/builtin-agents/alpha", http.MethodPut, true},
		{"/api/builtin-agents/alpha", http.MethodDelete, true},
		{"/api/humans", http.MethodGet, true},
		{"/api/humans/h1", http.MethodPut, true},
		{"/api/tasks", http.MethodGet, true},
		{"/api/traces", http.MethodGet, true},
		{"/api/contexts/alpha", http.MethodGet, true},
		{"/api/subagents/alpha", http.MethodGet, true},
		{"/api/events", http.MethodGet, true},
		{"/api/groups", http.MethodPost, true},
		{"/api/groups/g1", http.MethodPut, true},
		{"/api/groups/g1/members", http.MethodPost, true},
		{"/api/groups/g1/invites", http.MethodGet, true},
		{"/api/group-joins", http.MethodPost, false},
		{"/health", http.MethodGet, false},
	}

	for _, tt := range tests {
		if got := requiresAdmin(tt.path, tt.method); got != tt.want {
			t.Fatalf("requiresAdmin(%q, %q) = %v, want %v", tt.path, tt.method, got, tt.want)
		}
	}
}
```

- [ ] **Step 2: Run the matrix test**

Run:

```bash
go test ./cmd/server -run TestRequiresAdminProductionEndpointMatrix -count=1
```

Expected: PASS or a clear failure showing an endpoint that must be reconciled with the acceptance matrix.

- [ ] **Step 3: Add member-scoped AgentCard proxy test**

Append this test to `cmd/server/main_test.go`:

```go
func TestAuthMiddlewareRestrictsAgentCardProxyToSameGroup(t *testing.T) {
	svcCtx, groupID := setupGroupRouteTestContext(t)
	svcCtx.Config = &config.Config{AdminToken: "secret"}

	if err := svcCtx.GroupMembers.Upsert(&model.GroupMember{
		GroupID:   groupID,
		ActorType: model.GroupActorHuman,
		ActorID:   "human-route",
		Role:      "member",
	}); err != nil {
		t.Fatalf("create human member: %v", err)
	}
	accessToken, err := svcCtx.GroupTokens.Create(&model.GroupMemberToken{
		GroupID:   groupID,
		ActorType: model.GroupActorHuman,
		ActorID:   "human-route",
	})
	if err != nil {
		t.Fatalf("create member token: %v", err)
	}
	protected := authMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}), svcCtx)

	req := httptest.NewRequest(http.MethodGet, "/agent/leader-agent/.well-known/agent-card.json", nil)
	req.Header.Set("Authorization", "Bearer "+accessToken)
	rec := httptest.NewRecorder()
	protected.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("same group agent card status = %d, want 204", rec.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/agent/not-in-room/.well-known/agent-card.json", nil)
	req.Header.Set("Authorization", "Bearer "+accessToken)
	rec = httptest.NewRecorder()
	protected.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("other agent card status = %d, want 403", rec.Code)
	}
}
```

- [ ] **Step 4: Run the member AgentCard test and matrix test**

Run:

```bash
go test ./cmd/server -run 'TestRequiresAdminProductionEndpointMatrix|TestAuthMiddlewareRestrictsAgentCardProxyToSameGroup' -count=1
```

Expected: PASS.

- [ ] **Step 5: Reconcile a failing auth test**

If one of the tests fails, do one of these concrete actions:

- If the endpoint is intended to be protected, update `requiresAdmin` or `authMiddleware` so the test passes.
- If the endpoint is intentionally public, change the test expectation and update `docs/production-readiness/acceptance-matrix.md` with the public contract.

Run:

```bash
go test ./cmd/server -run 'TestRequiresAdminProductionEndpointMatrix|TestAuthMiddlewareRestrictsAgentCardProxyToSameGroup' -count=1
```

Expected: PASS after reconciliation.

- [ ] **Step 6: Commit auth matrix coverage**

Run:

```bash
git add cmd/server/main.go cmd/server/main_test.go docs/production-readiness/acceptance-matrix.md
git commit -m "test(server): cover production auth endpoint matrix"
```

Expected: commit succeeds. If `cmd/server/main.go` or the matrix did not change, omit those paths from `git add`.

---

## Task 6: Add Malicious External Agent E2E Coverage

**Files:**
- Modify: `tests/e2e/e2e_test.go`

- [ ] **Step 1: Add fake malicious agent helpers**

Append these helpers to `tests/e2e/e2e_test.go`:

```go
func registerStaticExternalAgent(t *testing.T, name, url string) {
	t.Helper()
	body := fmt.Sprintf(`{
		"name":%q,
		"type":"external",
		"url":%q,
		"context_mode":"context",
		"agent_card":{
			"name":%q,
			"description":"malicious e2e fake",
			"version":"1.0.0",
			"skills":[{"id":"chat","name":"Chat"}]
		}
	}`, name, url, name)
	status, data := req(t, http.MethodPost, "/api/agents", body, map[string]string{"X-Admin-Token": adminToken})
	if status != http.StatusOK {
		t.Fatalf("register malicious agent status = %d, body=%s", status, string(data))
	}
	t.Cleanup(func() {
		req(t, http.MethodDelete, "/api/agents/"+name, "", map[string]string{"X-Admin-Token": adminToken})
	})
}

func simpleA2ABody(text string) string {
	return fmt.Sprintf(`{"jsonrpc":"2.0","id":"1","method":"SendMessage","params":{"message":{"role":"ROLE_USER","parts":[{"text":%q}]}}}`, text)
}

func uniqueName(prefix string) string {
	return fmt.Sprintf("%s-%d", prefix, time.Now().UnixNano())
}
```

- [ ] **Step 2: Add invalid JSON e2e test**

Append:

```go
func TestMaliciousExternalAgentInvalidJSONDoesNotCrash(t *testing.T) {
	name := uniqueName("mal-json")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"result":`))
	}))
	defer srv.Close()

	registerStaticExternalAgent(t, name, srv.URL)
	status, data := req(t, http.MethodPost, "/agent/"+name, simpleA2ABody("hello"), map[string]string{"X-Admin-Token": adminToken})
	if status != http.StatusOK {
		t.Fatalf("status = %d, want upstream 200 relay, body=%s", status, string(data))
	}
	if strings.Contains(string(data), adminToken) {
		t.Fatalf("response leaked admin token: %s", string(data))
	}
}
```

- [ ] **Step 3: Add broken SSE e2e test**

Append:

```go
func TestMaliciousExternalAgentBrokenSSEDoesNotHang(t *testing.T) {
	name := uniqueName("mal-sse")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Write([]byte("data: {not-json}\n\n"))
	}))
	defer srv.Close()

	registerStaticExternalAgent(t, name, srv.URL)
	client := &http.Client{Timeout: 5 * time.Second}
	r, err := http.NewRequest(http.MethodPost, baseURL+"/agent/"+name, strings.NewReader(simpleA2ABody("hello")))
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Accept", "text/event-stream")
	r.Header.Set("X-Admin-Token", adminToken)
	resp, err := client.Do(r)
	if err != nil {
		t.Fatalf("broken SSE request should return promptly: %v", err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200 stream relay, body=%s", resp.StatusCode, string(data))
	}
	if strings.Contains(string(data), adminToken) {
		t.Fatalf("response leaked admin token: %s", string(data))
	}
}
```

- [ ] **Step 4: Import `net/http/httptest`**

Update the import block in `tests/e2e/e2e_test.go` to include:

```go
"net/http/httptest"
```

- [ ] **Step 5: Run e2e tests against a running platform**

Start the platform separately with Docker Compose or an existing local server, then run:

```bash
go test -tags e2e ./tests/e2e -run 'TestMaliciousExternalAgent' -count=1
```

Expected: PASS. If no platform is running, this command fails with connection refused; start the platform and rerun before marking the task complete.

- [ ] **Step 6: Commit malicious agent tests**

Run:

```bash
git add tests/e2e/e2e_test.go
git commit -m "test(e2e): cover malicious external agent behavior"
```

Expected: commit succeeds.

---

## Task 7: Update Docs After Verified Behavior Is Known

**Files:**
- Modify: `docs/production-readiness/acceptance-matrix.md`
- Modify if behavior changed: `docs/architecture/current-architecture.html`
- Modify if docs overstate behavior: `README.md`
- Modify if docs overstate behavior: `docs/USAGE.md`

- [ ] **Step 1: Mark verified rows in the acceptance matrix**

For each P0 row whose tests now pass, update `Status` from `missing-test` to `verified` and update `Evidence` with exact test names. Example row format:

```markdown
| Sensitive values are not written to traces or ordinary errors | architecture "Sensitive data protection"; config models | Admin token, human token, group member token, agent secret, builtin API key, Authorization bearer values, and `X-Admin-Token` values are redacted before trace/error persistence unless the endpoint is an explicit one-time credential issuance response. | P0 | `internal/handler/redaction_test.go`; `internal/handler/handler_test.go::TestAgentProxyRedactsRequestBodyInSendTrace` | verified | `internal/handler` |
```

- [ ] **Step 2: Update architecture document if current behavior changed**

If Tasks 2-6 changed trace redaction, proxy failure states, auth coverage, or A2A compatibility labels, update `docs/architecture/current-architecture.html` in the matching sections:

```html
<p>Proxy trace data is redacted before persistence for known sensitive fields such as admin tokens, bearer tokens, agent secrets, and builtin API keys. Large trace payloads are bounded before storage.</p>
```

If no current behavior changed, add no architecture edit and record that decision in the final implementation summary.

- [ ] **Step 3: Update README or USAGE only for overstated claims**

If the matrix identifies a feature that is partial, edit the relevant table row to say `partial` or link to the architecture compatibility gap. Use this wording:

```markdown
> A2A compatibility note: the platform currently supports the documented message proxy and AgentCard proxy paths. Additional JSON-RPC task management methods are tracked as planned compatibility work in `docs/architecture/current-architecture.html`.
```

- [ ] **Step 4: Run doc sanity checks**

Run:

```bash
rg -n -e 'TB''D' -e 'TO''DO' -e 'FIX''ME' docs/production-readiness docs/architecture/current-architecture.html README.md docs/USAGE.md
```

Expected: exit code `1` or only pre-existing unrelated matches that are documented in the final summary.

- [ ] **Step 5: Commit documentation updates**

Run:

```bash
git add docs/production-readiness/acceptance-matrix.md docs/architecture/current-architecture.html README.md docs/USAGE.md
git commit -m "docs: update production readiness evidence"
```

Expected: commit succeeds. Omit unchanged paths from `git add`.

---

## Task 8: Final Verification for First-Stage Readiness Slice

**Files:**
- No code changes expected.

- [ ] **Step 1: Run backend unit and handler tests**

Run:

```bash
make test
```

Expected: all packages PASS.

- [ ] **Step 2: Run Admin build**

Run:

```bash
npm --prefix web/admin run build
```

Expected: TypeScript and Vite build complete successfully. Vite chunk-size warnings are acceptable if the command exits `0`.

- [ ] **Step 3: Run Human Client build**

Run:

```bash
npm --prefix apps/human-client run build
```

Expected: TypeScript and Vite build complete successfully.

- [ ] **Step 4: Run malicious e2e tests if a local platform is running**

Run:

```bash
go test -tags e2e ./tests/e2e -run 'TestMaliciousExternalAgent' -count=1
```

Expected with platform running: PASS. If platform is not running, do not mark this step complete; start Docker Compose and rerun.

- [ ] **Step 5: Check working tree**

Run:

```bash
git status --short
```

Expected: no unstaged or uncommitted changes.

- [ ] **Step 6: Prepare implementation summary**

Write a final summary with:

```markdown
Implemented:
- Acceptance matrix created and updated with verified P0 evidence.
- Sensitive proxy trace data redacted and bounded.
- Proxy connection failures now record task and trace failure state.
- Admin/member auth contract matrix expanded.
- Malicious external Agent e2e coverage added.

Verified:
- `make test`
- `npm --prefix web/admin run build`
- `npm --prefix apps/human-client run build`
- `go test -tags e2e ./tests/e2e -run 'TestMaliciousExternalAgent' -count=1`

Remaining:
- P1 rows still marked planned or missing-test in `docs/production-readiness/acceptance-matrix.md`.
```

Expected: summary names any verification command that could not be run.
