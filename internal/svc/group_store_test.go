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
}
