package svc

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"a2a-platform/internal/model"

	"github.com/google/uuid"
)

// AgentStore handles DB operations for agents.
type AgentStore struct {
	db *sql.DB
	mu sync.RWMutex
}

func NewAgentStore(db *sql.DB) *AgentStore {
	return &AgentStore{db: db}
}

func (s *AgentStore) Upsert(a *model.Agent) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.db.Exec(
		`INSERT INTO agents (name, type, url, port, skills_json, status, connected_at, agent_card_json, error_message, secret)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON DUPLICATE KEY UPDATE
		   type=VALUES(type), url=VALUES(url), port=VALUES(port),
		   skills_json=VALUES(skills_json), status=VALUES(status),
		   connected_at=VALUES(connected_at), agent_card_json=VALUES(agent_card_json),
		   error_message=VALUES(error_message), secret=VALUES(secret)`,
		a.Name, a.Type, a.Url, a.Port, a.SkillsJson, a.Status,
		a.ConnectedAt, a.AgentCardJson, a.ErrorMessage, a.Secret,
	)
	return err
}

func (s *AgentStore) Get(name string) (*model.Agent, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var a model.Agent
	var connectedAt, errorMessage sql.NullString
	err := s.db.QueryRow(
		"SELECT id, name, type, url, port, skills_json, status, connected_at, agent_card_json, error_message, secret FROM agents WHERE name=?",
		name,
	).Scan(&a.Id, &a.Name, &a.Type, &a.Url, &a.Port, &a.SkillsJson, &a.Status, &connectedAt, &a.AgentCardJson, &errorMessage, &a.Secret)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if connectedAt.Valid {
		a.ConnectedAt = &connectedAt.String
	}
	if errorMessage.Valid {
		a.ErrorMessage = &errorMessage.String
	}
	return &a, nil
}

func (s *AgentStore) List(status string) ([]*model.Agent, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var rows *sql.Rows
	var err error
	if status != "" {
		rows, err = s.db.Query("SELECT id, name, type, url, port, skills_json, status, connected_at, agent_card_json, error_message, secret FROM agents WHERE status=?", status)
	} else {
		rows, err = s.db.Query("SELECT id, name, type, url, port, skills_json, status, connected_at, agent_card_json, error_message, secret FROM agents")
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []*model.Agent
	for rows.Next() {
		var a model.Agent
		var connectedAt, errorMessage sql.NullString
		if err := rows.Scan(&a.Id, &a.Name, &a.Type, &a.Url, &a.Port, &a.SkillsJson, &a.Status, &connectedAt, &a.AgentCardJson, &errorMessage, &a.Secret); err != nil {
			return nil, err
		}
		if connectedAt.Valid {
			a.ConnectedAt = &connectedAt.String
		}
		if errorMessage.Valid {
			a.ErrorMessage = &errorMessage.String
		}
		result = append(result, &a)
	}
	return result, nil
}

func (s *AgentStore) UpdateStatus(name string, status string, errorMsg *string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.db.Exec("UPDATE agents SET status=?, error_message=? WHERE name=?", status, errorMsg, name)
	return err
}

func (s *AgentStore) Delete(name string) error {
	_, err := s.db.Exec("DELETE FROM agents WHERE name=?", name)
	return err
}

// ===== TaskStore =====

type TaskStore struct {
	db *sql.DB
}

func NewTaskStore(db *sql.DB) *TaskStore {
	return &TaskStore{db: db}
}

func (s *TaskStore) Create(t *model.Task) error {
	_, err := s.db.Exec(
		"INSERT INTO tasks (local_task_id, server_task_id, agent_name, context_id, state) VALUES (?, ?, ?, ?, ?)",
		t.LocalTaskId, t.ServerTaskId, t.AgentName, t.ContextId, t.State,
	)
	return err
}

func (s *TaskStore) Update(localTaskId string, fields map[string]interface{}) error {
	if len(fields) == 0 {
		return nil
	}
	setClauses := ""
	args := []interface{}{}
	for k, v := range fields {
		if setClauses != "" {
			setClauses += ", "
		}
		setClauses += fmt.Sprintf("%s=?", k)
		args = append(args, v)
	}
	args = append(args, localTaskId)
	_, err := s.db.Exec(fmt.Sprintf("UPDATE tasks SET %s WHERE local_task_id=?", setClauses), args...)
	return err
}

func (s *TaskStore) Get(localTaskId string) (*model.Task, error) {
	var t model.Task
	var serverTaskId, contextId sql.NullString
	err := s.db.QueryRow(
		"SELECT id, local_task_id, server_task_id, agent_name, context_id, state, created_at, updated_at FROM tasks WHERE local_task_id=?",
		localTaskId,
	).Scan(&t.Id, &t.LocalTaskId, &serverTaskId, &t.AgentName, &contextId, &t.State, &t.CreatedAt, &t.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if serverTaskId.Valid {
		t.ServerTaskId = &serverTaskId.String
	}
	if contextId.Valid {
		t.ContextId = &contextId.String
	}
	return &t, nil
}

func (s *TaskStore) List(agentName, state, search string, page, size int) ([]*model.Task, int64, error) {
	where := ""
	args := []interface{}{}
	conditions := []string{}
	if agentName != "" {
		conditions = append(conditions, "agent_name=?")
		args = append(args, agentName)
	}
	if state != "" {
		conditions = append(conditions, "state=?")
		args = append(args, state)
	}
	if search != "" {
		like := "%" + search + "%"
		conditions = append(conditions, "(local_task_id LIKE ? OR server_task_id LIKE ? OR context_id LIKE ?)")
		args = append(args, like, like, like)
	}
	if len(conditions) > 0 {
		where = " WHERE " + joinStrings(conditions, " AND ")
	}

	// Count
	var total int64
	countSQL := "SELECT COUNT(*) FROM tasks" + where
	if err := s.db.QueryRow(countSQL, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	// Query
	offset := (page - 1) * size
	querySQL := "SELECT id, local_task_id, server_task_id, agent_name, context_id, state, created_at, updated_at FROM tasks" +
		where + " ORDER BY created_at DESC LIMIT ? OFFSET ?"
	queryArgs := append(args, size, offset)
	rows, err := s.db.Query(querySQL, queryArgs...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var result []*model.Task
	for rows.Next() {
		var t model.Task
		var serverTaskId, contextId sql.NullString
		if err := rows.Scan(&t.Id, &t.LocalTaskId, &serverTaskId, &t.AgentName, &contextId, &t.State, &t.CreatedAt, &t.UpdatedAt); err != nil {
			return nil, 0, err
		}
		if serverTaskId.Valid {
			t.ServerTaskId = &serverTaskId.String
		}
		if contextId.Valid {
			t.ContextId = &contextId.String
		}
		result = append(result, &t)
	}
	return result, total, nil
}

func (s *TaskStore) GetByContext(contextId string) (*model.Task, error) {
	var t model.Task
	var serverTaskId, ctxId sql.NullString
	err := s.db.QueryRow(
		"SELECT id, local_task_id, server_task_id, agent_name, context_id, state, created_at, updated_at FROM tasks WHERE context_id=? ORDER BY updated_at DESC LIMIT 1",
		contextId,
	).Scan(&t.Id, &t.LocalTaskId, &serverTaskId, &t.AgentName, &ctxId, &t.State, &t.CreatedAt, &t.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if serverTaskId.Valid {
		t.ServerTaskId = &serverTaskId.String
	}
	if ctxId.Valid {
		t.ContextId = &ctxId.String
	}
	return &t, nil
}

// NewTaskId generates a new UUID task ID.
func NewTaskId() string {
	return uuid.New().String()
}

// ===== MessageStore =====

type MessageStore struct {
	db *sql.DB
}

func NewMessageStore(db *sql.DB) *MessageStore {
	return &MessageStore{db: db}
}

func (s *MessageStore) Append(m *model.Message) error {
	_, err := s.db.Exec(
		"INSERT INTO messages (task_id, role, content) VALUES (?, ?, ?)",
		m.TaskId, m.Role, m.Content,
	)
	return err
}

func (s *MessageStore) GetByTask(taskId string) ([]*model.Message, error) {
	rows, err := s.db.Query("SELECT id, task_id, role, content, timestamp FROM messages WHERE task_id=? ORDER BY timestamp", taskId)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []*model.Message
	for rows.Next() {
		var m model.Message
		if err := rows.Scan(&m.Id, &m.TaskId, &m.Role, &m.Content, &m.Timestamp); err != nil {
			return nil, err
		}
		result = append(result, &m)
	}
	return result, nil
}

func (s *MessageStore) GetByContext(contextId string) ([]*model.Message, error) {
	rows, err := s.db.Query(
		"SELECT m.id, m.task_id, m.role, m.content, m.timestamp FROM messages m JOIN tasks t ON m.task_id=t.local_task_id WHERE t.context_id=? ORDER BY m.timestamp",
		contextId,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []*model.Message
	for rows.Next() {
		var m model.Message
		if err := rows.Scan(&m.Id, &m.TaskId, &m.Role, &m.Content, &m.Timestamp); err != nil {
			return nil, err
		}
		result = append(result, &m)
	}
	return result, nil
}

// ===== TraceStore =====

type TraceStore struct {
	db *sql.DB
}

func NewTraceStore(db *sql.DB) *TraceStore {
	return &TraceStore{db: db}
}

func (s *TraceStore) Append(e *model.TraceEvent) error {
	_, err := s.db.Exec(
		"INSERT INTO traces (task_id, context_id, event_type, agent_name, target_agent, data_json, duration_ms) VALUES (?, ?, ?, ?, ?, ?, ?)",
		e.TaskId, e.ContextId, e.EventType, e.AgentName, e.TargetAgent, e.DataJson, e.DurationMs,
	)
	return err
}

func (s *TraceStore) GetByTask(taskId string) ([]*model.TraceEvent, error) {
	rows, err := s.db.Query("SELECT id, task_id, context_id, timestamp, event_type, agent_name, target_agent, data_json, duration_ms FROM traces WHERE task_id=? ORDER BY timestamp", taskId)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []*model.TraceEvent
	for rows.Next() {
		var e model.TraceEvent
		var contextId, targetAgent sql.NullString
		var durationMs sql.NullInt64
		if err := rows.Scan(&e.Id, &e.TaskId, &contextId, &e.Timestamp, &e.EventType, &e.AgentName, &targetAgent, &e.DataJson, &durationMs); err != nil {
			return nil, err
		}
		if contextId.Valid {
			e.ContextId = &contextId.String
		}
		if targetAgent.Valid {
			e.TargetAgent = &targetAgent.String
		}
		if durationMs.Valid {
			e.DurationMs = &durationMs.Int64
		}
		result = append(result, &e)
	}
	return result, nil
}

func (s *TraceStore) GetByAgent(agentName string, limit int) ([]*model.TraceEvent, error) {
	rows, err := s.db.Query("SELECT id, task_id, context_id, timestamp, event_type, agent_name, target_agent, data_json, duration_ms FROM traces WHERE agent_name=? ORDER BY timestamp DESC LIMIT ?", agentName, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanTraces(rows)
}

func (s *TraceStore) GetByContext(contextId string) ([]*model.TraceEvent, error) {
	var rows *sql.Rows
	var err error
	if contextId == "" {
		rows, err = s.db.Query("SELECT id, task_id, context_id, timestamp, event_type, agent_name, target_agent, data_json, duration_ms FROM traces WHERE context_id IS NULL ORDER BY timestamp")
	} else {
		rows, err = s.db.Query("SELECT id, task_id, context_id, timestamp, event_type, agent_name, target_agent, data_json, duration_ms FROM traces WHERE context_id=? ORDER BY timestamp", contextId)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanTraces(rows)
}

func (s *TraceStore) ListRecent(limit int) ([]*model.TraceEvent, error) {
	rows, err := s.db.Query(
		"SELECT id, task_id, context_id, timestamp, event_type, agent_name, target_agent, data_json, duration_ms FROM traces ORDER BY timestamp DESC LIMIT ?",
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanTraces(rows)
}

func (s *TraceStore) ListContexts(limit int) ([]*model.TraceContextSummary, error) {
	rows, err := s.db.Query(
		`SELECT 
			COALESCE(context_id, '') as context_id,
			COUNT(*) as trace_count,
			MAX(timestamp) as last_active,
			GROUP_CONCAT(DISTINCT agent_name) as agents
		FROM traces
		GROUP BY COALESCE(context_id, '')
		ORDER BY last_active DESC
		LIMIT ?`,
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []*model.TraceContextSummary
	for rows.Next() {
		var cs model.TraceContextSummary
		var agentsStr sql.NullString
		if err := rows.Scan(&cs.ContextId, &cs.TraceCount, &cs.LastActive, &agentsStr); err != nil {
			return nil, err
		}
		if agentsStr.Valid && agentsStr.String != "" {
			cs.Agents = strings.Split(agentsStr.String, ",")
		}
		result = append(result, &cs)
	}
	return result, nil
}

func scanTraces(rows *sql.Rows) ([]*model.TraceEvent, error) {
	var result []*model.TraceEvent
	for rows.Next() {
		var e model.TraceEvent
		var contextId, targetAgent sql.NullString
		var durationMs sql.NullInt64
		if err := rows.Scan(&e.Id, &e.TaskId, &contextId, &e.Timestamp, &e.EventType, &e.AgentName, &targetAgent, &e.DataJson, &durationMs); err != nil {
			return nil, err
		}
		if contextId.Valid {
			e.ContextId = &contextId.String
		}
		if targetAgent.Valid {
			e.TargetAgent = &targetAgent.String
		}
		if durationMs.Valid {
			e.DurationMs = &durationMs.Int64
		}
		result = append(result, &e)
	}
	return result, nil
}

// ===== Helpers =====

func joinStrings(ss []string, sep string) string {
	result := ""
	for i, s := range ss {
		if i > 0 {
			result += sep
		}
		result += s
	}
	return result
}

func ParseSkillsJson(s string) []model.Skill {
	var skills []model.Skill
	if s == "" {
		return skills
	}
	json.Unmarshal([]byte(s), &skills)
	return skills
}
