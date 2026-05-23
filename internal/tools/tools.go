package tools

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"a2a-platform/internal/model"
)

// GetBuiltinTools returns all built-in tools.
func GetBuiltinTools() []model.BuiltinTool {
	return []model.BuiltinTool{
		{
			Name:        "fetch_url",
			Description: "Make an HTTP request to a URL and return the response. Supports GET, POST, PUT, DELETE methods.",
			Parameters: []model.ToolParameter{
				{Name: "url", Type: "string", Description: "Target URL", Required: true},
				{Name: "method", Type: "string", Description: "HTTP method: GET, POST, PUT, DELETE", Required: false},
				{Name: "headers", Type: "string", Description: "JSON string of headers", Required: false},
				{Name: "body", Type: "string", Description: "Request body for POST/PUT", Required: false},
				{Name: "timeout", Type: "number", Description: "Request timeout in seconds", Required: false},
			},
			Execute:    executeFetchURL,
			IsReadOnly: false, // can write via POST/PUT/DELETE
		},
		{
			Name:        "read_file",
			Description: "Read the contents of a file. Returns the full content.",
			Parameters: []model.ToolParameter{
				{Name: "path", Type: "string", Description: "File path to read", Required: true},
				{Name: "offset", Type: "number", Description: "Line number to start from (1-based)", Required: false},
				{Name: "limit", Type: "number", Description: "Maximum lines to read", Required: false},
			},
			Execute:    executeReadFile,
			IsReadOnly: true,
		},
		{
			Name:        "write_file",
			Description: "Write content to a file. Creates parent directories if needed.",
			Parameters: []model.ToolParameter{
				{Name: "path", Type: "string", Description: "File path to write", Required: true},
				{Name: "content", Type: "string", Description: "Content to write", Required: true},
				{Name: "append", Type: "boolean", Description: "Append to existing file instead of overwrite", Required: false},
			},
			Execute:    executeWriteFile,
			IsReadOnly: false,
		},
		{
			Name:        "list_directory",
			Description: "List files and directories in a path.",
			Parameters: []model.ToolParameter{
				{Name: "path", Type: "string", Description: "Directory path to list", Required: true},
				{Name: "recursive", Type: "boolean", Description: "List recursively", Required: false},
			},
			Execute:    executeListDirectory,
			IsReadOnly: true,
		},
		{
			Name:        "tool_search",
			Description: "Search for available tools by name pattern.",
			Parameters: []model.ToolParameter{
				{Name: "name", Type: "string", Description: "Pattern to search for", Required: true},
			},
			Execute:    executeToolSearch,
			IsReadOnly: true,
		},
	}
}

// Tool registry for dynamic tools (spawn_agent, etc)
var DynamicTools []model.BuiltinTool

func RegisterDynamicTools(tools []model.BuiltinTool) {
	DynamicTools = append(DynamicTools, tools...)
}

func GetAllTools() []model.BuiltinTool {
	all := append([]model.BuiltinTool{}, GetBuiltinTools()...)
	all = append(all, GetA2ATools()...)
	all = append(all, DynamicTools...)
	return all
}

func ExecuteTool(name string, args map[string]any) (string, error) {
	for _, tool := range GetAllTools() {
		if tool.Name == name {
			return tool.Execute(args)
		}
	}
	return "", fmt.Errorf("tool %q not found", name)
}

// ===== Tool Implementations =====

func executeFetchURL(args map[string]any) (string, error) {
	targetURL, ok := args["url"].(string)
	if !ok || targetURL == "" {
		return "", fmt.Errorf("url is required")
	}
	parsedURL, err := url.Parse(targetURL)
	if err != nil {
		return "", fmt.Errorf("invalid url: %w", err)
	}
	if parsedURL.Scheme != "http" && parsedURL.Scheme != "https" {
		return "", fmt.Errorf("unsupported url scheme: %s", parsedURL.Scheme)
	}

	method := "GET"
	if m, ok := args["method"].(string); ok && m != "" {
		method = strings.ToUpper(m)
	}
	switch method {
	case http.MethodGet, http.MethodPost, http.MethodPut, http.MethodDelete:
	default:
		return "", fmt.Errorf("unsupported method: %s", method)
	}

	timeout := 30
	if t, ok := args["timeout"].(float64); ok && t > 0 {
		timeout = int(t)
	}
	if timeout > 120 {
		timeout = 120
	}

	client := &http.Client{Timeout: time.Duration(timeout) * time.Second}

	var body io.Reader
	if b, ok := args["body"].(string); ok && b != "" {
		body = strings.NewReader(b)
	}

	req, err := http.NewRequest(method, targetURL, body)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	if h, ok := args["headers"].(string); ok && h != "" {
		var headers map[string]string
		if err := json.Unmarshal([]byte(h), &headers); err == nil {
			for k, v := range headers {
				req.Header.Set(k, v)
			}
		}
	}

	req.Header.Set("Accept", "text/html,application/json,*/*")

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	content := string(respBody)

	maxLen := 8000
	if len(content) > maxLen {
		content = content[:maxLen] + "\n... (truncated)"
	}

	return fmt.Sprintf("Status: %d %s\n\n%s", resp.StatusCode, resp.Status, content), nil
}

func executeReadFile(args map[string]any) (string, error) {
	path, ok := args["path"].(string)
	if !ok || path == "" {
		return "", fmt.Errorf("path is required")
	}

	absPath, err := resolveWorkspacePath(path)
	if err != nil {
		return "", err
	}

	content, err := os.ReadFile(absPath)
	if err != nil {
		return "", fmt.Errorf("failed to read file: %w", err)
	}

	lines := strings.Split(string(content), "\n")

	var start, end int
	if offset, ok := args["offset"].(float64); ok && offset > 0 {
		start = int(offset) - 1
	}
	if limit, ok := args["limit"].(float64); ok && limit > 0 {
		end = start + int(limit)
	} else {
		end = len(lines)
	}

	if start < 0 {
		start = 0
	}
	if end > len(lines) {
		end = len(lines)
	}
	if start >= end {
		return "", fmt.Errorf("invalid offset/limit range")
	}

	slicedLines := lines[start:end]
	header := fmt.Sprintf("(lines %d-%d of %d)", start+1, end, len(lines))

	return header + "\n\n" + strings.Join(slicedLines, "\n"), nil
}

func executeWriteFile(args map[string]any) (string, error) {
	path, ok := args["path"].(string)
	if !ok || path == "" {
		return "", fmt.Errorf("path is required")
	}

	content, ok := args["content"].(string)
	if !ok {
		return "", fmt.Errorf("content is required")
	}

	appendMode := false
	if a, ok := args["append"].(bool); ok && a {
		appendMode = true
	}

	absPath, err := resolveWorkspacePath(path)
	if err != nil {
		return "", err
	}

	if err := os.MkdirAll(filepath.Dir(absPath), 0755); err != nil {
		return "", fmt.Errorf("failed to create directories: %w", err)
	}

	var flags int
	if appendMode {
		flags = os.O_CREATE | os.O_WRONLY | os.O_APPEND
	} else {
		flags = os.O_CREATE | os.O_WRONLY | os.O_TRUNC
	}

	file, err := os.OpenFile(absPath, flags, 0644)
	if err != nil {
		return "", fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	if _, err := file.WriteString(content); err != nil {
		return "", fmt.Errorf("failed to write file: %w", err)
	}

	action := "Wrote"
	if appendMode {
		action = "Appended to"
	}

	return fmt.Sprintf("%s %s (%d chars)", action, path, len(content)), nil
}

func executeListDirectory(args map[string]any) (string, error) {
	path, ok := args["path"].(string)
	if !ok || path == "" {
		return "", fmt.Errorf("path is required")
	}

	recursive := false
	if r, ok := args["recursive"].(bool); ok {
		recursive = r
	}

	absPath, err := resolveWorkspacePath(path)
	if err != nil {
		return "", err
	}
	wd, _ := os.Getwd()

	var items []string
	if recursive {
		filepath.Walk(absPath, func(p string, info os.FileInfo, err error) error {
			if err != nil {
				return nil
			}
			if strings.HasPrefix(filepath.Base(p), ".") {
				if info.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
			rel, _ := filepath.Rel(wd, p)
			items = append(items, rel)
			return nil
		})
	} else {
		entries, err := os.ReadDir(absPath)
		if err != nil {
			return "", fmt.Errorf("failed to read directory: %w", err)
		}
		for _, entry := range entries {
			if strings.HasPrefix(entry.Name(), ".") {
				continue
			}
			rel := filepath.Join(path, entry.Name())
			absEntry, _ := filepath.Abs(rel)
			relPath, _ := filepath.Rel(wd, absEntry)
			items = append(items, relPath)
		}
	}

	if len(items) == 0 {
		return "(empty directory)", nil
	}

	return strings.Join(items, "\n"), nil
}

func resolveWorkspacePath(path string) (string, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("invalid path: %w", err)
	}
	wd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("failed to resolve working directory: %w", err)
	}
	rel, err := filepath.Rel(wd, absPath)
	if err != nil {
		return "", fmt.Errorf("invalid path: %w", err)
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) || filepath.IsAbs(rel) {
		return "", fmt.Errorf("path escapes working directory")
	}
	return absPath, nil
}

func executeToolSearch(args map[string]any) (string, error) {
	pattern, ok := args["name"].(string)
	if !ok {
		return "", fmt.Errorf("name pattern is required")
	}

	pattern = strings.ToLower(pattern)
	all := GetAllTools()

	var matches []string
	for _, tool := range all {
		if strings.Contains(strings.ToLower(tool.Name), pattern) {
			paramsJSON, _ := json.Marshal(tool.Parameters)
			matches = append(matches, fmt.Sprintf("%s: %s\nParameters: %s", tool.Name, tool.Description, paramsJSON))
		}
	}

	if len(matches) == 0 {
		return fmt.Sprintf("No tools found matching \"%s\"", pattern), nil
	}

	return strings.Join(matches, "\n\n"), nil
}
