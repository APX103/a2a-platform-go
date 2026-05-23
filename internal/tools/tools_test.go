package tools

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestExecuteReadFile(t *testing.T) {
	testDir := "test_read_dir_" + fmt.Sprint(time.Now().UnixNano())
	os.MkdirAll(testDir, 0755)
	defer os.RemoveAll(testDir)

	testFile := filepath.Join(testDir, "test.txt")
	testContent := "line1\nline2\nline3"
	os.WriteFile(testFile, []byte(testContent), 0644)

	result, err := executeReadFile(map[string]any{
		"path": testFile,
	})
	if err != nil {
		t.Fatalf("executeReadFile failed: %v", err)
	}
	expected := "(lines 1-3 of 3)\n\n" + testContent
	if result != expected {
		t.Errorf("Expected:\n%s\nGot:\n%s", expected, result)
	}

	result, err = executeReadFile(map[string]any{
		"path":   testFile,
		"offset": float64(2),
		"limit":  float64(1),
	})
	if err != nil {
		t.Fatalf("executeReadFile with offset/limit failed: %v", err)
	}
	expected = "(lines 2-2 of 3)\n\nline2"
	if result != expected {
		t.Errorf("Expected:\n%s\nGot:\n%s", expected, result)
	}

	if _, err := executeReadFile(map[string]any{"path": "/etc/passwd"}); err == nil {
		t.Error("Should reject absolute path")
	}
}

func TestExecuteWriteFile(t *testing.T) {
	testDir := "test_write_dir_" + fmt.Sprint(time.Now().UnixNano())
	os.MkdirAll(testDir, 0755)
	defer os.RemoveAll(testDir)

	testFile := filepath.Join(testDir, "write_test.txt")

	result, err := executeWriteFile(map[string]any{
		"path":    testFile,
		"content": "hello",
	})
	if err != nil {
		t.Fatalf("executeWriteFile failed: %v", err)
	}
	if !strings.Contains(result, "Wrote") {
		t.Errorf("Expected 'Wrote' in result, got: %s", result)
	}

	result, err = executeWriteFile(map[string]any{
		"path":    testFile,
		"content": " world",
		"append":  true,
	})
	if err != nil {
		t.Fatalf("executeWriteFile append failed: %v", err)
	}
	if !strings.Contains(result, "Appended") {
		t.Errorf("Expected 'Appended' in result, got: %s", result)
	}

	content, _ := os.ReadFile(testFile)
	if string(content) != "hello world" {
		t.Errorf("Expected 'hello world', got: %s", string(content))
	}

	if _, err := executeWriteFile(map[string]any{"path": "/etc/passwd", "content": "x"}); err == nil {
		t.Error("Should reject absolute path")
	}
}

func TestExecuteListDirectory(t *testing.T) {
	testDir := "test_list_dir_" + fmt.Sprint(time.Now().UnixNano())
	os.MkdirAll(testDir, 0755)
	os.MkdirAll(filepath.Join(testDir, "subdir"), 0755)
	defer os.RemoveAll(testDir)
	os.WriteFile(filepath.Join(testDir, "file1.txt"), []byte("test"), 0644)
	os.WriteFile(filepath.Join(testDir, ".hidden"), []byte("test"), 0644)

	result, err := executeListDirectory(map[string]any{
		"path": testDir,
	})
	if err != nil {
		t.Fatalf("executeListDirectory failed: %v", err)
	}
	if strings.Contains(result, ".hidden") {
		t.Error("Should not include hidden files")
	}
	if !strings.Contains(result, "file1.txt") {
		t.Error("Should include file1.txt")
	}
	if !strings.Contains(result, "subdir") {
		t.Error("Should include subdir")
	}

	if _, err := executeListDirectory(map[string]any{"path": "/etc"}); err == nil {
		t.Error("Should reject absolute path")
	}
}

func TestToolSearch(t *testing.T) {
	result, err := executeToolSearch(map[string]any{
		"name": "file",
	})
	if err != nil {
		t.Fatalf("executeToolSearch failed: %v", err)
	}
	if !strings.Contains(result, "read_file") {
		t.Error("Should find read_file")
	}
	if !strings.Contains(result, "write_file") {
		t.Error("Should find write_file")
	}

	result, err = executeToolSearch(map[string]any{
		"name": "nonexistent_tool_xyz",
	})
	if err != nil {
		t.Fatalf("executeToolSearch failed: %v", err)
	}
	if !strings.Contains(result, "No tools found") {
		t.Error("Should report no tools found")
	}
}

func TestExecuteListAgentsUsesAdminToken(t *testing.T) {
	var gotToken string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotToken = r.Header.Get("X-Admin-Token")
		if r.URL.Path != "/api/agents" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, `[{"name":"mi-1","type":"builtin","status":"connected","description":"leader"}]`)
	}))
	defer server.Close()

	oldBaseURL := platformBaseURL
	oldAdminToken := platformAdminToken
	platformBaseURL = server.URL
	platformAdminToken = "secret"
	t.Cleanup(func() {
		platformBaseURL = oldBaseURL
		platformAdminToken = oldAdminToken
	})

	result, err := executeListAgents(map[string]any{})
	if err != nil {
		t.Fatalf("executeListAgents failed: %v", err)
	}
	if gotToken != "secret" {
		t.Fatalf("admin token = %q, want secret", gotToken)
	}
	if !strings.Contains(result, "mi-1") {
		t.Fatalf("result = %q", result)
	}
}

func TestExecuteListGroupsFiltersBySourceAgent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Admin-Token") != "secret" {
			t.Fatalf("missing admin token on %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/groups":
			fmt.Fprintln(w, `[
				{"id":"g1","name":"Planning","orchestration_mode":"leader_led","status":"active"},
				{"id":"g2","name":"Hidden","orchestration_mode":"leader_led","status":"active"}
			]`)
		case "/api/groups/g1/members":
			fmt.Fprintln(w, `[{"actor_type":"agent","actor_id":"mi-1","role":"leader"}]`)
		case "/api/groups/g2/members":
			fmt.Fprintln(w, `[{"actor_type":"agent","actor_id":"mi-2","role":"leader"}]`)
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer server.Close()

	oldBaseURL := platformBaseURL
	oldAdminToken := platformAdminToken
	platformBaseURL = server.URL
	platformAdminToken = "secret"
	t.Cleanup(func() {
		platformBaseURL = oldBaseURL
		platformAdminToken = oldAdminToken
	})

	result, err := executeListGroups(map[string]any{"_source_agent": "mi-1"})
	if err != nil {
		t.Fatalf("executeListGroups failed: %v", err)
	}
	if !strings.Contains(result, "Planning") || strings.Contains(result, "Hidden") {
		t.Fatalf("result = %q", result)
	}
}

func TestExecuteListAgentsRequiresGroupForSourceAgent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Admin-Token") != "secret" {
			t.Fatalf("missing admin token on %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/groups":
			fmt.Fprintln(w, `[{"id":"g1","name":"Planning","orchestration_mode":"leader_led","status":"active"}]`)
		case "/api/groups/g1/members":
			fmt.Fprintln(w, `[{"actor_type":"agent","actor_id":"mi-1","role":"leader"}]`)
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer server.Close()

	oldBaseURL := platformBaseURL
	oldAdminToken := platformAdminToken
	platformBaseURL = server.URL
	platformAdminToken = "secret"
	t.Cleanup(func() {
		platformBaseURL = oldBaseURL
		platformAdminToken = oldAdminToken
	})

	result, err := executeListAgents(map[string]any{"_source_agent": "mi-1"})
	if err != nil {
		t.Fatalf("executeListAgents failed: %v", err)
	}
	if !strings.Contains(result, "group_id is required") || !strings.Contains(result, "Planning") {
		t.Fatalf("result = %q", result)
	}
}

func TestExecuteListAgentsGroupScoped(t *testing.T) {
	requestedAgents := map[string]bool{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Admin-Token") != "secret" {
			t.Fatalf("missing admin token on %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/groups/g1/members":
			fmt.Fprintln(w, `[
				{"actor_type":"agent","actor_id":"mi-1","role":"leader"},
				{"actor_type":"human","actor_id":"human-1","role":"member"},
				{"actor_type":"agent","actor_id":"mi-2","role":"member"}
			]`)
		case "/api/agents/mi-1":
			requestedAgents["mi-1"] = true
			fmt.Fprintln(w, `{"name":"mi-1","type":"builtin","status":"connected","description":"leader agent"}`)
		case "/api/agents/mi-2":
			requestedAgents["mi-2"] = true
			fmt.Fprintln(w, `{"name":"mi-2","type":"builtin","status":"connected","description":"member agent"}`)
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer server.Close()

	oldBaseURL := platformBaseURL
	oldAdminToken := platformAdminToken
	platformBaseURL = server.URL
	platformAdminToken = "secret"
	t.Cleanup(func() {
		platformBaseURL = oldBaseURL
		platformAdminToken = oldAdminToken
	})

	result, err := executeListAgents(map[string]any{"_group_id": "g1"})
	if err != nil {
		t.Fatalf("executeListAgents failed: %v", err)
	}
	if !requestedAgents["mi-1"] || !requestedAgents["mi-2"] {
		t.Fatalf("requestedAgents = %#v", requestedAgents)
	}
	if strings.Contains(result, "human-1") {
		t.Fatalf("human leaked into agent list: %s", result)
	}
	if !strings.Contains(result, "mi-1") || !strings.Contains(result, "mi-2") {
		t.Fatalf("result = %q", result)
	}
}

func TestExecuteSendToAgentPropagatesRootAndParentHeaders(t *testing.T) {
	var gotRoot, gotParentTask, gotParentTool, gotSource string
	var forwardedContextId any

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/groups/g1/members" {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintln(w, `[{"actor_type":"agent","actor_id":"mi-1","role":"leader"},{"actor_type":"agent","actor_id":"mi-2","role":"member"}]`)
			return
		}
		gotRoot = r.Header.Get("X-A2A-Root-Context-Id")
		gotParentTask = r.Header.Get("X-A2A-Parent-Task-Id")
		gotParentTool = r.Header.Get("X-A2A-Parent-Tool-Call-Id")
		gotSource = r.Header.Get("X-A2A-Source-Agent")

		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		if params, ok := body["params"].(map[string]any); ok {
			forwardedContextId = params["contextId"]
		}

		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprintln(w, `data: {"type":"text.delta","text":"ok"}`)
	}))
	defer server.Close()

	oldBaseURL := platformBaseURL
	platformBaseURL = server.URL
	t.Cleanup(func() { platformBaseURL = oldBaseURL })

	result, err := executeSendToAgent(map[string]any{
		"agent":                "mi-2",
		"message":              "hello",
		"_source_agent":        "mi-1",
		"_root_context_id":     "root-1",
		"_parent_task_id":      "task-1",
		"_parent_tool_call_id": "tool-1",
		"_group_id":            "g1",
	})
	if err != nil {
		t.Fatalf("executeSendToAgent failed: %v", err)
	}
	if result != "ok" {
		t.Fatalf("result = %q, want ok", result)
	}
	if gotSource != "mi-1" || gotRoot != "root-1" || gotParentTask != "task-1" || gotParentTool != "tool-1" {
		t.Fatalf("headers source/root/parent/tool = %q/%q/%q/%q", gotSource, gotRoot, gotParentTask, gotParentTool)
	}
	if forwardedContextId != nil {
		t.Fatalf("contextId was forwarded to child agent body: %#v", forwardedContextId)
	}
}

func TestExecuteSendToAgentGroupScoped(t *testing.T) {
	var gotGroup, gotToken string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/groups/g1/members":
			if r.Header.Get("X-Admin-Token") != "secret" {
				t.Fatalf("missing admin token on member check")
			}
			fmt.Fprintln(w, `[{"actor_type":"agent","actor_id":"mi-2","role":"member"}]`)
		case "/agent/mi-2":
			gotToken = r.Header.Get("X-Admin-Token")
			gotGroup = r.Header.Get("X-A2A-Tool-Group-ID")
			w.Header().Set("Content-Type", "text/event-stream")
			fmt.Fprintln(w, `data: {"type":"text.delta","text":"ok"}`)
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer server.Close()

	oldBaseURL := platformBaseURL
	oldAdminToken := platformAdminToken
	platformBaseURL = server.URL
	platformAdminToken = "secret"
	t.Cleanup(func() {
		platformBaseURL = oldBaseURL
		platformAdminToken = oldAdminToken
	})

	result, err := executeSendToAgent(map[string]any{
		"agent":     "mi-2",
		"message":   "hello",
		"_group_id": "g1",
	})
	if err != nil {
		t.Fatalf("executeSendToAgent failed: %v", err)
	}
	if result != "ok" {
		t.Fatalf("result = %q, want ok", result)
	}
	if gotToken != "secret" || gotGroup != "g1" {
		t.Fatalf("token/group = %q/%q, want secret/g1", gotToken, gotGroup)
	}
}

func TestExecuteFetchURLRejectsUnsupportedInputs(t *testing.T) {
	if _, err := executeFetchURL(map[string]any{"url": "file:///etc/passwd"}); err == nil {
		t.Fatal("expected unsupported scheme error")
	}
	if _, err := executeFetchURL(map[string]any{"url": "http://example.test", "method": "TRACE"}); err == nil {
		t.Fatal("expected unsupported method error")
	}
}

func TestResolveWorkspacePathRejectsSiblingPrefix(t *testing.T) {
	root := t.TempDir()
	workspace := filepath.Join(root, "workspace")
	sibling := filepath.Join(root, "workspace-other")
	if err := os.MkdirAll(workspace, 0755); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	if err := os.MkdirAll(sibling, 0755); err != nil {
		t.Fatalf("create sibling: %v", err)
	}

	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(workspace); err != nil {
		t.Fatalf("chdir workspace: %v", err)
	}
	defer os.Chdir(oldWd)

	if _, err := resolveWorkspacePath(filepath.Join(sibling, "secret.txt")); err == nil {
		t.Fatal("expected sibling path to be rejected")
	}
}

func TestGetBuiltinTools(t *testing.T) {
	tools := GetBuiltinTools()
	if len(tools) == 0 {
		t.Error("Should have at least one builtin tool")
	}

	foundFetch := false
	for _, tool := range tools {
		if tool.Name == "fetch_url" {
			foundFetch = true
			if tool.Description == "" {
				t.Error("fetch_url should have description")
			}
			if len(tool.Parameters) == 0 {
				t.Error("fetch_url should have parameters")
			}
		}
	}
	if !foundFetch {
		t.Error("Should find fetch_url tool")
	}
}
