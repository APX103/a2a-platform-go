package mcpclient

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"a2a-platform/internal/llm"
)

type Client struct {
	Name  string
	Tools []llm.ToolDef
	send  func(method string, params interface{}) (json.RawMessage, error)
	close func()
}

func (c *Client) CallTool(name string, arguments string) (string, error) {
	var args interface{}
	if arguments != "" {
		json.Unmarshal([]byte(arguments), &args)
	}
	result, err := c.send("tools/call", map[string]interface{}{
		"name":      name,
		"arguments": args,
	})
	if err != nil {
		return "", err
	}
	var resp struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
		IsError bool `json:"isError"`
	}
	if err := json.Unmarshal(result, &resp); err != nil {
		return string(result), nil
	}
	var texts []string
	for _, c := range resp.Content {
		if c.Type == "text" {
			texts = append(texts, c.Text)
		}
	}
	return strings.Join(texts, "\n"), nil
}

func (c *Client) Close() {
	if c.close != nil {
		c.close()
	}
}

// ConnectSSE connects to an MCP server via SSE transport.
func ConnectSSE(name, url string) (*Client, error) {
	slog.Info("Connecting to MCP server via SSE", "name", name, "url", url)

	connectClient := &http.Client{
		Transport: &http.Transport{ResponseHeaderTimeout: 10 * time.Second},
	}
	messagesClient := &http.Client{Timeout: 60 * time.Second}
	resp, err := connectClient.Get(url)
	if err != nil {
		return nil, fmt.Errorf("SSE connect failed: %w", err)
	}
	if resp.StatusCode != 200 {
		resp.Body.Close()
		return nil, fmt.Errorf("SSE connect returned %d", resp.StatusCode)
	}

	// Read SSE to find the endpoint event with the messages URL
	scanner := bufio.NewScanner(resp.Body)
	var messagesURL string
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "event: endpoint") {
			if scanner.Scan() {
				dataLine := scanner.Text()
				if strings.HasPrefix(dataLine, "data: ") {
					messagesURL = strings.TrimPrefix(dataLine, "data: ")
					break
				}
			}
		}
	}
	if messagesURL == "" {
		resp.Body.Close()
		return nil, fmt.Errorf("no endpoint event received from MCP SSE")
	}

	// If relative URL, resolve against base
	if strings.HasPrefix(messagesURL, "/") {
		// Extract base from url
		parts := strings.SplitN(url, "//", 2)
		if len(parts) == 2 {
			hostEnd := strings.Index(parts[1], "/")
			if hostEnd > 0 {
				messagesURL = parts[0] + "//" + parts[1][:hostEnd] + messagesURL
			}
		}
	}

	var reqID atomic.Int64

	sendFn := func(method string, params interface{}) (json.RawMessage, error) {
		id := reqID.Add(1)
		rpcReq := map[string]interface{}{
			"jsonrpc": "2.0",
			"id":      id,
			"method":  method,
			"params":  params,
		}
		body, _ := json.Marshal(rpcReq)
		httpResp, err := messagesClient.Post(messagesURL, "application/json", bytes.NewReader(body))
		if err != nil {
			return nil, err
		}
		defer httpResp.Body.Close()
		respBody, _ := io.ReadAll(io.LimitReader(httpResp.Body, 16<<20))
		var rpcResp struct {
			Result json.RawMessage `json:"result"`
			Error  *struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		if err := json.Unmarshal(respBody, &rpcResp); err != nil {
			return respBody, nil
		}
		if rpcResp.Error != nil {
			return nil, fmt.Errorf("MCP error: %s", rpcResp.Error.Message)
		}
		return rpcResp.Result, nil
	}

	client := &Client{
		Name: name,
		send: sendFn,
		close: func() {
			resp.Body.Close()
		},
	}

	// Initialize
	sendFn("initialize", map[string]interface{}{
		"protocolVersion": "2024-11-05",
		"clientInfo":      map[string]string{"name": "a2a-platform", "version": "1.0.0"},
		"capabilities":    map[string]interface{}{},
	})

	// List tools
	tools, err := listTools(sendFn)
	if err != nil {
		slog.Warn("Failed to list MCP tools", "name", name, "error", err)
	}
	client.Tools = tools
	slog.Info("MCP SSE client connected", "name", name, "tools", len(tools))
	return client, nil
}

// ConnectStdio connects to an MCP server via stdio transport.
func ConnectStdio(name, command string, args []string) (*Client, error) {
	slog.Info("Connecting to MCP server via stdio", "name", name, "command", command)

	cmd := exec.Command(command, args...)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	cmd.Stderr = io.Discard

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start MCP process: %w", err)
	}

	var mu sync.Mutex
	reader := bufio.NewReader(stdout)
	var reqID atomic.Int64

	sendFn := func(method string, params interface{}) (json.RawMessage, error) {
		mu.Lock()
		defer mu.Unlock()

		id := reqID.Add(1)
		rpcReq := map[string]interface{}{
			"jsonrpc": "2.0",
			"id":      id,
			"method":  method,
			"params":  params,
		}
		body, _ := json.Marshal(rpcReq)
		body = append(body, '\n')
		if _, err := stdin.Write(body); err != nil {
			return nil, err
		}

		// Read response line
		line, err := reader.ReadBytes('\n')
		if err != nil {
			return nil, err
		}
		var rpcResp struct {
			Result json.RawMessage `json:"result"`
			Error  *struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		if err := json.Unmarshal(line, &rpcResp); err != nil {
			return line, nil
		}
		if rpcResp.Error != nil {
			return nil, fmt.Errorf("MCP error: %s", rpcResp.Error.Message)
		}
		return rpcResp.Result, nil
	}

	client := &Client{
		Name: name,
		send: sendFn,
		close: func() {
			stdin.Close()
			cmd.Process.Kill()
			cmd.Wait()
		},
	}

	// Initialize
	sendFn("initialize", map[string]interface{}{
		"protocolVersion": "2024-11-05",
		"clientInfo":      map[string]string{"name": "a2a-platform", "version": "1.0.0"},
		"capabilities":    map[string]interface{}{},
	})
	// Notify initialized
	sendFn("notifications/initialized", nil)

	// List tools
	tools, err := listTools(sendFn)
	if err != nil {
		slog.Warn("Failed to list MCP tools", "name", name, "error", err)
	}
	client.Tools = tools
	slog.Info("MCP stdio client connected", "name", name, "tools", len(tools))
	return client, nil
}

func listTools(send func(string, interface{}) (json.RawMessage, error)) ([]llm.ToolDef, error) {
	result, err := send("tools/list", map[string]interface{}{})
	if err != nil {
		return nil, err
	}
	var resp struct {
		Tools []struct {
			Name        string                 `json:"name"`
			Description string                 `json:"description"`
			InputSchema map[string]interface{} `json:"inputSchema"`
		} `json:"tools"`
	}
	if err := json.Unmarshal(result, &resp); err != nil {
		return nil, err
	}
	tools := make([]llm.ToolDef, len(resp.Tools))
	for i, t := range resp.Tools {
		tools[i] = llm.ToolDef{
			Name:        t.Name,
			Description: t.Description,
			InputSchema: t.InputSchema,
		}
	}
	return tools, nil
}
