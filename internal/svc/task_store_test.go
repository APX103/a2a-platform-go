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

func TestTaskStoreUpdateRejectsUnknownColumns(t *testing.T) {
	db := setupRegistryTestDB(t)
	store := NewTaskStore(db)
	task := &model.Task{LocalTaskId: "task-whitelist", AgentName: "agent", State: "PENDING"}
	if err := store.Create(task); err != nil {
		t.Fatalf("create task: %v", err)
	}

	err := store.Update("task-whitelist", map[string]interface{}{
		"state = 'DONE', agent_name": "evil",
	})
	if err == nil {
		t.Fatal("Update accepted unsafe column")
	}
}

func TestTaskStoreUpdateAllowsMultipleKnownColumns(t *testing.T) {
	db := setupRegistryTestDB(t)
	store := NewTaskStore(db)
	task := &model.Task{LocalTaskId: "task-multi-update", AgentName: "agent", State: "PENDING"}
	if err := store.Create(task); err != nil {
		t.Fatalf("create task: %v", err)
	}

	if err := store.Update("task-multi-update", map[string]interface{}{
		"context_id":   "ctx-multi-update",
		"state":        "WORKING",
		"target_agent": "agent-2",
	}); err != nil {
		t.Fatalf("update task: %v", err)
	}

	got, err := store.Get("task-multi-update")
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	if got == nil {
		t.Fatal("expected task")
	}
	if got.ContextId == nil || *got.ContextId != "ctx-multi-update" {
		t.Fatalf("context_id = %v, want ctx-multi-update", got.ContextId)
	}
	if got.State != "WORKING" {
		t.Fatalf("state = %q, want WORKING", got.State)
	}
	if got.TargetAgent != "agent-2" {
		t.Fatalf("target_agent = %q, want agent-2", got.TargetAgent)
	}
}

func TestTaskItemClaimMissingTaskReturnsError(t *testing.T) {
	db := setupRegistryTestDB(t)
	store := NewTaskItemStore(db)

	if err := store.Claim("missing-task-item", "agent"); err == nil {
		t.Fatal("Claim missing task item succeeded")
	}
}

func TestTaskStoreRecordsRootAndParentLineage(t *testing.T) {
	db := setupRegistryTestDB(t)
	store := NewTaskStore(db)

	root := "root-context"
	parent := "parent-task"
	tool := "tool-call"
	task := &model.Task{
		LocalTaskId:      "child-task",
		TargetAgent:      "mi-2",
		AgentName:        "mi-2",
		ContextId:        ptrString("child-context"),
		RootContextId:    &root,
		ParentTaskId:     &parent,
		ParentToolCallId: &tool,
		State:            "PENDING",
	}
	if err := store.Create(task); err != nil {
		t.Fatalf("create task: %v", err)
	}

	got, err := store.Get("child-task")
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	if got.RootContextId == nil || *got.RootContextId != root {
		t.Fatalf("root_context_id = %v, want %q", got.RootContextId, root)
	}
	if got.ParentTaskId == nil || *got.ParentTaskId != parent {
		t.Fatalf("parent_task_id = %v, want %q", got.ParentTaskId, parent)
	}
	if got.ParentToolCallId == nil || *got.ParentToolCallId != tool {
		t.Fatalf("parent_tool_call_id = %v, want %q", got.ParentToolCallId, tool)
	}

	tasks, err := store.ListByRootContext(root)
	if err != nil {
		t.Fatalf("ListByRootContext: %v", err)
	}
	if len(tasks) != 1 || tasks[0].LocalTaskId != "child-task" {
		t.Fatalf("root tasks = %#v, want child-task", tasks)
	}
}

func TestTraceStoreRecordsRootLineage(t *testing.T) {
	db := setupRegistryTestDB(t)
	store := NewTraceStore(db)

	ctx := "child-context"
	root := "root-context"
	parent := "parent-task"
	target := "mi-2"
	if err := store.Append(&model.TraceEvent{
		TaskId:        "child-task",
		ContextId:     &ctx,
		RootContextId: &root,
		ParentTaskId:  &parent,
		EventType:     "send",
		AgentName:     "mi-1",
		TargetAgent:   &target,
		DataJson:      "{}",
	}); err != nil {
		t.Fatalf("append trace: %v", err)
	}

	traces, err := store.GetByRootContext(root)
	if err != nil {
		t.Fatalf("GetByRootContext: %v", err)
	}
	if len(traces) != 1 {
		t.Fatalf("traces len = %d, want 1", len(traces))
	}
	if traces[0].RootContextId == nil || *traces[0].RootContextId != root {
		t.Fatalf("root_context_id = %v, want %q", traces[0].RootContextId, root)
	}
	if traces[0].ParentTaskId == nil || *traces[0].ParentTaskId != parent {
		t.Fatalf("parent_task_id = %v, want %q", traces[0].ParentTaskId, parent)
	}
}

func TestTraceStoreListContextsGroupsByRootContext(t *testing.T) {
	db := setupRegistryTestDB(t)
	store := NewTraceStore(db)

	ctx1 := "child-context-1"
	ctx2 := "child-context-2"
	root := "root-context"
	for _, item := range []struct {
		taskId string
		ctx    string
		agent  string
	}{
		{taskId: "task-1", ctx: ctx1, agent: "mi-1"},
		{taskId: "task-2", ctx: ctx2, agent: "mi-2"},
	} {
		if err := store.Append(&model.TraceEvent{
			TaskId:        item.taskId,
			ContextId:     &item.ctx,
			RootContextId: &root,
			EventType:     "send",
			AgentName:     item.agent,
			DataJson:      "{}",
		}); err != nil {
			t.Fatalf("append trace: %v", err)
		}
	}

	contexts, err := store.ListContexts(10)
	if err != nil {
		t.Fatalf("ListContexts: %v", err)
	}
	if len(contexts) != 1 {
		t.Fatalf("contexts len = %d, want 1: %#v", len(contexts), contexts)
	}
	if contexts[0].ContextId != root {
		t.Fatalf("context id = %q, want root %q", contexts[0].ContextId, root)
	}
	if contexts[0].TraceCount != 2 {
		t.Fatalf("trace count = %d, want 2", contexts[0].TraceCount)
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

func ptrString(value string) *string {
	return &value
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
