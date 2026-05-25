# Production Hardening Audit Remediation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Correct the audit report and harden the A2A Platform Go server across lifecycle, auth, external calls, streams, and data consistency while preserving passwordless human handle login.

**Architecture:** This is one production-hardening program with independently testable tasks. Each task starts with failing regression tests, implements the smallest code change for that behavior, updates docs when behavior changes, and commits before moving on. Broad file reshuffling is avoided; new helpers are added only where they make lifecycle, auth, or error handling explicit.

**Tech Stack:** Go 1.25, `net/http`, `database/sql`, MySQL via `github.com/go-sql-driver/mysql`, `log/slog`, `httptest`, existing repository tests and docs.

---

## Scope Check

The design touches several subsystems, but they are not independent product features. They are connected by the audit remediation goal and shared operational risk model. Keep this as one plan with focused commits per subsystem.

## File Structure

- Modify `cmd/server/main.go`: middleware ordering, recovery, logging response wrapper, health status, admin route classification, shutdown cleanup.
- Modify `cmd/server/main_test.go`: HTTP lifecycle/auth regression tests.
- Modify `internal/svc/servicecontext.go`: migration error return, MySQL-compatible repair SQL, `ServiceContext.Close()`.
- Modify `internal/svc/registry.go`: stoppable health check, DB-before-memory state changes, safe event broadcast ordering.
- Modify `internal/svc/registry_test.go`: DB failure ordering and health lifecycle tests.
- Modify `internal/svc/store.go`: `TaskStore.Update()` whitelist and missing `rows.Err()` checks.
- Modify `internal/svc/group_store.go`: atomic invite consumption and rows affected checks.
- Modify `internal/svc/task_item_store.go`: claim rows affected checks.
- Modify `internal/svc/*_test.go`: store regressions and invite concurrency tests.
- Modify `internal/handler/group.go`: member deletion authorization based on trusted principal headers.
- Modify `internal/handler/human.go` tests in `cmd/server/main_test.go`: keep handle login behavior explicit.
- Modify `internal/llm/openai.go`, `internal/llm/anthropic.go`: provider timeouts, stream recovery, scanner errors.
- Add `internal/llm/stream_test.go`: stream parser regressions.
- Modify `internal/model/builtin_tools.go`: add context-aware tool execution field without breaking existing tool functions.
- Modify `internal/engine/engine.go`: trace nil guard, goroutine recovery, context-aware tool execution.
- Modify `internal/engine/engine_test.go`: panic and nil dependency regressions.
- Modify `internal/tools/tools.go`, `internal/tools/a2a.go`, `internal/tools/subagent.go`, `internal/tools/task_tools.go`: request context propagation and tool claim correctness.
- Modify `internal/tools/tools_test.go`: context cancellation and claim behavior tests.
- Modify `internal/bridge/http.go`, `internal/bridge/cli.go`: dedicated client, URL validation, bounded CLI output, safer command execution.
- Add `internal/bridge/bridge_test.go`: bridge HTTP and CLI safety regressions.
- Modify `docs/audit-report.md`: convert to verified/corrected/deferred/rejected audit status.
- Modify `docs/architecture/current-architecture.html`: update health, auth, lifecycle, human identity, migration, and external call behavior.

## Task 1: HTTP Lifecycle, Health, Stats Auth

**Files:**
- Modify: `cmd/server/main.go`
- Modify: `cmd/server/main_test.go`
- Modify: `internal/svc/servicecontext.go`
- Modify: `internal/svc/registry.go`

- [ ] **Step 1: Write failing tests for recovery, health 503, stats auth, and logging capture**

Add these tests to `cmd/server/main_test.go`:

```go
func TestRecoverMiddlewareReturnsJSON500(t *testing.T) {
	h := requestIDMiddleware(recoverMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic("boom")
	})))
	req := httptest.NewRequest(http.MethodGet, "/panic", nil)
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "application/json") {
		t.Fatalf("content-type = %q, want JSON", ct)
	}
	if !strings.Contains(rec.Body.String(), "internal server error") {
		t.Fatalf("body = %q, want generic error", rec.Body.String())
	}
}

func TestRequiresAdminProtectsStats(t *testing.T) {
	if !requiresAdmin("/api/stats", http.MethodGet) {
		t.Fatal("/api/stats must require admin auth")
	}
}

func TestLoggingResponseWriterCapturesStatusAndSize(t *testing.T) {
	rec := httptest.NewRecorder()
	lrw := &loggingResponseWriter{ResponseWriter: rec, status: http.StatusOK}

	lrw.WriteHeader(http.StatusCreated)
	n, err := lrw.Write([]byte("hello"))

	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}
	if n != 5 {
		t.Fatalf("bytes written = %d, want 5", n)
	}
	if lrw.status != http.StatusCreated {
		t.Fatalf("status = %d, want %d", lrw.status, http.StatusCreated)
	}
	if lrw.bytes != 5 {
		t.Fatalf("bytes = %d, want 5", lrw.bytes)
	}
}
```

Add this test to `internal/svc/registry_test.go`:

```go
func TestRegistryStopHealthCheckIsIdempotent(t *testing.T) {
	db := setupRegistryTestDB(t)
	registry := NewAgentRegistry(NewAgentStore(db))

	registry.StartHealthCheck(time.Hour)
	registry.StopHealthCheck()
	registry.StopHealthCheck()
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run:

```bash
go test ./cmd/server ./internal/svc
```

Expected: FAIL because `recoverMiddleware`, `loggingResponseWriter`, and `StopHealthCheck` do not exist, and `/api/stats` is not classified as admin-only.

- [ ] **Step 3: Implement recovery and logging wrapper**

Add to `cmd/server/main.go` near middleware helpers:

```go
type loggingResponseWriter struct {
	http.ResponseWriter
	status      int
	bytes       int
	wroteHeader bool
}

func (w *loggingResponseWriter) WriteHeader(status int) {
	if w.wroteHeader {
		return
	}
	w.wroteHeader = true
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

func (w *loggingResponseWriter) Write(b []byte) (int, error) {
	if !w.wroteHeader {
		w.WriteHeader(http.StatusOK)
	}
	n, err := w.ResponseWriter.Write(b)
	w.bytes += n
	return n, err
}

func recoverMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if v := recover(); v != nil {
				requestID, _ := r.Context().Value(requestIDContextKey{}).(string)
				slog.Error("panic recovered",
					"request_id", requestID,
					"method", r.Method,
					"path", r.URL.Path,
					"panic", v,
				)
				jsonError(w, "internal server error", http.StatusInternalServerError)
			}
		}()
		next.ServeHTTP(w, r)
	})
}
```

Update the middleware chain in `main()`:

```go
var h http.Handler = mux
h = loggingMiddleware(h)
h = authMiddleware(h, svcCtx)
h = rateLimitMiddleware(h, cfg)
h = corsMiddleware(h, cfg)
h = recoverMiddleware(h)
h = requestIDMiddleware(h)
```

Update `loggingMiddleware`:

```go
func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		lrw := &loggingResponseWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(lrw, r)
		requestID, _ := r.Context().Value(requestIDContextKey{}).(string)
		slog.Info("request",
			"request_id", requestID,
			"method", r.Method,
			"path", r.URL.Path,
			"remote", r.RemoteAddr,
			"status", lrw.status,
			"bytes", lrw.bytes,
			"duration", time.Since(start),
		)
	})
}
```

- [ ] **Step 4: Implement health 503 and stats admin**

Update `makeHealthHandler` in `cmd/server/main.go`:

```go
func makeHealthHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		status := "ok"
		dbStatus := "ok"
		httpStatus := http.StatusOK
		if err := svcCtx.DB.Ping(); err != nil {
			status = "degraded"
			dbStatus = "error"
			httpStatus = http.StatusServiceUnavailable
		}

		agentsConnected := svcCtx.Registry.CountConnected()
		agentsTotal, _ := svcCtx.Registry.CountTotal()

		w.WriteHeader(httpStatus)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status":           status,
			"db":               dbStatus,
			"agents_connected": agentsConnected,
			"agents_total":     agentsTotal,
		})
	}
}
```

Update `requiresAdmin`:

```go
if path == "/api/stats" {
	return true
}
```

- [ ] **Step 5: Implement service and health-check shutdown**

Add to `internal/svc/servicecontext.go`:

```go
func (s *ServiceContext) Close() error {
	if s == nil {
		return nil
	}
	if s.Registry != nil {
		s.Registry.StopHealthCheck()
	}
	if s.DB != nil {
		return s.DB.Close()
	}
	return nil
}
```

Add fields to `AgentRegistry` in `internal/svc/registry.go`:

```go
healthMu     sync.Mutex
healthStop   chan struct{}
healthDone   chan struct{}
healthActive bool
```

Replace `StartHealthCheck` and add `StopHealthCheck`:

```go
func (r *AgentRegistry) StartHealthCheck(interval time.Duration) {
	r.healthMu.Lock()
	if r.healthActive {
		r.healthMu.Unlock()
		return
	}
	stop := make(chan struct{})
	done := make(chan struct{})
	r.healthStop = stop
	r.healthDone = done
	r.healthActive = true
	r.healthMu.Unlock()

	go func() {
		defer close(done)
		defer func() {
			if v := recover(); v != nil {
				slog.Error("agent health check panic recovered", "panic", v)
			}
		}()
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				r.runHealthCheck()
			case <-stop:
				return
			}
		}
	}()
	slog.Info("Agent health check started", "interval", interval)
}

func (r *AgentRegistry) StopHealthCheck() {
	r.healthMu.Lock()
	if !r.healthActive {
		r.healthMu.Unlock()
		return
	}
	stop := r.healthStop
	done := r.healthDone
	r.healthActive = false
	r.healthStop = nil
	r.healthDone = nil
	close(stop)
	r.healthMu.Unlock()

	<-done
	slog.Info("Agent health check stopped")
}
```

Update shutdown goroutine in `main()`:

```go
go func() {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh
	slog.Info("Shutting down...")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		slog.Error("HTTP server shutdown failed", "error", err)
	}
	if err := svcCtx.Close(); err != nil {
		slog.Error("Service context close failed", "error", err)
	}
}()
```

- [ ] **Step 6: Run tests**

Run:

```bash
go test ./cmd/server ./internal/svc
```

Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add cmd/server/main.go cmd/server/main_test.go internal/svc/servicecontext.go internal/svc/registry.go internal/svc/registry_test.go
git commit -m "fix(server): harden lifecycle and admin boundaries"
```

## Task 2: Group Authorization And Invite Atomicity

**Files:**
- Modify: `cmd/server/main_test.go`
- Modify: `internal/handler/group.go`
- Modify: `internal/svc/group_store.go`
- Modify: `internal/svc/group_store_test.go`

- [ ] **Step 1: Write failing tests for group deletion rules**

Add to `cmd/server/main_test.go`:

```go
func TestMemberTokenCanDeleteSelfButNotOtherMember(t *testing.T) {
	svcCtx, groupID := setupGroupRouteTestContext(t)
	svcCtx.Config = &config.Config{AdminToken: "admin-token"}
	mustUpsertMember(t, svcCtx, groupID, model.GroupActorHuman, "alice")
	mustUpsertMember(t, svcCtx, groupID, model.GroupActorHuman, "bob")
	aliceToken := mustIssueGroupToken(t, svcCtx, groupID, model.GroupActorHuman, "alice")
	protected := authMiddleware(makeGroupRouteHandler(svcCtx), svcCtx)

	otherReq := httptest.NewRequest(http.MethodDelete, "/api/groups/"+groupID+"/members/human/bob", nil)
	otherReq.Header.Set("X-Group-Member-Token", aliceToken)
	otherRec := httptest.NewRecorder()
	protected.ServeHTTP(otherRec, otherReq)
	if otherRec.Code != http.StatusForbidden {
		t.Fatalf("delete other status = %d, want %d", otherRec.Code, http.StatusForbidden)
	}

	selfReq := httptest.NewRequest(http.MethodDelete, "/api/groups/"+groupID+"/members/human/alice", nil)
	selfReq.Header.Set("X-Group-Member-Token", aliceToken)
	selfRec := httptest.NewRecorder()
	protected.ServeHTTP(selfRec, selfReq)
	if selfRec.Code != http.StatusOK {
		t.Fatalf("delete self status = %d, want %d body=%s", selfRec.Code, http.StatusOK, selfRec.Body.String())
	}
}

func TestAdminCanDeleteAnyGroupMember(t *testing.T) {
	svcCtx, groupID := setupGroupRouteTestContext(t)
	svcCtx.Config.AdminToken = "admin-token"
	mustUpsertMember(t, svcCtx, groupID, model.GroupActorHuman, "bob")
	protected := authMiddleware(makeGroupRouteHandler(svcCtx), svcCtx)

	req := httptest.NewRequest(http.MethodDelete, "/api/groups/"+groupID+"/members/human/bob", nil)
	req.Header.Set("X-Admin-Token", "admin-token")
	rec := httptest.NewRecorder()
	protected.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
}
```

Add these helper functions to `cmd/server/main_test.go`:

```go
func mustUpsertMember(t *testing.T, svcCtx *svc.ServiceContext, groupID, actorType, actorID string) {
	t.Helper()
	err := svcCtx.GroupMembers.Upsert(&model.GroupMember{
		GroupID:   groupID,
		ActorType: actorType,
		ActorID:   actorID,
		Role:      "member",
	})
	if err != nil {
		t.Fatalf("upsert member: %v", err)
	}
}

func mustIssueGroupToken(t *testing.T, svcCtx *svc.ServiceContext, groupID, actorType, actorID string) string {
	t.Helper()
	token, err := svcCtx.GroupTokens.Create(&model.GroupMemberToken{
		GroupID:   groupID,
		ActorType: actorType,
		ActorID:   actorID,
	})
	if err != nil {
		t.Fatalf("issue member token: %v", err)
	}
	return token
}
```

- [ ] **Step 2: Write failing invite consume tests**

Add to `internal/svc/group_store_test.go`:

```go
func TestGroupInviteConsumeRespectsMaxUses(t *testing.T) {
	db := setupRegistryTestDB(t)
	store := NewGroupInviteStore(db)
	invite := &model.GroupInvite{
		GroupID: "group-invite",
		Role:    "member",
		MaxUses: 1,
		Status:  model.GroupStatusActive,
	}
	token, err := store.Create(invite)
	if err != nil {
		t.Fatalf("create invite: %v", err)
	}
	loaded, err := store.GetByToken(token)
	if err != nil {
		t.Fatalf("load invite: %v", err)
	}

	if err := store.Consume(loaded.ID); err != nil {
		t.Fatalf("first consume: %v", err)
	}
	if err := store.Consume(loaded.ID); err == nil {
		t.Fatal("second consume succeeded, want max uses error")
	}
}
```

- [ ] **Step 3: Run tests to verify they fail**

Run:

```bash
go test ./cmd/server ./internal/svc
```

Expected: FAIL because member delete permits deleting another member and invite consume does not enforce `max_uses`.

- [ ] **Step 4: Implement trusted-principal delete authorization**

Add to `internal/handler/group.go`:

```go
func authorizedToDeleteMember(r *http.Request, groupID, targetActorType, targetActorID string) bool {
	if r.Header.Get("X-A2A-Principal") == "admin" {
		return true
	}
	if r.Header.Get("X-A2A-Principal") != "member" {
		return false
	}
	return r.Header.Get("X-A2A-Group-ID") == groupID &&
		r.Header.Get("X-A2A-Actor-Type") == targetActorType &&
		r.Header.Get("X-A2A-Actor-ID") == targetActorID
}
```

In `GroupMemberHandler.ServeHTTP` before `GroupMembers.Delete`:

```go
actorType = svc.NormalizeActorType(actorType)
if !authorizedToDeleteMember(r, group.ID, actorType, actorID) {
	jsonError(w, "forbidden", http.StatusForbidden)
	return
}
```

- [ ] **Step 5: Implement atomic invite consume**

Replace `GroupInviteStore.Consume` in `internal/svc/group_store.go`:

```go
func (s *GroupInviteStore) Consume(id int64) error {
	res, err := s.db.Exec(`
		UPDATE group_invites
		SET used_count = used_count + 1
		WHERE id = ?
		  AND status = ?
		  AND (expires_at IS NULL OR expires_at > CURRENT_TIMESTAMP)
		  AND (max_uses <= 0 OR used_count < max_uses)`,
		id, model.GroupStatusActive,
	)
	if err != nil {
		return err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return fmt.Errorf("invite is no longer usable")
	}
	return nil
}
```

Ensure `fmt` and `model` imports are present in `internal/svc/group_store.go`.

- [ ] **Step 6: Run tests**

Run:

```bash
go test ./cmd/server ./internal/svc
```

Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add cmd/server/main_test.go internal/handler/group.go internal/svc/group_store.go internal/svc/group_store_test.go
git commit -m "fix(auth): enforce group member delete boundaries"
```

## Task 3: Store Safety, Registry Ordering, Migrations

**Files:**
- Modify: `internal/svc/store.go`
- Modify: `internal/svc/store_test.go`
- Modify: `internal/svc/task_item_store.go`
- Modify: `internal/svc/task_store_test.go`
- Modify: `internal/svc/registry.go`
- Modify: `internal/svc/registry_test.go`
- Modify: `internal/svc/servicecontext.go`

- [ ] **Step 1: Write failing TaskStore.Update whitelist test**

Add to `internal/svc/task_store_test.go`:

```go
func TestTaskStoreUpdateRejectsUnknownColumns(t *testing.T) {
	db := setupRegistryTestDB(t)
	store := NewTaskStore(db)
	task := &model.Task{LocalTaskId: "task-whitelist", AgentName: "agent", State: "PENDING"}
	if err := store.Create(task); err != nil {
		t.Fatalf("create task: %v", err)
	}

	err := store.Update("task-whitelist", map[string]interface{}{
		"state = 'DONE', agent_name": "evil",
	})
	if err == nil {
		t.Fatal("Update accepted unsafe column")
	}
}
```

- [ ] **Step 2: Write failing task claim rows affected test**

Add to `internal/svc/task_store_test.go`:

```go
func TestTaskItemClaimMissingTaskReturnsError(t *testing.T) {
	db := setupRegistryTestDB(t)
	store := NewTaskItemStore(db)

	if err := store.Claim("missing-task-item", "agent"); err == nil {
		t.Fatal("Claim missing task item succeeded")
	}
}
```

- [ ] **Step 3: Run tests to verify they fail**

Run:

```bash
go test ./internal/svc
```

Expected: FAIL because unsafe update columns and no-op claim are currently accepted.

- [ ] **Step 4: Implement task update whitelist**

Add near `TaskStore.Update` in `internal/svc/store.go`:

```go
var allowedTaskUpdateColumns = map[string]struct{}{
	"server_task_id":      {},
	"source_agent":        {},
	"target_agent":        {},
	"agent_name":          {},
	"context_id":          {},
	"root_context_id":     {},
	"parent_task_id":      {},
	"parent_tool_call_id": {},
	"state":               {},
}
```

Update the loop in `TaskStore.Update`:

```go
for k, v := range fields {
	if _, ok := allowedTaskUpdateColumns[k]; !ok {
		return fmt.Errorf("unsupported task update column %q", k)
	}
	if setClauses != "" {
		setClauses += ", "
	}
	setClauses += fmt.Sprintf("%s=?", k)
	args = append(args, v)
}
```

- [ ] **Step 5: Implement task item claim affected-row check**

Replace `TaskItemStore.Claim`:

```go
func (s *TaskItemStore) Claim(id, owner string) error {
	res, err := s.db.Exec(
		`UPDATE task_items SET owner = ?, status = 'in_progress' WHERE id = ? AND status = 'pending'`,
		owner, id,
	)
	if err != nil {
		return err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return fmt.Errorf("task item %s is not pending or does not exist", id)
	}
	return nil
}
```

- [ ] **Step 6: Add missing rows.Err checks**

In `internal/svc/store.go`, update list methods that return `result, nil` after a `rows.Next()` loop. For example, change `AgentStore.List`:

```go
for rows.Next() {
	var a model.Agent
	if err := rows.Scan(&a.Id, &a.Name, &a.Type, &a.Url, &a.Port, &a.SkillsJson, &a.Status, &a.ConnectedAt, &a.AgentCardJson, &a.ErrorMessage, &a.Secret, &a.CreatedAt, &a.UpdatedAt); err != nil {
		return nil, err
	}
	result = append(result, &a)
}
return result, rows.Err()
```

Apply the same pattern to these exact functions in `internal/svc/store.go`: `MessageStore.GetByTask`, `MessageStore.GetByContext`, `TraceStore.GetByTask`, `TraceStore.ListContexts`, and `scanTraces`.

- [ ] **Step 7: Fix registry DB-before-memory ordering**

In `AgentRegistry.RegisterAgent`, move connection insertion after successful `r.store.Upsert(dbRecord)`:

```go
if err := r.store.Upsert(dbRecord); err != nil {
	return nil, fmt.Errorf("DB persist error: %w", err)
}
conn := &AgentConnection{Card: *card, Url: url}
r.mu.Lock()
r.connections[name] = conn
r.mu.Unlock()
```

In `DisconnectAgent`, update DB first, then memory and event bus:

```go
func (r *AgentRegistry) DisconnectAgent(name string) error {
	if err := r.store.UpdateStatus(name, "disconnected", nil); err != nil {
		return err
	}
	r.mu.Lock()
	delete(r.connections, name)
	r.mu.Unlock()
	r.failMu.Lock()
	delete(r.failCounts, name)
	r.failMu.Unlock()
	if r.EventBus != nil {
		r.EventBus.AgentStatus(name, "disconnected", "")
	}
	return nil
}
```

- [ ] **Step 8: Make migrations fail on core schema errors**

Change `migrate` signature in `internal/svc/servicecontext.go`:

```go
func migrate(db *sql.DB) error {
	statements := splitStatements(mysqlSchema)
	for _, stmt := range statements {
		if stmt == "" {
			continue
		}
		if _, err := db.Exec(stmt); err != nil {
			return fmt.Errorf("core migration failed: %w", err)
		}
	}
	ensureTaskDirectionColumns(db)
	ensureContextLineageColumns(db)
	repairLegacyTaskSourcesFromToolCalls(db)
	repairLegacyContextLineageFromToolCalls(db)
	ensureMessageDirectionColumns(db)
	backfillMessageDirections(db)
	ensureHumanPresenceColumns(db)
	return nil
}
```

Update `NewServiceContext`:

```go
if err := migrate(db); err != nil {
	db.Close()
	return nil, err
}
```

Change SQLite-specific SQL in `repairLegacyContextLineagePass`:

```sql
SELECT task_id, COALESCE(root_context_id, context_id, ''), data_json, DATE_FORMAT(timestamp, '%Y-%m-%d %H:%i:%s')
FROM traces
WHERE event_type='tool_call'
ORDER BY timestamp
```

and:

```sql
AND t.created_at >= ?
```

- [ ] **Step 9: Run tests**

Run:

```bash
go test ./internal/svc
```

Expected: PASS.

- [ ] **Step 10: Commit**

```bash
git add internal/svc/store.go internal/svc/store_test.go internal/svc/task_item_store.go internal/svc/task_store_test.go internal/svc/registry.go internal/svc/registry_test.go internal/svc/servicecontext.go
git commit -m "fix(svc): harden store and registry consistency"
```

## Task 4: LLM Stream Safety

**Files:**
- Modify: `internal/llm/openai.go`
- Modify: `internal/llm/anthropic.go`
- Add: `internal/llm/stream_test.go`

- [ ] **Step 1: Write failing stream error tests**

Create `internal/llm/stream_test.go`:

```go
package llm

import (
	"io"
	"strings"
	"testing"
)

type errReader struct{}

func (errReader) Read(p []byte) (int, error) {
	copy(p, "data: ")
	return 6, io.ErrUnexpectedEOF
}

func (errReader) Close() error {
	return nil
}

func TestOpenAIReadStreamReportsMalformedJSON(t *testing.T) {
	provider := &OpenAIProvider{}
	ch := make(chan StreamEvent, 4)

	provider.readStream(io.NopCloser(strings.NewReader("data: {bad-json}\n\n")), ch)

	var sawError bool
	for evt := range ch {
		if evt.Type == "error" {
			sawError = true
		}
		if evt.Type == "done" {
			t.Fatal("malformed stream emitted done")
		}
	}
	if !sawError {
		t.Fatal("missing error event")
	}
}

func TestOpenAIReadStreamReportsScannerError(t *testing.T) {
	provider := &OpenAIProvider{}
	ch := make(chan StreamEvent, 4)

	provider.readStream(errReader{}, ch)

	var sawError bool
	for evt := range ch {
		if evt.Type == "error" {
			sawError = true
		}
	}
	if !sawError {
		t.Fatal("missing scanner error event")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run:

```bash
go test ./internal/llm
```

Expected: FAIL because malformed JSON is currently ignored and `done` is emitted.

- [ ] **Step 3: Add provider HTTP timeouts**

In `internal/llm/openai.go`, import `time` and change the client:

```go
Client: &http.Client{Timeout: 120 * time.Second},
```

In `internal/llm/anthropic.go`, import `time` and use the same timeout.

- [ ] **Step 4: Harden OpenAI stream reader**

Replace the end-to-end structure of `OpenAIProvider.readStream` with this control flow:

```go
func (p *OpenAIProvider) readStream(body io.ReadCloser, ch chan<- StreamEvent) {
	defer close(ch)
	defer body.Close()
	defer func() {
		if v := recover(); v != nil {
			ch <- StreamEvent{Type: "error", Error: fmt.Errorf("openai stream panic: %v", v)}
		}
	}()

	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	toolCalls := map[int]*ToolCall{}
	cleanDone := false

	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimPrefix(line, "data: ")
		if data == "[DONE]" {
			cleanDone = true
			break
		}

		var chunk struct {
			Choices []struct {
				Delta struct {
					Content          string `json:"content"`
					ReasoningContent string `json:"reasoning_content"`
					ToolCalls        []struct {
						Index    int    `json:"index"`
						ID       string `json:"id"`
						Function struct {
							Name      string `json:"name"`
							Arguments string `json:"arguments"`
						} `json:"function"`
					} `json:"tool_calls"`
				} `json:"delta"`
				FinishReason *string `json:"finish_reason"`
			} `json:"choices"`
		}
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			ch <- StreamEvent{Type: "error", Error: fmt.Errorf("openai stream decode: %w", err)}
			return
		}
		if len(chunk.Choices) == 0 {
			continue
		}

		delta := chunk.Choices[0].Delta
		if delta.ReasoningContent != "" {
			ch <- StreamEvent{Type: "reasoning", Reasoning: delta.ReasoningContent}
		}
		if delta.Content != "" {
			ch <- StreamEvent{Type: "text", Text: delta.Content}
		}
		for _, tc := range delta.ToolCalls {
			existing, ok := toolCalls[tc.Index]
			if !ok {
				existing = &ToolCall{ID: tc.ID, Name: tc.Function.Name}
				toolCalls[tc.Index] = existing
			}
			if tc.ID != "" {
				existing.ID = tc.ID
			}
			if tc.Function.Name != "" {
				existing.Name = tc.Function.Name
			}
			existing.Arguments += tc.Function.Arguments
		}
		if chunk.Choices[0].FinishReason != nil && *chunk.Choices[0].FinishReason == "tool_calls" {
			for _, tc := range toolCalls {
				ch <- StreamEvent{Type: "tool_call", ToolCall: &ToolCall{ID: tc.ID, Name: tc.Name, Arguments: tc.Arguments}}
			}
			toolCalls = map[int]*ToolCall{}
		}
	}
	if err := scanner.Err(); err != nil {
		ch <- StreamEvent{Type: "error", Error: fmt.Errorf("openai stream read: %w", err)}
		return
	}
	if cleanDone {
		ch <- StreamEvent{Type: "done"}
	}
}
```

- [ ] **Step 5: Apply Anthropic stream handling**

In `internal/llm/anthropic.go`, add the same timeout value as OpenAI, a recover guard that emits `StreamEvent{Type: "error"}`, a `scanner.Err()` check that emits `StreamEvent{Type: "error"}`, and a JSON decode branch that emits `StreamEvent{Type: "error"}` and returns. Anthropic emits `done` only on `message_stop`.

- [ ] **Step 6: Run tests**

Run:

```bash
go test ./internal/llm
```

Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/llm/openai.go internal/llm/anthropic.go internal/llm/stream_test.go
git commit -m "fix(llm): report stream failures"
```

## Task 5: Engine And Tool Context Propagation

**Files:**
- Modify: `internal/model/builtin_tools.go`
- Modify: `internal/engine/engine.go`
- Modify: `internal/engine/engine_test.go`
- Modify: `internal/tools/tools.go`
- Modify: `internal/tools/a2a.go`
- Modify: `internal/tools/subagent.go`
- Modify: `internal/tools/task_tools.go`
- Modify: `internal/tools/tools_test.go`

- [ ] **Step 1: Write failing engine tests**

Add to `internal/engine/engine_test.go`:

```go
func TestRunLoopAllowsNilRecordTrace(t *testing.T) {
	eng := New()
	provider := &mockProvider{events: []llm.StreamEvent{
		{Type: "tool_call", ToolCall: &llm.ToolCall{ID: "call-1", Name: "unknown_tool", Arguments: "{}"}},
		{Type: "done"},
	}}
	agent := &BuiltinAgent{Config: config.BuiltinAgent{Name: "agent", MaxToolRounds: 1}, Provider: provider}
	deps := &Deps{SaveMessage: func(m *model.Message) error { return nil }}
	rec := httptest.NewRecorder()
	flusher := http.Flusher(rec)

	if _, err := eng.runLoop(context.Background(), agent, []llm.ChatMessage{{Role: "user", Content: "hi"}}, rec, flusher, "task", "ctx", "root", "", deps); err == nil {
		t.Fatal("runLoop succeeded with unknown tool, want max/tool error path without panic")
	}
}
```

- [ ] **Step 2: Add context-aware tool API**

Modify `internal/model/builtin_tools.go`:

```go
type BuiltinTool struct {
	Name           string `json:"name"`
	Description    string `json:"description"`
	Parameters     []ToolParameter `json:"parameters"`
	Execute        func(args map[string]any) (string, error)
	ExecuteContext func(ctx context.Context, args map[string]any) (string, error)
	IsReadOnly     bool `json:"is_read_only,omitempty"`
}
```

Add import:

```go
import "context"
```

- [ ] **Step 3: Update engine tool execution**

In `internal/engine/engine.go`, wrap `deps.RecordTrace`:

```go
if deps.RecordTrace != nil {
	deps.RecordTrace(&model.TraceEvent{
		TaskId:        taskId,
		ContextId:     &contextId,
		RootContextId: stringPtr(rootContextId),
		EventType:     "tool_call",
		AgentName:     agent.Config.Name,
		DataJson:      redact.SafeTraceData(string(traceData), 4000),
	})
}
```

Add recover to read-only tool goroutine:

```go
defer func() {
	if v := recover(); v != nil {
		roMu.Lock()
		roResults[tcall.ID] = struct {
			result string
			err    error
		}{result: fmt.Sprintf("Error: tool panic: %v", v), err: fmt.Errorf("tool panic: %v", v)}
		roMu.Unlock()
	}
}()
```

Update `callToolWithTimeout` to create a child context:

```go
execCtxWithTimeout, cancel := context.WithTimeout(ctx, toolCallTimeout)
defer cancel()
go func() {
	text, err := e.callTool(execCtxWithTimeout, agent, name, arguments, execCtx)
	done <- result{text: text, err: err}
}()
```

Change `callTool` signature to:

```go
func (e *Engine) callTool(ctx context.Context, agent *BuiltinAgent, name string, arguments string, execCtx ToolExecutionContext) (string, error)
```

When invoking a builtin tool:

```go
if tool.ExecuteContext != nil {
	return tool.ExecuteContext(ctx, args)
}
return tool.Execute(args)
```

- [ ] **Step 4: Add context-aware tool functions**

In `internal/tools/tools.go`, set `ExecuteContext` for `fetch_url`:

```go
ExecuteContext: executeFetchURLContext,
```

Add:

```go
func executeFetchURL(args map[string]any) (string, error) {
	return executeFetchURLContext(context.Background(), args)
}

func executeFetchURLContext(ctx context.Context, args map[string]any) (string, error) {
```

Move the existing fetch implementation body into `executeFetchURLContext`, and replace request creation with:

```go
req, err := http.NewRequestWithContext(ctx, method, targetURL, body)
```

In `internal/tools/a2a.go`, add context-aware wrappers for `send_to_agent`, `list_groups`, `list_agents`, and `get_agent_info` by setting `ExecuteContext` and routing existing functions through `context.Background()`. Replace `http.NewRequest` calls with `http.NewRequestWithContext(ctx, ...)`.

In `internal/tools/subagent.go`, change `SpawnAgent` to return a context-aware function:

```go
func SpawnAgentContext(engine *SubagentEngine) func(context.Context, map[string]any) (string, error) {
	return func(ctx context.Context, args map[string]any) (string, error) {
		task, _ := args["task"].(string)
		if task == "" {
			return "", fmt.Errorf("task is required for spawn_agent")
		}
		contextStr, _ := args["context"].(string)
		parentContextId, _ := args["_parent_context_id"].(string)
		parentToolCallId, _ := args["_parent_tool_call_id"].(string)
		ctx, cancel := context.WithTimeout(ctx, subagentTimeout)
		defer cancel()
		result, err := engine.Run(ctx, task, contextStr, parentContextId, parentToolCallId)
		if err != nil {
			return "", fmt.Errorf("subagent execution failed: %w", err)
		}
		return result, nil
	}
}
```

Set both fields in `NewSpawnAgentTool`:

```go
Execute: func(args map[string]any) (string, error) {
	return SpawnAgentContext(engine)(context.Background(), args)
},
ExecuteContext: SpawnAgentContext(engine),
```

- [ ] **Step 5: Run tests**

Run:

```bash
go test ./internal/engine ./internal/tools
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/model/builtin_tools.go internal/engine/engine.go internal/engine/engine_test.go internal/tools/tools.go internal/tools/a2a.go internal/tools/subagent.go internal/tools/task_tools.go internal/tools/tools_test.go
git commit -m "fix(engine): propagate tool cancellation"
```

## Task 6: Bridge HTTP And CLI Safety

**Files:**
- Modify: `internal/bridge/http.go`
- Modify: `internal/bridge/cli.go`
- Add: `internal/bridge/bridge_test.go`

- [ ] **Step 1: Write failing bridge tests**

Create `internal/bridge/bridge_test.go`:

```go
package bridge

import (
	"context"
	"strings"
	"testing"

	"a2a-platform/internal/config"
)

func TestInvokeHTTPRejectsUnsupportedScheme(t *testing.T) {
	_, err := invokeHTTP(context.Background(), &config.SkillInvoke{
		Method: "GET",
		URL:    "file:///etc/passwd",
	}, nil, &TemplateContext{})
	if err == nil || !strings.Contains(err.Error(), "unsupported URL scheme") {
		t.Fatalf("err = %v, want unsupported scheme", err)
	}
}

func TestInvokeCLIBoundsOutput(t *testing.T) {
	out, err := invokeCLI(context.Background(), &config.SkillInvoke{
		Command: "printf",
		Args:    []string{"%02000000d", "1"},
		Timeout: 1000,
	}, &config.BridgeCLITarget{}, &TemplateContext{})
	if err != nil {
		t.Fatalf("invokeCLI: %v", err)
	}
	if len(out) > maxCLIOutputBytes {
		t.Fatalf("output len = %d, want <= %d", len(out), maxCLIOutputBytes)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run:

```bash
go test ./internal/bridge
```

Expected: FAIL because unsupported schemes are accepted and `maxCLIOutputBytes` does not exist.

- [ ] **Step 3: Implement HTTP URL validation and dedicated client**

In `internal/bridge/http.go`, add:

```go
func validateBridgeURL(raw string) error {
	parsed, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("parse URL: %w", err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return fmt.Errorf("unsupported URL scheme %q", parsed.Scheme)
	}
	if parsed.Host == "" {
		return fmt.Errorf("URL host is required")
	}
	return nil
}
```

Call before building the request:

```go
if err := validateBridgeURL(url); err != nil {
	return "", err
}
```

Replace `http.DefaultClient.Do(req)`:

```go
client := &http.Client{Timeout: timeout}
resp, err := client.Do(req)
```

- [ ] **Step 4: Implement bounded CLI output**

In `internal/bridge/cli.go`, add:

```go
const maxCLIOutputBytes = 1 << 20
```

After `cmd.Run()` succeeds:

```go
out := stdout.String()
if len(out) > maxCLIOutputBytes {
	out = out[:maxCLIOutputBytes]
}
return strings.TrimSpace(out), nil
```

Keep current shell mode for compatibility because bridge configs may intentionally use shell snippets. Add a comment above `exec.CommandContext`:

```go
// Bridge CLI commands are trusted operator configuration. Rendered user input
// must be passed through args in configs that require strict argument safety.
```

- [ ] **Step 5: Run tests**

Run:

```bash
go test ./internal/bridge
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/bridge/http.go internal/bridge/cli.go internal/bridge/bridge_test.go
git commit -m "fix(bridge): bound and validate external invocations"
```

## Task 7: Correct Audit And Architecture Documentation

**Files:**
- Modify: `docs/audit-report.md`
- Modify: `docs/architecture/current-architecture.html`
- Modify: `docs/production-readiness/acceptance-matrix.md`

- [ ] **Step 1: Rewrite audit report status model**

Update `docs/audit-report.md` so each audited issue is tagged:

```markdown
| Status | Meaning |
|--------|---------|
| Confirmed fixed | Verified against current code and fixed in this remediation. |
| Confirmed deferred | Verified but intentionally outside this remediation, with reason. |
| Corrected | Original finding was directionally useful but evidence, severity, or wording was wrong. |
| Rejected | Current code does not match the finding. |
```

Specific required corrections:

```markdown
- Project size corrected to 53 Go files and 14 Go test files as of 2026-05-25.
- Human handle login is reclassified from Critical vulnerability to documented passwordless convenience identity.
- Group member deletion finding is corrected to: valid same-group member tokens could delete other same-group members.
- Panic recovery finding is scoped to production code; tests may contain recover.
- ConfigureAuxiliaryAgentTools finding is scoped to DB-loaded builtin agents only.
- TaskItem claim finding is corrected to missing RowsAffected and non-transactional dependency check, not unconditional double-claim success.
```

- [ ] **Step 2: Update architecture document**

In `docs/architecture/current-architecture.html`, update these sections:

```html
<li><strong>Health checks:</strong> <code>/health</code> returns HTTP 503 when DB ping fails and marks the service as degraded.</li>
<li><strong>Human identity:</strong> Human handle login remains passwordless by design. It is a convenience identity for trusted collaboration flows, not strong account authentication.</li>
<li><strong>Group member permissions:</strong> Admin can remove any member. Member tokens can remove only their own actor from their bound group.</li>
<li><strong>Lifecycle:</strong> Shutdown stops the HTTP server, registry health checks, and DB resources through <code>ServiceContext.Close()</code>.</li>
<li><strong>External calls:</strong> LLM, bridge, tool, subagent, and A2A helper calls use bounded timeouts and propagate request cancellation where supported.</li>
```

- [ ] **Step 3: Update readiness matrix**

In `docs/production-readiness/acceptance-matrix.md`, add or update rows for:

```markdown
| Panic recovery keeps server alive | server middleware | Handler panic returns JSON 500 without crashing the process. | P0 | `cmd/server/main_test.go::TestRecoverMiddlewareReturnsJSON500` | verified | `cmd/server` |
| Passwordless human handle login is preserved and documented | human identity architecture | Handle login still issues a session and is documented as convenience identity. | P0 | existing human login tests | verified | `internal/handler`, `docs/architecture` |
| Group member cannot delete another member | group auth boundary | Same-group member token receives 403 when deleting another actor. | P0 | `cmd/server/main_test.go::TestMemberTokenCanDeleteSelfButNotOtherMember` | verified | `cmd/server`, `internal/handler` |
```

- [ ] **Step 4: Verify docs mention no stale classification**

Run:

```bash
rg -n "38 源文件|Handle-only login 无凭证验证|recover\\(\\) 出现次数为 \\*\\*0\\*\\*|仅对第一个 DB agent" docs/audit-report.md docs/architecture/current-architecture.html docs/production-readiness/acceptance-matrix.md
```

Expected: no output.

- [ ] **Step 5: Commit**

```bash
git add docs/audit-report.md docs/architecture/current-architecture.html docs/production-readiness/acceptance-matrix.md
git commit -m "docs: reconcile audit remediation status"
```

## Task 8: Full Verification

**Files:**
- Read: all modified files

- [ ] **Step 1: Run full test suite**

Run:

```bash
go test ./...
```

Expected: PASS for every package.

- [ ] **Step 2: Run targeted race-sensitive tests**

Run:

```bash
go test -race ./internal/svc ./internal/events
```

Expected: PASS.

- [ ] **Step 3: Inspect working tree**

Run:

```bash
git status --short
```

Expected: no unstaged or staged files.

- [ ] **Step 4: Summarize residual risks**

Add a final implementation note in the user-facing response with:

```markdown
Verified:
- go test ./...
- go test -race ./internal/svc ./internal/events

Residual risks:
- Passwordless handle login is intentionally convenient, not strong authentication.
- Bridge CLI shell snippets remain operator-trusted config; strict argument safety depends on config style.
```
