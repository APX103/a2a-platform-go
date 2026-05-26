package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"a2a-platform/internal/model"
)

var (
	platformBaseURL    string
	platformAdminToken string
	platformURLOnce    sync.Once
)

// SetPlatformBaseURL sets the base URL for A2A platform API calls
func SetPlatformBaseURL(url string) {
	platformURLOnce.Do(func() {
		platformBaseURL = url
	})
}

// SetPlatformAdminToken sets the internal admin token for platform API calls.
func SetPlatformAdminToken(token string) {
	platformAdminToken = token
}

// GetA2ATools returns tools for interacting with the A2A platform
func GetA2ATools() []model.BuiltinTool {
	return []model.BuiltinTool{
		{
			Name:        "list_groups",
			Description: "List groups visible to the current agent. Use this before list_agents or send_to_agent to choose the group_id boundary for collaboration.",
			Parameters: []model.ToolParameter{
				{Name: "status", Type: "string", Description: "Optional group status filter. Defaults to active.", Required: false},
			},
			Execute:        executeListGroups,
			ExecuteContext: executeListGroupsContext,
			IsReadOnly:     true,
		},
		{
			Name:        "list_agents",
			Description: "List agents in a specific group. Agents should call list_groups first, then pass group_id here. In an active group chat, group_id is inferred from the current group.",
			Parameters: []model.ToolParameter{
				{Name: "group_id", Type: "string", Description: "Group ID returned by list_groups. Required outside an active group chat.", Required: false},
				{Name: "filter_type", Type: "string", Description: "Optional filter by agent type (e.g., 'builtin', 'external', 'bridge')", Required: false},
			},
			Execute:        executeListAgents,
			ExecuteContext: executeListAgentsContext,
			IsReadOnly:     true,
		},
		{
			Name:        "send_to_agent",
			Description: "Send a message to another agent inside a group and get the response. Call list_groups then list_agents first; pass group_id unless it is inferred from the active group chat.",
			Parameters: []model.ToolParameter{
				{Name: "agent", Type: "string", Description: "Name of the target agent", Required: true},
				{Name: "message", Type: "string", Description: "Message to send to the agent", Required: true},
				{Name: "group_id", Type: "string", Description: "Group ID that authorizes this agent-to-agent interaction.", Required: false},
			},
			Execute:        executeSendToAgent,
			ExecuteContext: executeSendToAgentContext,
		},
		{
			Name:        "get_agent_info",
			Description: "Get information about a specific agent visible in a group. Call list_groups and list_agents first unless group_id is inferred from the active group chat.",
			Parameters: []model.ToolParameter{
				{Name: "name", Type: "string", Description: "Name of the agent", Required: true},
				{Name: "group_id", Type: "string", Description: "Group ID returned by list_groups.", Required: false},
			},
			Execute:        executeGetAgentInfo,
			ExecuteContext: executeGetAgentInfoContext,
			IsReadOnly:     true,
		},
	}
}

func executeListGroups(args map[string]any) (string, error) {
	return executeListGroupsContext(context.Background(), args)
}

func executeListGroupsContext(ctx context.Context, args map[string]any) (string, error) {
	groupID := groupIDFromArgs(args)
	if groupID != "" {
		group, err := fetchGroupContext(ctx, groupID)
		if err != nil {
			return "", err
		}
		body, _ := json.Marshal([]map[string]interface{}{group})
		return formatGroupList(body)
	}

	status := normalizeString(args["status"])
	if status == "" {
		status = model.GroupStatusActive
	}
	sourceAgent := normalizeString(args["_source_agent"])
	groups, err := fetchVisibleGroupsContext(ctx, sourceAgent, status)
	if err != nil {
		return "", err
	}
	body, _ := json.Marshal(groups)
	return formatGroupList(body)
}

func executeListAgents(args map[string]any) (string, error) {
	return executeListAgentsContext(context.Background(), args)
}

func executeListAgentsContext(ctx context.Context, args map[string]any) (string, error) {
	sourceAgent := normalizeString(args["_source_agent"])
	groupID := groupIDFromArgs(args)
	if groupID != "" {
		if err := ensureSourceCanUseGroupContext(ctx, sourceAgent, groupID); err != nil {
			return "", err
		}
		return executeListGroupAgentsContext(ctx, groupID, args)
	}
	if sourceAgent != "" {
		groups, err := fetchVisibleGroupsContext(ctx, sourceAgent, model.GroupStatusActive)
		if err != nil {
			return "", err
		}
		if hasGroup(groups, model.DefaultP2PGroupID) {
			return executeListGroupAgentsContext(ctx, model.DefaultP2PGroupID, args)
		}
		body, _ := json.Marshal(groups)
		list, _ := formatGroupList(body)
		return "group_id is required before listing agents. Call list_groups first, choose one group_id, then call list_agents with that group_id. Simple-mode agents can omit group_id only when they are members of default-p2p.\n" + list, nil
	}

	req, err := platformRequestContext(ctx, http.MethodGet, platformBaseURL+"/api/agents", nil)
	if err != nil {
		return "", err
	}
	body, err := doPlatformRequest(req, 10*time.Second)
	if err != nil {
		return "", fmt.Errorf("failed to list agents: %w", err)
	}
	return formatAgentList(body, args)
}

func executeListGroupAgents(groupID string, args map[string]any) (string, error) {
	return executeListGroupAgentsContext(context.Background(), groupID, args)
}

func executeListGroupAgentsContext(ctx context.Context, groupID string, args map[string]any) (string, error) {
	members, err := fetchGroupMembersContext(ctx, groupID)
	if err != nil {
		return "", err
	}

	var agents []map[string]interface{}
	for _, member := range members {
		if normalizeString(member["actor_type"]) != model.GroupActorAgent {
			continue
		}
		name := normalizeString(member["actor_id"])
		if name == "" {
			continue
		}
		agent, err := fetchAgentInfoContext(ctx, name)
		if err != nil {
			agent = map[string]interface{}{
				"name":   name,
				"type":   "unknown",
				"status": "unknown",
				"role":   normalizeString(member["role"]),
			}
		} else if role := normalizeString(member["role"]); role != "" {
			agent["role"] = role
		}
		agents = append(agents, agent)
	}
	body, _ := json.Marshal(agents)
	return formatAgentList(body, args)
}

func formatAgentList(body []byte, args map[string]any) (string, error) {
	var agents []map[string]interface{}
	if err := json.Unmarshal(body, &agents); err != nil {
		return string(body), nil
	}
	filterType, _ := args["filter_type"].(string)

	var lines []string
	for _, a := range agents {
		name, _ := a["name"].(string)
		status, _ := a["status"].(string)
		aType, _ := a["type"].(string)
		desc, _ := a["description"].(string)
		role, _ := a["role"].(string)
		if filterType != "" && aType != filterType {
			continue
		}

		line := fmt.Sprintf("  - %s (type: %s, status: %s)", name, aType, status)
		if role != "" {
			line += fmt.Sprintf(" role: %s", role)
		}
		if desc != "" {
			line += fmt.Sprintf(" - %s", desc)
		}
		lines = append(lines, line)
	}

	result := append([]string{fmt.Sprintf("Found %d agents:", len(lines))}, lines...)
	return "\n" + strings.Join(result, "\n"), nil
}

func executeSendToAgent(args map[string]any) (string, error) {
	return executeSendToAgentContext(context.Background(), args)
}

func executeSendToAgentContext(ctx context.Context, args map[string]any) (string, error) {
	agent, ok := args["agent"].(string)
	if !ok || agent == "" {
		return "", fmt.Errorf("agent name is required")
	}
	message, ok := args["message"].(string)
	if !ok || message == "" {
		return "", fmt.Errorf("message is required")
	}
	sourceAgent := normalizeString(args["_source_agent"])
	rootContextId := normalizeString(args["_root_context_id"])
	parentTaskId := normalizeString(args["_parent_task_id"])
	parentToolCallId := normalizeString(args["_parent_tool_call_id"])
	groupID := groupIDFromArgs(args)

	if groupID == "" && sourceAgent != "" {
		inferred, ok, err := inferDefaultP2PGroupContext(ctx, sourceAgent)
		if err != nil {
			return "", err
		}
		if !ok {
			return "", fmt.Errorf("group_id is required before sending to another agent; call list_groups, then list_agents with the selected group_id. Simple-mode agents can omit group_id only when they are members of default-p2p")
		}
		groupID = inferred
	}
	if groupID != "" {
		if err := ensureSourceCanUseGroupContext(ctx, sourceAgent, groupID); err != nil {
			return "", err
		}
		allowed, err := groupHasAgentContext(ctx, groupID, agent)
		if err != nil {
			return "", err
		}
		if !allowed {
			return "", fmt.Errorf("target agent %q is not visible in group %q", agent, groupID)
		}
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
	req, err := http.NewRequestWithContext(ctx, "POST", platformBaseURL+"/agent/"+agent, bytes.NewReader(jsonBody))
	if err != nil {
		return "", fmt.Errorf("failed to build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	addPlatformAuth(req)
	if sourceAgent != "" {
		req.Header.Set("X-A2A-Source-Agent", sourceAgent)
	}
	if groupID != "" {
		req.Header.Set("X-A2A-Tool-Group-ID", groupID)
	}
	if rootContextId != "" {
		req.Header.Set("X-A2A-Root-Context-Id", rootContextId)
	}
	if parentTaskId != "" {
		req.Header.Set("X-A2A-Parent-Task-Id", parentTaskId)
	}
	if parentToolCallId != "" {
		req.Header.Set("X-A2A-Parent-Tool-Call-Id", parentToolCallId)
	}
	resp, err := client.Do(req)
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
	return executeGetAgentInfoContext(context.Background(), args)
}

func executeGetAgentInfoContext(ctx context.Context, args map[string]any) (string, error) {
	name, ok := args["name"].(string)
	if !ok || name == "" {
		name, ok = args["agent_name"].(string)
	}
	if !ok || name == "" {
		return "", fmt.Errorf("agent name is required")
	}
	sourceAgent := normalizeString(args["_source_agent"])
	groupID := groupIDFromArgs(args)
	if groupID == "" && sourceAgent != "" {
		inferred, ok, err := inferDefaultP2PGroupContext(ctx, sourceAgent)
		if err != nil {
			return "", err
		}
		if !ok {
			return "", fmt.Errorf("group_id is required before reading agent info; call list_groups, then list_agents with the selected group_id. Simple-mode agents can omit group_id only when they are members of default-p2p")
		}
		groupID = inferred
	}
	if groupID != "" {
		if err := ensureSourceCanUseGroupContext(ctx, sourceAgent, groupID); err != nil {
			return "", err
		}
		allowed, err := groupHasAgentContext(ctx, groupID, name)
		if err != nil {
			return "", err
		}
		if !allowed {
			return "", fmt.Errorf("agent %q is not visible in group %q", name, groupID)
		}
	}

	agent, err := fetchAgentInfoContext(ctx, name)
	if err != nil {
		return "", err
	}
	body, _ := json.Marshal(agent)
	return string(body), nil
}

func platformRequest(method, url string, body io.Reader) (*http.Request, error) {
	return platformRequestContext(context.Background(), method, url, body)
}

func platformRequestContext(ctx context.Context, method, url string, body io.Reader) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return nil, fmt.Errorf("failed to build request: %w", err)
	}
	addPlatformAuth(req)
	return req, nil
}

func addPlatformAuth(req *http.Request) {
	if platformAdminToken != "" {
		req.Header.Set("X-Admin-Token", platformAdminToken)
	}
}

func doPlatformRequest(req *http.Request, timeout time.Duration) ([]byte, error) {
	client := &http.Client{Timeout: 10 * time.Second}
	if timeout > 0 {
		client.Timeout = timeout
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("request failed with status %d: %s", resp.StatusCode, string(body))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	return body, nil
}

func fetchVisibleGroups(sourceAgent, status string) ([]map[string]interface{}, error) {
	return fetchVisibleGroupsContext(context.Background(), sourceAgent, status)
}

func fetchVisibleGroupsContext(ctx context.Context, sourceAgent, status string) ([]map[string]interface{}, error) {
	endpoint := platformBaseURL + "/api/groups"
	if status != "" {
		endpoint += "?status=" + url.QueryEscape(status)
	}
	req, err := platformRequestContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	body, err := doPlatformRequest(req, 10*time.Second)
	if err != nil {
		return nil, fmt.Errorf("failed to list groups: %w", err)
	}
	var groups []map[string]interface{}
	if err := json.Unmarshal(body, &groups); err != nil {
		return nil, err
	}
	if sourceAgent == "" {
		return groups, nil
	}

	var visible []map[string]interface{}
	for _, group := range groups {
		groupID := normalizeString(group["id"])
		if groupID == "" {
			continue
		}
		ok, err := groupHasAgentContext(ctx, groupID, sourceAgent)
		if err != nil {
			return nil, err
		}
		if ok {
			visible = append(visible, group)
		}
	}
	return visible, nil
}

func fetchGroup(groupID string) (map[string]interface{}, error) {
	return fetchGroupContext(context.Background(), groupID)
}

func fetchGroupContext(ctx context.Context, groupID string) (map[string]interface{}, error) {
	req, err := platformRequestContext(ctx, http.MethodGet, platformBaseURL+"/api/groups/"+url.PathEscape(groupID), nil)
	if err != nil {
		return nil, err
	}
	body, err := doPlatformRequest(req, 10*time.Second)
	if err != nil {
		return nil, fmt.Errorf("failed to get group: %w", err)
	}
	var group map[string]interface{}
	if err := json.Unmarshal(body, &group); err != nil {
		return nil, err
	}
	return group, nil
}

func fetchAgentInfo(name string) (map[string]interface{}, error) {
	return fetchAgentInfoContext(context.Background(), name)
}

func fetchAgentInfoContext(ctx context.Context, name string) (map[string]interface{}, error) {
	req, err := platformRequestContext(ctx, http.MethodGet, platformBaseURL+"/api/agents/"+url.PathEscape(name), nil)
	if err != nil {
		return nil, err
	}
	body, err := doPlatformRequest(req, 10*time.Second)
	if err != nil {
		return nil, fmt.Errorf("failed to get agent info: %w", err)
	}
	var agent map[string]interface{}
	if err := json.Unmarshal(body, &agent); err != nil {
		return nil, err
	}
	return agent, nil
}

func fetchGroupMembers(groupID string) ([]map[string]interface{}, error) {
	return fetchGroupMembersContext(context.Background(), groupID)
}

func fetchGroupMembersContext(ctx context.Context, groupID string) ([]map[string]interface{}, error) {
	req, err := platformRequestContext(ctx, http.MethodGet, platformBaseURL+"/api/groups/"+url.PathEscape(groupID)+"/members", nil)
	if err != nil {
		return nil, err
	}
	body, err := doPlatformRequest(req, 10*time.Second)
	if err != nil {
		return nil, fmt.Errorf("failed to list group members: %w", err)
	}
	var members []map[string]interface{}
	if err := json.Unmarshal(body, &members); err != nil {
		return nil, err
	}
	return members, nil
}

func groupHasAgent(groupID, agent string) (bool, error) {
	return groupHasAgentContext(context.Background(), groupID, agent)
}

func groupHasAgentContext(ctx context.Context, groupID, agent string) (bool, error) {
	if groupID == "" || agent == "" {
		return false, nil
	}
	members, err := fetchGroupMembersContext(ctx, groupID)
	if err != nil {
		return false, err
	}
	for _, member := range members {
		if normalizeString(member["actor_type"]) == model.GroupActorAgent && normalizeString(member["actor_id"]) == agent {
			return true, nil
		}
	}
	return false, nil
}

func inferDefaultP2PGroup(sourceAgent string) (string, bool, error) {
	return inferDefaultP2PGroupContext(context.Background(), sourceAgent)
}

func inferDefaultP2PGroupContext(ctx context.Context, sourceAgent string) (string, bool, error) {
	if sourceAgent == "" {
		return "", false, nil
	}
	groups, err := fetchVisibleGroupsContext(ctx, sourceAgent, model.GroupStatusActive)
	if err != nil {
		return "", false, err
	}
	if hasGroup(groups, model.DefaultP2PGroupID) {
		return model.DefaultP2PGroupID, true, nil
	}
	return "", false, nil
}

func hasGroup(groups []map[string]interface{}, groupID string) bool {
	for _, group := range groups {
		if normalizeString(group["id"]) == groupID {
			return true
		}
	}
	return false
}

func ensureSourceCanUseGroup(sourceAgent, groupID string) error {
	return ensureSourceCanUseGroupContext(context.Background(), sourceAgent, groupID)
}

func ensureSourceCanUseGroupContext(ctx context.Context, sourceAgent, groupID string) error {
	if sourceAgent == "" || groupID == "" {
		return nil
	}
	ok, err := groupHasAgentContext(ctx, groupID, sourceAgent)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("source agent %q is not a member of group %q", sourceAgent, groupID)
	}
	return nil
}

func formatGroupList(body []byte) (string, error) {
	var groups []map[string]interface{}
	if err := json.Unmarshal(body, &groups); err != nil {
		return string(body), nil
	}
	var lines []string
	for _, group := range groups {
		id := normalizeString(group["id"])
		name := normalizeString(group["name"])
		mode := normalizeString(group["orchestration_mode"])
		status := normalizeString(group["status"])
		desc := normalizeString(group["description"])
		line := fmt.Sprintf("  - %s (id: %s, mode: %s, status: %s)", name, id, mode, status)
		if desc != "" {
			line += fmt.Sprintf(" - %s", desc)
		}
		lines = append(lines, line)
	}
	result := append([]string{fmt.Sprintf("Found %d groups:", len(lines))}, lines...)
	return "\n" + strings.Join(result, "\n"), nil
}

func groupIDFromArgs(args map[string]any) string {
	if groupID := normalizeString(args["_group_id"]); groupID != "" {
		return groupID
	}
	return normalizeString(args["group_id"])
}

func normalizeString(value interface{}) string {
	if s, ok := value.(string); ok {
		return strings.TrimSpace(s)
	}
	return ""
}
