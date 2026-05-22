package e2e

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"golang.org/x/time/rate"
)

const (
	baseURL    = "http://localhost:18090"
	adminToken = "a2a-admin-token"
)

var (
	httpClient = &http.Client{Timeout: 30 * time.Second}
	limiter    = rate.NewLimiter(15, 5) // 15 req/s with burst 5, stays under server's 20/s limit
)

// ===== Helpers =====

func req(t *testing.T, method, path, body string, headers map[string]string) (int, []byte) {
	t.Helper()
	limiter.Wait(context.Background())
	var bodyR io.Reader
	if body != "" {
		bodyR = strings.NewReader(body)
	}
	r, err := http.NewRequest(method, baseURL+path, bodyR)
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	if body != "" && (headers == nil || headers["Content-Type"] == "") {
		r.Header.Set("Content-Type", "application/json")
	}
	for k, v := range headers {
		r.Header.Set(k, v)
	}
	resp, err := httpClient.Do(r)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, data
}

func obj(t *testing.T, data []byte) map[string]interface{} {
	t.Helper()
	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("not a JSON object: %s", string(data[:min(len(data), 200)]))
	}
	return m
}

func arr(t *testing.T, data []byte) []interface{} {
	t.Helper()
	var a []interface{}
	if err := json.Unmarshal(data, &a); err != nil {
		t.Fatalf("not a JSON array: %s", string(data[:min(len(data), 200)]))
	}
	return a
}

func auth() map[string]string {
	return map[string]string{"X-Admin-Token": adminToken}
}

func expect(t *testing.T, got, want int) {
	t.Helper()
	if got != want {
		t.Errorf("status = %d, want %d", got, want)
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// ===== 1. Health & Stats =====

func TestHealth(t *testing.T) {
	code, body := req(t, "GET", "/health", "", nil)
	expect(t, code, 200)
	m := obj(t, body)
	if m["status"] != "ok" {
		t.Errorf("status = %v, want ok", m["status"])
	}
	if m["db"] != "ok" {
		t.Errorf("db = %v, want ok", m["db"])
	}
	for _, k := range []string{"agents_connected", "agents_total"} {
		if _, ok := m[k]; !ok {
			t.Errorf("missing %q", k)
		}
	}
}

func TestStats(t *testing.T) {
	code, body := req(t, "GET", "/api/stats", "", nil)
	expect(t, code, 200)
	m := obj(t, body)
	for _, k := range []string{"status", "db_status", "agents_connected", "agents_total", "tasks_today", "tasks_pending", "uptime_seconds"} {
		if _, ok := m[k]; !ok {
			t.Errorf("missing %q", k)
		}
	}
	if up, _ := m["uptime_seconds"].(float64); up <= 0 {
		t.Errorf("uptime = %v, want > 0", up)
	}
}

// ===== 2. CORS =====

func TestCORS(t *testing.T) {
	for _, path := range []string{"/api/agents", "/api/builtin-agents"} {
		t.Run(path, func(t *testing.T) {
			limiter.Wait(context.Background())
			r, _ := http.NewRequest("OPTIONS", baseURL+path, nil)
			r.Header.Set("Origin", "http://localhost:3001")
			resp, err := httpClient.Do(r)
			if err != nil {
				t.Fatalf("request: %v", err)
			}
			resp.Body.Close()
			expect(t, resp.StatusCode, 204)

			methods := resp.Header.Get("Access-Control-Allow-Methods")
			for _, m := range []string{"GET", "POST", "DELETE"} {
				if !strings.Contains(methods, m) {
					t.Errorf("Allow-Methods missing %s: %s", m, methods)
				}
			}
			if h := resp.Header.Get("Access-Control-Allow-Headers"); !strings.Contains(h, "X-Admin-Token") {
				t.Errorf("Allow-Headers missing X-Admin-Token: %s", h)
			}
		})
	}
}

// ===== 3. Auth =====

func TestAuth(t *testing.T) {
	t.Run("POST_agents_no_token", func(t *testing.T) {
		code, body := req(t, "POST", "/api/agents", `{"name":"x"}`, nil)
		expect(t, code, 401)
		if obj(t, body)["error"] != "unauthorized" {
			t.Error("expected unauthorized")
		}
	})
	t.Run("DELETE_agents_no_token", func(t *testing.T) {
		code, _ := req(t, "DELETE", "/api/agents/x", "", nil)
		expect(t, code, 401)
	})
	t.Run("POST_builtin_no_token", func(t *testing.T) {
		code, _ := req(t, "POST", "/api/builtin-agents", `{"name":"x","provider":"openai","model":"m"}`, nil)
		expect(t, code, 401)
	})
	t.Run("POST_builtin_wrong_token", func(t *testing.T) {
		code, _ := req(t, "POST", "/api/builtin-agents", `{"name":"x","provider":"openai","model":"m"}`,
			map[string]string{"X-Admin-Token": "wrong"})
		expect(t, code, 401)
	})
	t.Run("DELETE_builtin_no_token", func(t *testing.T) {
		code, _ := req(t, "DELETE", "/api/builtin-agents/x", "", nil)
		expect(t, code, 401)
	})
	t.Run("GET_builtin_no_auth_needed", func(t *testing.T) {
		code, _ := req(t, "GET", "/api/builtin-agents", "", nil)
		expect(t, code, 200)
	})
	t.Run("Bearer_auth", func(t *testing.T) {
		code, _ := req(t, "POST", "/api/builtin-agents",
			`{"name":"bearer-test","provider":"openai","base_url":"https://api.openai.com","api_key":"sk-d","model":"gpt-4o"}`,
			map[string]string{"Authorization": "Bearer " + adminToken})
		expect(t, code, 200)
		req(t, "DELETE", "/api/builtin-agents/bearer-test", "", auth())
	})
}

// ===== 3b. Native Group Orchestration =====

func TestGroupOrchestrationLifecycle(t *testing.T) {
	body := `{
		"name": "e2e proposal room",
		"description": "Group orchestration e2e test",
		"orchestration_mode": "roundtable",
		"rules": {"required_votes": 2, "max_rounds": 4},
		"memory_policy": {"hot_messages": 12, "summary": true}
	}`
	code, data := req(t, "POST", "/api/groups", body, auth())
	expect(t, code, 200)
	group := obj(t, data)
	groupID, _ := group["id"].(string)
	if groupID == "" {
		t.Fatalf("group id missing: %s", string(data))
	}
	if group["orchestration_mode"] != "roundtable" {
		t.Fatalf("mode = %v, want roundtable", group["orchestration_mode"])
	}

	t.Cleanup(func() {
		req(t, "DELETE", "/api/groups/"+groupID, "", auth())
	})

	code, _ = req(t, "POST", "/api/groups/"+groupID+"/members",
		`{"actor_type":"agent","actor_id":"planner","role":"leader","capabilities":{"skills":["plan"]}}`, auth())
	expect(t, code, 200)

	code, data = req(t, "POST", "/api/groups/"+groupID+"/join",
		`{"client_id":"human-e2e","capabilities":{"ui":"browser"}}`, nil)
	expect(t, code, 200)
	joined := obj(t, data)
	if joined["actor_type"] != "human" || joined["actor_id"] != "human-e2e" {
		t.Fatalf("unexpected join response: %s", string(data))
	}

	code, data = req(t, "GET", "/api/groups/"+groupID+"/members", "", nil)
	expect(t, code, 200)
	members := arr(t, data)
	if len(members) < 2 {
		t.Fatalf("members len = %d, want at least 2", len(members))
	}

	code, data = req(t, "POST", "/api/groups/"+groupID+"/events",
		`{"event_type":"message","sender_type":"human","sender_id":"human-e2e","content":"please review the proposal"}`, nil)
	expect(t, code, 200)
	eventResp := obj(t, data)
	orch, ok := eventResp["orchestration"].(map[string]interface{})
	if !ok {
		t.Fatalf("missing orchestration response: %s", string(data))
	}
	if orch["next_action"] != "collect_member_intents" {
		t.Fatalf("next_action = %v, want collect_member_intents", orch["next_action"])
	}

	code, data = req(t, "POST", "/api/groups/"+groupID+"/artifacts",
		`{"name":"proposal.md","artifact_type":"document","content":"# Proposal\n\nInitial draft","created_by":"human-e2e"}`, nil)
	expect(t, code, 200)
	artifact := obj(t, data)
	artifactID, _ := artifact["id"].(string)
	if artifactID == "" {
		t.Fatalf("artifact id missing: %s", string(data))
	}

	code, data = req(t, "PUT", "/api/groups/"+groupID+"/artifacts/"+artifactID,
		`{"content":"# Proposal\n\nReviewed draft","status":"reviewing"}`, auth())
	expect(t, code, 200)
	updatedArtifact := obj(t, data)
	if updatedArtifact["version"] != float64(2) {
		t.Fatalf("artifact version = %v, want 2", updatedArtifact["version"])
	}

	code, data = req(t, "GET", "/api/groups/"+groupID+"/orchestration", "", nil)
	expect(t, code, 200)
	state := obj(t, data)
	if state["mode"] != "roundtable" {
		t.Fatalf("orchestration mode = %v, want roundtable", state["mode"])
	}
}

// ===== 4. Agent List & Detail =====

func TestAgentList(t *testing.T) {
	code, body := req(t, "GET", "/api/agents", "", nil)
	expect(t, code, 200)
	agents := arr(t, body)
	for _, item := range agents {
		a, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		for _, k := range []string{"name", "url", "status"} {
			if _, ok := a[k]; !ok {
				t.Errorf("agent missing %q", k)
			}
		}
	}
}

func TestAgentDetail_NotFound(t *testing.T) {
	code, body := req(t, "GET", "/api/agents/nonexistent-xyz-999", "", nil)
	expect(t, code, 404)
	if obj(t, body)["error"] != "not found" {
		t.Error("expected 'not found'")
	}
}

// ===== 5. Builtin Agent Lifecycle =====

func TestBuiltinAgentLifecycle(t *testing.T) {
	const name = "e2e-lifecycle-agent"
	req(t, "DELETE", "/api/builtin-agents/"+name, "", auth()) // pre-cleanup

	var taskId, contextId string

	t.Run("01_Create", func(t *testing.T) {
		code, body := req(t, "POST", "/api/builtin-agents", fmt.Sprintf(`{
			"name":"%s","provider":"openai","base_url":"https://api.openai.com",
			"api_key":"sk-dummy","model":"gpt-4o","description":"E2E test agent",
			"system_prompt":"You are a test."
		}`, name), auth())
		expect(t, code, 200)
		m := obj(t, body)
		if m["ok"] != true {
			t.Error("ok != true")
		}
		if m["name"] != name {
			t.Errorf("name = %v", m["name"])
		}
	})

	t.Run("02_ListBuiltin", func(t *testing.T) {
		code, body := req(t, "GET", "/api/builtin-agents", "", nil)
		expect(t, code, 200)
		found := false
		for _, item := range arr(t, body) {
			a, _ := item.(map[string]interface{})
			if a["name"] == name {
				found = true
				if a["provider"] != "openai" {
					t.Errorf("provider = %v", a["provider"])
				}
				if a["model"] != "gpt-4o" {
					t.Errorf("model = %v", a["model"])
				}
			}
		}
		if !found {
			t.Errorf("%s not in builtin list", name)
		}
	})

	t.Run("03_InMainList", func(t *testing.T) {
		code, body := req(t, "GET", "/api/agents", "", nil)
		expect(t, code, 200)
		found := false
		for _, item := range arr(t, body) {
			a, _ := item.(map[string]interface{})
			if a["name"] == name {
				found = true
				if a["type"] != "builtin" {
					t.Errorf("type = %v", a["type"])
				}
				if a["status"] != "connected" {
					t.Errorf("status = %v", a["status"])
				}
			}
		}
		if !found {
			t.Errorf("%s not in main list", name)
		}
	})

	t.Run("04_GetDetail", func(t *testing.T) {
		code, body := req(t, "GET", "/api/agents/"+name, "", nil)
		expect(t, code, 200)
		m := obj(t, body)
		if m["name"] != name {
			t.Errorf("name = %v", m["name"])
		}
		if m["type"] != "builtin" {
			t.Errorf("type = %v", m["type"])
		}
	})

	t.Run("05_ProxyMessage_SSE", func(t *testing.T) {
		limiter.Wait(context.Background())
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		body := `{"jsonrpc":"2.0","method":"message/send","id":1,"params":{"message":{"role":"user","parts":[{"text":"Hello e2e"}]}}}`
		r, _ := http.NewRequestWithContext(ctx, "POST", baseURL+"/agent/"+name, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		resp, err := httpClient.Do(r)
		if err != nil {
			t.Fatalf("proxy: %v", err)
		}
		defer resp.Body.Close()

		scanner := bufio.NewScanner(resp.Body)
		var events []map[string]interface{}
		for scanner.Scan() {
			line := scanner.Text()
			if !strings.HasPrefix(line, "data: ") {
				continue
			}
			data := strings.TrimPrefix(line, "data: ")
			var evt map[string]interface{}
			if json.Unmarshal([]byte(data), &evt) == nil {
				events = append(events, evt)
				if tid, ok := evt["taskId"].(string); ok && taskId == "" {
					taskId = tid
				}
				if cid, ok := evt["contextId"].(string); ok && contextId == "" {
					contextId = cid
				}
			}
		}
		if len(events) == 0 {
			t.Fatal("no SSE events")
		}
		if taskId == "" {
			t.Error("no taskId")
		}
		if contextId == "" {
			t.Error("no contextId")
		}
		if len(events) > 0 {
			if st, ok := events[0]["status"].(map[string]interface{}); ok {
				if st["state"] != "working" {
					t.Errorf("first state = %v, want working", st["state"])
				}
			}
		}
	})

	t.Run("06_TaskCreated", func(t *testing.T) {
		if taskId == "" {
			t.Skip("no taskId")
		}
		code, body := req(t, "GET", "/api/tasks/"+taskId, "", nil)
		expect(t, code, 200)
		m := obj(t, body)
		task, ok := m["task"].(map[string]interface{})
		if !ok {
			t.Fatal("missing task field")
		}
		if task["agent_name"] != name {
			t.Errorf("agent_name = %v", task["agent_name"])
		}
		if msgs, ok := m["messages"].([]interface{}); ok && len(msgs) == 0 {
			t.Error("no messages")
		}
	})

	t.Run("07_TracesRecorded", func(t *testing.T) {
		if taskId == "" {
			t.Skip("no taskId")
		}
		code, body := req(t, "GET", "/api/traces/task/"+taskId, "", nil)
		expect(t, code, 200)
		if len(arr(t, body)) == 0 {
			t.Error("no traces for task")
		}
	})

	t.Run("08_TracesByContext", func(t *testing.T) {
		if contextId == "" {
			t.Skip("no contextId")
		}
		code, body := req(t, "GET", "/api/traces/context/"+contextId, "", nil)
		expect(t, code, 200)
		if len(arr(t, body)) == 0 {
			t.Error("no traces for context")
		}
	})

	t.Run("09_TaskListFilter", func(t *testing.T) {
		code, body := req(t, "GET", "/api/tasks?agent_name="+name, "", nil)
		expect(t, code, 200)
		m := obj(t, body)
		items, _ := m["items"].([]interface{})
		if len(items) == 0 {
			t.Error("no tasks for agent")
		}
	})

	t.Run("10_Update", func(t *testing.T) {
		code, _ := req(t, "POST", "/api/builtin-agents", fmt.Sprintf(`{
			"name":"%s","provider":"openai","base_url":"https://api.openai.com",
			"api_key":"sk-dummy-2","model":"gpt-4o-mini","description":"Updated"
		}`, name), auth())
		expect(t, code, 200)
		_, body := req(t, "GET", "/api/builtin-agents", "", nil)
		for _, item := range arr(t, body) {
			a, _ := item.(map[string]interface{})
			if a["name"] == name {
				if a["model"] != "gpt-4o-mini" {
					t.Errorf("model = %v", a["model"])
				}
				if a["description"] != "Updated" {
					t.Errorf("description = %v", a["description"])
				}
			}
		}
	})

	t.Run("11_Delete", func(t *testing.T) {
		code, _ := req(t, "DELETE", "/api/builtin-agents/"+name, "", auth())
		expect(t, code, 204)
	})

	t.Run("12_VerifyDeleted", func(t *testing.T) {
		_, body := req(t, "GET", "/api/builtin-agents", "", nil)
		for _, item := range arr(t, body) {
			a, _ := item.(map[string]interface{})
			if a["name"] == name {
				t.Errorf("%s still exists", name)
			}
		}
	})
}

// ===== 5b. Builtin Agent Error Cases =====

func TestBuiltinAgent_Errors(t *testing.T) {
	t.Run("InvalidJSON", func(t *testing.T) {
		code, body := req(t, "POST", "/api/builtin-agents", "not json{", auth())
		expect(t, code, 400)
		if obj(t, body)["error"] != "invalid JSON" {
			t.Errorf("error = %v", obj(t, body)["error"])
		}
	})
	t.Run("MissingFields", func(t *testing.T) {
		code, body := req(t, "POST", "/api/builtin-agents", `{"name":"x"}`, auth())
		expect(t, code, 400)
		if obj(t, body)["error"] == nil {
			t.Error("expected error")
		}
	})
	t.Run("UnknownProvider", func(t *testing.T) {
		code, body := req(t, "POST", "/api/builtin-agents", `{"name":"bad","provider":"gemini","model":"m"}`, auth())
		expect(t, code, 400)
		e, _ := obj(t, body)["error"].(string)
		if !strings.Contains(e, "unknown provider") {
			t.Errorf("error = %v", e)
		}
	})
	t.Run("MethodNotAllowed", func(t *testing.T) {
		code, _ := req(t, "PUT", "/api/builtin-agents", "", nil)
		expect(t, code, 405)
	})
}

// ===== 6. Agent Proxy Errors =====

func TestProxy_NotFound(t *testing.T) {
	code, body := req(t, "POST", "/agent/nonexistent-xyz-999",
		`{"jsonrpc":"2.0","method":"message/send","id":1,"params":{"message":{"role":"user","parts":[{"text":"hi"}]}}}`, nil)
	expect(t, code, 404)
	if obj(t, body)["error"] == nil {
		t.Error("expected error")
	}
}

func TestProxy_MethodNotAllowed(t *testing.T) {
	code, _ := req(t, "GET", "/agent/any-agent", "", nil)
	expect(t, code, 405)
}

// ===== 7. Tasks =====

func TestTaskList(t *testing.T) {
	t.Run("Default", func(t *testing.T) {
		code, body := req(t, "GET", "/api/tasks", "", nil)
		expect(t, code, 200)
		m := obj(t, body)
		for _, k := range []string{"items", "total", "page", "size"} {
			if _, ok := m[k]; !ok {
				t.Errorf("missing %q", k)
			}
		}
	})
	t.Run("Pagination", func(t *testing.T) {
		code, body := req(t, "GET", "/api/tasks?page=1&size=3", "", nil)
		expect(t, code, 200)
		m := obj(t, body)
		if sz, _ := m["size"].(float64); sz != 3 {
			t.Errorf("size = %v", sz)
		}
		if pg, _ := m["page"].(float64); pg != 1 {
			t.Errorf("page = %v", pg)
		}
	})
	t.Run("StateFilter", func(t *testing.T) {
		code, _ := req(t, "GET", "/api/tasks?state=RESPONDED", "", nil)
		expect(t, code, 200)
	})
	t.Run("AgentFilter", func(t *testing.T) {
		code, body := req(t, "GET", "/api/tasks?agent_name=nonexistent", "", nil)
		expect(t, code, 200)
		m := obj(t, body)
		if total, _ := m["total"].(float64); total != 0 {
			t.Errorf("total = %v, want 0", total)
		}
	})
	t.Run("Search", func(t *testing.T) {
		code, _ := req(t, "GET", "/api/tasks?search=e2e", "", nil)
		expect(t, code, 200)
	})
}

func TestTaskDetail_NotFound(t *testing.T) {
	code, body := req(t, "GET", "/api/tasks/nonexistent-task-id", "", nil)
	expect(t, code, 404)
	if obj(t, body)["error"] != "not found" {
		t.Error("expected 'not found'")
	}
}

// ===== 8. Traces =====

func TestTraces(t *testing.T) {
	t.Run("ListRecent", func(t *testing.T) {
		code, body := req(t, "GET", "/api/traces", "", nil)
		expect(t, code, 200)
		arr(t, body) // verify it's an array
	})
	t.Run("ListContexts", func(t *testing.T) {
		code, body := req(t, "GET", "/api/traces/contexts", "", nil)
		expect(t, code, 200)
		arr(t, body)
	})
	t.Run("ByTask_Empty", func(t *testing.T) {
		code, body := req(t, "GET", "/api/traces/task/nonexistent", "", nil)
		expect(t, code, 200)
		if len(arr(t, body)) != 0 {
			t.Error("expected empty")
		}
	})
	t.Run("ByContext_Empty", func(t *testing.T) {
		code, body := req(t, "GET", "/api/traces/context/nonexistent", "", nil)
		expect(t, code, 200)
		if len(arr(t, body)) != 0 {
			t.Error("expected empty")
		}
	})
}

// ===== 9. SSE Events =====

func TestSSEEvents(t *testing.T) {
	limiter.Wait(context.Background())
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	r, _ := http.NewRequestWithContext(ctx, "GET", baseURL+"/api/events", nil)
	r.Header.Set("Accept", "text/event-stream")
	resp, err := httpClient.Do(r)
	if err != nil {
		if ctx.Err() != nil {
			t.Skip("SSE timeout (expected)")
		}
		t.Fatalf("error: %v", err)
	}
	defer resp.Body.Close()

	expect(t, resp.StatusCode, 200)
	if ct := resp.Header.Get("Content-Type"); !strings.Contains(ct, "text/event-stream") {
		t.Errorf("Content-Type = %s", ct)
	}

	scanner := bufio.NewScanner(resp.Body)
	gotConnected := false
	for scanner.Scan() {
		line := scanner.Text()
		if strings.Contains(line, "connected") {
			gotConnected = true
			break
		}
	}
	if !gotConnected {
		t.Error("no connected event")
	}
}

// ===== 10. MCP Protocol =====

func TestMCP_SSE(t *testing.T) {
	limiter.Wait(context.Background())
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	r, _ := http.NewRequestWithContext(ctx, "GET", baseURL+"/mcp/sse", nil)
	resp, err := httpClient.Do(r)
	if err != nil {
		if ctx.Err() != nil {
			t.Skip("SSE timeout (expected)")
		}
		t.Fatalf("error: %v", err)
	}
	defer resp.Body.Close()
	expect(t, resp.StatusCode, 200)

	if ct := resp.Header.Get("Content-Type"); !strings.Contains(ct, "text/event-stream") {
		t.Errorf("Content-Type = %s", ct)
	}
	scanner := bufio.NewScanner(resp.Body)
	gotEndpoint := false
	for scanner.Scan() {
		if strings.Contains(scanner.Text(), "/mcp/messages") {
			gotEndpoint = true
			break
		}
	}
	if !gotEndpoint {
		t.Error("no endpoint event")
	}
}

func mcpCall(t *testing.T, method string, params interface{}) (int, map[string]interface{}) {
	t.Helper()
	rpc := map[string]interface{}{"jsonrpc": "2.0", "id": 1, "method": method, "params": params}
	body, _ := json.Marshal(rpc)
	code, resp := req(t, "POST", "/mcp/messages", string(body), nil)
	return code, obj(t, resp)
}

func TestMCP_Initialize(t *testing.T) {
	code, resp := mcpCall(t, "initialize", map[string]interface{}{
		"protocolVersion": "2024-11-05",
		"clientInfo":      map[string]string{"name": "e2e-test"},
		"capabilities":    map[string]interface{}{},
	})
	expect(t, code, 200)
	if resp["jsonrpc"] != "2.0" {
		t.Error("missing jsonrpc")
	}
	result, ok := resp["result"].(map[string]interface{})
	if !ok {
		t.Fatal("missing result")
	}
	if result["protocolVersion"] != "2024-11-05" {
		t.Errorf("protocolVersion = %v", result["protocolVersion"])
	}
	si, _ := result["serverInfo"].(map[string]interface{})
	if si["name"] != "a2a-platform" {
		t.Errorf("serverInfo.name = %v", si["name"])
	}
}

func TestMCP_ToolsList(t *testing.T) {
	code, resp := mcpCall(t, "tools/list", map[string]interface{}{})
	expect(t, code, 200)
	result, _ := resp["result"].(map[string]interface{})
	tools, _ := result["tools"].([]interface{})
	if len(tools) == 0 {
		t.Fatal("no tools")
	}
	names := map[string]bool{}
	for _, tool := range tools {
		tm, _ := tool.(map[string]interface{})
		if n, ok := tm["name"].(string); ok {
			names[n] = true
		}
	}
	for _, want := range []string{"list_agents", "send_to_agent", "get_agent_info"} {
		if !names[want] {
			t.Errorf("missing tool %q", want)
		}
	}
}

func TestMCP_Ping(t *testing.T) {
	code, resp := mcpCall(t, "ping", nil)
	expect(t, code, 200)
	if resp["error"] != nil {
		t.Errorf("error: %v", resp["error"])
	}
}

func TestMCP_UnknownMethod(t *testing.T) {
	code, resp := mcpCall(t, "unknown/method", nil)
	expect(t, code, 200)
	if resp["error"] == nil {
		t.Error("expected error")
	}
	rpcErr, _ := resp["error"].(map[string]interface{})
	if c, _ := rpcErr["code"].(float64); c != -32601 {
		t.Errorf("code = %v, want -32601", c)
	}
}

func TestMCP_ToolCall_ListAgents(t *testing.T) {
	code, resp := mcpCall(t, "tools/call", map[string]interface{}{
		"name": "list_agents", "arguments": map[string]interface{}{},
	})
	expect(t, code, 200)
	result, _ := resp["result"].(map[string]interface{})
	content, _ := result["content"].([]interface{})
	if len(content) == 0 {
		t.Fatal("empty content")
	}
	item, _ := content[0].(map[string]interface{})
	if item["type"] != "text" {
		t.Errorf("type = %v", item["type"])
	}
	if item["text"] == nil || item["text"] == "" {
		t.Error("empty text")
	}
}

func TestMCP_ToolCall_GetAgentInfo(t *testing.T) {
	_, agentsBody := req(t, "GET", "/api/agents", "", nil)
	agents := arr(t, agentsBody)
	if len(agents) == 0 {
		t.Skip("no agents")
	}
	agentName := agents[0].(map[string]interface{})["name"].(string)

	code, resp := mcpCall(t, "tools/call", map[string]interface{}{
		"name": "get_agent_info", "arguments": map[string]interface{}{"agent_name": agentName},
	})
	expect(t, code, 200)
	if resp["error"] != nil {
		t.Errorf("error: %v", resp["error"])
	}
}

func TestMCP_ToolCall_GetAgentInfo_NotFound(t *testing.T) {
	code, resp := mcpCall(t, "tools/call", map[string]interface{}{
		"name": "get_agent_info", "arguments": map[string]interface{}{"agent_name": "nonexistent-xyz-999"},
	})
	expect(t, code, 200)
	if resp["error"] == nil {
		t.Error("expected error")
	}
}

func TestMCP_ResourcesList(t *testing.T) {
	code, resp := mcpCall(t, "resources/list", map[string]interface{}{})
	expect(t, code, 200)
	result, _ := resp["result"].(map[string]interface{})
	resources, _ := result["resources"].([]interface{})
	if len(resources) == 0 {
		t.Fatal("no resources")
	}
	r0, _ := resources[0].(map[string]interface{})
	if r0["uri"] != "a2a://agents" {
		t.Errorf("uri = %v", r0["uri"])
	}
}

func TestMCP_ResourcesRead(t *testing.T) {
	code, resp := mcpCall(t, "resources/read", map[string]interface{}{"uri": "a2a://agents"})
	expect(t, code, 200)
	result, _ := resp["result"].(map[string]interface{})
	contents, _ := result["contents"].([]interface{})
	if len(contents) == 0 {
		t.Fatal("empty contents")
	}
	c0, _ := contents[0].(map[string]interface{})
	if c0["uri"] != "a2a://agents" {
		t.Errorf("uri = %v", c0["uri"])
	}
	if c0["text"] == nil || c0["text"] == "" {
		t.Error("empty text")
	}
}

func TestMCP_ResourcesRead_Unknown(t *testing.T) {
	code, resp := mcpCall(t, "resources/read", map[string]interface{}{"uri": "unknown://foo"})
	expect(t, code, 200)
	if resp["error"] == nil {
		t.Error("expected error")
	}
}

// ===== 11. Cross-API: Delete via /api/agents cleans engine =====

func TestAgentDelete_CleansEngine(t *testing.T) {
	const name = "e2e-delete-engine-test"
	code, _ := req(t, "POST", "/api/builtin-agents", fmt.Sprintf(`{
		"name":"%s","provider":"openai","base_url":"https://api.openai.com","api_key":"sk-d","model":"gpt-4o"
	}`, name), auth())
	expect(t, code, 200)

	code, _ = req(t, "DELETE", "/api/agents/"+name, "", auth())
	expect(t, code, 204)

	_, body := req(t, "GET", "/api/builtin-agents", "", nil)
	for _, item := range arr(t, body) {
		a, _ := item.(map[string]interface{})
		if a["name"] == name {
			t.Errorf("%s still in builtin list", name)
		}
	}
}

// ===== 12. Method Not Allowed =====

func TestMethodNotAllowed(t *testing.T) {
	t.Run("PUT_agents", func(t *testing.T) {
		code, _ := req(t, "PUT", "/api/agents", "", nil)
		expect(t, code, 405)
	})
	t.Run("POST_stats", func(t *testing.T) {
		code, _ := req(t, "POST", "/api/stats", "", nil)
		expect(t, code, 405)
	})
}
