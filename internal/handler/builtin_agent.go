package handler

import (
	"encoding/json"
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

	h.svcCtx.Registry.RegisterBuiltinAgent(cfg.Name, cfg.Description, nil)

	okJSON(w, map[string]interface{}{
		"ok":     true,
		"name":   cfg.Name,
		"status": "connected",
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
	h.svcCtx.Agents.Delete(name)

	w.WriteHeader(204)
}
