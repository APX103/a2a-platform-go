package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"a2a-platform/internal/model"
	"a2a-platform/internal/svc"

	"github.com/google/uuid"
)

// MCPSSEHandler implements the MCP SSE transport protocol.
// GET /mcp/sse — establishes SSE connection
// POST /mcp/messages — receives JSON-RPC requests
type MCPSSEHandler struct {
	svcCtx   *svc.ServiceContext
	sessions map[string]*SSESession
	mu       sync.RWMutex
	hostURL  string
}

type SSESession struct {
	ID     string
	Events chan string
	Done   chan struct{}
	Client http.Client
}

func NewMCPSSEHandler(svcCtx *svc.ServiceContext, hostURL string) *MCPSSEHandler {
	return &MCPSSEHandler{
		svcCtx:   svcCtx,
		sessions: make(map[string]*SSESession),
		hostURL:  hostURL,
	}
}

// ServeSSE handles GET /mcp/sse — establishes SSE connection.
func (h *MCPSSEHandler) ServeSSE(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	sessionID := uuid.New().String()
	session := &SSESession{
		ID:     sessionID,
		Events: make(chan string, 100),
		Done:   make(chan struct{}),
	}

	h.mu.Lock()
	h.sessions[sessionID] = session
	h.mu.Unlock()

	// Send the endpoint event
	endpoint := fmt.Sprintf("/mcp/messages?session_id=%s", sessionID)
	fmt.Fprintf(w, "event: endpoint\ndata: %s\n\n", endpoint)
	w.(http.Flusher).Flush()

	// Keep alive, relay events
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	defer func() {
		h.mu.Lock()
		delete(h.sessions, sessionID)
		h.mu.Unlock()
		close(session.Done)
	}()

	for {
		select {
		case evt, ok := <-session.Events:
			if !ok {
				return
			}
			fmt.Fprintf(w, "event: message\ndata: %s\n\n", evt)
			w.(http.Flusher).Flush()
		case <-ticker.C:
			fmt.Fprintf(w, ": keepalive\n\n")
			w.(http.Flusher).Flush()
		case <-r.Context().Done():
			return
		}
	}
}

// ServeMessages handles POST /mcp/messages — receives JSON-RPC requests.
func (h *MCPSSEHandler) ServeMessages(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, "method not allowed", 405)
		return
	}

	sessionID := r.URL.Query().Get("session_id")
	h.mu.RLock()
	session, ok := h.sessions[sessionID]
	h.mu.RUnlock()
	if !ok {
		// No active SSE session — process statelessly and return JSON directly.
		// This allows curl/testing without maintaining an SSE connection.
		session = nil
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodyBytes)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			jsonError(w, "request body too large", http.StatusRequestEntityTooLarge)
			return
		}
		jsonError(w, "read body failed", 500)
		return
	}

	// Parse JSON-RPC request
	var rpcReq struct {
		Jsonrpc string          `json:"jsonrpc"`
		ID      interface{}     `json:"id"`
		Method  string          `json:"method"`
		Params  json.RawMessage `json:"params"`
	}
	if err := json.Unmarshal(body, &rpcReq); err != nil {
		h.sendError(session, rpcReq.ID, -32700, "Parse error")
		return
	}

	var result interface{}
	var rpcErr *RPCError

	switch rpcReq.Method {
	case "initialize":
		result = h.handleInitialize()
	case "tools/list":
		result = h.handleToolsList()
	case "tools/call":
		result, rpcErr = h.handleToolsCall(rpcReq.Params)
	case "resources/list":
		result = h.handleResourcesList()
	case "resources/read":
		result, rpcErr = h.handleResourcesRead(rpcReq.Params)
	case "ping":
		result = map[string]interface{}{}
	default:
		rpcErr = &RPCError{Code: -32601, Message: "Method not found"}
	}

	// Build JSON-RPC response
	var rpcResp JSONRPCResponse
	rpcResp.Jsonrpc = "2.0"
	rpcResp.ID = rpcReq.ID
	if rpcErr != nil {
		rpcResp.Error = &struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		}{Code: rpcErr.Code, Message: rpcErr.Message}
	} else {
		rpcResp.Result = result
	}

	// Send response via SSE channel (if session is active)
	respBytes, _ := json.Marshal(rpcResp)
	if session != nil {
		session.Events <- string(respBytes)
	}

	// Also return JSON in HTTP response body
	w.Header().Set("Content-Type", "application/json")
	w.Write(respBytes)
}

// ===== JSON-RPC Response helpers =====

type RPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type JSONRPCResponse struct {
	Jsonrpc string      `json:"jsonrpc"`
	ID      interface{} `json:"id"`
	Result  interface{} `json:"result,omitempty"`
	Error   *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

func (h *MCPSSEHandler) sendResult(session *SSESession, id interface{}, result interface{}) {
	resp := JSONRPCResponse{
		Jsonrpc: "2.0",
		ID:      id,
		Result:  result,
	}
	data, _ := json.Marshal(resp)
	select {
	case session.Events <- string(data):
	case <-session.Done:
	}
}

func (h *MCPSSEHandler) sendError(session *SSESession, id interface{}, code int, message string) {
	resp := JSONRPCResponse{
		Jsonrpc: "2.0",
		ID:      id,
		Error: &struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		}{Code: code, Message: message},
	}
	data, _ := json.Marshal(resp)
	select {
	case session.Events <- string(data):
	case <-session.Done:
	}
}

// ===== MCP Method Handlers =====

func (h *MCPSSEHandler) handleInitialize() interface{} {
	return map[string]interface{}{
		"protocolVersion": "2024-11-05",
		"capabilities": map[string]interface{}{
			"tools":     map[string]interface{}{},
			"resources": map[string]interface{}{},
		},
		"serverInfo": map[string]interface{}{
			"name":    "a2a-platform",
			"version": "1.0.0",
		},
	}
}

func (h *MCPSSEHandler) handleToolsList() interface{} {
	return map[string]interface{}{
		"tools": []interface{}{
			map[string]interface{}{
				"name":        "list_agents",
				"description": "List all available A2A agents and their skills",
				"inputSchema": map[string]interface{}{
					"type":       "object",
					"properties": map[string]interface{}{},
				},
			},
			map[string]interface{}{
				"name":        "send_to_agent",
				"description": "Send a message to another A2A agent via the platform",
				"inputSchema": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"agent_name": map[string]interface{}{"type": "string", "description": "Target agent name"},
						"message":    map[string]interface{}{"type": "string", "description": "Message text to send"},
					},
					"required": []string{"agent_name", "message"},
				},
			},
			map[string]interface{}{
				"name":        "get_agent_info",
				"description": "Get detailed info about a specific A2A agent",
				"inputSchema": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"agent_name": map[string]interface{}{"type": "string", "description": "Agent name to query"},
					},
					"required": []string{"agent_name"},
				},
			},
		},
	}
}

func (h *MCPSSEHandler) handleToolsCall(params json.RawMessage) (interface{}, *RPCError) {
	var call struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	}
	if err := json.Unmarshal(params, &call); err != nil {
		return nil, &RPCError{Code: -32602, Message: "Invalid params"}
	}

	switch call.Name {
	case "list_agents":
		return h.toolListAgents()
	case "send_to_agent":
		return h.toolSendToAgent(call.Arguments)
	case "get_agent_info":
		return h.toolGetAgentInfo(call.Arguments)
	default:
		return nil, &RPCError{Code: -32601, Message: "Unknown tool: " + call.Name}
	}
}

func (h *MCPSSEHandler) toolListAgents() (interface{}, *RPCError) {
	agents, err := h.svcCtx.Registry.ListAgents()
	if err != nil {
		return nil, &RPCError{Code: -32000, Message: err.Error()}
	}
	var lines []string
	for _, a := range agents {
		lines = append(lines, fmt.Sprintf("- %s: %s (status: %s)", a.Name, a.Description, a.Status))
	}
	text := strings.Join(lines, "\n")
	if text == "" {
		text = "(no agents registered)"
	}
	return map[string]interface{}{
		"content": []interface{}{
			map[string]interface{}{"type": "text", "text": text},
		},
	}, nil
}

func (h *MCPSSEHandler) toolSendToAgent(args json.RawMessage) (interface{}, *RPCError) {
	var params struct {
		AgentName string `json:"agent_name"`
		Message   string `json:"message"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return nil, &RPCError{Code: -32602, Message: "Invalid arguments"}
	}

	// Send A2A message via host proxy
	resultText, err := sendA2AMessage(h.hostURL, params.AgentName, params.Message)
	if err != nil {
		return nil, &RPCError{Code: -32000, Message: err.Error()}
	}
	return map[string]interface{}{
		"content": []interface{}{
			map[string]interface{}{"type": "text", "text": resultText},
		},
	}, nil
}

func (h *MCPSSEHandler) toolGetAgentInfo(args json.RawMessage) (interface{}, *RPCError) {
	var params struct {
		AgentName string `json:"agent_name"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return nil, &RPCError{Code: -32602, Message: "Invalid arguments"}
	}
	agent, err := h.svcCtx.Agents.Get(params.AgentName)
	if err != nil {
		return nil, &RPCError{Code: -32000, Message: err.Error()}
	}
	if agent == nil {
		return nil, &RPCError{Code: -32000, Message: fmt.Sprintf("Agent '%s' not found", params.AgentName)}
	}

	info := model.AgentInfo{
		Name:         agent.Name,
		Url:          "/agent/" + agent.Name,
		Status:       agent.Status,
		Type:         agent.Type,
		Skills:       svc.ParseSkillsJson(agent.SkillsJson),
		ErrorMessage: agent.ErrorMessage,
	}
	conn := h.svcCtx.Registry.GetClient(params.AgentName)
	if conn != nil {
		ci := conn.Info()
		info.Description = ci.Description
		info.Version = ci.Version
	}
	data, _ := json.MarshalIndent(info, "", "  ")
	return map[string]interface{}{
		"content": []interface{}{
			map[string]interface{}{"type": "text", "text": string(data)},
		},
	}, nil
}

func (h *MCPSSEHandler) handleResourcesList() interface{} {
	return map[string]interface{}{
		"resources": []interface{}{
			map[string]interface{}{
				"uri":         "a2a://agents",
				"name":        "A2A Agents",
				"description": "List of all registered A2A agents",
				"mimeType":    "application/json",
			},
		},
	}
}

func (h *MCPSSEHandler) handleResourcesRead(params json.RawMessage) (interface{}, *RPCError) {
	var read struct {
		URI string `json:"uri"`
	}
	json.Unmarshal(params, &read)

	if read.URI == "a2a://agents" {
		agents, err := h.svcCtx.Registry.ListAgents()
		if err != nil {
			return nil, &RPCError{Code: -32000, Message: err.Error()}
		}
		data, _ := json.MarshalIndent(agents, "", "  ")
		return map[string]interface{}{
			"contents": []interface{}{
				map[string]interface{}{
					"uri":      "a2a://agents",
					"mimeType": "application/json",
					"text":     string(data),
				},
			},
		}, nil
	}
	return nil, &RPCError{Code: -32000, Message: "Unknown resource: " + read.URI}
}

// ===== A2A Message Sender =====

func sendA2AMessage(hostURL, agentName, message string) (string, error) {
	req := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      uuid.New().String()[:12],
		"method":  "SendStreamingMessage",
		"params": map[string]interface{}{
			"message": map[string]interface{}{
				"message_id": uuid.New().String(),
				"role":       "ROLE_USER",
				"parts":      []map[string]string{{"text": message}},
			},
		},
	}
	body, _ := json.Marshal(req)
	url := hostURL + "/agent/" + agentName

	client := &http.Client{Timeout: 120 * time.Second}
	httpReq, _ := http.NewRequest("POST", url, strings.NewReader(string(body)))
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("A2A-Version", "1.0")
	httpReq.Header.Set("Accept", "text/event-stream")

	resp, err := client.Do(httpReq)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	resultText := ""
	buf := make([]byte, 4096)
	for {
		n, err := resp.Body.Read(buf)
		if n > 0 {
			text := extractSSETextFromBuf(buf[:n])
			if text != "" {
				resultText = text
			}
		}
		if err != nil {
			break
		}
	}

	if resultText == "" {
		resultText = "(empty response)"
	}
	return resultText, nil
}

func extractSSETextFromBuf(chunk []byte) string {
	lines := strings.Split(string(chunk), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimPrefix(line, "data:")
		data = strings.TrimSpace(data)
		var evt map[string]interface{}
		if json.Unmarshal([]byte(data), &evt) != nil {
			continue
		}
		// Check result.artifactUpdate.artifact.parts (a2a-bridge format)
		if result, ok := evt["result"].(map[string]interface{}); ok {
			if au, ok := result["artifactUpdate"].(map[string]interface{}); ok {
				if artifact, ok := au["artifact"].(map[string]interface{}); ok {
					text := extractPartsTextFromMap(artifact)
					if text != "" {
						return text
					}
				}
			}
			// Check result.message.parts
			if msg, ok := result["message"].(map[string]interface{}); ok {
				text := extractPartsTextFromMap(msg)
				if text != "" {
					return text
				}
			}
		}
		// Check top-level message.parts
		if msg, ok := evt["message"].(map[string]interface{}); ok {
			text := extractPartsTextFromMap(msg)
			if text != "" {
				return text
			}
		}
	}
	return ""
}

func extractPartsTextFromMap(msg map[string]interface{}) string {
	parts, ok := msg["parts"].([]interface{})
	if !ok {
		return ""
	}
	var texts []string
	for _, p := range parts {
		if pm, ok := p.(map[string]interface{}); ok {
			if t, ok := pm["text"].(string); ok {
				texts = append(texts, t)
			}
		}
	}
	return strings.Join(texts, "\n")
}
