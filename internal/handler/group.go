package handler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"time"

	"a2a-platform/internal/model"
	"a2a-platform/internal/svc"
)

const (
	authPrincipalHeader = "X-A2A-Principal"
	authGroupIDHeader   = "X-A2A-Group-ID"
	authActorTypeHeader = "X-A2A-Actor-Type"
	authActorIDHeader   = "X-A2A-Actor-ID"
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

type inviteReq struct {
	ActorTypeAllowed string `json:"actor_type_allowed"`
	Role             string `json:"role"`
	MaxUses          int    `json:"max_uses"`
	ExpiresAt        string `json:"expires_at"`
}

type inviteResp struct {
	*model.GroupInvite
	Token string `json:"token,omitempty"`
}

type groupJoinReq struct {
	InviteToken  string          `json:"invite_token"`
	ActorType    string          `json:"actor_type"`
	ActorID      string          `json:"actor_id"`
	ClientID     string          `json:"client_id"`
	Capabilities json.RawMessage `json:"capabilities"`
}

type groupJoinResp struct {
	Group         *model.Group                  `json:"group"`
	Member        *model.GroupMember            `json:"member"`
	AccessToken   string                        `json:"access_token"`
	Orchestration model.GroupOrchestrationState `json:"orchestration"`
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
	Triggered     []*model.GroupEvent           `json:"triggered,omitempty"`
}

type groupStreamWriter struct {
	header   http.Header
	status   int
	buffer   strings.Builder
	raw      strings.Builder
	onFrame  func(map[string]interface{})
	text     strings.Builder
	thinking strings.Builder
}

func newGroupStreamWriter(onFrame func(map[string]interface{})) *groupStreamWriter {
	return &groupStreamWriter{
		header:  make(http.Header),
		status:  http.StatusOK,
		onFrame: onFrame,
	}
}

func (w *groupStreamWriter) Header() http.Header {
	return w.header
}

func (w *groupStreamWriter) WriteHeader(statusCode int) {
	w.status = statusCode
}

func (w *groupStreamWriter) Flush() {}

func (w *groupStreamWriter) Write(p []byte) (int, error) {
	chunk := string(p)
	w.raw.WriteString(chunk)
	w.buffer.WriteString(chunk)
	for {
		buffer := w.buffer.String()
		idx := strings.Index(buffer, "\n\n")
		if idx < 0 {
			break
		}
		frame := buffer[:idx]
		w.buffer.Reset()
		w.buffer.WriteString(buffer[idx+2:])
		w.handleFrame(frame)
	}
	return len(p), nil
}

func (w *groupStreamWriter) handleFrame(frame string) {
	dataLines := []string{}
	for _, line := range strings.Split(frame, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "data:") {
			dataLines = append(dataLines, strings.TrimSpace(strings.TrimPrefix(line, "data:")))
		}
	}
	if len(dataLines) == 0 {
		return
	}
	data := strings.Join(dataLines, "\n")
	var evt map[string]interface{}
	if json.Unmarshal([]byte(data), &evt) != nil {
		return
	}
	switch evt["type"] {
	case "text.delta":
		if text, ok := evt["text"].(string); ok {
			w.text.WriteString(text)
		}
	case "thinking.delta":
		if thinking, ok := evt["thinking"].(string); ok {
			w.thinking.WriteString(thinking)
		}
	}
	if w.onFrame != nil {
		w.onFrame(evt)
	}
}

func responseWriterCode(w http.ResponseWriter) int {
	switch writer := w.(type) {
	case *httptest.ResponseRecorder:
		return writer.Code
	case *groupStreamWriter:
		return writer.status
	default:
		return http.StatusOK
	}
}

func responseWriterBody(w http.ResponseWriter) string {
	switch writer := w.(type) {
	case *httptest.ResponseRecorder:
		return writer.Body.String()
	case *groupStreamWriter:
		return writer.raw.String()
	default:
		return ""
	}
}

type groupRules struct {
	MaxSpeakers  int    `json:"max_speakers"`
	MaxRounds    int    `json:"max_rounds"`
	AutoArtifact *bool  `json:"auto_artifact"`
	ArtifactName string `json:"artifact_name"`
}

type groupMemoryPolicy struct {
	HotMessages int  `json:"hot_messages"`
	Summary     bool `json:"summary"`
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
		var groups []*model.Group
		var err error
		if isMemberPrincipal(r) {
			groups, err = h.svcCtx.Groups.ListByActor(r.Header.Get(authActorTypeHeader), r.Header.Get(authActorIDHeader), status)
		} else {
			groups, err = h.svcCtx.Groups.List(status)
		}
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

type GroupInviteHandler struct {
	svcCtx *svc.ServiceContext
}

func NewGroupInviteHandler(svcCtx *svc.ServiceContext) *GroupInviteHandler {
	return &GroupInviteHandler{svcCtx: svcCtx}
}

func (h *GroupInviteHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
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
		invites, err := h.svcCtx.GroupInvites.List(group.ID)
		if err != nil {
			errHTTP(w, err)
			return
		}
		if invites == nil {
			invites = []*model.GroupInvite{}
		}
		okJSON(w, invites)
	case http.MethodPost:
		var req inviteReq
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			jsonError(w, "invalid JSON", 400)
			return
		}
		invite := &model.GroupInvite{
			GroupID:          group.ID,
			ActorTypeAllowed: req.ActorTypeAllowed,
			Role:             req.Role,
			MaxUses:          req.MaxUses,
			Status:           model.GroupStatusActive,
		}
		if req.ExpiresAt != "" {
			expiresAt, err := time.Parse(time.RFC3339, req.ExpiresAt)
			if err != nil {
				jsonError(w, "expires_at must be RFC3339", 400)
				return
			}
			invite.ExpiresAt = &expiresAt
		}
		token, err := h.svcCtx.GroupInvites.Create(invite)
		if err != nil {
			errHTTP(w, err)
			return
		}
		okJSON(w, inviteResp{GroupInvite: invite, Token: token})
	default:
		jsonError(w, "method not allowed", 405)
	}
}

type GroupJoinByInviteHandler struct {
	svcCtx *svc.ServiceContext
}

func NewGroupJoinByInviteHandler(svcCtx *svc.ServiceContext) *GroupJoinByInviteHandler {
	return &GroupJoinByInviteHandler{svcCtx: svcCtx}
}

func (h *GroupJoinByInviteHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, "method not allowed", 405)
		return
	}
	var req groupJoinReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid JSON", 400)
		return
	}
	inviteToken := strings.TrimSpace(req.InviteToken)
	if inviteToken == "" {
		jsonError(w, "invite_token is required", 400)
		return
	}
	actorType := req.ActorType
	if actorType == "" && req.ClientID != "" {
		actorType = model.GroupActorHuman
	}
	actorType = svc.NormalizeActorType(actorType)
	actorID := strings.TrimSpace(req.ActorID)
	if actorID == "" {
		actorID = strings.TrimSpace(req.ClientID)
	}
	if actorID == "" {
		jsonError(w, "actor_id is required", 400)
		return
	}
	invite, err := h.svcCtx.GroupInvites.GetByToken(inviteToken)
	if err != nil {
		errHTTP(w, err)
		return
	}
	if !svc.InviteUsable(invite, actorType, time.Now()) {
		jsonError(w, "invalid invite token", 403)
		return
	}
	group, err := h.svcCtx.Groups.Get(invite.GroupID)
	if err != nil {
		errHTTP(w, err)
		return
	}
	if group == nil || group.Status != model.GroupStatusActive {
		jsonError(w, "group is not joinable", 403)
		return
	}
	member := &model.GroupMember{
		GroupID:          group.ID,
		ActorType:        actorType,
		ActorID:          actorID,
		Role:             invite.Role,
		CapabilitiesJson: rawJSONToString(req.Capabilities),
	}
	if err := h.svcCtx.GroupMembers.Upsert(member); err != nil {
		errHTTP(w, err)
		return
	}
	if err := h.svcCtx.GroupInvites.Consume(invite.ID); err != nil {
		errHTTP(w, err)
		return
	}
	memberToken := &model.GroupMemberToken{GroupID: group.ID, ActorType: actorType, ActorID: actorID}
	accessToken, err := h.svcCtx.GroupTokens.Create(memberToken)
	if err != nil {
		errHTTP(w, err)
		return
	}
	members, err := h.svcCtx.GroupMembers.List(group.ID)
	if err != nil {
		errHTTP(w, err)
		return
	}
	okJSON(w, groupJoinResp{
		Group:         group,
		Member:        member,
		AccessToken:   accessToken,
		Orchestration: svc.BuildGroupOrchestrationState(group, members),
	})
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
		if isMemberPrincipal(r) && (event.SenderType != r.Header.Get(authActorTypeHeader) || event.SenderID != r.Header.Get(authActorIDHeader)) {
			jsonError(w, "sender does not match access token", 403)
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
		if wantsEventStream(r) {
			h.streamGroupEventResponse(w, r, group, event, members)
			return
		}
		triggered, err := h.maybeRunGroupTurn(r, group, event, members)
		if err != nil {
			errHTTP(w, err)
			return
		}
		okJSON(w, groupEventResp{
			Event:         event,
			Orchestration: svc.BuildGroupOrchestrationState(group, members),
			Triggered:     triggered,
		})
	default:
		jsonError(w, "method not allowed", 405)
	}
}

func wantsEventStream(r *http.Request) bool {
	return strings.Contains(strings.ToLower(r.Header.Get("Accept")), "text/event-stream")
}

func (h *GroupEventHandler) streamGroupEventResponse(w http.ResponseWriter, r *http.Request, group *model.Group, event *model.GroupEvent, members []*model.GroupMember) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		jsonError(w, "streaming not supported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	writeGroupSSE(w, flusher, "group.event", map[string]interface{}{"event": event})
	triggered, err := h.streamGroupTurn(w, flusher, r, group, event, members)
	if err != nil {
		writeGroupSSE(w, flusher, "group.error", map[string]interface{}{"error": err.Error()})
		return
	}
	writeGroupSSE(w, flusher, "group.done", map[string]interface{}{
		"event":         event,
		"orchestration": svc.BuildGroupOrchestrationState(group, members),
		"triggered":     triggered,
	})
}

func writeGroupSSE(w io.Writer, flusher http.Flusher, event string, data map[string]interface{}) {
	payload := make(map[string]interface{}, len(data)+1)
	for k, v := range data {
		payload[k] = v
	}
	payload["type"] = event
	jsonData, err := json.Marshal(payload)
	if err != nil {
		return
	}
	fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, string(jsonData))
	flusher.Flush()
}

func (h *GroupEventHandler) maybeRunGroupTurn(r *http.Request, group *model.Group, event *model.GroupEvent, members []*model.GroupMember) ([]*model.GroupEvent, error) {
	switch group.OrchestrationMode {
	case model.GroupModeFreeChat:
		return h.maybeRunFreeChatTurn(r, group, event, members)
	default:
		return h.maybeRunLeaderTurn(r, group, event, members)
	}
}

func (h *GroupEventHandler) streamGroupTurn(w io.Writer, flusher http.Flusher, r *http.Request, group *model.Group, event *model.GroupEvent, members []*model.GroupMember) ([]*model.GroupEvent, error) {
	switch group.OrchestrationMode {
	case model.GroupModeFreeChat:
		return h.streamFreeChatTurn(w, flusher, r, group, event, members)
	default:
		return h.streamLeaderTurn(w, flusher, r, group, event, members)
	}
}

func (h *GroupEventHandler) streamLeaderTurn(w io.Writer, flusher http.Flusher, r *http.Request, group *model.Group, event *model.GroupEvent, members []*model.GroupMember) ([]*model.GroupEvent, error) {
	if h == nil || h.svcCtx == nil || group == nil || event == nil {
		return nil, nil
	}
	if group.OrchestrationMode != model.GroupModeLeaderLed || event.EventType != "message" || event.SenderType == model.GroupActorAgent {
		return nil, nil
	}
	leader := selectGroupLeader(members)
	if leader == "" || !h.canInvokeAgent(leader) {
		return nil, nil
	}
	prompt := h.buildLeaderPrompt(group, event, members)
	text, statusCode, rawBody := h.invokeGroupAgentStreaming(r, group, leader, event, prompt, "leader", func(delta string) {
		writeGroupSSE(w, flusher, "group.agent_delta", map[string]interface{}{"sender_id": leader, "sender_type": model.GroupActorAgent, "text": delta})
	}, func(thinking string) {
		writeGroupSSE(w, flusher, "group.agent_thinking", map[string]interface{}{"sender_id": leader, "sender_type": model.GroupActorAgent, "thinking": thinking})
	})
	if statusCode >= 400 {
		text = strings.TrimSpace(rawBody)
		if text == "" {
			text = fmt.Sprintf("leader %s failed with status %d", leader, statusCode)
		}
		return h.appendSystemGroupEvent(group.ID, text, event.ID, group.OrchestrationMode)
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return nil, nil
	}
	metadata, _ := json.Marshal(map[string]interface{}{
		"trigger_event_id": event.ID,
		"orchestration":    model.GroupModeLeaderLed,
		"role":             "leader",
	})
	leaderEvent := &model.GroupEvent{
		GroupID:      group.ID,
		EventType:    "message",
		SenderType:   model.GroupActorAgent,
		SenderID:     leader,
		Content:      text,
		MetadataJson: string(metadata),
	}
	if err := h.svcCtx.GroupEvents.Append(leaderEvent); err != nil {
		return nil, err
	}
	writeGroupSSE(w, flusher, "group.event", map[string]interface{}{"event": leaderEvent})
	if artifact, err := h.upsertAutoArtifact(group, event, []*model.GroupEvent{leaderEvent}); err != nil {
		return nil, err
	} else if artifact != nil {
		writeGroupSSE(w, flusher, "group.artifact", map[string]interface{}{"artifact": artifact})
	}
	return []*model.GroupEvent{leaderEvent}, nil
}

func (h *GroupEventHandler) streamFreeChatTurn(w io.Writer, flusher http.Flusher, r *http.Request, group *model.Group, event *model.GroupEvent, members []*model.GroupMember) ([]*model.GroupEvent, error) {
	if h == nil || h.svcCtx == nil || group == nil || event == nil {
		return nil, nil
	}
	if event.EventType != "message" || event.SenderType == model.GroupActorSystem {
		return nil, nil
	}
	rules := parseGroupRules(group.RulesJson)
	maxSpeakers := rules.MaxSpeakers
	if maxSpeakers <= 0 {
		maxSpeakers = 3
	}
	if maxSpeakers > 8 {
		maxSpeakers = 8
	}
	candidates := freeChatCandidates(members, event.SenderType, event.SenderID)
	if len(candidates) == 0 {
		return nil, nil
	}
	recentEvents, err := h.svcCtx.GroupEvents.List(group.ID, freeChatHotMessageLimit(group.MemoryPolicyJson))
	if err != nil {
		return nil, err
	}

	triggered := []*model.GroupEvent{}
	for _, agentName := range candidates {
		if len(triggered) >= maxSpeakers {
			break
		}
		if !h.canInvokeAgent(agentName) {
			continue
		}
		writeGroupSSE(w, flusher, "group.agent_start", map[string]interface{}{"sender_id": agentName, "sender_type": model.GroupActorAgent})
		prompt := h.buildFreeChatPrompt(group, event, members, recentEvents, agentName)
		text, statusCode, rawBody := h.invokeGroupAgentStreaming(r, group, agentName, event, prompt, "free_chat", func(delta string) {
			writeGroupSSE(w, flusher, "group.agent_delta", map[string]interface{}{"sender_id": agentName, "sender_type": model.GroupActorAgent, "text": delta})
		}, func(thinking string) {
			writeGroupSSE(w, flusher, "group.agent_thinking", map[string]interface{}{"sender_id": agentName, "sender_type": model.GroupActorAgent, "thinking": thinking})
		})
		if statusCode >= 400 {
			text = strings.TrimSpace(rawBody)
			if text == "" {
				text = fmt.Sprintf("agent %s failed with status %d", agentName, statusCode)
			}
			systemEvents, err := h.appendSystemGroupEvent(group.ID, text, event.ID, group.OrchestrationMode)
			if err != nil {
				return nil, err
			}
			for _, systemEvent := range systemEvents {
				writeGroupSSE(w, flusher, "group.event", map[string]interface{}{"event": systemEvent})
			}
			triggered = append(triggered, systemEvents...)
			continue
		}
		text = strings.TrimSpace(text)
		if text == "" || isNoReply(text) {
			writeGroupSSE(w, flusher, "group.agent_skip", map[string]interface{}{"sender_id": agentName, "sender_type": model.GroupActorAgent})
			continue
		}
		metadata, _ := json.Marshal(map[string]interface{}{
			"trigger_event_id": event.ID,
			"orchestration":    model.GroupModeFreeChat,
			"role":             "participant",
		})
		agentEvent := &model.GroupEvent{
			GroupID:      group.ID,
			EventType:    "message",
			SenderType:   model.GroupActorAgent,
			SenderID:     agentName,
			Content:      text,
			MetadataJson: string(metadata),
		}
		if err := h.svcCtx.GroupEvents.Append(agentEvent); err != nil {
			return nil, err
		}
		writeGroupSSE(w, flusher, "group.event", map[string]interface{}{"event": agentEvent})
		triggered = append(triggered, agentEvent)
	}
	if artifact, err := h.upsertAutoArtifact(group, event, triggered); err != nil {
		return nil, err
	} else if artifact != nil {
		writeGroupSSE(w, flusher, "group.artifact", map[string]interface{}{"artifact": artifact})
	}
	return triggered, nil
}

func (h *GroupEventHandler) maybeRunLeaderTurn(r *http.Request, group *model.Group, event *model.GroupEvent, members []*model.GroupMember) ([]*model.GroupEvent, error) {
	if h == nil || h.svcCtx == nil || group == nil || event == nil {
		return nil, nil
	}
	if group.OrchestrationMode != model.GroupModeLeaderLed || event.EventType != "message" || event.SenderType == model.GroupActorAgent {
		return nil, nil
	}
	leader := selectGroupLeader(members)
	if leader == "" || !h.canInvokeAgent(leader) {
		return nil, nil
	}

	prompt := h.buildLeaderPrompt(group, event, members)
	text, statusCode, rawBody := h.invokeGroupAgent(r, group, leader, event, prompt, "leader")
	if statusCode >= 400 {
		text = strings.TrimSpace(rawBody)
		if text == "" {
			text = fmt.Sprintf("leader %s failed with status %d", leader, statusCode)
		}
		return h.appendSystemGroupEvent(group.ID, text, event.ID, group.OrchestrationMode)
	}
	if strings.TrimSpace(text) == "" {
		return nil, nil
	}

	metadata, _ := json.Marshal(map[string]interface{}{
		"trigger_event_id": event.ID,
		"orchestration":    model.GroupModeLeaderLed,
		"role":             "leader",
	})
	leaderEvent := &model.GroupEvent{
		GroupID:      group.ID,
		EventType:    "message",
		SenderType:   model.GroupActorAgent,
		SenderID:     leader,
		Content:      text,
		MetadataJson: string(metadata),
	}
	if err := h.svcCtx.GroupEvents.Append(leaderEvent); err != nil {
		return nil, err
	}
	if _, err := h.upsertAutoArtifact(group, event, []*model.GroupEvent{leaderEvent}); err != nil {
		return nil, err
	}
	return []*model.GroupEvent{leaderEvent}, nil
}

func (h *GroupEventHandler) maybeRunFreeChatTurn(r *http.Request, group *model.Group, event *model.GroupEvent, members []*model.GroupMember) ([]*model.GroupEvent, error) {
	if h == nil || h.svcCtx == nil || group == nil || event == nil {
		return nil, nil
	}
	if event.EventType != "message" || event.SenderType == model.GroupActorSystem {
		return nil, nil
	}
	rules := parseGroupRules(group.RulesJson)
	maxSpeakers := rules.MaxSpeakers
	if maxSpeakers <= 0 {
		maxSpeakers = 3
	}
	if maxSpeakers > 8 {
		maxSpeakers = 8
	}
	candidates := freeChatCandidates(members, event.SenderType, event.SenderID)
	if len(candidates) == 0 {
		return nil, nil
	}
	recentEvents, err := h.svcCtx.GroupEvents.List(group.ID, freeChatHotMessageLimit(group.MemoryPolicyJson))
	if err != nil {
		return nil, err
	}

	triggered := []*model.GroupEvent{}
	for _, agentName := range candidates {
		if len(triggered) >= maxSpeakers {
			break
		}
		if !h.canInvokeAgent(agentName) {
			continue
		}
		prompt := h.buildFreeChatPrompt(group, event, members, recentEvents, agentName)
		text, statusCode, rawBody := h.invokeGroupAgent(r, group, agentName, event, prompt, "free_chat")
		if statusCode >= 400 {
			text = strings.TrimSpace(rawBody)
			if text == "" {
				text = fmt.Sprintf("agent %s failed with status %d", agentName, statusCode)
			}
			systemEvents, err := h.appendSystemGroupEvent(group.ID, text, event.ID, group.OrchestrationMode)
			if err != nil {
				return nil, err
			}
			triggered = append(triggered, systemEvents...)
			continue
		}
		text = strings.TrimSpace(text)
		if text == "" || isNoReply(text) {
			continue
		}
		metadata, _ := json.Marshal(map[string]interface{}{
			"trigger_event_id": event.ID,
			"orchestration":    model.GroupModeFreeChat,
			"role":             "participant",
		})
		agentEvent := &model.GroupEvent{
			GroupID:      group.ID,
			EventType:    "message",
			SenderType:   model.GroupActorAgent,
			SenderID:     agentName,
			Content:      text,
			MetadataJson: string(metadata),
		}
		if err := h.svcCtx.GroupEvents.Append(agentEvent); err != nil {
			return nil, err
		}
		triggered = append(triggered, agentEvent)
	}
	if _, err := h.upsertAutoArtifact(group, event, triggered); err != nil {
		return nil, err
	}
	return triggered, nil
}

func (h *GroupEventHandler) upsertAutoArtifact(group *model.Group, trigger *model.GroupEvent, triggered []*model.GroupEvent) (*model.GroupArtifact, error) {
	if h == nil || h.svcCtx == nil || h.svcCtx.GroupArtifacts == nil || group == nil || trigger == nil {
		return nil, nil
	}
	if !autoArtifactEnabled(group) {
		return nil, nil
	}
	agentEvents := make([]*model.GroupEvent, 0, len(triggered))
	for _, event := range triggered {
		if event == nil || event.EventType != "message" || event.SenderType != model.GroupActorAgent || strings.TrimSpace(event.Content) == "" {
			continue
		}
		agentEvents = append(agentEvents, event)
	}
	if len(agentEvents) == 0 {
		return nil, nil
	}
	artifact := &model.GroupArtifact{
		GroupID:      group.ID,
		Name:         autoArtifactName(group),
		ArtifactType: "document",
		Content:      buildAutoArtifactContent(group, trigger, agentEvents),
		Status:       "updated",
		CreatedBy:    "platform",
	}
	if err := h.svcCtx.GroupArtifacts.UpsertByName(artifact); err != nil {
		return nil, err
	}
	return artifact, nil
}

func autoArtifactEnabled(group *model.Group) bool {
	if group == nil {
		return false
	}
	rules := parseGroupRules(group.RulesJson)
	if rules.AutoArtifact != nil {
		return *rules.AutoArtifact
	}
	return group.OrchestrationMode == model.GroupModeLeaderLed ||
		group.OrchestrationMode == model.GroupModeFreeChat ||
		group.OrchestrationMode == model.GroupModeResearchLongRun
}

func autoArtifactName(group *model.Group) string {
	if group == nil {
		return "group-output.md"
	}
	rules := parseGroupRules(group.RulesJson)
	if strings.TrimSpace(rules.ArtifactName) != "" {
		return strings.TrimSpace(rules.ArtifactName)
	}
	switch group.OrchestrationMode {
	case model.GroupModeLeaderLed:
		return "leader-summary.md"
	case model.GroupModeFreeChat:
		return "group-discussion.md"
	case model.GroupModeResearchLongRun:
		return "research-checkpoint.md"
	default:
		return "group-output.md"
	}
}

func buildAutoArtifactContent(group *model.Group, trigger *model.GroupEvent, agentEvents []*model.GroupEvent) string {
	var b strings.Builder
	title := "Group Output"
	if group != nil && strings.TrimSpace(group.Name) != "" {
		title = group.Name
	}
	b.WriteString("# " + title + "\n\n")
	b.WriteString("_Automatically generated by A2A Platform group orchestration._\n\n")
	b.WriteString("Updated: " + time.Now().Format(time.RFC3339) + "\n\n")
	if group != nil {
		b.WriteString("Mode: `" + group.OrchestrationMode + "`\n\n")
	}
	if trigger != nil {
		b.WriteString("## Latest Human Request\n\n")
		b.WriteString(fmt.Sprintf("From `%s:%s` at %s:\n\n", trigger.SenderType, trigger.SenderID, trigger.CreatedAt.Format(time.RFC3339)))
		b.WriteString(strings.TrimSpace(trigger.Content) + "\n\n")
	}
	b.WriteString("## Agent Outputs\n\n")
	for _, event := range agentEvents {
		if event == nil {
			continue
		}
		b.WriteString("### " + event.SenderID + "\n\n")
		b.WriteString(strings.TrimSpace(event.Content) + "\n\n")
	}
	return b.String()
}

func (h *GroupEventHandler) canInvokeAgent(name string) bool {
	if h.svcCtx.Tasks == nil || h.svcCtx.Messages == nil || h.svcCtx.Traces == nil {
		return false
	}
	if h.svcCtx.Engine != nil && h.svcCtx.Engine.GetAgent(name) != nil {
		return true
	}
	if h.svcCtx.Registry != nil && h.svcCtx.Registry.GetClient(name) != nil {
		return true
	}
	if h.svcCtx.BridgeRegistry != nil && h.svcCtx.BridgeRegistry.Get(name) != nil {
		return true
	}
	return false
}

func (h *GroupEventHandler) invokeGroupAgent(r *http.Request, group *model.Group, agentName string, event *model.GroupEvent, prompt, role string) (string, int, string) {
	return h.invokeGroupAgentWithWriter(r, group, agentName, event, prompt, role, httptest.NewRecorder())
}

func (h *GroupEventHandler) invokeGroupAgentStreaming(r *http.Request, group *model.Group, agentName string, event *model.GroupEvent, prompt, role string, onDelta func(string), onThinking func(string)) (string, int, string) {
	writer := newGroupStreamWriter(func(evt map[string]interface{}) {
		switch evt["type"] {
		case "text.delta":
			if text, ok := evt["text"].(string); ok && text != "" && onDelta != nil {
				onDelta(text)
			}
		case "thinking.delta":
			if thinking, ok := evt["thinking"].(string); ok && thinking != "" && onThinking != nil {
				onThinking(thinking)
			}
		}
	})
	return h.invokeGroupAgentWithWriter(r, group, agentName, event, prompt, role, writer)
}

func (h *GroupEventHandler) invokeGroupAgentWithWriter(r *http.Request, group *model.Group, agentName string, event *model.GroupEvent, prompt, role string, writer http.ResponseWriter) (string, int, string) {
	body, _ := json.Marshal(map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      fmt.Sprintf("group-event-%d-%s", event.ID, agentName),
		"method":  "SendStreamingMessage",
		"params": map[string]interface{}{
			"contextId":     groupContextID(group.ID),
			"rootContextId": groupContextID(group.ID),
			"message": map[string]interface{}{
				"role":  "ROLE_USER",
				"parts": []map[string]string{{"text": prompt}},
			},
		},
	})

	req := httptest.NewRequest(http.MethodPost, "/agent/"+agentName, bytes.NewReader(body))
	req = req.WithContext(r.Context())
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("X-Path-Param-Name", agentName)
	req.Header.Set("X-A2A-Source-Agent", event.SenderID)
	req.Header.Set("X-A2A-Group-ID", group.ID)
	req.Header.Set("X-A2A-Root-Context-Id", groupContextID(group.ID))
	if role != "" {
		req.Header.Set("X-A2A-Group-Role", role)
	}

	NewAgentProxyHandler(h.svcCtx).ServeHTTP(writer, req)
	rawBody := responseWriterBody(writer)
	text := extractGroupAgentResponseText(rawBody)
	if streamWriter, ok := writer.(*groupStreamWriter); ok {
		if streamed := strings.TrimSpace(streamWriter.text.String()); streamed != "" {
			text = streamed
		}
	}
	return text, responseWriterCode(writer), rawBody
}

func (h *GroupEventHandler) buildLeaderPrompt(group *model.Group, event *model.GroupEvent, members []*model.GroupMember) string {
	var b strings.Builder
	b.WriteString("You are the leader of an A2A platform group chat.\n")
	b.WriteString("Reply to the group as the leader. Keep the response useful and concise.\n")
	b.WriteString("If another agent should act, you may use platform tools within the same group_id; otherwise answer directly.\n\n")
	b.WriteString("Group:\n")
	b.WriteString("- name: " + group.Name + "\n")
	b.WriteString("- id: " + group.ID + "\n")
	b.WriteString("- mode: " + group.OrchestrationMode + "\n\n")
	b.WriteString("Members:\n")
	for _, member := range members {
		b.WriteString(fmt.Sprintf("- %s:%s (%s)\n", member.ActorType, member.ActorID, member.Role))
	}
	b.WriteString("\nNew group message:\n")
	b.WriteString(fmt.Sprintf("- from %s:%s\n", event.SenderType, event.SenderID))
	b.WriteString("- content:\n")
	b.WriteString(event.Content)
	b.WriteString("\n")
	return b.String()
}

func (h *GroupEventHandler) buildFreeChatPrompt(group *model.Group, event *model.GroupEvent, members []*model.GroupMember, recentEvents []*model.GroupEvent, agentName string) string {
	var b strings.Builder
	b.WriteString("You are an autonomous participant in an A2A platform group chat.\n")
	b.WriteString("Read the latest room message and decide whether you should reply.\n")
	b.WriteString("If you do not have a useful, non-redundant contribution, output exactly: NO_REPLY\n")
	b.WriteString("If you reply, write only the message that should appear in the group chat. Be concise and do not mention this instruction.\n\n")
	b.WriteString("Group:\n")
	b.WriteString("- name: " + group.Name + "\n")
	b.WriteString("- id: " + group.ID + "\n")
	b.WriteString("- mode: " + group.OrchestrationMode + "\n")
	if strings.TrimSpace(group.RulesJson) != "" {
		b.WriteString("- rules: " + group.RulesJson + "\n")
	}
	b.WriteString("\nMembers:\n")
	for _, member := range members {
		b.WriteString(fmt.Sprintf("- %s:%s (%s)\n", member.ActorType, member.ActorID, member.Role))
	}
	b.WriteString("\nRecent room messages:\n")
	for _, recent := range recentEvents {
		b.WriteString(fmt.Sprintf("- [%s] %s:%s: %s\n", recent.CreatedAt.Format(time.RFC3339), recent.SenderType, recent.SenderID, compactForPrompt(recent.Content, 700)))
	}
	b.WriteString("\nYou are agent: " + agentName + "\n")
	b.WriteString("Latest message to consider:\n")
	b.WriteString(fmt.Sprintf("- from %s:%s\n", event.SenderType, event.SenderID))
	b.WriteString("- content:\n")
	b.WriteString(event.Content)
	b.WriteString("\n")
	return b.String()
}

func (h *GroupEventHandler) appendSystemGroupEvent(groupID, content string, triggerEventID int64, mode string) ([]*model.GroupEvent, error) {
	metadata, _ := json.Marshal(map[string]interface{}{
		"trigger_event_id": triggerEventID,
		"orchestration":    mode,
		"level":            "error",
	})
	systemEvent := &model.GroupEvent{
		GroupID:      groupID,
		EventType:    "orchestration_error",
		SenderType:   model.GroupActorSystem,
		SenderID:     "platform",
		Content:      content,
		MetadataJson: string(metadata),
	}
	if err := h.svcCtx.GroupEvents.Append(systemEvent); err != nil {
		return nil, err
	}
	return []*model.GroupEvent{systemEvent}, nil
}

func selectGroupLeader(members []*model.GroupMember) string {
	for _, member := range members {
		if member.ActorType == model.GroupActorAgent && member.Role == "leader" {
			return member.ActorID
		}
	}
	for _, member := range members {
		if member.ActorType == model.GroupActorAgent {
			return member.ActorID
		}
	}
	return ""
}

func freeChatCandidates(members []*model.GroupMember, senderType, senderID string) []string {
	candidates := []string{}
	seen := map[string]bool{}
	for _, member := range members {
		if member.ActorType != model.GroupActorAgent || member.Role == "observer" {
			continue
		}
		if senderType == model.GroupActorAgent && member.ActorID == senderID {
			continue
		}
		if seen[member.ActorID] {
			continue
		}
		seen[member.ActorID] = true
		candidates = append(candidates, member.ActorID)
	}
	return candidates
}

func parseGroupRules(raw string) groupRules {
	rules := groupRules{}
	if strings.TrimSpace(raw) == "" {
		return rules
	}
	_ = json.Unmarshal([]byte(raw), &rules)
	return rules
}

func freeChatHotMessageLimit(raw string) int {
	policy := groupMemoryPolicy{}
	if strings.TrimSpace(raw) != "" {
		_ = json.Unmarshal([]byte(raw), &policy)
	}
	if policy.HotMessages <= 0 {
		return 12
	}
	if policy.HotMessages > 80 {
		return 80
	}
	return policy.HotMessages
}

func isNoReply(text string) bool {
	normalized := strings.TrimSpace(strings.Trim(text, "`\"' "))
	return strings.EqualFold(normalized, "NO_REPLY") ||
		strings.EqualFold(normalized, "NO REPLY") ||
		strings.EqualFold(normalized, "不回复")
}

func compactForPrompt(text string, limit int) string {
	text = strings.TrimSpace(strings.ReplaceAll(text, "\n", " "))
	if limit <= 0 || len(text) <= limit {
		return text
	}
	return text[:limit] + "..."
}

func groupContextID(groupID string) string {
	return "group:" + groupID
}

func extractGroupAgentResponseText(body string) string {
	var streamed strings.Builder
	var wrapped string
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "" {
			continue
		}
		var evt map[string]interface{}
		if json.Unmarshal([]byte(data), &evt) == nil {
			if evt["type"] == "text.delta" {
				if text, ok := evt["text"].(string); ok {
					streamed.WriteString(text)
				}
			}
		}
		if text := extractTextFromSSEData(data); text != "" {
			wrapped = text
		}
	}
	if text := strings.TrimSpace(streamed.String()); text != "" {
		return text
	}
	if strings.TrimSpace(wrapped) != "" {
		return wrapped
	}
	return extractResponseText([]byte(body))
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
		if isMemberPrincipal(r) {
			actorID := r.Header.Get(authActorIDHeader)
			if artifact.CreatedBy == "" {
				artifact.CreatedBy = actorID
			}
			if artifact.CreatedBy != actorID {
				jsonError(w, "created_by does not match access token", 403)
				return
			}
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

func isMemberPrincipal(r *http.Request) bool {
	return r.Header.Get(authPrincipalHeader) == "member"
}
