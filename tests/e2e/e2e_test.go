package e2e

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"golang.org/x/time/rate"
)

const (
	defaultBaseURL    = "http://localhost:18090"
	defaultAdminToken = "a2a-admin-token"
)

var (
	testConfig = loadE2EConfig()
	baseURL    = strings.TrimRight(testConfig.BaseURL, "/")
	adminToken = testConfig.AdminToken
	httpClient = &http.Client{Timeout: 180 * time.Second}
	limiter    = rate.NewLimiter(15, 5) // 15 req/s with burst 5, stays under server's 20/s limit
)

// ===== Helpers =====

type e2eConfig struct {
	BaseURL    string            `json:"base_url"`
	AdminToken string            `json:"admin_token"`
	FreeChat   e2eFreeChatConfig `json:"free_chat"`
}

type e2eFreeChatConfig struct {
	Enabled          bool     `json:"enabled"`
	AgentNames       []string `json:"agent_names"`
	MinBuiltinAgents int      `json:"min_builtin_agents"`
	Prompt           string   `json:"prompt"`
}

func loadE2EConfig() e2eConfig {
	cfg := e2eConfig{
		BaseURL:    defaultBaseURL,
		AdminToken: defaultAdminToken,
		FreeChat: e2eFreeChatConfig{
			MinBuiltinAgents: 3,
			Prompt:           "编排 e2e 测试：请群内每个 agent 各用一句中文说明自己在这个问题讨论中能承担的角色。不要调用工具，不要输出 NO_REPLY。",
		},
	}
	for _, path := range e2eConfigPaths() {
		data, err := os.ReadFile(path)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: read e2e config %s: %v\n", path, err)
			continue
		}
		if err := json.Unmarshal(data, &cfg); err != nil {
			fmt.Fprintf(os.Stderr, "warning: parse e2e config %s: %v\n", path, err)
		}
		break
	}
	if env := strings.TrimSpace(os.Getenv("A2A_SERVER_URL")); env != "" {
		cfg.BaseURL = env
	}
	if env := strings.TrimSpace(os.Getenv("ADMIN_TOKEN")); env != "" {
		cfg.AdminToken = env
	}
	if strings.TrimSpace(cfg.BaseURL) == "" {
		cfg.BaseURL = defaultBaseURL
	}
	if strings.TrimSpace(cfg.AdminToken) == "" {
		cfg.AdminToken = defaultAdminToken
	}
	if cfg.FreeChat.MinBuiltinAgents <= 0 {
		cfg.FreeChat.MinBuiltinAgents = 3
	}
	if strings.TrimSpace(cfg.FreeChat.Prompt) == "" {
		cfg.FreeChat.Prompt = "编排 e2e 测试：请群内每个 agent 各用一句中文说明自己在这个问题讨论中能承担的角色。不要调用工具，不要输出 NO_REPLY。"
	}
	return cfg
}

func e2eConfigPaths() []string {
	if path := strings.TrimSpace(os.Getenv("A2A_E2E_CONFIG")); path != "" {
		return []string{path}
	}
	return []string{
		"e2e.config.json",
		filepath.Join("tests", "e2e", "e2e.config.json"),
	}
}

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

func streamReq(t *testing.T, path, body string, headers map[string]string) []map[string]interface{} {
	t.Helper()
	limiter.Wait(context.Background())
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()
	r, err := http.NewRequestWithContext(ctx, "POST", baseURL+path, strings.NewReader(body))
	if err != nil {
		t.Fatalf("create stream request: %v", err)
	}
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Accept", "text/event-stream")
	for k, v := range headers {
		r.Header.Set(k, v)
	}
	resp, err := httpClient.Do(r)
	if err != nil {
		t.Fatalf("stream POST %s: %v", path, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		data, _ := io.ReadAll(resp.Body)
		t.Fatalf("stream status = %d, want 200: %s", resp.StatusCode, string(data))
	}
	if ct := resp.Header.Get("Content-Type"); !strings.Contains(ct, "text/event-stream") {
		t.Fatalf("stream content-type = %s, want text/event-stream", ct)
	}
	scanner := bufio.NewScanner(resp.Body)
	events := []map[string]interface{}{}
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		var item map[string]interface{}
		if err := json.Unmarshal([]byte(strings.TrimSpace(strings.TrimPrefix(line, "data:"))), &item); err != nil {
			t.Fatalf("invalid SSE data: %v", err)
		}
		events = append(events, item)
		if item["type"] == "group.error" {
			t.Fatalf("group stream error: %#v", item)
		}
		if item["type"] == "group.done" {
			break
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("read stream: %v", err)
	}
	if len(events) == 0 {
		t.Fatal("no SSE events")
	}
	return events
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

func connectedBuiltinAgents(t *testing.T, minCount int) []string {
	t.Helper()
	code, body := req(t, "GET", "/api/agents", "", auth())
	expect(t, code, 200)
	agents := []string{}
	for _, item := range arr(t, body) {
		agent, _ := item.(map[string]interface{})
		if agent["type"] == "builtin" && agent["status"] == "connected" {
			if name, _ := agent["name"].(string); name != "" {
				agents = append(agents, name)
			}
		}
	}
	sort.Strings(agents)
	preferred := make([]string, 0, len(agents))
	for _, name := range agents {
		if !strings.HasPrefix(name, "e2e-") && name != "bearer-test" {
			preferred = append(preferred, name)
		}
	}
	if len(preferred) >= minCount {
		agents = preferred
	}
	if len(agents) < minCount {
		t.Fatalf("connected builtin agents = %v, want at least %d", agents, minCount)
	}
	return agents
}

func freeChatE2EAgents(t *testing.T) []string {
	t.Helper()
	cfg := testConfig.FreeChat
	if !cfg.Enabled {
		t.Skip("free_chat e2e disabled; copy tests/e2e/e2e.config.example.json to e2e.config.json and set free_chat.enabled=true")
	}
	if len(cfg.AgentNames) > 0 {
		return cfg.AgentNames
	}
	agents := connectedBuiltinAgents(t, cfg.MinBuiltinAgents)
	return agents[:cfg.MinBuiltinAgents]
}

func joinHumanByInvite(t *testing.T, groupID, actorID string) map[string]string {
	t.Helper()
	code, data := req(t, "POST", "/api/groups/"+groupID+"/invites",
		`{"actor_type_allowed":"human","role":"member","max_uses":1}`, auth())
	expect(t, code, 200)
	inviteToken, _ := obj(t, data)["token"].(string)
	if inviteToken == "" {
		t.Fatalf("invite token missing: %s", string(data))
	}
	code, data = req(t, "POST", "/api/group-joins",
		fmt.Sprintf(`{"invite_token":%q,"actor_type":"human","actor_id":%q,"capabilities":{"ui":"e2e"}}`, inviteToken, actorID), nil)
	expect(t, code, 200)
	accessToken, _ := obj(t, data)["access_token"].(string)
	if accessToken == "" {
		t.Fatalf("access token missing: %s", string(data))
	}
	return map[string]string{"Authorization": "Bearer " + accessToken}
}

func assertGroupTraceRecorded(t *testing.T, groupID string) {
	t.Helper()
	code, body := req(t, "GET", "/api/traces/context/group:"+groupID, "", auth())
	expect(t, code, 200)
	traces := arr(t, body)
	if len(traces) == 0 {
		t.Fatalf("no traces recorded for group context %s", groupID)
	}
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
	t.Run("GET_builtin_requires_token", func(t *testing.T) {
		code, _ := req(t, "GET", "/api/builtin-agents", "", nil)
		expect(t, code, 401)
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

	code, data = req(t, "POST", "/api/groups/"+groupID+"/invites",
		`{"actor_type_allowed":"human","role":"member","max_uses":2}`, auth())
	expect(t, code, 200)
	invite := obj(t, data)
	inviteToken, _ := invite["token"].(string)
	if inviteToken == "" {
		t.Fatalf("invite token missing: %s", string(data))
	}

	code, data = req(t, "POST", "/api/groups/"+groupID+"/members", "", nil)
	expect(t, code, 401)

	code, data = req(t, "GET", "/api/groups/"+groupID+"/members", "", nil)
	expect(t, code, 401)

	code, data = req(t, "POST", "/api/group-joins",
		fmt.Sprintf(`{"invite_token":%q,"actor_type":"human","actor_id":"human-e2e","capabilities":{"ui":"browser"}}`, inviteToken), nil)
	expect(t, code, 200)
	joined := obj(t, data)
	member, _ := joined["member"].(map[string]interface{})
	accessToken, _ := joined["access_token"].(string)
	if member["actor_type"] != "human" || member["actor_id"] != "human-e2e" || accessToken == "" {
		t.Fatalf("unexpected join response: %s", string(data))
	}
	memberAuth := map[string]string{"Authorization": "Bearer " + accessToken}

	code, data = req(t, "GET", "/api/groups/"+groupID+"/members", "", memberAuth)
	expect(t, code, 200)
	members := arr(t, data)
	if len(members) < 2 {
		t.Fatalf("members len = %d, want at least 2", len(members))
	}

	code, data = req(t, "POST", "/api/groups/"+groupID+"/events",
		`{"event_type":"message","sender_type":"human","sender_id":"intruder","content":"please review the proposal"}`, memberAuth)
	expect(t, code, 403)

	code, data = req(t, "POST", "/api/groups/"+groupID+"/events",
		`{"event_type":"message","sender_type":"human","sender_id":"human-e2e","content":"please review the proposal"}`, memberAuth)
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
		`{"name":"proposal.md","artifact_type":"document","content":"# Proposal\n\nInitial draft","created_by":"human-e2e"}`, memberAuth)
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

	code, data = req(t, "GET", "/api/groups/"+groupID+"/orchestration", "", memberAuth)
	expect(t, code, 200)
	state := obj(t, data)
	if state["mode"] != "roundtable" {
		t.Fatalf("orchestration mode = %v, want roundtable", state["mode"])
	}
}

func TestGroupFreeChatInvokesBuiltinAgents(t *testing.T) {
	agents := freeChatE2EAgents(t)

	body := fmt.Sprintf(`{
		"name": %q,
		"description": "E2E free-chat orchestration with builtin agents",
		"orchestration_mode": "free_chat",
		"rules": {"max_speakers": %d, "max_rounds": 1},
		"memory_policy": {"hot_messages": 20, "summary": true}
	}`, "e2e free chat "+time.Now().Format("150405.000"), len(agents))
	code, data := req(t, "POST", "/api/groups", body, auth())
	expect(t, code, 200)
	group := obj(t, data)
	groupID, _ := group["id"].(string)
	if groupID == "" {
		t.Fatalf("group id missing: %s", string(data))
	}
	if group["orchestration_mode"] != "free_chat" {
		t.Fatalf("mode = %v, want free_chat", group["orchestration_mode"])
	}
	t.Cleanup(func() {
		req(t, "DELETE", "/api/groups/"+groupID, "", auth())
	})

	for _, name := range agents {
		code, data = req(t, "POST", "/api/groups/"+groupID+"/members",
			fmt.Sprintf(`{"actor_type":"agent","actor_id":%q,"role":"member","capabilities":{"source":"e2e"}}`, name), auth())
		expect(t, code, 200)
	}

	humanID := "human-e2e-free-chat-" + strings.ReplaceAll(time.Now().Format("150405.000"), ".", "")
	memberAuth := joinHumanByInvite(t, groupID, humanID)

	code, data = req(t, "GET", "/api/groups/"+groupID+"/orchestration", "", memberAuth)
	expect(t, code, 200)
	state := obj(t, data)
	if state["mode"] != "free_chat" {
		t.Fatalf("state mode = %v, want free_chat", state["mode"])
	}
	if state["next_action"] != "agents_observe_and_optionally_reply" {
		t.Fatalf("next_action = %v, want agents_observe_and_optionally_reply", state["next_action"])
	}

	streamEvents := streamReq(t, "/api/groups/"+groupID+"/events",
		fmt.Sprintf(`{"event_type":"message","sender_type":"human","sender_id":%q,"content":%q}`, humanID, testConfig.FreeChat.Prompt), memberAuth)
	var event map[string]interface{}
	var done map[string]interface{}
	var streamArtifact map[string]interface{}
	deltaCount := 0
	for _, item := range streamEvents {
		switch item["type"] {
		case "group.event":
			ev, _ := item["event"].(map[string]interface{})
			if ev["sender_type"] == "human" && ev["sender_id"] == humanID {
				event = ev
			}
		case "group.agent_delta":
			if strings.TrimSpace(fmt.Sprint(item["text"])) != "" {
				deltaCount++
			}
		case "group.artifact":
			streamArtifact, _ = item["artifact"].(map[string]interface{})
		case "group.done":
			done = item
		}
	}
	if event == nil {
		t.Fatalf("stream did not include human group.event: %#v", streamEvents)
	}
	if done == nil {
		t.Fatalf("stream did not include group.done: %#v", streamEvents)
	}
	if deltaCount == 0 {
		t.Fatalf("stream did not include any group.agent_delta events: %#v", streamEvents)
	}
	if streamArtifact == nil {
		t.Fatalf("stream did not include group.artifact: %#v", streamEvents)
	}
	triggerEventID, _ := event["id"].(float64)
	if triggerEventID == 0 {
		t.Fatalf("trigger event id missing: %#v", event)
	}
	triggered, _ := done["triggered"].([]interface{})
	if len(triggered) == 0 {
		t.Fatalf("free_chat produced no triggered agent events: %#v", done)
	}
	if len(triggered) > len(agents) {
		t.Fatalf("triggered len = %d, want <= %d", len(triggered), len(agents))
	}

	allowed := map[string]bool{}
	for _, name := range agents {
		allowed[name] = true
	}
	for _, item := range triggered {
		ev, _ := item.(map[string]interface{})
		if ev["event_type"] != "message" || ev["sender_type"] != "agent" {
			t.Fatalf("triggered event is not an agent message: %#v", ev)
		}
		senderID, _ := ev["sender_id"].(string)
		if !allowed[senderID] {
			t.Fatalf("unexpected triggered sender %q, allowed %v", senderID, agents)
		}
		if strings.TrimSpace(fmt.Sprint(ev["content"])) == "" {
			t.Fatalf("triggered event has empty content: %#v", ev)
		}
		metadataJSON, _ := ev["metadata_json"].(string)
		var metadata map[string]interface{}
		if err := json.Unmarshal([]byte(metadataJSON), &metadata); err != nil {
			t.Fatalf("invalid metadata_json %q: %v", metadataJSON, err)
		}
		if metadata["orchestration"] != "free_chat" {
			t.Fatalf("metadata orchestration = %v, want free_chat", metadata["orchestration"])
		}
		if metadata["trigger_event_id"] != triggerEventID {
			t.Fatalf("metadata trigger_event_id = %v, want %v", metadata["trigger_event_id"], triggerEventID)
		}
	}

	code, data = req(t, "GET", "/api/groups/"+groupID+"/events?limit=20", "", memberAuth)
	expect(t, code, 200)
	events := arr(t, data)
	seenHuman := false
	seenAgentReplies := 0
	for _, item := range events {
		ev, _ := item.(map[string]interface{})
		if ev["sender_type"] == "human" && ev["sender_id"] == humanID {
			seenHuman = true
		}
		if ev["sender_type"] == "agent" {
			metadataJSON, _ := ev["metadata_json"].(string)
			if strings.Contains(metadataJSON, `"orchestration":"free_chat"`) {
				seenAgentReplies++
			}
		}
	}
	if !seenHuman {
		t.Fatalf("human trigger message was not persisted in group events")
	}
	if seenAgentReplies < len(triggered) {
		t.Fatalf("persisted free_chat replies = %d, triggered = %d", seenAgentReplies, len(triggered))
	}

	code, data = req(t, "GET", "/api/groups/"+groupID+"/artifacts", "", memberAuth)
	expect(t, code, 200)
	artifacts := arr(t, data)
	if len(artifacts) == 0 {
		t.Fatalf("free_chat did not create an auto artifact")
	}
	autoArtifact := map[string]interface{}{}
	for _, item := range artifacts {
		artifact, _ := item.(map[string]interface{})
		if artifact["created_by"] == "platform" {
			autoArtifact = artifact
			break
		}
	}
	if len(autoArtifact) == 0 {
		t.Fatalf("no platform-created artifact found: %#v", artifacts)
	}
	if autoArtifact["name"] != "group-discussion.md" {
		t.Fatalf("artifact name = %v, want group-discussion.md", autoArtifact["name"])
	}
	content := fmt.Sprint(autoArtifact["content"])
	if !strings.Contains(content, testConfig.FreeChat.Prompt) || !strings.Contains(content, "Agent Outputs") {
		t.Fatalf("auto artifact content did not include prompt and agent output section: %s", content)
	}

	assertGroupTraceRecorded(t, groupID)
}

// ===== 4. Agent List & Detail =====

func TestAgentList(t *testing.T) {
	code, _ := req(t, "GET", "/api/agents", "", nil)
	expect(t, code, 401)

	code, body := req(t, "GET", "/api/agents", "", auth())
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
	code, body := req(t, "GET", "/api/agents/nonexistent-xyz-999", "", auth())
	expect(t, code, 404)
	if obj(t, body)["error"] != "not found" {
		t.Error("expected 'not found'")
	}
}

func TestExternalAgentCardHostingAndUpdate(t *testing.T) {
	const name = "e2e-external-card-agent"
	req(t, "DELETE", "/api/agents/"+name, "", auth())

	body := fmt.Sprintf(`{
		"name": %q,
		"type": "external",
		"url": "http://127.0.0.1:65530/a2a",
		"context_mode": "stateless",
		"agent_card": {
			"description": "Original hosted card",
			"version": "1.0.0",
			"skills": [{"id":"chat","name":"Chat","description":"General chat"}]
		}
	}`, name)
	code, data := req(t, "POST", "/api/agents", body, auth())
	expect(t, code, 200)
	if obj(t, data)["ok"] != true {
		t.Fatalf("register response = %s", string(data))
	}
	defer req(t, "DELETE", "/api/agents/"+name, "", auth())

	code, data = req(t, "GET", "/api/agents/"+name, "", auth())
	expect(t, code, 200)
	agent := obj(t, data)
	if agent["url"] != "/agent/"+name {
		t.Fatalf("public agent url = %v", agent["url"])
	}
	var card map[string]interface{}
	if err := json.Unmarshal([]byte(agent["agent_card_json"].(string)), &card); err != nil {
		t.Fatalf("decode card: %v", err)
	}
	if card["url"] != "/agent/"+name {
		t.Fatalf("hosted card url = %v", card["url"])
	}
	if card["x_context_mode"] != "stateless" {
		t.Fatalf("context mode = %v", card["x_context_mode"])
	}

	code, _ = req(t, "GET", "/.well-known/agent-card/"+name, "", nil)
	expect(t, code, 401)
	code, data = req(t, "GET", "/.well-known/agent-card/"+name, "", auth())
	expect(t, code, 200)
	card = obj(t, data)
	if card["url"] != "/agent/"+name {
		t.Fatalf("well-known card url = %v", card["url"])
	}

	update := fmt.Sprintf(`{
		"url": "http://127.0.0.1:65531/a2a",
		"context_mode": "context",
		"agent_card": {
			"name": %q,
			"description": "Updated hosted card",
			"version": "2.0.0",
			"skills": [{"id":"review","name":"Review","description":"Review work"}]
		}
	}`, name)
	code, data = req(t, "PUT", "/api/agents/"+name, update, auth())
	expect(t, code, 200)
	agent = obj(t, data)
	if agent["description"] != "Updated hosted card" || agent["version"] != "2.0.0" {
		t.Fatalf("updated agent = %#v", agent)
	}
	if agent["context_mode"] != "context" {
		t.Fatalf("updated context mode = %v", agent["context_mode"])
	}
}

func TestSimpleModeAgentJoinsDefaultP2PGroup(t *testing.T) {
	const name = "e2e-simple-mode-agent"
	req(t, "DELETE", "/api/groups/default-p2p/members/agent/"+name, "", auth())
	req(t, "DELETE", "/api/agents/"+name, "", auth())
	t.Cleanup(func() {
		req(t, "DELETE", "/api/groups/default-p2p/members/agent/"+name, "", auth())
		req(t, "DELETE", "/api/agents/"+name, "", auth())
	})

	body := fmt.Sprintf(`{
		"name": %q,
		"type": "external",
		"url": "http://127.0.0.1:9",
		"context_mode": "stateless",
		"simple_mode": true,
		"agent_card": {
			"description": "Simple mode test agent",
			"version": "1.0.0",
			"skills": [{"id":"chat","name":"Chat","description":"P2P chat"}]
		}
	}`, name)
	code, data := req(t, "POST", "/api/agents", body, auth())
	expect(t, code, 200)
	resp := obj(t, data)
	if resp["simple_mode"] != true || resp["default_group_id"] != "default-p2p" {
		t.Fatalf("unexpected simple mode response: %s", string(data))
	}

	code, data = req(t, "GET", "/api/groups/default-p2p", "", auth())
	expect(t, code, 200)
	group := obj(t, data)
	if group["orchestration_mode"] != "p2p" || group["status"] != "active" {
		t.Fatalf("default group = %#v, want active p2p", group)
	}

	code, data = req(t, "GET", "/api/groups/default-p2p/members", "", auth())
	expect(t, code, 200)
	members := arr(t, data)
	seen := false
	for _, item := range members {
		member, _ := item.(map[string]interface{})
		if member["actor_type"] == "agent" && member["actor_id"] == name {
			seen = true
		}
	}
	if !seen {
		t.Fatalf("simple mode agent not found in default-p2p members: %#v", members)
	}

	code, data = req(t, "DELETE", "/api/groups/default-p2p/members/agent/"+name, "", auth())
	expect(t, code, 200)
	members = arr(t, data)
	for _, item := range members {
		member, _ := item.(map[string]interface{})
		if member["actor_type"] == "agent" && member["actor_id"] == name {
			t.Fatalf("simple mode agent still present after remove: %#v", members)
		}
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
		code, body := req(t, "GET", "/api/builtin-agents", "", auth())
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
		code, body := req(t, "GET", "/api/agents", "", auth())
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
		code, body := req(t, "GET", "/api/agents/"+name, "", auth())
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
		r.Header.Set("X-Admin-Token", adminToken)
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
		code, body := req(t, "GET", "/api/tasks/"+taskId, "", auth())
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
		code, body := req(t, "GET", "/api/traces/task/"+taskId, "", auth())
		expect(t, code, 200)
		if len(arr(t, body)) == 0 {
			t.Error("no traces for task")
		}
	})

	t.Run("08_TracesByContext", func(t *testing.T) {
		if contextId == "" {
			t.Skip("no contextId")
		}
		code, body := req(t, "GET", "/api/traces/context/"+contextId, "", auth())
		expect(t, code, 200)
		if len(arr(t, body)) == 0 {
			t.Error("no traces for context")
		}
	})

	t.Run("09_TaskListFilter", func(t *testing.T) {
		code, body := req(t, "GET", "/api/tasks?agent_name="+name, "", auth())
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
		_, body := req(t, "GET", "/api/builtin-agents", "", auth())
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
		_, body := req(t, "GET", "/api/builtin-agents", "", auth())
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
		code, _ := req(t, "PUT", "/api/builtin-agents", "", auth())
		expect(t, code, 405)
	})
}

// ===== 6. Agent Proxy Errors =====

func TestProxy_NotFound(t *testing.T) {
	code, body := req(t, "POST", "/agent/nonexistent-xyz-999",
		`{"jsonrpc":"2.0","method":"message/send","id":1,"params":{"message":{"role":"user","parts":[{"text":"hi"}]}}}`, auth())
	expect(t, code, 404)
	if obj(t, body)["error"] == nil {
		t.Error("expected error")
	}
}

func TestProxy_MethodNotAllowed(t *testing.T) {
	code, _ := req(t, "GET", "/agent/any-agent", "", auth())
	expect(t, code, 405)
}

// ===== 7. Tasks =====

func TestTaskList(t *testing.T) {
	t.Run("Default", func(t *testing.T) {
		code, body := req(t, "GET", "/api/tasks", "", auth())
		expect(t, code, 200)
		m := obj(t, body)
		for _, k := range []string{"items", "total", "page", "size"} {
			if _, ok := m[k]; !ok {
				t.Errorf("missing %q", k)
			}
		}
	})
	t.Run("Pagination", func(t *testing.T) {
		code, body := req(t, "GET", "/api/tasks?page=1&size=3", "", auth())
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
		code, _ := req(t, "GET", "/api/tasks?state=RESPONDED", "", auth())
		expect(t, code, 200)
	})
	t.Run("AgentFilter", func(t *testing.T) {
		code, body := req(t, "GET", "/api/tasks?agent_name=nonexistent", "", auth())
		expect(t, code, 200)
		m := obj(t, body)
		if total, _ := m["total"].(float64); total != 0 {
			t.Errorf("total = %v, want 0", total)
		}
	})
	t.Run("Search", func(t *testing.T) {
		code, _ := req(t, "GET", "/api/tasks?search=e2e", "", auth())
		expect(t, code, 200)
	})
}

func TestTaskDetail_NotFound(t *testing.T) {
	code, body := req(t, "GET", "/api/tasks/nonexistent-task-id", "", auth())
	expect(t, code, 404)
	if obj(t, body)["error"] != "not found" {
		t.Error("expected 'not found'")
	}
}

// ===== 8. Traces =====

func TestTraces(t *testing.T) {
	t.Run("ListRecent", func(t *testing.T) {
		code, body := req(t, "GET", "/api/traces", "", auth())
		expect(t, code, 200)
		arr(t, body) // verify it's an array
	})
	t.Run("ListContexts", func(t *testing.T) {
		code, body := req(t, "GET", "/api/traces/contexts", "", auth())
		expect(t, code, 200)
		arr(t, body)
	})
	t.Run("ByTask_Empty", func(t *testing.T) {
		code, body := req(t, "GET", "/api/traces/task/nonexistent", "", auth())
		expect(t, code, 200)
		if len(arr(t, body)) != 0 {
			t.Error("expected empty")
		}
	})
	t.Run("ByContext_Empty", func(t *testing.T) {
		code, body := req(t, "GET", "/api/traces/context/nonexistent", "", auth())
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
	r.Header.Set("X-Admin-Token", adminToken)
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

// ===== 10. External agents use HTTP API directly.

// ===== 11. Cross-API: Delete via /api/agents cleans engine =====

func TestAgentDelete_CleansEngine(t *testing.T) {
	const name = "e2e-delete-engine-test"
	code, _ := req(t, "POST", "/api/builtin-agents", fmt.Sprintf(`{
		"name":"%s","provider":"openai","base_url":"https://api.openai.com","api_key":"sk-d","model":"gpt-4o"
	}`, name), auth())
	expect(t, code, 200)

	code, _ = req(t, "DELETE", "/api/agents/"+name, "", auth())
	expect(t, code, 204)

	_, body := req(t, "GET", "/api/builtin-agents", "", auth())
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
		code, _ := req(t, "PUT", "/api/agents", "", auth())
		expect(t, code, 405)
	})
	t.Run("POST_stats", func(t *testing.T) {
		code, _ := req(t, "POST", "/api/stats", "", nil)
		expect(t, code, 405)
	})
}
