package handler

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"a2a-platform/internal/bridge"
	"a2a-platform/internal/engine"
	"a2a-platform/internal/model"
	"a2a-platform/internal/svc"

	_ "modernc.org/sqlite"
)

func TestAgentProxyRejectsInvalidJSON(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/agent/test", strings.NewReader("{"))
	req.Header.Set("X-Path-Param-Name", "test")
	rec := httptest.NewRecorder()

	NewAgentProxyHandler(&svc.ServiceContext{}).ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400, body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "invalid JSON") {
		t.Fatalf("body = %q, want invalid JSON error", rec.Body.String())
	}
}

func TestApplyContextModeToRPC_StatelessStripsContextId(t *testing.T) {
	rpcReq := map[string]interface{}{
		"params": map[string]interface{}{
			"contextId": "client-context",
			"message": map[string]interface{}{
				"parts": []interface{}{map[string]interface{}{"text": "hello"}},
			},
		},
	}

	contextId := applyContextModeToRPC(rpcReq, model.ContextModeStateless)
	if contextId != nil {
		t.Fatalf("contextId = %v, want nil", *contextId)
	}
	params, ok := rpcReq["params"].(map[string]interface{})
	if !ok {
		t.Fatalf("params missing: %#v", rpcReq)
	}
	if _, ok := params["contextId"]; ok {
		t.Fatalf("contextId was forwarded in stateless mode: %#v", params)
	}
}

func TestApplyContextModeToRPC_ContextInjectsContextId(t *testing.T) {
	rpcReq := map[string]interface{}{
		"params": map[string]interface{}{
			"message": map[string]interface{}{
				"parts": []interface{}{map[string]interface{}{"text": "hello"}},
			},
		},
	}

	contextId := applyContextModeToRPC(rpcReq, model.ContextModeContext)
	if contextId == nil || *contextId == "" {
		t.Fatal("contextId was not generated")
	}
	params, ok := rpcReq["params"].(map[string]interface{})
	if !ok {
		t.Fatalf("params missing: %#v", rpcReq)
	}
	if got, _ := params["contextId"].(string); got != *contextId {
		t.Fatalf("injected contextId = %q, want %q", got, *contextId)
	}
}

func TestResolveRootContextId_HeaderWins(t *testing.T) {
	rpcReq := map[string]interface{}{
		"params": map[string]interface{}{
			"rootContextId": "params-root",
			"contextId":     "ctx-1",
		},
	}
	req := httptest.NewRequest(http.MethodPost, "/agent/test", nil)
	req.Header.Set("X-A2A-Root-Context-Id", "header-root")
	ctx := "ctx-1"

	root := resolveRootContextId(req, rpcReq, &ctx, "")
	if root == nil || *root != "header-root" {
		t.Fatalf("root = %v, want header-root", root)
	}
}

func TestResolveRootContextId_FallsBackToOriginalContextForStateless(t *testing.T) {
	rpcReq := map[string]interface{}{
		"params": map[string]interface{}{
			"contextId": "ui-context",
		},
	}
	originalContextId := getRPCStringParam(rpcReq, "contextId")
	contextId := applyContextModeToRPC(rpcReq, model.ContextModeStateless)
	req := httptest.NewRequest(http.MethodPost, "/agent/test", nil)

	root := resolveRootContextId(req, rpcReq, contextId, originalContextId)
	if root == nil || *root != "ui-context" {
		t.Fatalf("root = %v, want ui-context", root)
	}
}

func TestAgentProxyRecordsRootAndParentLineage(t *testing.T) {
	db := setupAgentProxyLineageDB(t)
	svcCtx := &svc.ServiceContext{
		DB:             db,
		Agents:         svc.NewAgentStore(db),
		Tasks:          svc.NewTaskStore(db),
		Messages:       svc.NewMessageStore(db),
		Traces:         svc.NewTraceStore(db),
		Registry:       svc.NewAgentRegistry(svc.NewAgentStore(db)),
		Engine:         engine.New(),
		BridgeRegistry: bridge.NewRegistry(),
	}

	body := `{"jsonrpc":"2.0","id":"1","method":"SendStreamingMessage","params":{"message":{"role":"ROLE_USER","parts":[{"text":"hello child"}]}}}`
	req := httptest.NewRequest(http.MethodPost, "/agent/missing-child", strings.NewReader(body))
	req.Header.Set("X-Path-Param-Name", "missing-child")
	req.Header.Set("X-A2A-Source-Agent", "mi-1")
	req.Header.Set("X-A2A-Root-Context-Id", "root-1")
	req.Header.Set("X-A2A-Parent-Task-Id", "task-parent")
	req.Header.Set("X-A2A-Parent-Tool-Call-Id", "tool-parent")
	rec := httptest.NewRecorder()

	NewAgentProxyHandler(svcCtx).ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404, body=%s", rec.Code, rec.Body.String())
	}

	var task model.Task
	var contextId, rootContextId, parentTaskId, parentToolCallId, sourceAgent sql.NullString
	if err := db.QueryRow(`SELECT local_task_id, source_agent, context_id, root_context_id, parent_task_id, parent_tool_call_id
		FROM tasks WHERE target_agent='missing-child'`).Scan(&task.LocalTaskId, &sourceAgent, &contextId, &rootContextId, &parentTaskId, &parentToolCallId); err != nil {
		t.Fatalf("query task: %v", err)
	}
	if !contextId.Valid || contextId.String == "" {
		t.Fatal("child context_id was not generated")
	}
	if !rootContextId.Valid || rootContextId.String != "root-1" {
		t.Fatalf("root_context_id = %v, want root-1", rootContextId)
	}
	if !parentTaskId.Valid || parentTaskId.String != "task-parent" {
		t.Fatalf("parent_task_id = %v, want task-parent", parentTaskId)
	}
	if !parentToolCallId.Valid || parentToolCallId.String != "tool-parent" {
		t.Fatalf("parent_tool_call_id = %v, want tool-parent", parentToolCallId)
	}
	if !sourceAgent.Valid || sourceAgent.String != "mi-1" {
		t.Fatalf("source_agent = %v, want mi-1", sourceAgent)
	}

	var traceRoot, traceParent sql.NullString
	var traceBody string
	if err := db.QueryRow(`SELECT root_context_id, parent_task_id, data_json FROM traces WHERE task_id=? AND event_type='send'`, task.LocalTaskId).
		Scan(&traceRoot, &traceParent, &traceBody); err != nil {
		t.Fatalf("query trace: %v", err)
	}
	if !traceRoot.Valid || traceRoot.String != "root-1" {
		t.Fatalf("trace root_context_id = %v, want root-1", traceRoot)
	}
	if !traceParent.Valid || traceParent.String != "task-parent" {
		t.Fatalf("trace parent_task_id = %v, want task-parent", traceParent)
	}
	var rpc map[string]any
	if err := json.Unmarshal([]byte(traceBody), &rpc); err != nil {
		t.Fatalf("decode trace body: %v", err)
	}
	params, _ := rpc["params"].(map[string]any)
	if got, _ := params["contextId"].(string); got != contextId.String {
		t.Fatalf("forwarded contextId = %q, want generated child context %q", got, contextId.String)
	}
	if got, ok := params["rootContextId"]; ok {
		t.Fatalf("rootContextId leaked into forwarded params: %#v", got)
	}
}

func setupAgentProxyLineageDB(t *testing.T) *sql.DB {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "lineage.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	svc.DBDriver = "sqlite"
	schema := `
	CREATE TABLE tasks (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		local_task_id TEXT NOT NULL UNIQUE,
		server_task_id TEXT,
		source_agent TEXT,
		target_agent TEXT,
		agent_name TEXT NOT NULL,
		context_id TEXT,
		root_context_id TEXT,
		parent_task_id TEXT,
		parent_tool_call_id TEXT,
		state TEXT NOT NULL DEFAULT 'PENDING',
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	);
	CREATE TABLE messages (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		task_id TEXT NOT NULL,
		context_id TEXT,
		role TEXT NOT NULL,
		sender_agent TEXT,
		recipient_agent TEXT,
		content TEXT,
		reasoning_content TEXT,
		tool_calls TEXT,
		tool_call_id TEXT,
		thinking_blocks TEXT,
		timestamp TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	);
	CREATE TABLE traces (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		task_id TEXT NOT NULL,
		context_id TEXT,
		root_context_id TEXT,
		parent_task_id TEXT,
		timestamp TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		event_type TEXT NOT NULL,
		agent_name TEXT NOT NULL,
		target_agent TEXT,
		data_json TEXT,
		duration_ms INTEGER
	);
	CREATE TABLE agents (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT NOT NULL UNIQUE,
		type TEXT NOT NULL DEFAULT '',
		url TEXT NOT NULL DEFAULT '',
		port INTEGER NOT NULL DEFAULT 0,
		skills_json TEXT,
		status TEXT NOT NULL DEFAULT 'disconnected',
		connected_at TEXT,
		agent_card_json TEXT,
		error_message TEXT,
		secret TEXT NOT NULL DEFAULT '',
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	);`
	if _, err := db.Exec(schema); err != nil {
		t.Fatalf("create schema: %v", err)
	}
	return db
}
