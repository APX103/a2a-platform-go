package svc

import (
	"encoding/json"
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

func TestLegacyRepairInfersTaskSourceFromSendToAgentTrace(t *testing.T) {
	db := setupRegistryTestDB(t)

	if _, err := db.Exec(
		"INSERT INTO tasks (local_task_id, target_agent, agent_name, state) VALUES (?, ?, ?, ?)",
		"child-task", "mi-3", "mi-3", "RESPONDED",
	); err != nil {
		t.Fatalf("insert child task: %v", err)
	}
	if _, err := db.Exec(
		"INSERT INTO messages (task_id, role, content) VALUES (?, ?, ?)",
		"child-task", "user", "hello from mi-2",
	); err != nil {
		t.Fatalf("insert child message: %v", err)
	}
	args, _ := json.Marshal(map[string]string{"agent": "mi-3", "message": "hello from mi-2"})
	traceData, _ := json.Marshal(map[string]string{"tool": "send_to_agent", "arguments": string(args)})
	if _, err := db.Exec(
		"INSERT INTO traces (task_id, event_type, agent_name, data_json) VALUES (?, ?, ?, ?)",
		"parent-task", "tool_call", "mi-2", string(traceData),
	); err != nil {
		t.Fatalf("insert parent trace: %v", err)
	}
	if _, err := db.Exec(
		"INSERT INTO traces (task_id, event_type, agent_name, target_agent, data_json) VALUES (?, ?, ?, ?, ?)",
		"child-task", "send", "host", "mi-3", "{}",
	); err != nil {
		t.Fatalf("insert child trace: %v", err)
	}

	repairLegacyTaskSourcesFromToolCalls(db)
	backfillMessageDirections(db)

	task, err := NewTaskStore(db).Get("child-task")
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	if task.SourceAgent == nil || *task.SourceAgent != "mi-2" {
		t.Fatalf("source_agent = %v, want mi-2", task.SourceAgent)
	}

	messages, err := NewMessageStore(db).GetByTask("child-task")
	if err != nil {
		t.Fatalf("get messages: %v", err)
	}
	if len(messages) != 1 {
		t.Fatalf("messages len = %d, want 1", len(messages))
	}
	if messages[0].SenderAgent == nil || *messages[0].SenderAgent != "mi-2" {
		t.Fatalf("message sender_agent = %v, want mi-2", messages[0].SenderAgent)
	}
	if messages[0].RecipientAgent == nil || *messages[0].RecipientAgent != "mi-3" {
		t.Fatalf("message recipient_agent = %v, want mi-3", messages[0].RecipientAgent)
	}
}
