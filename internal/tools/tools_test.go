package tools

import (
	"fmt"
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
