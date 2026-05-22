package main

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

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
