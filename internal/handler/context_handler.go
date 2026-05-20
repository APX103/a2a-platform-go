package handler

import (
	"encoding/json"
	"fmt"
	"net/http"

	"a2a-platform/internal/model"
	"a2a-platform/internal/svc"
)

// ListContextsHandler lists contexts for an agent with pagination.
type ListContextsHandler struct {
	svcCtx *svc.ServiceContext
}

func NewListContextsHandler(svcCtx *svc.ServiceContext) *ListContextsHandler {
	return &ListContextsHandler{svcCtx: svcCtx}
}

func (h *ListContextsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	agentName := r.Header.Get("X-Path-Param-AgentName")
	if agentName == "" {
		jsonError(w, "missing agent name", 400)
		return
	}

	page := 1
	size := 20
	if p := r.URL.Query().Get("page"); p != "" {
		if n, err := parseInt(p); err == nil && n > 0 {
			page = n
		}
	}
	if s := r.URL.Query().Get("size"); s != "" {
		if n, err := parseInt(s); err == nil && n > 0 && n <= 100 {
			size = n
		}
	}

	contexts, total, err := h.svcCtx.Contexts.List(agentName, page, size)
	if err != nil {
		errHTTP(w, err)
		return
	}

	items := make([]model.ContextListItem, 0, len(contexts))
	for _, c := range contexts {
		items = append(items, model.ContextListItem{
			ID:           c.ID,
			AgentName:    c.AgentName,
			Title:        c.Title,
			MessageCount: c.MessageCount,
			CreatedAt:    c.CreatedAt.Format("2006-01-02T15:04:05Z"),
			UpdatedAt:    c.UpdatedAt.Format("2006-01-02T15:04:05Z"),
		})
	}

	okJSON(w, model.ListContextsResp{
		Items: items,
		Total: total,
		Page:  page,
		Size:  size,
	})
}

// GetContextHandler retrieves a context with its messages.
type GetContextHandler struct {
	svcCtx *svc.ServiceContext
}

func NewGetContextHandler(svcCtx *svc.ServiceContext) *GetContextHandler {
	return &GetContextHandler{svcCtx: svcCtx}
}

func (h *GetContextHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	id := r.Header.Get("X-Path-Param-Id")
	if id == "" {
		jsonError(w, "missing context id", 400)
		return
	}

	ctx, err := h.svcCtx.Contexts.Get(id)
	if err != nil || ctx == nil {
		jsonError(w, "context not found", 404)
		return
	}

	messages, err := h.svcCtx.Messages.GetByContext(id)
	if err != nil {
		messages = []*model.Message{}
	}

	msgValues := make([]model.Message, 0, len(messages))
	for _, m := range messages {
		msgValues = append(msgValues, *m)
	}

	okJSON(w, model.ContextDetailResp{
		Context:  ctx,
		Messages: msgValues,
	})
}

// CreateContextHandler creates a new context/session.
type CreateContextHandler struct {
	svcCtx *svc.ServiceContext
}

func NewCreateContextHandler(svcCtx *svc.ServiceContext) *CreateContextHandler {
	return &CreateContextHandler{svcCtx: svcCtx}
}

func (h *CreateContextHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, "method not allowed", 405)
		return
	}

	var req model.CreateContextReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid JSON", 400)
		return
	}

	if req.AgentName == "" {
		jsonError(w, "agent_name is required", 400)
		return
	}

	title := req.Title
	if title == "" {
		title = "New Chat"
	}

	ctx, err := h.svcCtx.Contexts.Create(req.AgentName, title)
	if err != nil {
		errHTTP(w, err)
		return
	}

	w.WriteHeader(http.StatusCreated)
	okJSON(w, ctx)
}

// DeleteContextHandler deletes a context.
type DeleteContextHandler struct {
	svcCtx *svc.ServiceContext
}

func NewDeleteContextHandler(svcCtx *svc.ServiceContext) *DeleteContextHandler {
	return &DeleteContextHandler{svcCtx: svcCtx}
}

func (h *DeleteContextHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	id := r.Header.Get("X-Path-Param-Id")
	if id == "" {
		jsonError(w, "missing context id", 400)
		return
	}

	err := h.svcCtx.Contexts.Delete(id)
	if err != nil {
		errHTTP(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// UpdateContextTitleHandler updates a context title.
type UpdateContextTitleHandler struct {
	svcCtx *svc.ServiceContext
}

func NewUpdateContextTitleHandler(svcCtx *svc.ServiceContext) *UpdateContextTitleHandler {
	return &UpdateContextTitleHandler{svcCtx: svcCtx}
}

func (h *UpdateContextTitleHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	id := r.Header.Get("X-Path-Param-Id")
	if id == "" {
		jsonError(w, "missing context id", 400)
		return
	}

	var req struct {
		Title string `json:"title"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid JSON", 400)
		return
	}

	if req.Title == "" {
		jsonError(w, "title is required", 400)
		return
	}

	err := h.svcCtx.Contexts.UpdateTitle(id, req.Title)
	if err != nil {
		errHTTP(w, err)
		return
	}

	ctx, err := h.svcCtx.Contexts.Get(id)
	if err != nil {
		errHTTP(w, err)
		return
	}

	okJSON(w, ctx)
}

// ListSubagentsHandler lists subagents for a context.
type ListSubagentsHandler struct {
	svcCtx *svc.ServiceContext
}

func NewListSubagentsHandler(svcCtx *svc.ServiceContext) *ListSubagentsHandler {
	return &ListSubagentsHandler{svcCtx: svcCtx}
}

func (h *ListSubagentsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	contextId := r.Header.Get("X-Path-Param-ContextId")
	if contextId == "" {
		jsonError(w, "missing context_id", 400)
		return
	}

	subagents, err := h.svcCtx.Subagents.ListByParent(contextId)
	if err != nil {
		errHTTP(w, err)
		return
	}

	// Ensure empty slice, not nil
	if subagents == nil {
		subagents = make([]*model.SubagentSession, 0)
	}

	okJSON(w, map[string]interface{}{
		"context_id": contextId,
		"subagents":  subagents,
	})
}

// GetSubagentHandler retrieves a subagent by ID.
type GetSubagentHandler struct {
	svcCtx *svc.ServiceContext
}

func NewGetSubagentHandler(svcCtx *svc.ServiceContext) *GetSubagentHandler {
	return &GetSubagentHandler{svcCtx: svcCtx}
}

func (h *GetSubagentHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	id := r.Header.Get("X-Path-Param-Id")
	if id == "" {
		jsonError(w, "missing subagent id", 400)
		return
	}

	subagent, err := h.svcCtx.Subagents.Get(id)
	if err != nil || subagent == nil {
		jsonError(w, "subagent not found", 404)
		return
	}

	okJSON(w, subagent)
}

// parseInt helper
func parseInt(s string) (int, error) {
	var n int
	_, err := fmt.Sscanf(s, "%d", &n)
	return n, err
}