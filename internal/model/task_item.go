package model

import "time"

// TaskItem represents a persistent task in the Task System.
// Decoupled from Subagent — a TaskItem is a goal, Subagent is an executor.
type TaskItem struct {
	ID          string     `db:"id" json:"id"`
	ContextId   string     `db:"context_id" json:"context_id"`
	Subject     string     `db:"subject" json:"subject"`
	Description string     `db:"description" json:"description"`
	Status      string     `db:"status" json:"status"` // pending | in_progress | completed | failed
	Owner       string     `db:"owner" json:"owner,omitempty"` // agent name or "user"
	BlockedBy   string     `db:"blocked_by" json:"blocked_by,omitempty"` // JSON array of task IDs
	Result      string     `db:"result" json:"result,omitempty"`
	Error       string     `db:"error" json:"error,omitempty"`
	CreatedAt   time.Time  `db:"created_at" json:"created_at"`
	CompletedAt *time.Time `db:"completed_at" json:"completed_at,omitempty"`
}
