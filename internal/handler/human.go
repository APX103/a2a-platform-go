package handler

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"a2a-platform/internal/model"
	"a2a-platform/internal/svc"
)

const (
	humanSessionTTL   = 0
	humanOnlineWindow = 90 * time.Second
)

type humanAuthReq struct {
	Handle      string `json:"handle"`
	DisplayName string `json:"display_name"`
	Token       string `json:"token"`
}

type humanUpdateReq struct {
	Handle      *string `json:"handle"`
	DisplayName *string `json:"display_name"`
}

type humanAuthResp struct {
	Human              *humanPublic                   `json:"human"`
	SessionToken       string                         `json:"session_token"`
	ExpiresAt          *time.Time                     `json:"expires_at,omitempty"`
	Created            bool                           `json:"created,omitempty"`
	DefaultGroup       *model.Group                   `json:"default_group,omitempty"`
	DefaultMember      *model.GroupMember             `json:"default_member,omitempty"`
	DefaultAccessToken string                         `json:"default_access_token,omitempty"`
	Orchestration      *model.GroupOrchestrationState `json:"orchestration,omitempty"`
}

type humanTokenIssueResp struct {
	Human        *humanPublic `json:"human"`
	SessionToken string       `json:"session_token"`
	ExpiresAt    *time.Time   `json:"expires_at,omitempty"`
}

type humanPublic struct {
	ID          string `json:"id"`
	Handle      string `json:"handle"`
	DisplayName string `json:"display_name"`
}

type HumanRegisterHandler struct {
	svcCtx *svc.ServiceContext
}

func NewHumanRegisterHandler(svcCtx *svc.ServiceContext) *HumanRegisterHandler {
	return &HumanRegisterHandler{svcCtx: svcCtx}
}

func (h *HumanRegisterHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, "method not allowed", 405)
		return
	}
	var req humanAuthReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid JSON", 400)
		return
	}
	user, err := h.svcCtx.Humans.Create(req.Handle, req.DisplayName)
	if err != nil {
		if strings.Contains(err.Error(), "already exists") {
			jsonError(w, err.Error(), http.StatusConflict)
			return
		}
		jsonError(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeHumanAuth(w, h.svcCtx, user, true)
}

type HumanLoginHandler struct {
	svcCtx *svc.ServiceContext
}

func NewHumanLoginHandler(svcCtx *svc.ServiceContext) *HumanLoginHandler {
	return &HumanLoginHandler{svcCtx: svcCtx}
}

func (h *HumanLoginHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, "method not allowed", 405)
		return
	}
	var req humanAuthReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid JSON", 400)
		return
	}
	token := strings.TrimSpace(req.Token)
	if token == "" {
		token = bearerTokenFromRequest(r)
	}
	if token == "" {
		handle := strings.TrimSpace(req.Handle)
		if handle == "" {
			jsonError(w, "token or handle is required", http.StatusBadRequest)
			return
		}
		user, err := h.svcCtx.Humans.GetByHandle(handle)
		if err != nil {
			errHTTP(w, err)
			return
		}
		if user == nil {
			jsonError(w, "unknown handle", http.StatusUnauthorized)
			return
		}
		writeHumanAuth(w, h.svcCtx, user, false)
		return
	}
	session, err := h.svcCtx.HumanSessions.GetByToken(token)
	if err != nil {
		errHTTP(w, err)
		return
	}
	if !svc.HumanSessionUsable(session, time.Now()) {
		jsonError(w, "invalid token", http.StatusUnauthorized)
		return
	}
	user, err := h.svcCtx.Humans.Get(session.HumanID)
	if err != nil {
		errHTTP(w, err)
		return
	}
	if user == nil {
		jsonError(w, "invalid token", http.StatusUnauthorized)
		return
	}
	writeHumanAuthWithSession(w, h.svcCtx, user, session, token, false)
}

type HumanMeHandler struct {
	svcCtx *svc.ServiceContext
}

func NewHumanMeHandler(svcCtx *svc.ServiceContext) *HumanMeHandler {
	return &HumanMeHandler{svcCtx: svcCtx}
}

func (h *HumanMeHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		jsonError(w, "method not allowed", 405)
		return
	}
	user, _, ok, err := humanFromRequest(r, h.svcCtx)
	if err != nil {
		errHTTP(w, err)
		return
	}
	if !ok {
		jsonError(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	okJSON(w, map[string]interface{}{"human": publicHuman(user)})
}

type HumanListHandler struct {
	svcCtx *svc.ServiceContext
}

func NewHumanListHandler(svcCtx *svc.ServiceContext) *HumanListHandler {
	return &HumanListHandler{svcCtx: svcCtx}
}

func (h *HumanListHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		jsonError(w, "method not allowed", 405)
		return
	}
	humans, err := h.svcCtx.Humans.ListPresence(time.Now(), humanOnlineWindow)
	if err != nil {
		errHTTP(w, err)
		return
	}
	okJSON(w, humans)
}

type HumanDetailHandler struct {
	svcCtx *svc.ServiceContext
}

func NewHumanDetailHandler(svcCtx *svc.ServiceContext) *HumanDetailHandler {
	return &HumanDetailHandler{svcCtx: svcCtx}
}

func (h *HumanDetailHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	tail := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/humans/"), "/")
	parts := strings.Split(tail, "/")
	key := ""
	if len(parts) > 0 {
		key = parts[0]
	}
	if key == "" {
		jsonError(w, "human id is required", http.StatusBadRequest)
		return
	}
	if len(parts) == 2 && parts[1] == "tokens" {
		h.issueToken(w, r, key)
		return
	}
	if len(parts) != 1 {
		jsonError(w, "not found", http.StatusNotFound)
		return
	}
	user, err := h.lookupHuman(key)
	if err != nil {
		errHTTP(w, err)
		return
	}
	if user == nil {
		jsonError(w, "human not found", http.StatusNotFound)
		return
	}

	switch r.Method {
	case http.MethodGet:
		okJSON(w, publicHuman(user))
	case http.MethodPut:
		var req humanUpdateReq
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			jsonError(w, "invalid JSON", 400)
			return
		}
		handle := user.Handle
		if req.Handle != nil {
			handle = *req.Handle
		}
		displayName := user.DisplayName
		if req.DisplayName != nil {
			displayName = *req.DisplayName
		}
		updated, err := h.svcCtx.Humans.Update(user.ID, handle, displayName)
		if err != nil {
			if strings.Contains(err.Error(), "already exists") {
				jsonError(w, err.Error(), http.StatusConflict)
				return
			}
			jsonError(w, err.Error(), http.StatusBadRequest)
			return
		}
		if updated == nil {
			jsonError(w, "human not found", http.StatusNotFound)
			return
		}
		okJSON(w, publicHuman(updated))
	case http.MethodDelete:
		deleted, err := h.svcCtx.Humans.Delete(user.ID)
		if err != nil {
			errHTTP(w, err)
			return
		}
		if !deleted {
			jsonError(w, "human not found", http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		jsonError(w, "method not allowed", 405)
	}
}

func (h *HumanDetailHandler) lookupHuman(key string) (*model.HumanUser, error) {
	user, err := h.svcCtx.Humans.Get(key)
	if err != nil || user != nil {
		return user, err
	}
	return h.svcCtx.Humans.GetByHandle(key)
}

func (h *HumanDetailHandler) issueToken(w http.ResponseWriter, r *http.Request, key string) {
	if r.Method != http.MethodPost {
		jsonError(w, "method not allowed", 405)
		return
	}
	user, err := h.lookupHuman(key)
	if err != nil {
		errHTTP(w, err)
		return
	}
	if user == nil {
		jsonError(w, "human not found", http.StatusNotFound)
		return
	}
	session, token, err := h.svcCtx.HumanSessions.Create(user.ID, humanSessionTTL)
	if err != nil {
		errHTTP(w, err)
		return
	}
	okJSON(w, humanTokenIssueResp{
		Human:        publicHuman(user),
		SessionToken: token,
		ExpiresAt:    session.ExpiresAt,
	})
}

func writeHumanAuth(w http.ResponseWriter, svcCtx *svc.ServiceContext, user *model.HumanUser, created bool) {
	session, token, err := svcCtx.HumanSessions.Create(user.ID, humanSessionTTL)
	if err != nil {
		errHTTP(w, err)
		return
	}
	writeHumanAuthWithSession(w, svcCtx, user, session, token, created)
}

func writeHumanAuthWithSession(w http.ResponseWriter, svcCtx *svc.ServiceContext, user *model.HumanUser, session *model.HumanSession, token string, created bool) {
	group, member, accessToken, orchestration, err := ensureDefaultHumanGroupAccess(svcCtx, user)
	if err != nil {
		errHTTP(w, err)
		return
	}
	if err := svcCtx.Humans.TouchLastSeen(user.ID); err != nil {
		errHTTP(w, err)
		return
	}
	okJSON(w, humanAuthResp{
		Human:              publicHuman(user),
		SessionToken:       token,
		ExpiresAt:          session.ExpiresAt,
		Created:            created,
		DefaultGroup:       group,
		DefaultMember:      member,
		DefaultAccessToken: accessToken,
		Orchestration:      orchestration,
	})
}

func ensureDefaultHumanGroupAccess(svcCtx *svc.ServiceContext, user *model.HumanUser) (*model.Group, *model.GroupMember, string, *model.GroupOrchestrationState, error) {
	if svcCtx == nil || user == nil || svcCtx.Groups == nil || svcCtx.GroupMembers == nil || svcCtx.GroupTokens == nil {
		return nil, nil, "", nil, nil
	}
	if err := ensureSimpleModeGroup(svcCtx); err != nil {
		return nil, nil, "", nil, err
	}
	member := &model.GroupMember{
		GroupID:          model.DefaultP2PGroupID,
		ActorType:        model.GroupActorHuman,
		ActorID:          user.ID,
		Role:             "member",
		CapabilitiesJson: mergeHumanCapabilities(json.RawMessage(`{"simple_mode":true,"p2p":true}`), user),
	}
	if err := svcCtx.GroupMembers.Upsert(member); err != nil {
		return nil, nil, "", nil, err
	}
	accessToken, err := svcCtx.GroupTokens.Create(&model.GroupMemberToken{
		GroupID:   model.DefaultP2PGroupID,
		ActorType: model.GroupActorHuman,
		ActorID:   user.ID,
	})
	if err != nil {
		return nil, nil, "", nil, err
	}
	group, err := svcCtx.Groups.Get(model.DefaultP2PGroupID)
	if err != nil {
		return nil, nil, "", nil, err
	}
	members, err := svcCtx.GroupMembers.List(model.DefaultP2PGroupID)
	if err != nil {
		return nil, nil, "", nil, err
	}
	state := svc.BuildGroupOrchestrationState(group, members)
	return group, member, accessToken, &state, nil
}

func publicHuman(user *model.HumanUser) *humanPublic {
	if user == nil {
		return nil
	}
	return &humanPublic{
		ID:          user.ID,
		Handle:      user.Handle,
		DisplayName: user.DisplayName,
	}
}

func humanFromRequest(r *http.Request, svcCtx *svc.ServiceContext) (*model.HumanUser, *model.HumanSession, bool, error) {
	if svcCtx == nil || svcCtx.HumanSessions == nil || svcCtx.Humans == nil {
		return nil, nil, false, nil
	}
	token := r.Header.Get("X-Human-Session-Token")
	if token == "" {
		token = bearerTokenFromRequest(r)
	}
	if token == "" {
		return nil, nil, false, nil
	}
	session, err := svcCtx.HumanSessions.GetByToken(token)
	if err != nil {
		return nil, nil, false, err
	}
	if !svc.HumanSessionUsable(session, time.Now()) {
		return nil, nil, false, nil
	}
	user, err := svcCtx.Humans.Get(session.HumanID)
	if err != nil {
		return nil, nil, false, err
	}
	if user == nil {
		return nil, nil, false, nil
	}
	if err := svcCtx.Humans.TouchLastSeen(user.ID); err != nil {
		return nil, nil, false, err
	}
	return user, session, true, nil
}

func bearerTokenFromRequest(r *http.Request) string {
	auth := r.Header.Get("Authorization")
	if strings.HasPrefix(auth, "Bearer ") {
		return strings.TrimSpace(strings.TrimPrefix(auth, "Bearer "))
	}
	return ""
}
