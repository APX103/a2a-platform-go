package svc

import (
	"testing"

	"a2a-platform/internal/model"
)

func TestTaskStoreRecordsSourceAndTargetAgents(t *testing.T) {
	db := setupRegistryTestDB(t)
	store := NewTaskStore(db)

	source := "planner"
	task := &model.Task{
		LocalTaskId: "task-direction",
		SourceAgent: &source,
		TargetAgent: "coder",
		AgentName:   "coder",
		State:       "PENDING",
	}
	if err := store.Create(task); err != nil {
		t.Fatalf("create task: %v", err)
	}

	got, err := store.Get("task-direction")
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	if got == nil {
		t.Fatal("expected task")
	}
	if got.SourceAgent == nil || *got.SourceAgent != "planner" {
		t.Fatalf("source_agent = %v, want planner", got.SourceAgent)
	}
	if got.TargetAgent != "coder" {
		t.Fatalf("target_agent = %q, want coder", got.TargetAgent)
	}
	if got.AgentName != got.TargetAgent {
		t.Fatalf("agent_name = %q, want legacy target %q", got.AgentName, got.TargetAgent)
	}
}

func TestTaskStoreBackfillsTargetFromLegacyAgentName(t *testing.T) {
	db := setupRegistryTestDB(t)
	store := NewTaskStore(db)

	if _, err := db.Exec(
		"INSERT INTO tasks (local_task_id, agent_name, state) VALUES (?, ?, ?)",
		"legacy-task", "legacy-agent", "PENDING",
	); err != nil {
		t.Fatalf("insert legacy task: %v", err)
	}

	got, err := store.Get("legacy-task")
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	if got == nil {
		t.Fatal("expected task")
	}
	if got.TargetAgent != "legacy-agent" {
		t.Fatalf("target_agent = %q, want legacy-agent", got.TargetAgent)
	}
}
