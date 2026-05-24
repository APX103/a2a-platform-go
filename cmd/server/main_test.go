package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"a2a-platform/internal/config"
	"a2a-platform/internal/model"
	"a2a-platform/internal/svc"
	"a2a-platform/internal/testutil"
)

func setupSubagentRouteTestContext(t *testing.T) (*svc.ServiceContext, string, string) {
	t.Helper()

	db := testutil.TempMySQLDB(t)

	_, err := db.Exec(`CREATE TABLE subagent_sessions (
		id VARCHAR(36) PRIMARY KEY,
		parent_context_id VARCHAR(36) NOT NULL,
		parent_tool_call_id VARCHAR(64),
		task TEXT,
		context TEXT,
		status VARCHAR(16) NOT NULL DEFAULT 'running',
		messages JSON,
		result TEXT,
		error TEXT,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		completed_at TIMESTAMP
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`)
	if err != nil {
		t.Fatalf("create schema: %v", err)
	}

	parentContextId := "11111111-1111-1111-1111-111111111111"
	session, err := svc.NewSubagentStore(db).Create(parentContextId, "tool-1", "inspect", "context")
	if err != nil {
		t.Fatalf("create subagent: %v", err)
	}

	return &svc.ServiceContext{Subagents: svc.NewSubagentStore(db)}, parentContextId, session.ID
}

func setupGroupRouteTestContext(t *testing.T) (*svc.ServiceContext, string) {
	t.Helper()

	db := testutil.TempMySQLDB(t)

	_, err := db.Exec(`
	CREATE TABLE a2a_groups (
		id VARCHAR(36) PRIMARY KEY,
		name VARCHAR(255) NOT NULL,
		description TEXT,
		orchestration_mode VARCHAR(64) NOT NULL DEFAULT 'leader_led',
		rules_json TEXT,
		memory_policy_json TEXT,
		status VARCHAR(32) NOT NULL DEFAULT 'active',
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
	CREATE TABLE group_members (
		id BIGINT AUTO_INCREMENT PRIMARY KEY,
		group_id VARCHAR(36) NOT NULL,
		actor_type VARCHAR(32) NOT NULL,
		actor_id VARCHAR(255) NOT NULL,
		role VARCHAR(64) NOT NULL DEFAULT 'member',
		capabilities_json TEXT,
		joined_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		UNIQUE KEY uniq_group_actor (group_id, actor_type, actor_id)
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
	CREATE TABLE group_invites (
		id BIGINT AUTO_INCREMENT PRIMARY KEY,
		group_id VARCHAR(36) NOT NULL,
		token_hash VARCHAR(64) NOT NULL UNIQUE,
		actor_type_allowed VARCHAR(32),
		role VARCHAR(64) NOT NULL DEFAULT 'member',
		max_uses INT NOT NULL DEFAULT 1,
		used_count INT NOT NULL DEFAULT 0,
		expires_at TIMESTAMP NULL,
		status VARCHAR(32) NOT NULL DEFAULT 'active',
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
	CREATE TABLE group_member_tokens (
		id BIGINT AUTO_INCREMENT PRIMARY KEY,
		group_id VARCHAR(36) NOT NULL,
		actor_type VARCHAR(32) NOT NULL,
		actor_id VARCHAR(255) NOT NULL,
		token_hash VARCHAR(64) NOT NULL UNIQUE,
		expires_at TIMESTAMP NULL,
		revoked_at TIMESTAMP NULL,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
	CREATE TABLE group_events (
		id BIGINT AUTO_INCREMENT PRIMARY KEY,
		group_id VARCHAR(36) NOT NULL,
		event_type VARCHAR(64) NOT NULL,
		sender_type VARCHAR(32) NOT NULL,
		sender_id VARCHAR(255) NOT NULL,
		content TEXT,
		metadata_json TEXT,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
	CREATE TABLE group_artifacts (
		id VARCHAR(36) PRIMARY KEY,
		group_id VARCHAR(36) NOT NULL,
		name VARCHAR(255) NOT NULL,
		artifact_type VARCHAR(64) NOT NULL DEFAULT 'document',
		version INT NOT NULL DEFAULT 1,
		content MEDIUMTEXT,
		status VARCHAR(32) NOT NULL DEFAULT 'draft',
		created_by VARCHAR(255),
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;`)
	if err != nil {
		t.Fatalf("create schema: %v", err)
	}

	ctx := &svc.ServiceContext{
		Groups:         svc.NewGroupStore(db),
		GroupMembers:   svc.NewGroupMemberStore(db),
		GroupInvites:   svc.NewGroupInviteStore(db),
		GroupTokens:    svc.NewGroupMemberTokenStore(db),
		GroupEvents:    svc.NewGroupEventStore(db),
		GroupArtifacts: svc.NewGroupArtifactStore(db),
	}
	group := &model.Group{Name: "route group", OrchestrationMode: model.GroupModeLeaderLed}
	if err := ctx.Groups.Create(group); err != nil {
		t.Fatalf("create group: %v", err)
	}
	if err := ctx.GroupMembers.Upsert(&model.GroupMember{GroupID: group.ID, ActorType: model.GroupActorAgent, ActorID: "leader-agent", Role: "leader"}); err != nil {
		t.Fatalf("create member: %v", err)
	}
	return ctx, group.ID
}

func TestGroupRoute_JoinAndEventReturnOrchestration(t *testing.T) {
	svcCtx, groupID := setupGroupRouteTestContext(t)

	joinReq := httptest.NewRequest(http.MethodPost, "/api/groups/"+groupID+"/join", strings.NewReader(`{"client_id":"human-route"}`))
	joinRec := httptest.NewRecorder()
	makeGroupRouteHandler(svcCtx).ServeHTTP(joinRec, joinReq)
	if joinRec.Code != http.StatusOK {
		t.Fatalf("join status = %d, body=%s", joinRec.Code, joinRec.Body.String())
	}

	eventReq := httptest.NewRequest(http.MethodPost, "/api/groups/"+groupID+"/events", strings.NewReader(`{"event_type":"message","sender_type":"human","sender_id":"human-route","content":"hello"}`))
	eventRec := httptest.NewRecorder()
	makeGroupRouteHandler(svcCtx).ServeHTTP(eventRec, eventReq)
	if eventRec.Code != http.StatusOK {
		t.Fatalf("event status = %d, body=%s", eventRec.Code, eventRec.Body.String())
	}

	var resp struct {
		Orchestration model.GroupOrchestrationState `json:"orchestration"`
	}
	if err := json.Unmarshal(eventRec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Orchestration.NextAction != "leader_selects_next_speaker" {
		t.Fatalf("next action = %q", resp.Orchestration.NextAction)
	}
	if len(resp.Orchestration.EligibleSpeakers) != 1 || resp.Orchestration.EligibleSpeakers[0] != "leader-agent" {
		t.Fatalf("eligible speakers = %#v", resp.Orchestration.EligibleSpeakers)
	}
}

func TestGroupRoute_P2PRejectsGroupEvents(t *testing.T) {
	svcCtx, _ := setupGroupRouteTestContext(t)
	group := &model.Group{
		ID:                "p2p-route",
		Name:              "p2p route",
		OrchestrationMode: model.GroupModeP2P,
	}
	if err := svcCtx.Groups.Create(group); err != nil {
		t.Fatalf("create p2p group: %v", err)
	}

	eventReq := httptest.NewRequest(http.MethodPost, "/api/groups/"+group.ID+"/events", strings.NewReader(`{"event_type":"message","sender_type":"human","sender_id":"human-route","content":"hello"}`))
	eventRec := httptest.NewRecorder()
	makeGroupRouteHandler(svcCtx).ServeHTTP(eventRec, eventReq)
	if eventRec.Code != http.StatusConflict {
		t.Fatalf("event status = %d, want 409, body=%s", eventRec.Code, eventRec.Body.String())
	}

	events, err := svcCtx.GroupEvents.List(group.ID, 10)
	if err != nil {
		t.Fatalf("list events: %v", err)
	}
	if len(events) != 0 {
		t.Fatalf("p2p events persisted = %d, want 0", len(events))
	}
}

func TestSubagentRoute_UUIDContextListsSubagents(t *testing.T) {
	svcCtx, parentContextId, _ := setupSubagentRouteTestContext(t)
	req := httptest.NewRequest(http.MethodGet, "/api/subagents/"+parentContextId, nil)
	rec := httptest.NewRecorder()

	makeSubagentRouteHandler(svcCtx).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body=%s", rec.Code, rec.Body.String())
	}
	var resp struct {
		ContextId string            `json:"context_id"`
		Subagents []json.RawMessage `json:"subagents"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.ContextId != parentContextId {
		t.Fatalf("context_id = %q, want %q", resp.ContextId, parentContextId)
	}
	if len(resp.Subagents) != 1 {
		t.Fatalf("subagents len = %d, want 1", len(resp.Subagents))
	}
}

func TestSubagentRoute_UUIDSubagentGetsDetail(t *testing.T) {
	svcCtx, _, subagentId := setupSubagentRouteTestContext(t)
	req := httptest.NewRequest(http.MethodGet, "/api/subagents/"+subagentId, nil)
	rec := httptest.NewRecorder()

	makeSubagentRouteHandler(svcCtx).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body=%s", rec.Code, rec.Body.String())
	}
	var resp struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.ID != subagentId {
		t.Fatalf("id = %q, want %q", resp.ID, subagentId)
	}
}

func TestRequestIDMiddlewareSetsResponseHeader(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	req.Header.Set("X-Request-ID", "req-test-123")
	rec := httptest.NewRecorder()

	requestIDMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Context().Value(requestIDContextKey{}); got != "req-test-123" {
			t.Fatalf("context request id = %v, want req-test-123", got)
		}
		w.WriteHeader(http.StatusNoContent)
	})).ServeHTTP(rec, req)

	if got := rec.Header().Get("X-Request-ID"); got != "req-test-123" {
		t.Fatalf("response X-Request-ID = %q, want req-test-123", got)
	}
}

func TestAuthMiddlewareProtectsGroupManagement(t *testing.T) {
	svcCtx := &svc.ServiceContext{Config: &config.Config{AdminToken: "secret"}}
	called := false
	protected := authMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusNoContent)
	}), svcCtx)

	req := httptest.NewRequest(http.MethodPost, "/api/groups", strings.NewReader(`{"name":"x"}`))
	rec := httptest.NewRecorder()
	protected.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
	if called {
		t.Fatal("protected handler was called without admin token")
	}

	req = httptest.NewRequest(http.MethodPost, "/api/groups", strings.NewReader(`{"name":"x"}`))
	req.Header.Set("X-Admin-Token", "secret")
	rec = httptest.NewRecorder()
	protected.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("authorized status = %d, want 204", rec.Code)
	}
}

func TestAuthMiddlewareRequiresGroupMembershipForGroupReads(t *testing.T) {
	svcCtx, groupID := setupGroupRouteTestContext(t)
	svcCtx.Config = &config.Config{AdminToken: "secret"}

	protected := authMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}), svcCtx)

	req := httptest.NewRequest(http.MethodGet, "/api/groups/"+groupID+"/members", nil)
	rec := httptest.NewRecorder()
	protected.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("no token status = %d, want 401", rec.Code)
	}

	if err := svcCtx.GroupMembers.Upsert(&model.GroupMember{
		GroupID:   groupID,
		ActorType: model.GroupActorHuman,
		ActorID:   "human-route",
		Role:      "member",
	}); err != nil {
		t.Fatalf("create human member: %v", err)
	}
	accessToken, err := svcCtx.GroupTokens.Create(&model.GroupMemberToken{
		GroupID:   groupID,
		ActorType: model.GroupActorHuman,
		ActorID:   "human-route",
	})
	if err != nil {
		t.Fatalf("create member token: %v", err)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/groups/"+groupID+"/members", nil)
	req.Header.Set("Authorization", "Bearer "+accessToken)
	rec = httptest.NewRecorder()
	protected.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("member status = %d, want 204", rec.Code)
	}
	if got := req.Header.Get(authPrincipalHeader); got != "member" {
		t.Fatalf("principal = %q, want member", got)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/groups/other-group/members", nil)
	req.Header.Set("Authorization", "Bearer "+accessToken)
	rec = httptest.NewRecorder()
	protected.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("other group status = %d, want 403", rec.Code)
	}
}

func TestGroupMemberDeleteRevokesMemberToken(t *testing.T) {
	svcCtx, groupID := setupGroupRouteTestContext(t)
	svcCtx.Config = &config.Config{AdminToken: "secret"}
	if err := svcCtx.GroupMembers.Upsert(&model.GroupMember{
		GroupID:   groupID,
		ActorType: model.GroupActorHuman,
		ActorID:   "human-route",
		Role:      "member",
	}); err != nil {
		t.Fatalf("create human member: %v", err)
	}
	accessToken, err := svcCtx.GroupTokens.Create(&model.GroupMemberToken{
		GroupID:   groupID,
		ActorType: model.GroupActorHuman,
		ActorID:   "human-route",
	})
	if err != nil {
		t.Fatalf("create member token: %v", err)
	}

	deleteReq := httptest.NewRequest(http.MethodDelete, "/api/groups/"+groupID+"/members/human/human-route", nil)
	deleteRec := httptest.NewRecorder()
	makeGroupRouteHandler(svcCtx).ServeHTTP(deleteRec, deleteReq)
	if deleteRec.Code != http.StatusOK {
		t.Fatalf("delete member status = %d, want 200, body=%s", deleteRec.Code, deleteRec.Body.String())
	}
	storedToken, err := svcCtx.GroupTokens.GetByToken(accessToken)
	if err != nil {
		t.Fatalf("load member token: %v", err)
	}
	if storedToken == nil || storedToken.RevokedAt == nil {
		t.Fatalf("member token was not revoked: %#v", storedToken)
	}

	protected := authMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}), svcCtx)
	readReq := httptest.NewRequest(http.MethodGet, "/api/groups/"+groupID+"/members", nil)
	readReq.Header.Set("Authorization", "Bearer "+accessToken)
	readRec := httptest.NewRecorder()
	protected.ServeHTTP(readRec, readReq)
	if readRec.Code != http.StatusUnauthorized {
		t.Fatalf("removed member status = %d, want 401", readRec.Code)
	}
}

func TestAuthMiddlewareRestrictsAgentProxyToSameGroup(t *testing.T) {
	svcCtx, groupID := setupGroupRouteTestContext(t)
	svcCtx.Config = &config.Config{AdminToken: "secret"}

	if err := svcCtx.GroupMembers.Upsert(&model.GroupMember{
		GroupID:   groupID,
		ActorType: model.GroupActorHuman,
		ActorID:   "human-route",
		Role:      "member",
	}); err != nil {
		t.Fatalf("create human member: %v", err)
	}
	accessToken, err := svcCtx.GroupTokens.Create(&model.GroupMemberToken{
		GroupID:   groupID,
		ActorType: model.GroupActorHuman,
		ActorID:   "human-route",
	})
	if err != nil {
		t.Fatalf("create member token: %v", err)
	}
	protected := authMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}), svcCtx)

	req := httptest.NewRequest(http.MethodPost, "/agent/leader-agent", nil)
	req.Header.Set("Authorization", "Bearer "+accessToken)
	rec := httptest.NewRecorder()
	protected.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("same group agent status = %d, want 204", rec.Code)
	}

	req = httptest.NewRequest(http.MethodPost, "/agent/not-in-room", nil)
	req.Header.Set("Authorization", "Bearer "+accessToken)
	rec = httptest.NewRecorder()
	protected.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("other agent status = %d, want 403", rec.Code)
	}
}
