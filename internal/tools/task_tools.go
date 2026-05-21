package tools

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"a2a-platform/internal/model"
)

// TaskItemStore is the interface for task persistence.
// Defined here to avoid import cycle with svc package.
type TaskItemStore interface {
	Create(item *model.TaskItem) error
	Get(id string) (*model.TaskItem, error)
	ListByContext(contextId string) ([]*model.TaskItem, error)
	UpdateStatus(id, status, result, errorMsg string) error
	Claim(id, owner string) error
	CanStart(id string) (bool, error)
	ListUnblocked(contextId string) ([]*model.TaskItem, error)
}

// NewTaskTools returns the 5 Task System tools.
func NewTaskTools(store TaskItemStore) []model.BuiltinTool {
	return []model.BuiltinTool{
		{
			Name:        "create_task",
			Description: "Create a new task with optional blockedBy dependencies. Use this to break down large goals into smaller actionable tasks.",
			Parameters: []model.ToolParameter{
				{Name: "subject", Type: "string", Description: "Short title of the task", Required: true},
				{Name: "description", Type: "string", Description: "Detailed description of what needs to be done", Required: false},
				{Name: "context_id", Type: "string", Description: "Context/session ID this task belongs to", Required: true},
				{Name: "blocked_by", Type: "string", Description: "Comma-separated list of task IDs that must be completed before this task can start", Required: false},
			},
			Execute: executeCreateTask(store),
			IsReadOnly: false,
		},
		{
			Name:        "list_tasks",
			Description: "List all tasks for a given context, showing status, owner, and dependencies.",
			Parameters: []model.ToolParameter{
				{Name: "context_id", Type: "string", Description: "Context/session ID to list tasks for", Required: true},
			},
			Execute: executeListTasks(store),
			IsReadOnly: true,
		},
		{
			Name:        "get_task",
			Description: "Get full details of a specific task by ID, including description and dependencies.",
			Parameters: []model.ToolParameter{
				{Name: "task_id", Type: "string", Description: "The task ID", Required: true},
			},
			Execute: executeGetTask(store),
			IsReadOnly: true,
		},
		{
			Name:        "claim_task",
			Description: "Claim a pending task. Sets owner and changes status to in_progress. Only succeeds if all blockedBy dependencies are completed.",
			Parameters: []model.ToolParameter{
				{Name: "task_id", Type: "string", Description: "The task ID to claim", Required: true},
				{Name: "owner", Type: "string", Description: "Name of the agent claiming the task", Required: false},
			},
			Execute: executeClaimTask(store),
			IsReadOnly: false,
		},
		{
			Name:        "complete_task",
			Description: "Complete an in_progress task. Reports any downstream tasks that are now unblocked.",
			Parameters: []model.ToolParameter{
				{Name: "task_id", Type: "string", Description: "The task ID to complete", Required: true},
				{Name: "result", Type: "string", Description: "Summary of what was accomplished", Required: false},
			},
			Execute: executeCompleteTask(store),
			IsReadOnly: false,
		},
	}
}

func executeCreateTask(store TaskItemStore) func(args map[string]any) (string, error) {
	return func(args map[string]any) (string, error) {
		subject, _ := args["subject"].(string)
		if subject == "" {
			return "", fmt.Errorf("subject is required")
		}
		description, _ := args["description"].(string)
		contextId, _ := args["context_id"].(string)
		if contextId == "" {
			return "", fmt.Errorf("context_id is required")
		}

		var blockedByJSON string
		if bb, ok := args["blocked_by"].(string); ok && bb != "" {
			ids := strings.Split(bb, ",")
			for i := range ids {
				ids[i] = strings.TrimSpace(ids[i])
			}
			b, _ := json.Marshal(ids)
			blockedByJSON = string(b)
		}

		item := &model.TaskItem{
			ID:          fmt.Sprintf("task_%d", time.Now().UnixNano()),
			ContextId:   contextId,
			Subject:     subject,
			Description: description,
			Status:      "pending",
			CreatedAt:   time.Now(),
			BlockedBy:   blockedByJSON,
		}
		if err := store.Create(item); err != nil {
			return "", fmt.Errorf("failed to create task: %w", err)
		}

		deps := ""
		if blockedByJSON != "" {
			deps = fmt.Sprintf(" (blockedBy: %s)", blockedByJSON)
		}
		return fmt.Sprintf("Created task %s: %s%s", item.ID, subject, deps), nil
	}
}

func executeListTasks(store TaskItemStore) func(args map[string]any) (string, error) {
	return func(args map[string]any) (string, error) {
		contextId, _ := args["context_id"].(string)
		if contextId == "" {
			return "", fmt.Errorf("context_id is required")
		}

		items, err := store.ListByContext(contextId)
		if err != nil {
			return "", err
		}
		if len(items) == 0 {
			return "No tasks found. Use create_task to add some.", nil
		}

		lines := []string{fmt.Sprintf("Tasks for context %s:", contextId)}
		for _, item := range items {
			icon := map[string]string{
				"pending":     "○",
				"in_progress": "●",
				"completed":   "✓",
				"failed":      "✗",
			}[item.Status]
			owner := ""
			if item.Owner != "" {
				owner = fmt.Sprintf(" [%s]", item.Owner)
			}
			deps := ""
			if item.BlockedBy != "" {
				deps = fmt.Sprintf(" (blockedBy: %s)", item.BlockedBy)
			}
			lines = append(lines, fmt.Sprintf(" %s %s: %s [%s]%s%s", icon, item.ID, item.Subject, item.Status, owner, deps))
		}
		return strings.Join(lines, "\n"), nil
	}
}

func executeGetTask(store TaskItemStore) func(args map[string]any) (string, error) {
	return func(args map[string]any) (string, error) {
		taskId, _ := args["task_id"].(string)
		if taskId == "" {
			return "", fmt.Errorf("task_id is required")
		}

		item, err := store.Get(taskId)
		if err != nil {
			return "", err
		}
		if item == nil {
			return fmt.Sprintf("Task %s not found", taskId), nil
		}

		b, _ := json.MarshalIndent(item, "", "  ")
		return string(b), nil
	}
}

func executeClaimTask(store TaskItemStore) func(args map[string]any) (string, error) {
	return func(args map[string]any) (string, error) {
		taskId, _ := args["task_id"].(string)
		if taskId == "" {
			return "", fmt.Errorf("task_id is required")
		}
		owner, _ := args["owner"].(string)
		if owner == "" {
			owner = "agent"
		}

		// Check dependencies
		ok, err := store.CanStart(taskId)
		if err != nil {
			return "", err
		}
		if !ok {
			return fmt.Sprintf("Task %s is blocked by incomplete dependencies", taskId), nil
		}

		if err := store.Claim(taskId, owner); err != nil {
			return "", err
		}
		return fmt.Sprintf("Claimed task %s (owner: %s)", taskId, owner), nil
	}
}

func executeCompleteTask(store TaskItemStore) func(args map[string]any) (string, error) {
	return func(args map[string]any) (string, error) {
		taskId, _ := args["task_id"].(string)
		if taskId == "" {
			return "", fmt.Errorf("task_id is required")
		}
		result, _ := args["result"].(string)

		item, err := store.Get(taskId)
		if err != nil {
			return "", err
		}
		if item == nil {
			return fmt.Sprintf("Task %s not found", taskId), nil
		}
		if item.Status != "in_progress" {
			return fmt.Sprintf("Task %s is %s, cannot complete", taskId, item.Status), nil
		}

		if err := store.UpdateStatus(taskId, "completed", result, ""); err != nil {
			return "", err
		}

		// Find newly unblocked tasks
		unblocked, err := store.ListUnblocked(item.ContextId)
		if err != nil {
			return "", err
		}

		msg := fmt.Sprintf("Completed task %s (%s)", taskId, item.Subject)
		if len(unblocked) > 0 {
			names := make([]string, len(unblocked))
			for i, u := range unblocked {
				names[i] = fmt.Sprintf("%s (%s)", u.ID, u.Subject)
			}
			msg += fmt.Sprintf("\nUnblocked tasks: %s", strings.Join(names, ", "))
		}
		return msg, nil
	}
}
