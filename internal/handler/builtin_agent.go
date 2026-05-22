package handler

import (
	"encoding/json"
	"fmt"
	"net/http"

	"a2a-platform/internal/config"
	"a2a-platform/internal/svc"
)

type builtinAgentReq struct {
	Name          string             `json:"name"`
	Provider      string             `json:"provider"`
	BaseURL       string             `json:"base_url"`
	APIKey        string             `json:"api_key"`
	Model         string             `json:"model"`
	Description   string             `json:"description"`
	SystemPrompt  string             `json:"system_prompt"`
	MaxTokens     int                `json:"max_tokens"`
	MaxToolRounds int                `json:"max_tool_rounds"`
	MCPServers    []config.MCPServer `json:"mcp_servers"`
}

func (r builtinAgentReq) toConfig() config.BuiltinAgent {
	cfg := config.BuiltinAgent{
		Name:          r.Name,
		Provider:      r.Provider,
		BaseURL:       r.BaseURL,
		APIKey:        r.APIKey,
		Model:         r.Model,
		Description:   r.Description,
		SystemPrompt:  r.SystemPrompt,
		MaxTokens:     r.MaxTokens,
		MaxToolRounds: r.MaxToolRounds,
		MCPServers:    r.MCPServers,
	}
	if cfg.MaxTokens == 0 {
		cfg.MaxTokens = 4096
	}
	if cfg.MaxToolRounds == 0 {
		cfg.MaxToolRounds = 10
	}
	return cfg
}

type ListBuiltinAgentsHandler struct {
	svcCtx *svc.ServiceContext
}

func NewListBuiltinAgentsHandler(svcCtx *svc.ServiceContext) *ListBuiltinAgentsHandler {
	return &ListBuiltinAgentsHandler{svcCtx: svcCtx}
}

func (h *ListBuiltinAgentsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	agents := h.svcCtx.Engine.ListAgents()
	if agents == nil {
		agents = []config.BuiltinAgent{}
	}
	okJSON(w, agents)
}

type CreateBuiltinAgentHandler struct {
	svcCtx *svc.ServiceContext
}

func NewCreateBuiltinAgentHandler(svcCtx *svc.ServiceContext) *CreateBuiltinAgentHandler {
	return &CreateBuiltinAgentHandler{svcCtx: svcCtx}
}

func (h *CreateBuiltinAgentHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var req builtinAgentReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid JSON", 400)
		return
	}
	if req.Name == "" || req.Provider == "" || req.Model == "" {
		jsonError(w, "name, provider, and model are required", 400)
		return
	}

	cfg := req.toConfig()

	if existing := h.svcCtx.Engine.GetAgent(cfg.Name); existing != nil {
		h.svcCtx.Engine.RemoveAgent(cfg.Name)
	}
	if err := h.svcCtx.Engine.RegisterAgent(cfg); err != nil {
		jsonError(w, err.Error(), 400)
		return
	}

	// Persist after validation/registration succeeds. POST is intentionally
	// idempotent for an existing builtin agent name, matching the existing
	// API/e2e contract.
	existingCfg, err := h.svcCtx.BuiltinAgents.Get(cfg.Name)
	if err != nil {
		h.svcCtx.Engine.RemoveAgent(cfg.Name)
		jsonError(w, fmt.Sprintf("failed to load builtin agent: %v", err), 500)
		return
	}
	if existingCfg == nil {
		if _, err := h.svcCtx.BuiltinAgents.Create(cfg); err != nil {
			h.svcCtx.Engine.RemoveAgent(cfg.Name)
			jsonError(w, fmt.Sprintf("failed to save builtin agent: %v", err), 500)
			return
		}
	} else {
		if err := h.svcCtx.BuiltinAgents.Update(cfg); err != nil {
			h.svcCtx.Engine.RemoveAgent(cfg.Name)
			jsonError(w, fmt.Sprintf("failed to update builtin agent: %v", err), 500)
			return
		}
	}

	h.svcCtx.Registry.RegisterBuiltinAgent(cfg.Name, cfg.Description, nil)

	okJSON(w, map[string]interface{}{
		"ok":     true,
		"name":   cfg.Name,
		"status": "connected",
	})
}

type UpdateBuiltinAgentHandler struct {
	svcCtx *svc.ServiceContext
}

func NewUpdateBuiltinAgentHandler(svcCtx *svc.ServiceContext) *UpdateBuiltinAgentHandler {
	return &UpdateBuiltinAgentHandler{svcCtx: svcCtx}
}

func (h *UpdateBuiltinAgentHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	name := getPathParam(r, "name")
	if name == "" {
		jsonError(w, "missing agent name", 400)
		return
	}

	var req builtinAgentReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid JSON", 400)
		return
	}
	if req.Provider == "" || req.Model == "" {
		jsonError(w, "provider and model are required", 400)
		return
	}

	// Use the path name (can't change name via update)
	req.Name = name
	cfg := req.toConfig()

	// Preserve existing API key if not provided in the request
	if cfg.APIKey == "" {
		existing, err := h.svcCtx.BuiltinAgents.Get(name)
		if err == nil && existing != nil {
			cfg.APIKey = existing.APIKey
		}
	}

	// Persist to database
	if err := h.svcCtx.BuiltinAgents.Update(cfg); err != nil {
		jsonError(w, fmt.Sprintf("failed to update builtin agent: %v", err), 500)
		return
	}

	// Re-register in engine: remove old, register new
	h.svcCtx.Engine.RemoveAgent(name)
	if err := h.svcCtx.Engine.RegisterAgent(cfg); err != nil {
		jsonError(w, err.Error(), 400)
		return
	}

	h.svcCtx.Registry.RegisterBuiltinAgent(cfg.Name, cfg.Description, nil)

	okJSON(w, map[string]interface{}{
		"ok":     true,
		"name":   cfg.Name,
		"status": "updated",
	})
}

type DeleteBuiltinAgentHandler struct {
	svcCtx *svc.ServiceContext
}

func NewDeleteBuiltinAgentHandler(svcCtx *svc.ServiceContext) *DeleteBuiltinAgentHandler {
	return &DeleteBuiltinAgentHandler{svcCtx: svcCtx}
}

func (h *DeleteBuiltinAgentHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	name := getPathParam(r, "name")
	if name == "" {
		jsonError(w, "missing agent name", 400)
		return
	}

	h.svcCtx.Engine.RemoveAgent(name)
	h.svcCtx.Registry.DisconnectAgent(name)
	_ = h.svcCtx.Agents.Delete(name)
	_ = h.svcCtx.BuiltinAgents.Delete(name)

	w.WriteHeader(204)
}
