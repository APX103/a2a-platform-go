package tools

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"a2a-platform/internal/model"
)

var (
	platformBaseURL string
	platformURLOnce sync.Once
)

// SetPlatformBaseURL sets the base URL for A2A platform API calls
func SetPlatformBaseURL(url string) {
	platformURLOnce.Do(func() {
		platformBaseURL = url
	})
}

// GetA2ATools returns tools for interacting with the A2A platform
func GetA2ATools() []model.BuiltinTool {
	return []model.BuiltinTool{
		{
			Name:        "list_agents",
			Description: "List all registered agents in the A2A platform. Returns agent names, types, descriptions, and connection status.",
			Parameters: []model.ToolParameter{
				{Name: "filter_type", Type: "string", Description: "Optional filter by agent type (e.g., 'builtin', 'external', 'bridge')", Required: false},
			},
			Execute: executeListAgents,
		},
		{
			Name:        "send_to_agent",
			Description: "Send a message to another agent in the platform and get the response. Useful for delegating tasks to specialized agents.",
			Parameters: []model.ToolParameter{
				{Name: "agent", Type: "string", Description: "Name of the target agent", Required: true},
				{Name: "message", Type: "string", Description: "Message to send to the agent", Required: true},
			},
			Execute: executeSendToAgent,
		},
		{
			Name:        "get_agent_info",
			Description: "Get detailed information about a specific agent including skills, version, and description.",
			Parameters: []model.ToolParameter{
				{Name: "name", Type: "string", Description: "Name of the agent", Required: true},
			},
			Execute: executeGetAgentInfo,
		},
	}
}

func executeListAgents(args map[string]any) (string, error) {
	url := platformBaseURL + "/api/agents"

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return "", fmt.Errorf("failed to list agents: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("request failed with status %d: %s", resp.StatusCode, string(body))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %w", err)
	}

	var agents []map[string]interface{}
	if err := json.Unmarshal(body, &agents); err != nil {
		return string(body), nil // Return raw response if parsing fails
	}

	var result []string
	result = append(result, fmt.Sprintf("Found %d agents:", len(agents)))
	for _, a := range agents {
		name, _ := a["name"].(string)
		status, _ := a["status"].(string)
		aType, _ := a["type"].(string)
		desc, _ := a["description"].(string)

		line := fmt.Sprintf("  - %s (type: %s, status: %s)", name, aType, status)
		if desc != "" {
			line += fmt.Sprintf(" - %s", desc)
		}
		result = append(result, line)
	}

	return "\n" + strings.Join(result, "\n"), nil
}

func executeSendToAgent(args map[string]any) (string, error) {
	agent, ok := args["agent"].(string)
	if !ok || agent == "" {
		return "", fmt.Errorf("agent name is required")
	}
	message, ok := args["message"].(string)
	if !ok || message == "" {
		return "", fmt.Errorf("message is required")
	}

	reqBody := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      "1",
		"method":  "SendStreamingMessage",
		"params": map[string]interface{}{
			"message": map[string]interface{}{
				"role":  "ROLE_USER",
				"parts": []map[string]string{{"text": message}},
			},
		},
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Post(platformBaseURL+"/agent/"+agent, "application/json", bytes.NewReader(jsonBody))
	if err != nil {
		return "", fmt.Errorf("failed to send message: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("request failed with status %d: %s", resp.StatusCode, string(body))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %w", err)
	}

	// Parse SSE response to extract text
	lines := strings.Split(string(body), "\n")
	var responseText strings.Builder
	for _, line := range lines {
		if strings.HasPrefix(line, "data:") {
			data := strings.TrimPrefix(line, "data:")
			data = strings.TrimSpace(data)
			if data == "" {
				continue
			}
			var evt map[string]interface{}
			if json.Unmarshal([]byte(data), &evt) != nil {
				continue
			}
			// Extract text from text.delta events
			if evt["type"] == "text.delta" {
				if text, ok := evt["text"].(string); ok {
					responseText.WriteString(text)
				}
			}
			// Extract from completed task.status
			if evt["type"] == "task.status" {
				if status, ok := evt["status"].(map[string]interface{}); ok {
					if msg, ok := status["message"].(map[string]interface{}); ok {
						if parts, ok := msg["parts"].([]interface{}); ok && len(parts) > 0 {
							if part, ok := parts[0].(map[string]interface{}); ok {
								if text, ok := part["text"].(string); ok && responseText.Len() == 0 {
									responseText.WriteString(text)
								}
							}
						}
					}
				}
			}
		}
	}

	result := responseText.String()
	if result == "" {
		result = string(body) // Fallback to raw response
	}

	return result, nil
}

func executeGetAgentInfo(args map[string]any) (string, error) {
	name, ok := args["name"].(string)
	if !ok || name == "" {
		return "", fmt.Errorf("agent name is required")
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(platformBaseURL + "/api/agents/" + name)
	if err != nil {
		return "", fmt.Errorf("failed to get agent info: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("request failed with status %d: %s", resp.StatusCode, string(body))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %w", err)
	}

	return string(body), nil
}