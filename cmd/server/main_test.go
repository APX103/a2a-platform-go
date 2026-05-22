package main

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"a2a-platform/internal/config"
	"a2a-platform/internal/model"
	"a2a-platform/internal/svc"

	_ "modernc.org/sqlite"
)

func setupSubagentRouteTestContext(t *testing.T) (*svc.ServiceContext, string, string) {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "test.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	_, err = db.Exec(`CREATE TABLE subagent_sessions (
		id TEXT PRIMARY KEY,
		parent_context_id TEXT NOT NULL,
		parent_tool_call_id TEXT,
		task TEXT,
		context TEXT,
		status TEXT NOT NULL DEFAULT 'running',
		messages TEXT,
		result TEXT,
		error TEXT,
		created_at TIMESTAMP,
		completed_at TIMESTAMP
	)`)
	if err != nil {
		t.Fatalf("create schema: %v", err)
	}

	parentContextId := "11111111-1111-1111-1111-111111111111"
	session, err := svc.NewSubagentStore(db).Create(parentContextId, "tool-1", "inspect", "context")
	if err != nil {
		t.Fatalf("create subagent: %v", err)
	}

	return &svc.ServiceContext{Subagents: svc.NewSubagentStore(db)}, parentContextId, session.ID
}

func setupGroupRouteTestContext(t *testing.T) (*svc.ServiceContext, string) {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "groups.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	svc.DBDriver = "sqlite"
	_, err = db.Exec(`
	CREATE TABLE a2a_groups (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		description TEXT,
		orchestration_mode TEXT NOT NULL DEFAULT 'leader_led',
		rules_json TEXT,
		memory_policy_json TEXT,
		status TEXT NOT NULL DEFAULT 'active',
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	);
	CREATE TABLE group_members (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		group_id TEXT NOT NULL,
		actor_type TEXT NOT NULL,
		actor_id TEXT NOT NULL,
		role TEXT NOT NULL DEFAULT 'member',
		capabilities_json TEXT,
		joined_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		UNIQUE(group_id, actor_type, actor_id)
	);
	CREATE TABLE group_events (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		group_id TEXT NOT NULL,
		event_type TEXT NOT NULL,
		sender_type TEXT NOT NULL,
		sender_id TEXT NOT NULL,
		content TEXT,
		metadata_json TEXT,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	);
	CREATE TABLE group_artifacts (
		id TEXT PRIMARY KEY,
		group_id TEXT NOT NULL,
		name TEXT NOT NULL,
		artifact_type TEXT NOT NULL DEFAULT 'document',
		version INTEGER NOT NULL DEFAULT 1,
		content TEXT,
		status TEXT NOT NULL DEFAULT 'draft',
		created_by TEXT,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	);`)
	if err != nil {
		t.Fatalf("create schema: %v", err)
	}

	ctx := &svc.ServiceContext{
		Groups:         svc.NewGroupStore(db),
		GroupMembers:   svc.NewGroupMemberStore(db),
		GroupEvents:    svc.NewGroupEventStore(db),
		GroupArtifacts: svc.NewGroupArtifactStore(db),
	}
	group := &model.Group{Name: "route group", OrchestrationMode: model.GroupModeLeaderLed}
	if err := ctx.Groups.Create(group); err != nil {
		t.Fatalf("create group: %v", err)
	}
	if err := ctx.GroupMembers.Upsert(&model.GroupMember{GroupID: group.ID, ActorType: model.GroupActorAgent, ActorID: "leader-agent", Role: "leader"}); err != nil {
		t.Fatalf("create member: %v", err)
	}
	return ctx, group.ID
}

func TestGroupRoute_JoinAndEventReturnOrchestration(t *testing.T) {
	svcCtx, groupID := setupGroupRouteTestContext(t)

	joinReq := httptest.NewRequest(http.MethodPost, "/api/groups/"+groupID+"/join", strings.NewReader(`{"client_id":"human-route"}`))
	joinRec := httptest.NewRecorder()
	makeGroupRouteHandler(svcCtx).ServeHTTP(joinRec, joinReq)
	if joinRec.Code != http.StatusOK {
		t.Fatalf("join status = %d, body=%s", joinRec.Code, joinRec.Body.String())
	}

	eventReq := httptest.NewRequest(http.MethodPost, "/api/groups/"+groupID+"/events", strings.NewReader(`{"event_type":"message","sender_type":"human","sender_id":"human-route","content":"hello"}`))
	eventRec := httptest.NewRecorder()
	makeGroupRouteHandler(svcCtx).ServeHTTP(eventRec, eventReq)
	if eventRec.Code != http.StatusOK {
		t.Fatalf("event status = %d, body=%s", eventRec.Code, eventRec.Body.String())
	}

	var resp struct {
		Orchestration model.GroupOrchestrationState `json:"orchestration"`
	}
	if err := json.Unmarshal(eventRec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Orchestration.NextAction != "leader_selects_next_speaker" {
		t.Fatalf("next action = %q", resp.Orchestration.NextAction)
	}
	if len(resp.Orchestration.EligibleSpeakers) != 1 || resp.Orchestration.EligibleSpeakers[0] != "leader-agent" {
		t.Fatalf("eligible speakers = %#v", resp.Orchestration.EligibleSpeakers)
	}
}

func TestSubagentRoute_UUIDContextListsSubagents(t *testing.T) {
	svcCtx, parentContextId, _ := setupSubagentRouteTestContext(t)
	req := httptest.NewRequest(http.MethodGet, "/api/subagents/"+parentContextId, nil)
	rec := httptest.NewRecorder()

	makeSubagentRouteHandler(svcCtx).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body=%s", rec.Code, rec.Body.String())
	}
	var resp struct {
		ContextId string            `json:"context_id"`
		Subagents []json.RawMessage `json:"subagents"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.ContextId != parentContextId {
		t.Fatalf("context_id = %q, want %q", resp.ContextId, parentContextId)
	}
	if len(resp.Subagents) != 1 {
		t.Fatalf("subagents len = %d, want 1", len(resp.Subagents))
	}
}

func TestSubagentRoute_UUIDSubagentGetsDetail(t *testing.T) {
	svcCtx, _, subagentId := setupSubagentRouteTestContext(t)
	req := httptest.NewRequest(http.MethodGet, "/api/subagents/"+subagentId, nil)
	rec := httptest.NewRecorder()

	makeSubagentRouteHandler(svcCtx).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body=%s", rec.Code, rec.Body.String())
	}
	var resp struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.ID != subagentId {
		t.Fatalf("id = %q, want %q", resp.ID, subagentId)
	}
}

func TestRequestIDMiddlewareSetsResponseHeader(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	req.Header.Set("X-Request-ID", "req-test-123")
	rec := httptest.NewRecorder()

	requestIDMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Context().Value(requestIDContextKey{}); got != "req-test-123" {
			t.Fatalf("context request id = %v, want req-test-123", got)
		}
		w.WriteHeader(http.StatusNoContent)
	})).ServeHTTP(rec, req)

	if got := rec.Header().Get("X-Request-ID"); got != "req-test-123" {
		t.Fatalf("response X-Request-ID = %q, want req-test-123", got)
	}
}

func TestAuthMiddlewareProtectsGroupManagement(t *testing.T) {
	svcCtx := &svc.ServiceContext{Config: &config.Config{AdminToken: "secret"}}
	called := false
	protected := authMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusNoContent)
	}), svcCtx)

	req := httptest.NewRequest(http.MethodPost, "/api/groups", strings.NewReader(`{"name":"x"}`))
	rec := httptest.NewRecorder()
	protected.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
	if called {
		t.Fatal("protected handler was called without admin token")
	}

	req = httptest.NewRequest(http.MethodPost, "/api/groups", strings.NewReader(`{"name":"x"}`))
	req.Header.Set("X-Admin-Token", "secret")
	rec = httptest.NewRecorder()
	protected.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("authorized status = %d, want 204", rec.Code)
	}
}
