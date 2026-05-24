package svc

import (
	"database/sql"
	"fmt"
	"time"

	"a2a-platform/internal/model"
	"a2a-platform/internal/redact"

	"github.com/google/uuid"
)

// ContextStore handles DB operations for contexts (chat sessions).
type ContextStore struct {
	db *sql.DB
}

func NewContextStore(db *sql.DB) *ContextStore {
	return &ContextStore{db: db}
}

// Create creates a new context/session.
func (s *ContextStore) Create(agentName, title string) (*model.Context, error) {
	id := uuid.New().String()
	now := time.Now()

	query := `INSERT INTO contexts (id, agent_name, title, message_count, created_at, updated_at)
			  VALUES (?, ?, ?, 0, ?, ?)`
	_, err := s.db.Exec(query, id, agentName, title, now, now)
	if err != nil {
		return nil, fmt.Errorf("failed to create context: %w", err)
	}

	return &model.Context{
		ID:           id,
		AgentName:    agentName,
		Title:        title,
		MessageCount: 0,
		CreatedAt:    now,
		UpdatedAt:    now,
	}, nil
}

// Get retrieves a context by ID.
func (s *ContextStore) Get(id string) (*model.Context, error) {
	var c model.Context
	var createdAt, updatedAt time.Time

	query := `SELECT id, agent_name, title, message_count, created_at, updated_at
			  FROM contexts WHERE id = ?`

	err := s.db.QueryRow(query, id).Scan(&c.ID, &c.AgentName, &c.Title, &c.MessageCount, &createdAt, &updatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	c.CreatedAt = createdAt
	c.UpdatedAt = updatedAt
	return &c, nil
}

// List retrieves contexts for an agent with pagination.
func (s *ContextStore) List(agentName string, page, size int) ([]*model.Context, int64, error) {
	where := ""
	args := []interface{}{}

	if agentName != "" {
		where = " WHERE agent_name = ?"
		args = append(args, agentName)
	}

	// Count
	var total int64
	countQuery := "SELECT COUNT(*) FROM contexts" + where
	if err := s.db.QueryRow(countQuery, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	// Query
	offset := (page - 1) * size
	args = append(args, size, offset)

	query := `SELECT id, agent_name, title, message_count, created_at, updated_at
			  FROM contexts` + where +
		` ORDER BY updated_at DESC LIMIT ? OFFSET ?`

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var result []*model.Context
	for rows.Next() {
		var c model.Context
		var createdAt, updatedAt time.Time
		if err := rows.Scan(&c.ID, &c.AgentName, &c.Title, &c.MessageCount, &createdAt, &updatedAt); err != nil {
			return nil, 0, err
		}
		c.CreatedAt = createdAt
		c.UpdatedAt = updatedAt
		result = append(result, &c)
	}

	return result, total, nil
}

// Update increments message count and updates timestamp.
func (s *ContextStore) Update(id string) error {
	now := time.Now()
	_, err := s.db.Exec(`UPDATE contexts SET message_count = message_count + 1, updated_at = ? WHERE id = ?`, now, id)
	return err
}

// UpdateTitle changes the context title.
func (s *ContextStore) UpdateTitle(id, title string) error {
	now := time.Now()
	_, err := s.db.Exec(`UPDATE contexts SET title = ?, updated_at = ? WHERE id = ?`, title, now, id)
	return err
}

// Delete removes a context and all its messages.
func (s *ContextStore) Delete(id string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Delete messages first (foreign key cleanup)
	_, err = tx.Exec(`DELETE FROM messages WHERE context_id = ?`, id)
	if err != nil {
		return err
	}

	// Delete context
	_, err = tx.Exec(`DELETE FROM contexts WHERE id = ?`, id)
	if err != nil {
		return err
	}

	return tx.Commit()
}

// GetLatestForAgent returns the most recent context for an agent.
func (s *ContextStore) GetLatestForAgent(agentName string) (*model.Context, error) {
	var c model.Context
	var createdAt, updatedAt time.Time

	query := `SELECT id, agent_name, title, message_count, created_at, updated_at
			  FROM contexts WHERE agent_name = ? ORDER BY updated_at DESC LIMIT 1`

	err := s.db.QueryRow(query, agentName).Scan(&c.ID, &c.AgentName, &c.Title, &c.MessageCount, &createdAt, &updatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	c.CreatedAt = createdAt
	c.UpdatedAt = updatedAt
	return &c, nil
}

// SubagentStore handles DB operations for subagent sessions.
type SubagentStore struct {
	db *sql.DB
}

func NewSubagentStore(db *sql.DB) *SubagentStore {
	return &SubagentStore{db: db}
}

// Create creates a new subagent session.
func (s *SubagentStore) Create(parentContextId, parentToolCallId, task, context string) (*model.SubagentSession, error) {
	id := uuid.New().String()
	now := time.Now()
	safeTask := redact.Text(task)
	safeContext := redact.Text(context)

	query := `INSERT INTO subagent_sessions (id, parent_context_id, parent_tool_call_id, task, context, status, created_at)
			  VALUES (?, ?, ?, ?, ?, 'running', ?)`
	_, err := s.db.Exec(query, id, parentContextId, parentToolCallId, safeTask, safeContext, now)
	if err != nil {
		return nil, fmt.Errorf("failed to create subagent session: %w", err)
	}

	return &model.SubagentSession{
		ID:               id,
		ParentContextId:  parentContextId,
		ParentToolCallId: parentToolCallId,
		Task:             safeTask,
		Context:          safeContext,
		Status:           "running",
		CreatedAt:        now,
	}, nil
}

// Get retrieves a subagent session by ID.
func (s *SubagentStore) Get(id string) (*model.SubagentSession, error) {
	var ss model.SubagentSession
	var createdAt time.Time
	var completedAt sql.NullTime
	var messages, result, errorMsg sql.NullString

	query := `SELECT id, parent_context_id, parent_tool_call_id, task, context, status,
			  messages, result, error, created_at, completed_at
			  FROM subagent_sessions WHERE id = ?`

	err := s.db.QueryRow(query, id).Scan(
		&ss.ID, &ss.ParentContextId, &ss.ParentToolCallId, &ss.Task, &ss.Context,
		&ss.Status, &messages, &result, &errorMsg, &createdAt, &completedAt,
	)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	ss.CreatedAt = createdAt
	if completedAt.Valid {
		ss.CompletedAt = &completedAt.Time
	}
	if messages.Valid {
		ss.Messages = messages.String
	}
	if result.Valid {
		ss.Result = result.String
	}
	if errorMsg.Valid {
		ss.Error = errorMsg.String
	}

	return &ss, nil
}

// UpdateStatus changes the status of a subagent session.
func (s *SubagentStore) UpdateStatus(id, status string) error {
	now := time.Now()
	_, err := s.db.Exec(`UPDATE subagent_sessions SET status = ?, completed_at = ? WHERE id = ?`, status, now, id)
	return err
}

// UpdateMessages stores the message history as JSON.
func (s *SubagentStore) UpdateMessages(id, messagesJSON string) error {
	_, err := s.db.Exec(`UPDATE subagent_sessions SET messages = ? WHERE id = ?`, redact.Text(messagesJSON), id)
	return err
}

// Complete marks a subagent as completed with a result.
func (s *SubagentStore) Complete(id, result string) error {
	now := time.Now()
	_, err := s.db.Exec(`UPDATE subagent_sessions SET status = 'completed', result = ?, completed_at = ? WHERE id = ?`, redact.Text(result), now, id)
	return err
}

// Fail marks a subagent as failed with an error.
func (s *SubagentStore) Fail(id, errorMsg string) error {
	now := time.Now()
	_, err := s.db.Exec(`UPDATE subagent_sessions SET status = 'failed', error = ?, completed_at = ? WHERE id = ?`, redact.Text(errorMsg), now, id)
	return err
}

// ListByParent retrieves all subagents for a parent context.
func (s *SubagentStore) ListByParent(parentContextId string) ([]*model.SubagentSession, error) {
	query := `SELECT id, parent_context_id, parent_tool_call_id, task, context, status,
			  messages, result, error, created_at, completed_at
			  FROM subagent_sessions WHERE parent_context_id = ? ORDER BY created_at DESC`

	rows, err := s.db.Query(query, parentContextId)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []*model.SubagentSession
	for rows.Next() {
		var ss model.SubagentSession
		var createdAt time.Time
		var completedAt sql.NullTime
		var messages, resultVal, errorMsg sql.NullString

		if err := rows.Scan(
			&ss.ID, &ss.ParentContextId, &ss.ParentToolCallId, &ss.Task, &ss.Context,
			&ss.Status, &messages, &resultVal, &errorMsg, &createdAt, &completedAt,
		); err != nil {
			return nil, err
		}

		ss.CreatedAt = createdAt
		if completedAt.Valid {
			ss.CompletedAt = &completedAt.Time
		}
		if messages.Valid {
			ss.Messages = messages.String
		}
		if resultVal.Valid {
			ss.Result = resultVal.String
		}
		if errorMsg.Valid {
			ss.Error = errorMsg.String
		}

		result = append(result, &ss)
	}

	return result, nil
}
