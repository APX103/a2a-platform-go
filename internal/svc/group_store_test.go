package svc

import (
	"testing"

	"a2a-platform/internal/model"
)

func TestGroupStores_Lifecycle(t *testing.T) {
	db := setupRegistryTestDB(t)

	groups := NewGroupStore(db)
	members := NewGroupMemberStore(db)
	events := NewGroupEventStore(db)
	artifacts := NewGroupArtifactStore(db)
	invites := NewGroupInviteStore(db)
	tokens := NewGroupMemberTokenStore(db)

	group := &model.Group{
		Name:              "proposal review",
		Description:       "Review an A2A proposal",
		OrchestrationMode: model.GroupModeRoundtable,
		RulesJson:         `{"required_votes":2}`,
	}
	if err := groups.Create(group); err != nil {
		t.Fatalf("create group: %v", err)
	}
	if group.ID == "" {
		t.Fatal("group id was not generated")
	}

	if err := members.Upsert(&model.GroupMember{GroupID: group.ID, ActorType: model.GroupActorAgent, ActorID: "planner", Role: "leader"}); err != nil {
		t.Fatalf("upsert leader: %v", err)
	}
	if err := members.Upsert(&model.GroupMember{GroupID: group.ID, ActorType: model.GroupActorHuman, ActorID: "client-1", Role: "member"}); err != nil {
		t.Fatalf("upsert human: %v", err)
	}
	memberList, err := members.List(group.ID)
	if err != nil {
		t.Fatalf("list members: %v", err)
	}
	if len(memberList) != 2 {
		t.Fatalf("members len = %d, want 2", len(memberList))
	}

	invite := &model.GroupInvite{GroupID: group.ID, ActorTypeAllowed: model.GroupActorHuman, Role: "member", MaxUses: 2}
	plainInvite, err := invites.Create(invite)
	if err != nil {
		t.Fatalf("create invite: %v", err)
	}
	if plainInvite == "" || invite.TokenHash == "" {
		t.Fatal("invite token was not generated")
	}
	loadedInvite, err := invites.GetByToken(plainInvite)
	if err != nil {
		t.Fatalf("get invite by token: %v", err)
	}
	if !InviteUsable(loadedInvite, model.GroupActorHuman, loadedInvite.CreatedAt) {
		t.Fatalf("invite should be usable: %#v", loadedInvite)
	}
	if InviteUsable(loadedInvite, model.GroupActorAgent, loadedInvite.CreatedAt) {
		t.Fatal("human-only invite was usable by agent")
	}

	memberToken := &model.GroupMemberToken{GroupID: group.ID, ActorType: model.GroupActorHuman, ActorID: "client-1"}
	plainAccess, err := tokens.Create(memberToken)
	if err != nil {
		t.Fatalf("create member token: %v", err)
	}
	loadedToken, err := tokens.GetByToken(plainAccess)
	if err != nil {
		t.Fatalf("get member token: %v", err)
	}
	if !MemberTokenUsable(loadedToken, loadedToken.CreatedAt) {
		t.Fatalf("member token should be usable: %#v", loadedToken)
	}

	if err := events.Append(&model.GroupEvent{GroupID: group.ID, EventType: "message", SenderType: model.GroupActorHuman, SenderID: "client-1", Content: "please review"}); err != nil {
		t.Fatalf("append event: %v", err)
	}
	eventList, err := events.List(group.ID, 10)
	if err != nil {
		t.Fatalf("list events: %v", err)
	}
	if len(eventList) != 1 || eventList[0].Content != "please review" {
		t.Fatalf("unexpected events: %#v", eventList)
	}

	artifact := &model.GroupArtifact{GroupID: group.ID, Name: "draft", Content: "v1", CreatedBy: "client-1"}
	if err := artifacts.Create(artifact); err != nil {
		t.Fatalf("create artifact: %v", err)
	}
	artifact.Content = "v2"
	if err := artifacts.Update(artifact); err != nil {
		t.Fatalf("update artifact: %v", err)
	}
	refreshed, err := artifacts.Get(artifact.ID)
	if err != nil {
		t.Fatalf("get artifact: %v", err)
	}
	if refreshed.Version != 2 || refreshed.Content != "v2" {
		t.Fatalf("artifact = version %d content %q, want version 2 content v2", refreshed.Version, refreshed.Content)
	}

	state := BuildGroupOrchestrationState(group, memberList)
	if state.Mode != model.GroupModeRoundtable {
		t.Fatalf("mode = %q, want %q", state.Mode, model.GroupModeRoundtable)
	}
	if state.NextAction != "collect_member_intents" {
		t.Fatalf("next action = %q", state.NextAction)
	}

	group.OrchestrationMode = model.GroupModeFreeChat
	state = BuildGroupOrchestrationState(group, memberList)
	if state.Mode != model.GroupModeFreeChat {
		t.Fatalf("mode = %q, want %q", state.Mode, model.GroupModeFreeChat)
	}
	if state.NextAction != "agents_observe_and_optionally_reply" {
		t.Fatalf("free chat next action = %q", state.NextAction)
	}
}

func TestGroupInviteConsumeRespectsMaxUses(t *testing.T) {
	db := setupRegistryTestDB(t)
	store := NewGroupInviteStore(db)
	invite := &model.GroupInvite{
		GroupID: "group-invite",
		Role:    "member",
		MaxUses: 1,
		Status:  model.GroupStatusActive,
	}
	token, err := store.Create(invite)
	if err != nil {
		t.Fatalf("create invite: %v", err)
	}
	loaded, err := store.GetByToken(token)
	if err != nil {
		t.Fatalf("load invite: %v", err)
	}

	if err := store.Consume(loaded.ID); err != nil {
		t.Fatalf("first consume: %v", err)
	}
	if err := store.Consume(loaded.ID); err == nil {
		t.Fatal("second consume succeeded, want max uses error")
	}
}
