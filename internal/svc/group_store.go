package svc

import (
	"database/sql"
	"fmt"
	"strings"

	"a2a-platform/internal/model"

	"github.com/google/uuid"
)

type GroupStore struct {
	db *sql.DB
}

func NewGroupStore(db *sql.DB) *GroupStore {
	return &GroupStore{db: db}
}

func NewGroupID() string {
	return uuid.New().String()
}

func (s *GroupStore) Create(g *model.Group) error {
	normalizeGroup(g)
	_, err := s.db.Exec(
		`INSERT INTO a2a_groups (id, name, description, orchestration_mode, rules_json, memory_policy_json, status)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		g.ID, g.Name, g.Description, g.OrchestrationMode, g.RulesJson, g.MemoryPolicyJson, g.Status,
	)
	return err
}

func (s *GroupStore) Get(id string) (*model.Group, error) {
	var g model.Group
	var description, rulesJson, memoryPolicyJson sql.NullString
	err := s.db.QueryRow(
		`SELECT id, name, description, orchestration_mode, rules_json, memory_policy_json, status, created_at, updated_at
		 FROM a2a_groups WHERE id = ?`,
		id,
	).Scan(&g.ID, &g.Name, &description, &g.OrchestrationMode, &rulesJson, &memoryPolicyJson, &g.Status, &g.CreatedAt, &g.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if description.Valid {
		g.Description = description.String
	}
	if rulesJson.Valid {
		g.RulesJson = rulesJson.String
	}
	if memoryPolicyJson.Valid {
		g.MemoryPolicyJson = memoryPolicyJson.String
	}
	return &g, nil
}

func (s *GroupStore) List(status string) ([]*model.Group, error) {
	query := `SELECT id, name, description, orchestration_mode, rules_json, memory_policy_json, status, created_at, updated_at FROM a2a_groups`
	args := []interface{}{}
	if status != "" {
		query += " WHERE status = ?"
		args = append(args, status)
	}
	query += " ORDER BY updated_at DESC, created_at DESC"
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []*model.Group
	for rows.Next() {
		var g model.Group
		var description, rulesJson, memoryPolicyJson sql.NullString
		if err := rows.Scan(&g.ID, &g.Name, &description, &g.OrchestrationMode, &rulesJson, &memoryPolicyJson, &g.Status, &g.CreatedAt, &g.UpdatedAt); err != nil {
			return nil, err
		}
		if description.Valid {
			g.Description = description.String
		}
		if rulesJson.Valid {
			g.RulesJson = rulesJson.String
		}
		if memoryPolicyJson.Valid {
			g.MemoryPolicyJson = memoryPolicyJson.String
		}
		result = append(result, &g)
	}
	return result, rows.Err()
}

func (s *GroupStore) Update(g *model.Group) error {
	normalizeGroup(g)
	_, err := s.db.Exec(
		`UPDATE a2a_groups
		 SET name = ?, description = ?, orchestration_mode = ?, rules_json = ?, memory_policy_json = ?, status = ?, updated_at = CURRENT_TIMESTAMP
		 WHERE id = ?`,
		g.Name, g.Description, g.OrchestrationMode, g.RulesJson, g.MemoryPolicyJson, g.Status, g.ID,
	)
	return err
}

func (s *GroupStore) Archive(id string) error {
	_, err := s.db.Exec("UPDATE a2a_groups SET status = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?", model.GroupStatusArchived, id)
	return err
}

func normalizeGroup(g *model.Group) {
	if g.ID == "" {
		g.ID = NewGroupID()
	}
	g.Name = strings.TrimSpace(g.Name)
	g.OrchestrationMode = NormalizeGroupMode(g.OrchestrationMode)
	if g.Status == "" {
		g.Status = model.GroupStatusActive
	}
}

func NormalizeGroupMode(mode string) string {
	switch strings.TrimSpace(mode) {
	case model.GroupModeRoundtable:
		return model.GroupModeRoundtable
	case model.GroupModeStateflow:
		return model.GroupModeStateflow
	case model.GroupModeResearchLongRun:
		return model.GroupModeResearchLongRun
	default:
		return model.GroupModeLeaderLed
	}
}

func NormalizeActorType(actorType string) string {
	switch strings.TrimSpace(actorType) {
	case model.GroupActorHuman:
		return model.GroupActorHuman
	case model.GroupActorSystem:
		return model.GroupActorSystem
	default:
		return model.GroupActorAgent
	}
}

type GroupMemberStore struct {
	db *sql.DB
}

func NewGroupMemberStore(db *sql.DB) *GroupMemberStore {
	return &GroupMemberStore{db: db}
}

func (s *GroupMemberStore) Upsert(m *model.GroupMember) error {
	m.ActorType = NormalizeActorType(m.ActorType)
	m.ActorID = strings.TrimSpace(m.ActorID)
	if m.Role == "" {
		m.Role = "member"
	}
	var query string
	if DBDriver == "mysql" {
		query = `INSERT INTO group_members (group_id, actor_type, actor_id, role, capabilities_json)
			 VALUES (?, ?, ?, ?, ?)
			 ON DUPLICATE KEY UPDATE role = VALUES(role), capabilities_json = VALUES(capabilities_json)`
	} else {
		query = `INSERT INTO group_members (group_id, actor_type, actor_id, role, capabilities_json)
			 VALUES (?, ?, ?, ?, ?)
			 ON CONFLICT(group_id, actor_type, actor_id) DO UPDATE SET
			   role = excluded.role, capabilities_json = excluded.capabilities_json`
	}
	_, err := s.db.Exec(query, m.GroupID, m.ActorType, m.ActorID, m.Role, m.CapabilitiesJson)
	return err
}

func (s *GroupMemberStore) List(groupID string) ([]*model.GroupMember, error) {
	rows, err := s.db.Query(
		`SELECT id, group_id, actor_type, actor_id, role, capabilities_json, joined_at
		 FROM group_members WHERE group_id = ? ORDER BY role, actor_type, actor_id`,
		groupID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []*model.GroupMember
	for rows.Next() {
		var m model.GroupMember
		var capabilities sql.NullString
		if err := rows.Scan(&m.ID, &m.GroupID, &m.ActorType, &m.ActorID, &m.Role, &capabilities, &m.JoinedAt); err != nil {
			return nil, err
		}
		if capabilities.Valid {
			m.CapabilitiesJson = capabilities.String
		}
		result = append(result, &m)
	}
	return result, rows.Err()
}

func (s *GroupMemberStore) Delete(groupID, actorType, actorID string) error {
	_, err := s.db.Exec("DELETE FROM group_members WHERE group_id = ? AND actor_type = ? AND actor_id = ?", groupID, NormalizeActorType(actorType), actorID)
	return err
}

type GroupEventStore struct {
	db *sql.DB
}

func NewGroupEventStore(db *sql.DB) *GroupEventStore {
	return &GroupEventStore{db: db}
}

func (s *GroupEventStore) Append(e *model.GroupEvent) error {
	e.EventType = strings.TrimSpace(e.EventType)
	if e.EventType == "" {
		e.EventType = "message"
	}
	e.SenderType = NormalizeActorType(e.SenderType)
	res, err := s.db.Exec(
		`INSERT INTO group_events (group_id, event_type, sender_type, sender_id, content, metadata_json)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		e.GroupID, e.EventType, e.SenderType, e.SenderID, e.Content, e.MetadataJson,
	)
	if err == nil {
		if id, idErr := res.LastInsertId(); idErr == nil {
			e.ID = id
		}
	}
	return err
}

func (s *GroupEventStore) List(groupID string, limit int) ([]*model.GroupEvent, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := s.db.Query(
		`SELECT id, group_id, event_type, sender_type, sender_id, content, metadata_json, created_at
		 FROM group_events WHERE group_id = ? ORDER BY id DESC LIMIT ?`,
		groupID, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var reversed []*model.GroupEvent
	for rows.Next() {
		var e model.GroupEvent
		var metadata sql.NullString
		if err := rows.Scan(&e.ID, &e.GroupID, &e.EventType, &e.SenderType, &e.SenderID, &e.Content, &metadata, &e.CreatedAt); err != nil {
			return nil, err
		}
		if metadata.Valid {
			e.MetadataJson = metadata.String
		}
		reversed = append(reversed, &e)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for i, j := 0, len(reversed)-1; i < j; i, j = i+1, j-1 {
		reversed[i], reversed[j] = reversed[j], reversed[i]
	}
	return reversed, nil
}

type GroupArtifactStore struct {
	db *sql.DB
}

func NewGroupArtifactStore(db *sql.DB) *GroupArtifactStore {
	return &GroupArtifactStore{db: db}
}

func (s *GroupArtifactStore) Create(a *model.GroupArtifact) error {
	if a.ID == "" {
		a.ID = uuid.New().String()
	}
	if a.ArtifactType == "" {
		a.ArtifactType = "document"
	}
	if a.Status == "" {
		a.Status = "draft"
	}
	if a.Version == 0 {
		a.Version = 1
	}
	_, err := s.db.Exec(
		`INSERT INTO group_artifacts (id, group_id, name, artifact_type, version, content, status, created_by)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		a.ID, a.GroupID, a.Name, a.ArtifactType, a.Version, a.Content, a.Status, a.CreatedBy,
	)
	return err
}

func (s *GroupArtifactStore) Get(id string) (*model.GroupArtifact, error) {
	var a model.GroupArtifact
	var content, createdBy sql.NullString
	err := s.db.QueryRow(
		`SELECT id, group_id, name, artifact_type, version, content, status, created_by, created_at, updated_at
		 FROM group_artifacts WHERE id = ?`,
		id,
	).Scan(&a.ID, &a.GroupID, &a.Name, &a.ArtifactType, &a.Version, &content, &a.Status, &createdBy, &a.CreatedAt, &a.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if content.Valid {
		a.Content = content.String
	}
	if createdBy.Valid {
		a.CreatedBy = createdBy.String
	}
	return &a, nil
}

func (s *GroupArtifactStore) List(groupID string) ([]*model.GroupArtifact, error) {
	rows, err := s.db.Query(
		`SELECT id, group_id, name, artifact_type, version, content, status, created_by, created_at, updated_at
		 FROM group_artifacts WHERE group_id = ? ORDER BY updated_at DESC, created_at DESC`,
		groupID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []*model.GroupArtifact
	for rows.Next() {
		var a model.GroupArtifact
		var content, createdBy sql.NullString
		if err := rows.Scan(&a.ID, &a.GroupID, &a.Name, &a.ArtifactType, &a.Version, &content, &a.Status, &createdBy, &a.CreatedAt, &a.UpdatedAt); err != nil {
			return nil, err
		}
		if content.Valid {
			a.Content = content.String
		}
		if createdBy.Valid {
			a.CreatedBy = createdBy.String
		}
		result = append(result, &a)
	}
	return result, rows.Err()
}

func (s *GroupArtifactStore) Update(a *model.GroupArtifact) error {
	if a.ID == "" {
		return fmt.Errorf("artifact id is required")
	}
	_, err := s.db.Exec(
		`UPDATE group_artifacts
		 SET name = ?, artifact_type = ?, version = version + 1, content = ?, status = ?, updated_at = CURRENT_TIMESTAMP
		 WHERE id = ? AND group_id = ?`,
		a.Name, a.ArtifactType, a.Content, a.Status, a.ID, a.GroupID,
	)
	return err
}

func BuildGroupOrchestrationState(group *model.Group, members []*model.GroupMember) model.GroupOrchestrationState {
	state := model.GroupOrchestrationState{
		GroupID: group.ID,
		Mode:    group.OrchestrationMode,
	}

	switch group.OrchestrationMode {
	case model.GroupModeRoundtable:
		state.NextAction = "collect_member_intents"
		state.ContextPolicy = "all members receive the shared artifact, rolling summary, open questions, and recent key events"
		state.TerminationPolicy = "finish when every required reviewer votes approved or max_rounds is reached"
		state.EligibleSpeakers = groupSpeakers(members, false)
	case model.GroupModeStateflow:
		state.NextAction = "advance_configured_phase"
		state.ContextPolicy = "only members assigned to the current phase receive phase-specific context"
		state.TerminationPolicy = "finish when the configured terminal phase emits a final artifact"
		state.EligibleSpeakers = groupSpeakers(members, false)
	case model.GroupModeResearchLongRun:
		state.NextAction = "checkpoint_or_dispatch_workstream"
		state.ContextPolicy = "members receive their workstream state, relevant retrieval memory, artifacts, and checkpoint summaries"
		state.TerminationPolicy = "finish only after evidence, critic, reproduction, and human checkpoint gates pass"
		state.EligibleSpeakers = groupSpeakers(members, false)
	default:
		state.NextAction = "leader_selects_next_speaker"
		state.ContextPolicy = "leader receives full room state; selected members receive task-specific context slices"
		state.TerminationPolicy = "leader publishes final summary or max_rounds is reached"
		state.EligibleSpeakers = groupSpeakers(members, true)
	}
	return state
}

func groupSpeakers(members []*model.GroupMember, leadersOnly bool) []string {
	speakers := []string{}
	for _, member := range members {
		if member.ActorType != model.GroupActorAgent {
			continue
		}
		if leadersOnly && member.Role != "leader" {
			continue
		}
		speakers = append(speakers, member.ActorID)
	}
	if leadersOnly && len(speakers) > 0 {
		return speakers
	}
	if leadersOnly {
		return groupSpeakers(members, false)
	}
	return speakers
}
