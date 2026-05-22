package handler

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"a2a-platform/internal/engine"
	"a2a-platform/internal/model"
	"a2a-platform/internal/svc"
)

const maxRequestBodyBytes = 16 << 20

// ===== Agent CRUD =====

type ListAgentsHandler struct {
	svcCtx *svc.ServiceContext
}

func NewListAgentsHandler(svcCtx *svc.ServiceContext) *ListAgentsHandler {
	return &ListAgentsHandler{svcCtx: svcCtx}
}

func (h *ListAgentsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	agents, err := h.svcCtx.Registry.ListAgents()
	if err != nil {
		errHTTP(w, err)
		return
	}
	if agents == nil {
		agents = []model.AgentInfo{}
	}
	okJSON(w, agents)
}

// =====

type GetAgentHandler struct {
	svcCtx *svc.ServiceContext
}

func NewGetAgentHandler(svcCtx *svc.ServiceContext) *GetAgentHandler {
	return &GetAgentHandler{svcCtx: svcCtx}
}

func (h *GetAgentHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	name := getPathParam(r, "name")
	if name == "" {
		errHTTP(w, fmt.Errorf("missing agent name"))
		return
	}
	agent, err := h.svcCtx.Agents.Get(name)
	if err != nil {
		errHTTP(w, err)
		return
	}
	if agent == nil {
		jsonError(w, "not found", 404)
		return
	}
	conn := h.svcCtx.Registry.GetClient(name)
	info := model.AgentInfo{
		Name:         agent.Name,
		Url:          "/agent/" + agent.Name,
		Status:       agent.Status,
		Type:         agent.Type,
		Skills:       svc.ParseSkillsJson(agent.SkillsJson),
		ErrorMessage: agent.ErrorMessage,
	}
	if conn != nil {
		ci := conn.Info()
		info.Description = ci.Description
		info.Version = ci.Version
	}
	okJSON(w, info)
}

// =====

type RegisterAgentHandler struct {
	svcCtx *svc.ServiceContext
}

func NewRegisterAgentHandler(svcCtx *svc.ServiceContext) *RegisterAgentHandler {
	return &RegisterAgentHandler{svcCtx: svcCtx}
}

func (h *RegisterAgentHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var req model.RegisterAgentReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid JSON", 400)
		return
	}
	if req.Name == "" {
		jsonError(w, "missing name", 400)
		return
	}
	conn, err := h.svcCtx.Registry.RegisterAgent(
		req.Name, req.Type, req.Url, req.Port, req.Skills, req.Secret,
	)
	if err != nil {
		jsonError(w, err.Error(), 400)
		return
	}
	okJSON(w, model.RegisterAgentResp{
		Ok:     true,
		Name:   req.Name,
		Url:    conn.Url,
		Status: "connected",
	})
}

// ===== Discovery (well-known) =====

type DiscoveryHandler struct {
	svcCtx *svc.ServiceContext
}

func NewDiscoveryHandler(svcCtx *svc.ServiceContext) *DiscoveryHandler {
	return &DiscoveryHandler{svcCtx: svcCtx}
}

func (h *DiscoveryHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	name := getPathParam(r, "name")
	if name == "" {
		jsonError(w, "missing agent name", 400)
		return
	}
	agent, err := h.svcCtx.Agents.Get(name)
	if err != nil || agent == nil {
		jsonError(w, "not found", 404)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(agent.AgentCardJson))
}

// ===== Agent Proxy (core A2A message routing) =====

type AgentProxyHandler struct {
	svcCtx *svc.ServiceContext
}

func NewAgentProxyHandler(svcCtx *svc.ServiceContext) *AgentProxyHandler {
	return &AgentProxyHandler{svcCtx: svcCtx}
}

func (h *AgentProxyHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, "method not allowed", 405)
		return
	}

	name := getPathParam(r, "name")

	// Read the incoming request body
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

	// Parse JSON-RPC to extract message and contextId for tracing
	var rpcReq map[string]interface{}
	if err := json.Unmarshal(body, &rpcReq); err != nil {
		jsonError(w, "invalid JSON", 400)
		return
	}

	// Extract or auto-generate contextId
	var contextId *string
	if params, ok := rpcReq["params"].(map[string]interface{}); ok {
		if cid, ok := params["contextId"].(string); ok && cid != "" {
			contextId = &cid
		}
	}
	if contextId == nil {
		cid := svc.NewTaskId() // use UUID
		contextId = &cid
		// Inject contextId into request so bridge/agent can see it
		if params, ok := rpcReq["params"].(map[string]interface{}); ok {
			params["contextId"] = *contextId
		} else {
			rpcReq["params"] = map[string]interface{}{"contextId": *contextId}
		}
	}

	// Re-marshal body with injected contextId for forwarding
	newBody, err := json.Marshal(rpcReq)
	if err != nil {
		jsonError(w, "request marshal failed", 500)
		return
	}
	body = newBody

	// Create a task for tracking
	taskId := svc.NewTaskId()
	task := &model.Task{
		LocalTaskId: taskId,
		AgentName:   name,
		State:       "PENDING",
		ContextId:   contextId,
	}
	if err := h.svcCtx.Tasks.Create(task); err != nil {
		jsonError(w, fmt.Sprintf("task create failed: %s", err), 500)
		return
	}
	if h.svcCtx.EventBus != nil {
		h.svcCtx.EventBus.Task("create", taskId, name, "PENDING")
	}

	// Extract user text for message recording
	userText := extractUserText(rpcReq)
	if userText != "" {
		if err := h.svcCtx.Messages.Append(&model.Message{
			TaskId:    taskId,
			ContextId: contextId,
			Role:      "user",
			Content:   userText,
		}); err != nil {
			jsonError(w, fmt.Sprintf("message save failed: %s", err), 500)
			return
		}
	}

	// Record trace
	sendTrace := &model.TraceEvent{
		TaskId:      taskId,
		ContextId:   contextId,
		EventType:   "send",
		AgentName:   "host",
		TargetAgent: &name,
		DataJson:    string(body),
	}
	h.svcCtx.Traces.Append(sendTrace)
	if h.svcCtx.EventBus != nil {
		h.svcCtx.EventBus.TraceEvent(sendTrace)
	}

	// Check if this is a builtin agent
	if builtinAgent := h.svcCtx.Engine.GetAgent(name); builtinAgent != nil {
		h.svcCtx.Tasks.Update(taskId, map[string]interface{}{"state": "WORKING"})
		deps := &engine.Deps{
			LoadHistory: func(cid string) ([]*model.Message, error) {
				return h.svcCtx.Messages.GetByContext(cid)
			},
			RecordTrace: func(e *model.TraceEvent) error {
				return h.svcCtx.Traces.Append(e)
			},
			SaveMessage: func(m *model.Message) error {
				return h.svcCtx.Messages.Append(m)
			},
		}
		h.svcCtx.Engine.HandleRequest(r.Context(), w, name, userText, *contextId, taskId, deps)

		// Record final agent response from the engine
		// The engine already streamed SSE to the client; now record the final message
		h.svcCtx.Tasks.Update(taskId, map[string]interface{}{"state": "RESPONDED"})
		if h.svcCtx.EventBus != nil {
			h.svcCtx.EventBus.Task("update", taskId, name, "RESPONDED")
		}
		return
	}

	// Check if this is a bridge agent
	if bridgeAgent := h.svcCtx.BridgeRegistry.Get(name); bridgeAgent != nil {
		h.svcCtx.Tasks.Update(taskId, map[string]interface{}{"state": "WORKING"})
		bridgeAgent.HandleRequest(r.Context(), w, userText, taskId, *contextId)
		h.svcCtx.Tasks.Update(taskId, map[string]interface{}{"state": "RESPONDED"})
		if h.svcCtx.EventBus != nil {
			h.svcCtx.EventBus.Task("update", taskId, name, "RESPONDED")
		}
		return
	}

	// External agent: check connection exists
	conn := h.svcCtx.Registry.GetClient(name)
	if conn == nil {
		jsonError(w, fmt.Sprintf("agent '%s' not found", name), 404)
		return
	}

	// Forward to the bridge agent with timeout
	targetURL := conn.Url + "/"
	client := &http.Client{Timeout: 180 * time.Second}
	ctx, cancel := context.WithTimeout(r.Context(), 180*time.Second)
	defer cancel()
	proxyReq, err := http.NewRequestWithContext(ctx, "POST", targetURL, bytes.NewReader(body))
	if err != nil {
		jsonError(w, fmt.Sprintf("proxy create failed: %s", err), 500)
		return
	}
	proxyReq.Header.Set("Content-Type", "application/json")
	proxyReq.Header.Set("A2A-Version", "1.0")
	proxyReq.Header.Set("Accept", "text/event-stream")

	proxyResp, err := client.Do(proxyReq)
	if err != nil {
		h.svcCtx.Tasks.Update(taskId, map[string]interface{}{"state": "ERROR"})
		jsonError(w, fmt.Sprintf("proxy failed: %s", err), 502)
		return
	}
	defer proxyResp.Body.Close()

	// Check if response is SSE (streaming) or JSON
	contentType := proxyResp.Header.Get("Content-Type")
	w.Header().Set("Content-Type", contentType)

	if strings.Contains(contentType, "text/event-stream") {
		// SSE streaming: relay events to client
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		flusher, ok := w.(http.Flusher)
		if !ok {
			jsonError(w, "streaming not supported", 500)
			return
		}

		buf := make([]byte, 4096)
		finalText := ""
		for {
			n, err := proxyResp.Body.Read(buf)
			if n > 0 {
				chunk := buf[:n]
				w.Write(chunk)

				// Parse each SSE data line in the chunk and record stream traces
				lines := strings.Split(string(chunk), "\n")
				for _, line := range lines {
					line = strings.TrimSpace(line)
					if !strings.HasPrefix(line, "data:") {
						continue
					}
					data := strings.TrimPrefix(line, "data:")
					data = strings.TrimSpace(data)
					if data == "" {
						continue
					}

					// Record every SSE data frame as a stream trace
					streamTrace := &model.TraceEvent{
						TaskId:    taskId,
						ContextId: contextId,
						EventType: "stream",
						AgentName: name,
						DataJson:  truncateString(data, 500),
					}
					h.svcCtx.Traces.Append(streamTrace)
					if h.svcCtx.EventBus != nil {
						h.svcCtx.EventBus.TraceEvent(streamTrace)
					}

					// Try to extract meaningful text for final response
					text := extractTextFromSSEData(data)
					if text != "" {
						finalText = text
					}
				}

				flusher.Flush()
			}
			if err != nil {
				break
			}
		}

		// Record agent response
		if finalText != "" {
			h.svcCtx.Messages.Append(&model.Message{
				TaskId:    taskId,
				ContextId: contextId,
				Role:      "agent",
				Content:   finalText,
			})
			h.svcCtx.Tasks.Update(taskId, map[string]interface{}{"state": "RESPONDED"})
			if h.svcCtx.EventBus != nil {
				h.svcCtx.EventBus.Task("update", taskId, name, "RESPONDED")
			}
		}
		respTrace := &model.TraceEvent{
			TaskId:    taskId,
			ContextId: contextId,
			EventType: "response",
			AgentName: name,
			DataJson:  finalText,
		}
		if finalText == "" {
			respTrace.DataJson = `{"text_length":0}`
		}
		h.svcCtx.Traces.Append(respTrace)
		if h.svcCtx.EventBus != nil {
			h.svcCtx.EventBus.TraceEvent(respTrace)
		}
	} else {
		// Non-streaming: relay the full response
		respBody, _ := io.ReadAll(io.LimitReader(proxyResp.Body, 16<<20))
		w.WriteHeader(proxyResp.StatusCode)
		w.Write(respBody)

		// Record response
		respText := extractResponseText(respBody)
		if respText != "" {
			h.svcCtx.Messages.Append(&model.Message{
				TaskId:    taskId,
				ContextId: contextId,
				Role:      "agent",
				Content:   respText,
			})
			h.svcCtx.Tasks.Update(taskId, map[string]interface{}{"state": "RESPONDED"})
		}
	}
}

// ===== Task Management =====

type ListTasksHandler struct {
	svcCtx *svc.ServiceContext
}

func NewListTasksHandler(svcCtx *svc.ServiceContext) *ListTasksHandler {
	return &ListTasksHandler{svcCtx: svcCtx}
}

func (h *ListTasksHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	agentName := q.Get("agent_name")
	state := q.Get("state")
	search := q.Get("search")
	page, _ := strconv.Atoi(q.Get("page"))
	size, _ := strconv.Atoi(q.Get("size"))
	if page <= 0 {
		page = 1
	}
	if size <= 0 {
		size = 20
	}

	tasks, total, err := h.svcCtx.Tasks.List(agentName, state, search, page, size)
	if err != nil {
		errHTTP(w, err)
		return
	}

	items := make([]model.TaskListItem, 0, len(tasks))
	for _, t := range tasks {
		displayId := t.LocalTaskId[:8]
		if t.ServerTaskId != nil && *t.ServerTaskId != "" {
			displayId = *t.ServerTaskId
		}
		items = append(items, model.TaskListItem{
			LocalTaskId: t.LocalTaskId,
			DisplayId:   displayId,
			AgentName:   t.AgentName,
			State:       t.State,
			ContextId:   t.ContextId,
			CreatedAt:   t.CreatedAt.Format("2006-01-02T15:04:05Z"),
			UpdatedAt:   t.UpdatedAt.Format("2006-01-02T15:04:05Z"),
		})
	}

	okJSON(w, model.ListTasksResp{
		Items: items,
		Total: total,
		Page:  page,
		Size:  size,
	})
}

// ===== Task Detail =====

type GetTaskHandler struct {
	svcCtx *svc.ServiceContext
}

func NewGetTaskHandler(svcCtx *svc.ServiceContext) *GetTaskHandler {
	return &GetTaskHandler{svcCtx: svcCtx}
}

func (h *GetTaskHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	taskId := getPathParam(r, "taskId")
	task, err := h.svcCtx.Tasks.Get(taskId)
	if err != nil || task == nil {
		jsonError(w, "not found", 404)
		return
	}
	messages, _ := h.svcCtx.Messages.GetByTask(taskId)
	traces, _ := h.svcCtx.Traces.GetByTask(taskId)
	okJSON(w, map[string]interface{}{
		"task":     task,
		"messages": messages,
		"traces":   traces,
	})
}

// ===== Trace Contexts =====

type TraceContextHandler struct {
	svcCtx *svc.ServiceContext
}

func NewTraceContextHandler(svcCtx *svc.ServiceContext) *TraceContextHandler {
	return &TraceContextHandler{svcCtx: svcCtx}
}

func (h *TraceContextHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	contexts, err := h.svcCtx.Traces.ListContexts(200)
	if err != nil {
		errHTTP(w, err)
		return
	}
	okJSON(w, contexts)
}

// ===== Trace =====

type TraceHandler struct {
	svcCtx *svc.ServiceContext
}

func NewTraceHandler(svcCtx *svc.ServiceContext) *TraceHandler {
	return &TraceHandler{svcCtx: svcCtx}
}

func (h *TraceHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// List recent traces when accessed without specific filter
	if r.URL.Path == "/api/traces" || r.URL.Path == "/api/traces/" {
		traces, err := h.svcCtx.Traces.ListRecent(200)
		if err != nil {
			errHTTP(w, err)
			return
		}
		okJSON(w, traces)
		return
	}

	contextId := getPathParam(r, "contextId")
	// Check if contextId header was explicitly set (even if empty)
	if r.Header.Get("X-Path-Param-ContextId") != "" || contextId != "" {
		// Empty contextId means NULL in DB (default group)
		traces, err := h.svcCtx.Traces.GetByContext(contextId)
		if err != nil {
			errHTTP(w, err)
			return
		}
		okJSON(w, traces)
		return
	}
	taskId := getPathParam(r, "taskId")
	if taskId != "" {
		traces, err := h.svcCtx.Traces.GetByTask(taskId)
		if err != nil {
			errHTTP(w, err)
			return
		}
		okJSON(w, traces)
		return
	}
	jsonError(w, "missing context_id or task_id", 400)
}

// ===== Helper functions =====

func getPathParam(r *http.Request, key string) string {
	// Read from X-Path-Param-* headers set by main.go route helpers
	var headerName string
	switch key {
	case "name":
		headerName = "X-Path-Param-Name"
	case "taskId":
		headerName = "X-Path-Param-TaskId"
	case "contextId":
		headerName = "X-Path-Param-ContextId"
	default:
		headerName = "X-Path-Param-" + key
	}
	if v := r.Header.Get(headerName); v != "" {
		return v
	}
	return ""
}

func extractUserText(rpcReq map[string]interface{}) string {
	if rpcReq == nil {
		return ""
	}
	params, ok := rpcReq["params"].(map[string]interface{})
	if !ok {
		return ""
	}
	msg, ok := params["message"].(map[string]interface{})
	if !ok {
		return ""
	}
	parts, ok := msg["parts"].([]interface{})
	if !ok {
		return ""
	}
	for _, p := range parts {
		if pm, ok := p.(map[string]interface{}); ok {
			if t, ok := pm["text"].(string); ok {
				return t
			}
		}
	}
	return ""
}

func extractTextFromSSEData(data string) string {
	var evt map[string]interface{}
	if json.Unmarshal([]byte(data), &evt) != nil {
		return ""
	}
	// Check for direct message
	if msg, ok := evt["message"].(map[string]interface{}); ok {
		return extractPartsText(msg)
	}
	// Check for result wrappers
	if result, ok := evt["result"].(map[string]interface{}); ok {
		// result.message.parts
		if msg, ok := result["message"].(map[string]interface{}); ok {
			return extractPartsText(msg)
		}
		// result.artifactUpdate.artifact.parts
		if artifactUpdate, ok := result["artifactUpdate"].(map[string]interface{}); ok {
			if artifact, ok := artifactUpdate["artifact"].(map[string]interface{}); ok {
				return extractPartsText(artifact)
			}
		}
		// result.statusUpdate.status.message.parts
		if statusUpdate, ok := result["statusUpdate"].(map[string]interface{}); ok {
			if status, ok := statusUpdate["status"].(map[string]interface{}); ok {
				if msg, ok := status["message"].(map[string]interface{}); ok {
					text := extractPartsText(msg)
					if text != "" {
						return text
					}
				}
			}
		}
	}
	return ""
}

func extractSSEText(chunk []byte) string {
	// Parse SSE data lines to find text content
	lines := strings.Split(string(chunk), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimPrefix(line, "data:")
		data = strings.TrimSpace(data)
		text := extractTextFromSSEData(data)
		if text != "" {
			return text
		}
	}
	return ""
}

func extractResponseText(body []byte) string {
	var resp map[string]interface{}
	if json.Unmarshal(body, &resp) != nil {
		return string(body)
	}
	// Try result.message.parts
	if result, ok := resp["result"].(map[string]interface{}); ok {
		if msg, ok := result["message"].(map[string]interface{}); ok {
			return extractPartsText(msg)
		}
		if artifactUpdate, ok := result["artifactUpdate"].(map[string]interface{}); ok {
			if artifact, ok := artifactUpdate["artifact"].(map[string]interface{}); ok {
				return extractPartsText(artifact)
			}
		}
	}
	return string(body)
}

func extractPartsText(msg map[string]interface{}) string {
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

func truncateString(s string, max int) string {
	if len(s) <= max {
		return s
	}
	if max <= 3 {
		return s[:max]
	}
	return s[:max-3] + "..."
}

// ===== Health check =====

type HealthHandler struct{}

func NewHealthHandler() *HealthHandler {
	return &HealthHandler{}
}

func (h *HealthHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	okJSON(w, map[string]string{"status": "ok"})
}

// ===== JSON response helpers (replace go-zero httpx) =====

func okJSON(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	json.NewEncoder(w).Encode(v)
}

func errHTTP(w http.ResponseWriter, err error) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(500)
	json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
}

// jsonError writes a structured JSON error response.
func jsonError(w http.ResponseWriter, message string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": message})
}

// Ensure sql.DB is imported (needed by some handlers indirectly)
var _ = sql.ErrNoRows
var _ = io.ReadAll
