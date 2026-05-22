package handler

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"a2a-platform/internal/model"
	"a2a-platform/internal/svc"
)

type groupReq struct {
	ID                string          `json:"id"`
	Name              string          `json:"name"`
	Description       string          `json:"description"`
	OrchestrationMode string          `json:"orchestration_mode"`
	Rules             json.RawMessage `json:"rules"`
	MemoryPolicy      json.RawMessage `json:"memory_policy"`
	Status            string          `json:"status"`
}

type memberReq struct {
	ActorType    string          `json:"actor_type"`
	ActorID      string          `json:"actor_id"`
	Role         string          `json:"role"`
	Capabilities json.RawMessage `json:"capabilities"`
	ClientID     string          `json:"client_id"`
}

type eventReq struct {
	EventType  string          `json:"event_type"`
	SenderType string          `json:"sender_type"`
	SenderID   string          `json:"sender_id"`
	Content    string          `json:"content"`
	Metadata   json.RawMessage `json:"metadata"`
}

type artifactReq struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	ArtifactType string `json:"artifact_type"`
	Content      string `json:"content"`
	Status       string `json:"status"`
	CreatedBy    string `json:"created_by"`
}

type groupEventResp struct {
	Event         *model.GroupEvent             `json:"event"`
	Orchestration model.GroupOrchestrationState `json:"orchestration"`
}

type GroupListHandler struct {
	svcCtx *svc.ServiceContext
}

func NewGroupListHandler(svcCtx *svc.ServiceContext) *GroupListHandler {
	return &GroupListHandler{svcCtx: svcCtx}
}

func (h *GroupListHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		status := r.URL.Query().Get("status")
		groups, err := h.svcCtx.Groups.List(status)
		if err != nil {
			errHTTP(w, err)
			return
		}
		if groups == nil {
			groups = []*model.Group{}
		}
		okJSON(w, groups)
	case http.MethodPost:
		var req groupReq
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			jsonError(w, "invalid JSON", 400)
			return
		}
		group := groupFromReq(req)
		if group.Name == "" {
			jsonError(w, "name is required", 400)
			return
		}
		if err := h.svcCtx.Groups.Create(group); err != nil {
			errHTTP(w, err)
			return
		}
		okJSON(w, group)
	default:
		jsonError(w, "method not allowed", 405)
	}
}

type GroupDetailHandler struct {
	svcCtx *svc.ServiceContext
}

func NewGroupDetailHandler(svcCtx *svc.ServiceContext) *GroupDetailHandler {
	return &GroupDetailHandler{svcCtx: svcCtx}
}

func (h *GroupDetailHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	groupID := getPathParam(r, "GroupId")
	group, err := h.svcCtx.Groups.Get(groupID)
	if err != nil {
		errHTTP(w, err)
		return
	}
	if group == nil {
		jsonError(w, "group not found", 404)
		return
	}

	switch r.Method {
	case http.MethodGet:
		okJSON(w, group)
	case http.MethodPut:
		var req groupReq
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			jsonError(w, "invalid JSON", 400)
			return
		}
		updated := groupFromReq(req)
		updated.ID = group.ID
		if updated.Name == "" {
			updated.Name = group.Name
		}
		if updated.OrchestrationMode == "" {
			updated.OrchestrationMode = group.OrchestrationMode
		}
		if updated.RulesJson == "" {
			updated.RulesJson = group.RulesJson
		}
		if updated.MemoryPolicyJson == "" {
			updated.MemoryPolicyJson = group.MemoryPolicyJson
		}
		if updated.Status == "" {
			updated.Status = group.Status
		}
		if err := h.svcCtx.Groups.Update(updated); err != nil {
			errHTTP(w, err)
			return
		}
		refreshed, err := h.svcCtx.Groups.Get(group.ID)
		if err != nil {
			errHTTP(w, err)
			return
		}
		okJSON(w, refreshed)
	case http.MethodDelete:
		if err := h.svcCtx.Groups.Archive(group.ID); err != nil {
			errHTTP(w, err)
			return
		}
		w.WriteHeader(204)
	default:
		jsonError(w, "method not allowed", 405)
	}
}

type GroupMemberHandler struct {
	svcCtx *svc.ServiceContext
}

func NewGroupMemberHandler(svcCtx *svc.ServiceContext) *GroupMemberHandler {
	return &GroupMemberHandler{svcCtx: svcCtx}
}

func (h *GroupMemberHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	group := h.requireGroup(w, r)
	if group == nil {
		return
	}
	switch r.Method {
	case http.MethodGet:
		h.listMembers(w, group.ID)
	case http.MethodPost:
		var req memberReq
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			jsonError(w, "invalid JSON", 400)
			return
		}
		member := memberFromReq(group.ID, req, false)
		if member.ActorID == "" {
			jsonError(w, "actor_id is required", 400)
			return
		}
		if err := h.svcCtx.GroupMembers.Upsert(member); err != nil {
			errHTTP(w, err)
			return
		}
		h.listMembers(w, group.ID)
	default:
		jsonError(w, "method not allowed", 405)
	}
}

func (h *GroupMemberHandler) requireGroup(w http.ResponseWriter, r *http.Request) *model.Group {
	groupID := getPathParam(r, "GroupId")
	group, err := h.svcCtx.Groups.Get(groupID)
	if err != nil {
		errHTTP(w, err)
		return nil
	}
	if group == nil {
		jsonError(w, "group not found", 404)
		return nil
	}
	return group
}

func (h *GroupMemberHandler) listMembers(w http.ResponseWriter, groupID string) {
	members, err := h.svcCtx.GroupMembers.List(groupID)
	if err != nil {
		errHTTP(w, err)
		return
	}
	if members == nil {
		members = []*model.GroupMember{}
	}
	okJSON(w, members)
}

type GroupJoinHandler struct {
	svcCtx *svc.ServiceContext
}

func NewGroupJoinHandler(svcCtx *svc.ServiceContext) *GroupJoinHandler {
	return &GroupJoinHandler{svcCtx: svcCtx}
}

func (h *GroupJoinHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, "method not allowed", 405)
		return
	}
	groupID := getPathParam(r, "GroupId")
	group, err := h.svcCtx.Groups.Get(groupID)
	if err != nil {
		errHTTP(w, err)
		return
	}
	if group == nil {
		jsonError(w, "group not found", 404)
		return
	}
	var req memberReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid JSON", 400)
		return
	}
	member := memberFromReq(group.ID, req, true)
	if member.ActorID == "" {
		jsonError(w, "client_id is required", 400)
		return
	}
	if err := h.svcCtx.GroupMembers.Upsert(member); err != nil {
		errHTTP(w, err)
		return
	}
	okJSON(w, member)
}

type GroupEventHandler struct {
	svcCtx *svc.ServiceContext
}

func NewGroupEventHandler(svcCtx *svc.ServiceContext) *GroupEventHandler {
	return &GroupEventHandler{svcCtx: svcCtx}
}

func (h *GroupEventHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	groupID := getPathParam(r, "GroupId")
	group, err := h.svcCtx.Groups.Get(groupID)
	if err != nil {
		errHTTP(w, err)
		return
	}
	if group == nil {
		jsonError(w, "group not found", 404)
		return
	}

	switch r.Method {
	case http.MethodGet:
		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		events, err := h.svcCtx.GroupEvents.List(group.ID, limit)
		if err != nil {
			errHTTP(w, err)
			return
		}
		if events == nil {
			events = []*model.GroupEvent{}
		}
		okJSON(w, events)
	case http.MethodPost:
		var req eventReq
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			jsonError(w, "invalid JSON", 400)
			return
		}
		event := &model.GroupEvent{
			GroupID:      group.ID,
			EventType:    req.EventType,
			SenderType:   req.SenderType,
			SenderID:     strings.TrimSpace(req.SenderID),
			Content:      req.Content,
			MetadataJson: rawJSONToString(req.Metadata),
		}
		if event.SenderID == "" {
			jsonError(w, "sender_id is required", 400)
			return
		}
		if err := h.svcCtx.GroupEvents.Append(event); err != nil {
			errHTTP(w, err)
			return
		}
		members, err := h.svcCtx.GroupMembers.List(group.ID)
		if err != nil {
			errHTTP(w, err)
			return
		}
		okJSON(w, groupEventResp{
			Event:         event,
			Orchestration: svc.BuildGroupOrchestrationState(group, members),
		})
	default:
		jsonError(w, "method not allowed", 405)
	}
}

type GroupArtifactHandler struct {
	svcCtx *svc.ServiceContext
}

func NewGroupArtifactHandler(svcCtx *svc.ServiceContext) *GroupArtifactHandler {
	return &GroupArtifactHandler{svcCtx: svcCtx}
}

func (h *GroupArtifactHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	groupID := getPathParam(r, "GroupId")
	group, err := h.svcCtx.Groups.Get(groupID)
	if err != nil {
		errHTTP(w, err)
		return
	}
	if group == nil {
		jsonError(w, "group not found", 404)
		return
	}

	switch r.Method {
	case http.MethodGet:
		artifacts, err := h.svcCtx.GroupArtifacts.List(group.ID)
		if err != nil {
			errHTTP(w, err)
			return
		}
		if artifacts == nil {
			artifacts = []*model.GroupArtifact{}
		}
		okJSON(w, artifacts)
	case http.MethodPost:
		var req artifactReq
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			jsonError(w, "invalid JSON", 400)
			return
		}
		artifact := artifactFromReq(group.ID, req)
		if artifact.Name == "" {
			jsonError(w, "name is required", 400)
			return
		}
		if err := h.svcCtx.GroupArtifacts.Create(artifact); err != nil {
			errHTTP(w, err)
			return
		}
		okJSON(w, artifact)
	default:
		jsonError(w, "method not allowed", 405)
	}
}

type GroupArtifactDetailHandler struct {
	svcCtx *svc.ServiceContext
}

func NewGroupArtifactDetailHandler(svcCtx *svc.ServiceContext) *GroupArtifactDetailHandler {
	return &GroupArtifactDetailHandler{svcCtx: svcCtx}
}

func (h *GroupArtifactDetailHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	groupID := getPathParam(r, "GroupId")
	artifactID := getPathParam(r, "ArtifactId")
	artifact, err := h.svcCtx.GroupArtifacts.Get(artifactID)
	if err != nil {
		errHTTP(w, err)
		return
	}
	if artifact == nil || artifact.GroupID != groupID {
		jsonError(w, "artifact not found", 404)
		return
	}
	switch r.Method {
	case http.MethodGet:
		okJSON(w, artifact)
	case http.MethodPut:
		var req artifactReq
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			jsonError(w, "invalid JSON", 400)
			return
		}
		updated := artifactFromReq(groupID, req)
		updated.ID = artifact.ID
		if updated.Name == "" {
			updated.Name = artifact.Name
		}
		if updated.ArtifactType == "" {
			updated.ArtifactType = artifact.ArtifactType
		}
		if updated.Status == "" {
			updated.Status = artifact.Status
		}
		if err := h.svcCtx.GroupArtifacts.Update(updated); err != nil {
			errHTTP(w, err)
			return
		}
		refreshed, err := h.svcCtx.GroupArtifacts.Get(artifact.ID)
		if err != nil {
			errHTTP(w, err)
			return
		}
		okJSON(w, refreshed)
	default:
		jsonError(w, "method not allowed", 405)
	}
}

type GroupOrchestrationHandler struct {
	svcCtx *svc.ServiceContext
}

func NewGroupOrchestrationHandler(svcCtx *svc.ServiceContext) *GroupOrchestrationHandler {
	return &GroupOrchestrationHandler{svcCtx: svcCtx}
}

func (h *GroupOrchestrationHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		jsonError(w, "method not allowed", 405)
		return
	}
	groupID := getPathParam(r, "GroupId")
	group, err := h.svcCtx.Groups.Get(groupID)
	if err != nil {
		errHTTP(w, err)
		return
	}
	if group == nil {
		jsonError(w, "group not found", 404)
		return
	}
	members, err := h.svcCtx.GroupMembers.List(group.ID)
	if err != nil {
		errHTTP(w, err)
		return
	}
	okJSON(w, svc.BuildGroupOrchestrationState(group, members))
}

func groupFromReq(req groupReq) *model.Group {
	return &model.Group{
		ID:                strings.TrimSpace(req.ID),
		Name:              strings.TrimSpace(req.Name),
		Description:       req.Description,
		OrchestrationMode: req.OrchestrationMode,
		RulesJson:         rawJSONToString(req.Rules),
		MemoryPolicyJson:  rawJSONToString(req.MemoryPolicy),
		Status:            req.Status,
	}
}

func memberFromReq(groupID string, req memberReq, humanJoin bool) *model.GroupMember {
	actorType := req.ActorType
	actorID := req.ActorID
	role := req.Role
	if humanJoin {
		actorType = model.GroupActorHuman
		actorID = req.ClientID
		if actorID == "" {
			actorID = req.ActorID
		}
		if role == "" {
			role = "member"
		}
	}
	return &model.GroupMember{
		GroupID:          groupID,
		ActorType:        actorType,
		ActorID:          strings.TrimSpace(actorID),
		Role:             role,
		CapabilitiesJson: rawJSONToString(req.Capabilities),
	}
}

func artifactFromReq(groupID string, req artifactReq) *model.GroupArtifact {
	return &model.GroupArtifact{
		ID:           strings.TrimSpace(req.ID),
		GroupID:      groupID,
		Name:         strings.TrimSpace(req.Name),
		ArtifactType: req.ArtifactType,
		Content:      req.Content,
		Status:       req.Status,
		CreatedBy:    strings.TrimSpace(req.CreatedBy),
	}
}

func rawJSONToString(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	return string(raw)
}
