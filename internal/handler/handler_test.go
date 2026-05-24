package handler

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"a2a-platform/internal/bridge"
	"a2a-platform/internal/engine"
	"a2a-platform/internal/model"
	"a2a-platform/internal/svc"
	"a2a-platform/internal/testutil"
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

func TestAgentProxyRecordsPlatformTextDeltaSSE(t *testing.T) {
	db := setupAgentProxyLineageDB(t)
	agentServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher := w.(http.Flusher)
		w.Write([]byte(`event: text.delta
data: {"type":"text.delta","text":"hel"}

event: text.delta
data: {"type":"text.delta","text":"lo"}

`))
		flusher.Flush()
	}))
	defer agentServer.Close()

	agentStore := svc.NewAgentStore(db)
	registry := svc.NewAgentRegistry(agentStore)
	if _, err := registry.RegisterAgent("delta-agent", "external", agentServer.URL, 0, nil, "", model.ContextModeContext, &model.AgentCard{
		Name:        "delta-agent",
		Description: "delta agent",
		Version:     "1.0.0",
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

	body := `{"jsonrpc":"2.0","id":"1","method":"SendStreamingMessage","params":{"message":{"role":"ROLE_USER","parts":[{"text":"hello"}]}}}`
	req := httptest.NewRequest(http.MethodPost, "/agent/delta-agent", strings.NewReader(body))
	req.Header.Set("X-Path-Param-Name", "delta-agent")
	rec := httptest.NewRecorder()

	NewAgentProxyHandler(svcCtx).ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body=%s", rec.Code, rec.Body.String())
	}

	var taskID, state string
	if err := db.QueryRow(`SELECT local_task_id, state FROM tasks WHERE target_agent='delta-agent'`).Scan(&taskID, &state); err != nil {
		t.Fatalf("query task: %v", err)
	}
	if state != "RESPONDED" {
		t.Fatalf("state = %q, want RESPONDED", state)
	}
	var content string
	if err := db.QueryRow(`SELECT content FROM messages WHERE task_id=? AND role='agent'`, taskID).Scan(&content); err != nil {
		t.Fatalf("query agent message: %v", err)
	}
	if content != "hello" {
		t.Fatalf("agent message = %q, want hello", content)
	}
}

func TestAgentProxyRedactsRequestBodyInSendTrace(t *testing.T) {
	db := setupAgentProxyLineageDB(t)
	agentServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"jsonrpc":"2.0","id":"1","result":{"message":{"role":"ROLE_AGENT","parts":[{"text":"ok"}]}}}`))
	}))
	defer agentServer.Close()

	agentStore := svc.NewAgentStore(db)
	registry := svc.NewAgentRegistry(agentStore)
	if _, err := registry.RegisterAgent("secret-agent", "external", agentServer.URL, 0, nil, "", model.ContextModeContext, &model.AgentCard{
		Name:        "secret-agent",
		Description: "secret agent",
		Version:     "1.0.0",
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

	body := `{"jsonrpc":"2.0","id":"1","method":"SendMessage","params":{"message":{"role":"ROLE_USER","parts":[{"text":"use sk-secret-value"}],"metadata":{"secret":"root-secret"}}}}`
	req := httptest.NewRequest(http.MethodPost, "/agent/secret-agent", strings.NewReader(body))
	req.Header.Set("X-Path-Param-Name", "secret-agent")
	rec := httptest.NewRecorder()

	NewAgentProxyHandler(svcCtx).ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body=%s", rec.Code, rec.Body.String())
	}

	rows, err := db.Query(`SELECT data_json FROM traces`)
	if err != nil {
		t.Fatalf("query traces: %v", err)
	}
	defer rows.Close()
	traceCount := 0
	for rows.Next() {
		traceCount++
		var data string
		if err := rows.Scan(&data); err != nil {
			t.Fatalf("scan trace data: %v", err)
		}
		if strings.Contains(data, "sk-secret-value") || strings.Contains(data, "root-secret") {
			t.Fatalf("trace data leaked secret: %s", data)
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate traces: %v", err)
	}
	if traceCount == 0 {
		t.Fatal("expected traces to be recorded")
	}
}

func TestAgentProxyRedactsStreamingResponseTrace(t *testing.T) {
	db := setupAgentProxyLineageDB(t)
	agentServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher := w.(http.Flusher)
		w.Write([]byte(`event: text.delta
data: {"type":"text.delta","text":"sk-stream-secret"}

`))
		flusher.Flush()
	}))
	defer agentServer.Close()

	agentStore := svc.NewAgentStore(db)
	registry := svc.NewAgentRegistry(agentStore)
	if _, err := registry.RegisterAgent("stream-secret-agent", "external", agentServer.URL, 0, nil, "", model.ContextModeContext, &model.AgentCard{
		Name:        "stream-secret-agent",
		Description: "stream secret agent",
		Version:     "1.0.0",
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

	body := `{"jsonrpc":"2.0","id":"1","method":"SendStreamingMessage","params":{"message":{"role":"ROLE_USER","parts":[{"text":"hello"}]}}}`
	req := httptest.NewRequest(http.MethodPost, "/agent/stream-secret-agent", strings.NewReader(body))
	req.Header.Set("X-Path-Param-Name", "stream-secret-agent")
	rec := httptest.NewRecorder()

	NewAgentProxyHandler(svcCtx).ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "sk-stream-secret") {
		t.Fatalf("relayed body = %q, want original stream secret", rec.Body.String())
	}

	rows, err := db.Query(`SELECT data_json FROM traces`)
	if err != nil {
		t.Fatalf("query traces: %v", err)
	}
	defer rows.Close()
	traceCount := 0
	for rows.Next() {
		traceCount++
		var data string
		if err := rows.Scan(&data); err != nil {
			t.Fatalf("scan trace data: %v", err)
		}
		if strings.Contains(data, "sk-stream-secret") {
			t.Fatalf("trace data leaked streaming secret: %s", data)
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate traces: %v", err)
	}
	if traceCount == 0 {
		t.Fatal("expected traces to be recorded")
	}
}

func TestAgentProxyConnectionFailureRecordsErrorTrace(t *testing.T) {
	db := setupAgentProxyLineageDB(t)
	agentStore := svc.NewAgentStore(db)
	registry := svc.NewAgentRegistry(agentStore)
	if _, err := registry.RegisterAgent("down-agent", "external", "http://127.0.0.1:1", 0, nil, "", model.ContextModeContext, &model.AgentCard{
		Name:        "down-agent",
		Description: "down agent",
		Version:     "1.0.0",
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

	var taskID, state string
	if err := db.QueryRow(`SELECT local_task_id, state FROM tasks WHERE target_agent='down-agent'`).Scan(&taskID, &state); err != nil {
		t.Fatalf("query task: %v", err)
	}
	if state != "ERROR" {
		t.Fatalf("state = %q, want ERROR", state)
	}

	var errorTraceCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM traces WHERE task_id=? AND event_type='error' AND target_agent='down-agent'`, taskID).Scan(&errorTraceCount); err != nil {
		t.Fatalf("query error traces: %v", err)
	}
	if errorTraceCount != 1 {
		t.Fatalf("error trace count = %d, want 1", errorTraceCount)
	}
}

func TestFreeChatCandidates(t *testing.T) {
	members := []*model.GroupMember{
		{ActorType: model.GroupActorAgent, ActorID: "planner", Role: "member"},
		{ActorType: model.GroupActorAgent, ActorID: "reviewer", Role: "reviewer"},
		{ActorType: model.GroupActorAgent, ActorID: "observer", Role: "observer"},
		{ActorType: model.GroupActorHuman, ActorID: "human-local", Role: "member"},
		{ActorType: model.GroupActorAgent, ActorID: "planner", Role: "member"},
	}

	got := freeChatCandidates(members, model.GroupActorAgent, "planner")
	want := []string{"reviewer"}
	if len(got) != len(want) || got[0] != want[0] {
		t.Fatalf("candidates = %#v, want %#v", got, want)
	}

	got = freeChatCandidates(members, model.GroupActorHuman, "human-local")
	want = []string{"planner", "reviewer"}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("human-triggered candidates = %#v, want %#v", got, want)
	}
}

func TestFreeChatRulesAndNoReply(t *testing.T) {
	rules := parseGroupRules(`{"max_speakers":2,"max_rounds":4}`)
	if rules.MaxSpeakers != 2 || rules.MaxRounds != 4 {
		t.Fatalf("rules = %#v", rules)
	}
	if got := freeChatHotMessageLimit(`{"hot_messages":200}`); got != 80 {
		t.Fatalf("hot limit = %d, want 80", got)
	}
	for _, text := range []string{"NO_REPLY", "`NO_REPLY`", "不回复"} {
		if !isNoReply(text) {
			t.Fatalf("%q should be treated as no reply", text)
		}
	}
	if isNoReply("我来补充一点") {
		t.Fatal("normal reply was treated as no reply")
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
	db := testutil.TempMySQLDB(t)
	schema := `
	CREATE TABLE tasks (
		id BIGINT AUTO_INCREMENT PRIMARY KEY,
		local_task_id VARCHAR(64) NOT NULL UNIQUE,
		server_task_id VARCHAR(64),
		source_agent VARCHAR(255),
		target_agent VARCHAR(255),
		agent_name VARCHAR(255) NOT NULL,
		context_id VARCHAR(64),
		root_context_id VARCHAR(64),
		parent_task_id VARCHAR(64),
		parent_tool_call_id VARCHAR(128),
		state VARCHAR(32) NOT NULL DEFAULT 'PENDING',
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
	CREATE TABLE messages (
		id BIGINT AUTO_INCREMENT PRIMARY KEY,
		task_id VARCHAR(64) NOT NULL,
		context_id VARCHAR(64),
		role VARCHAR(16) NOT NULL,
		sender_agent VARCHAR(255),
		recipient_agent VARCHAR(255),
		content TEXT,
		reasoning_content TEXT,
		tool_calls JSON,
		tool_call_id VARCHAR(64),
		thinking_blocks JSON,
		timestamp TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
	CREATE TABLE traces (
		id BIGINT AUTO_INCREMENT PRIMARY KEY,
		task_id VARCHAR(64) NOT NULL,
		context_id VARCHAR(64),
		root_context_id VARCHAR(64),
		parent_task_id VARCHAR(64),
		timestamp TIMESTAMP(3) DEFAULT CURRENT_TIMESTAMP(3),
		event_type VARCHAR(32) NOT NULL,
		agent_name VARCHAR(255) NOT NULL,
		target_agent VARCHAR(255),
		data_json TEXT,
		duration_ms BIGINT
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
	CREATE TABLE agents (
		id BIGINT AUTO_INCREMENT PRIMARY KEY,
		name VARCHAR(255) NOT NULL UNIQUE,
		type VARCHAR(64) NOT NULL DEFAULT '',
		url VARCHAR(512) NOT NULL DEFAULT '',
		port INT NOT NULL DEFAULT 0,
		skills_json TEXT,
		status VARCHAR(32) NOT NULL DEFAULT 'disconnected',
		connected_at VARCHAR(64),
		agent_card_json TEXT,
		error_message TEXT,
		secret VARCHAR(255) NOT NULL DEFAULT '',
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;`
	if _, err := db.Exec(schema); err != nil {
		t.Fatalf("create schema: %v", err)
	}
	return db
}
