package svc

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"a2a-platform/internal/model"
)

// TaskItemStore persists TaskItem records for the Task System.
type TaskItemStore struct {
	db *sql.DB
}

func NewTaskItemStore(db *sql.DB) *TaskItemStore {
	return &TaskItemStore{db: db}
}

// Create inserts a new task item.
func (s *TaskItemStore) Create(item *model.TaskItem) error {
	_, err := s.db.Exec(
		`INSERT INTO task_items (id, context_id, subject, description, status, owner, blocked_by, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		item.ID, item.ContextId, item.Subject, item.Description,
		item.Status, item.Owner, item.BlockedBy, item.CreatedAt,
	)
	return err
}

// Get retrieves a task by ID.
func (s *TaskItemStore) Get(id string) (*model.TaskItem, error) {
	var item model.TaskItem
	var completedAt sql.NullTime
	err := s.db.QueryRow(
		`SELECT id, context_id, subject, description, status, owner, blocked_by, result, error, created_at, completed_at
		 FROM task_items WHERE id = ?`, id,
	).Scan(
		&item.ID, &item.ContextId, &item.Subject, &item.Description,
		&item.Status, &item.Owner, &item.BlockedBy, &item.Result,
		&item.Error, &item.CreatedAt, &completedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if completedAt.Valid {
		item.CompletedAt = &completedAt.Time
	}
	return &item, nil
}

// ListByContext returns all tasks for a context, ordered by creation time.
func (s *TaskItemStore) ListByContext(contextId string) ([]*model.TaskItem, error) {
	rows, err := s.db.Query(
		`SELECT id, context_id, subject, description, status, owner, blocked_by, result, error, created_at, completed_at
		 FROM task_items WHERE context_id = ? ORDER BY created_at ASC`,
		contextId,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []*model.TaskItem
	for rows.Next() {
		var item model.TaskItem
		var completedAt sql.NullTime
		if err := rows.Scan(
			&item.ID, &item.ContextId, &item.Subject, &item.Description,
			&item.Status, &item.Owner, &item.BlockedBy, &item.Result,
			&item.Error, &item.CreatedAt, &completedAt,
		); err != nil {
			return nil, err
		}
		if completedAt.Valid {
			item.CompletedAt = &completedAt.Time
		}
		results = append(results, &item)
	}
	return results, rows.Err()
}

// UpdateStatus changes status and optional result/error.
func (s *TaskItemStore) UpdateStatus(id, status, result, errorMsg string) error {
	var completedAt interface{}
	if status == "completed" || status == "failed" {
		completedAt = time.Now()
	} else {
		completedAt = nil
	}
	_, err := s.db.Exec(
		`UPDATE task_items SET status = ?, result = ?, error = ?, completed_at = ? WHERE id = ?`,
		status, result, errorMsg, completedAt, id,
	)
	return err
}

// Claim sets owner and status to in_progress.
func (s *TaskItemStore) Claim(id, owner string) error {
	res, err := s.db.Exec(
		`UPDATE task_items SET owner = ?, status = 'in_progress' WHERE id = ? AND status = 'pending'`,
		owner, id,
	)
	if err != nil {
		return err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return fmt.Errorf("task item %s is not pending or does not exist", id)
	}
	return nil
}

// CanStart checks if all blockedBy dependencies are completed.
func (s *TaskItemStore) CanStart(id string) (bool, error) {
	item, err := s.Get(id)
	if err != nil || item == nil {
		return false, err
	}
	if item.BlockedBy == "" {
		return true, nil
	}
	var deps []string
	if err := json.Unmarshal([]byte(item.BlockedBy), &deps); err != nil {
		return false, fmt.Errorf("invalid blocked_by JSON: %w", err)
	}
	for _, depId := range deps {
		dep, err := s.Get(depId)
		if err != nil {
			return false, err
		}
		if dep == nil || dep.Status != "completed" {
			return false, nil
		}
	}
	return true, nil
}

// ListUnblocked returns pending tasks whose dependencies are all completed.
func (s *TaskItemStore) ListUnblocked(contextId string) ([]*model.TaskItem, error) {
	all, err := s.ListByContext(contextId)
	if err != nil {
		return nil, err
	}
	var unblocked []*model.TaskItem
	for _, item := range all {
		if item.Status != "pending" {
			continue
		}
		ok, _ := s.CanStart(item.ID)
		if ok {
			unblocked = append(unblocked, item)
		}
	}
	return unblocked, nil
}
