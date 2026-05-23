package svc

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

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
	if err != nil {
		return err
	}
	stored, err := s.Get(g.ID)
	if err != nil || stored == nil {
		return err
	}
	*g = *stored
	return nil
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

func (s *GroupStore) ListByActor(actorType, actorID, status string) ([]*model.Group, error) {
	query := `SELECT g.id, g.name, g.description, g.orchestration_mode, g.rules_json, g.memory_policy_json, g.status, g.created_at, g.updated_at
		FROM a2a_groups g
		INNER JOIN group_members m ON m.group_id = g.id
		WHERE m.actor_type = ? AND m.actor_id = ?`
	args := []interface{}{NormalizeActorType(actorType), strings.TrimSpace(actorID)}
	if status != "" {
		query += " AND g.status = ?"
		args = append(args, status)
	}
	query += " ORDER BY g.updated_at DESC, g.created_at DESC"
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
	case model.GroupModeP2P:
		return model.GroupModeP2P
	case model.GroupModeFreeChat:
		return model.GroupModeFreeChat
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
	if err != nil {
		return err
	}
	stored, err := s.Get(m.GroupID, m.ActorType, m.ActorID)
	if err != nil || stored == nil {
		return err
	}
	*m = *stored
	return nil
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

func (s *GroupMemberStore) Get(groupID, actorType, actorID string) (*model.GroupMember, error) {
	var m model.GroupMember
	var capabilities sql.NullString
	err := s.db.QueryRow(
		`SELECT id, group_id, actor_type, actor_id, role, capabilities_json, joined_at
		 FROM group_members WHERE group_id = ? AND actor_type = ? AND actor_id = ?`,
		groupID, NormalizeActorType(actorType), strings.TrimSpace(actorID),
	).Scan(&m.ID, &m.GroupID, &m.ActorType, &m.ActorID, &m.Role, &capabilities, &m.JoinedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if capabilities.Valid {
		m.CapabilitiesJson = capabilities.String
	}
	return &m, nil
}

func (s *GroupMemberStore) Delete(groupID, actorType, actorID string) error {
	_, err := s.db.Exec("DELETE FROM group_members WHERE group_id = ? AND actor_type = ? AND actor_id = ?", groupID, NormalizeActorType(actorType), actorID)
	return err
}

type GroupInviteStore struct {
	db *sql.DB
}

func NewGroupInviteStore(db *sql.DB) *GroupInviteStore {
	return &GroupInviteStore{db: db}
}

func (s *GroupInviteStore) Create(invite *model.GroupInvite) (string, error) {
	normalizeInvite(invite)
	token, err := NewAccessToken()
	if err != nil {
		return "", err
	}
	invite.TokenHash = HashAccessToken(token)
	res, err := s.db.Exec(
		`INSERT INTO group_invites (group_id, token_hash, actor_type_allowed, role, max_uses, used_count, expires_at, status)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		invite.GroupID, invite.TokenHash, nullableText(invite.ActorTypeAllowed), invite.Role, invite.MaxUses, invite.UsedCount, invite.ExpiresAt, invite.Status,
	)
	if err == nil {
		if id, idErr := res.LastInsertId(); idErr == nil {
			invite.ID = id
		}
	}
	if err != nil {
		return "", err
	}
	stored, err := s.GetByToken(token)
	if err != nil || stored == nil {
		return "", err
	}
	*invite = *stored
	return token, nil
}

func (s *GroupInviteStore) List(groupID string) ([]*model.GroupInvite, error) {
	rows, err := s.db.Query(
		`SELECT id, group_id, token_hash, actor_type_allowed, role, max_uses, used_count, expires_at, status, created_at
		 FROM group_invites WHERE group_id = ? ORDER BY created_at DESC, id DESC`,
		groupID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []*model.GroupInvite
	for rows.Next() {
		invite, err := scanInvite(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, invite)
	}
	return result, rows.Err()
}

func (s *GroupInviteStore) GetByToken(token string) (*model.GroupInvite, error) {
	var row = s.db.QueryRow(
		`SELECT id, group_id, token_hash, actor_type_allowed, role, max_uses, used_count, expires_at, status, created_at
		 FROM group_invites WHERE token_hash = ?`,
		HashAccessToken(token),
	)
	invite, err := scanInvite(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return invite, err
}

func (s *GroupInviteStore) Consume(id int64) error {
	_, err := s.db.Exec(`UPDATE group_invites SET used_count = used_count + 1 WHERE id = ?`, id)
	return err
}

type inviteScanner interface {
	Scan(dest ...interface{}) error
}

func scanInvite(scanner inviteScanner) (*model.GroupInvite, error) {
	var invite model.GroupInvite
	var actorTypeAllowed sql.NullString
	var expiresAt sql.NullTime
	err := scanner.Scan(&invite.ID, &invite.GroupID, &invite.TokenHash, &actorTypeAllowed, &invite.Role, &invite.MaxUses, &invite.UsedCount, &expiresAt, &invite.Status, &invite.CreatedAt)
	if err != nil {
		return nil, err
	}
	if actorTypeAllowed.Valid {
		invite.ActorTypeAllowed = actorTypeAllowed.String
	}
	if expiresAt.Valid {
		invite.ExpiresAt = &expiresAt.Time
	}
	return &invite, nil
}

func normalizeInvite(invite *model.GroupInvite) {
	invite.GroupID = strings.TrimSpace(invite.GroupID)
	invite.ActorTypeAllowed = strings.TrimSpace(invite.ActorTypeAllowed)
	if invite.ActorTypeAllowed != "" {
		invite.ActorTypeAllowed = NormalizeActorType(invite.ActorTypeAllowed)
	}
	if invite.Role == "" {
		invite.Role = "member"
	}
	if invite.MaxUses <= 0 {
		invite.MaxUses = 1
	}
	if invite.Status == "" {
		invite.Status = model.GroupStatusActive
	}
}

func InviteUsable(invite *model.GroupInvite, actorType string, now time.Time) bool {
	if invite == nil || invite.Status != model.GroupStatusActive {
		return false
	}
	if invite.ActorTypeAllowed != "" && invite.ActorTypeAllowed != NormalizeActorType(actorType) {
		return false
	}
	if invite.MaxUses > 0 && invite.UsedCount >= invite.MaxUses {
		return false
	}
	if invite.ExpiresAt != nil && !invite.ExpiresAt.After(now) {
		return false
	}
	return true
}

type GroupMemberTokenStore struct {
	db *sql.DB
}

func NewGroupMemberTokenStore(db *sql.DB) *GroupMemberTokenStore {
	return &GroupMemberTokenStore{db: db}
}

func (s *GroupMemberTokenStore) Create(token *model.GroupMemberToken) (string, error) {
	token.GroupID = strings.TrimSpace(token.GroupID)
	token.ActorType = NormalizeActorType(token.ActorType)
	token.ActorID = strings.TrimSpace(token.ActorID)
	plain, err := NewAccessToken()
	if err != nil {
		return "", err
	}
	token.TokenHash = HashAccessToken(plain)
	res, err := s.db.Exec(
		`INSERT INTO group_member_tokens (group_id, actor_type, actor_id, token_hash, expires_at, revoked_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		token.GroupID, token.ActorType, token.ActorID, token.TokenHash, token.ExpiresAt, token.RevokedAt,
	)
	if err == nil {
		if id, idErr := res.LastInsertId(); idErr == nil {
			token.ID = id
		}
	}
	return plain, err
}

func (s *GroupMemberTokenStore) GetByToken(token string) (*model.GroupMemberToken, error) {
	row := s.db.QueryRow(
		`SELECT id, group_id, actor_type, actor_id, token_hash, expires_at, revoked_at, created_at
		 FROM group_member_tokens WHERE token_hash = ?`,
		HashAccessToken(token),
	)
	memberToken, err := scanMemberToken(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return memberToken, err
}

func (s *GroupMemberTokenStore) RevokeActor(groupID, actorType, actorID string) error {
	_, err := s.db.Exec(
		`UPDATE group_member_tokens SET revoked_at = CURRENT_TIMESTAMP
		 WHERE group_id = ? AND actor_type = ? AND actor_id = ? AND revoked_at IS NULL`,
		groupID, NormalizeActorType(actorType), strings.TrimSpace(actorID),
	)
	return err
}

type memberTokenScanner interface {
	Scan(dest ...interface{}) error
}

func scanMemberToken(scanner memberTokenScanner) (*model.GroupMemberToken, error) {
	var token model.GroupMemberToken
	var expiresAt, revokedAt sql.NullTime
	err := scanner.Scan(&token.ID, &token.GroupID, &token.ActorType, &token.ActorID, &token.TokenHash, &expiresAt, &revokedAt, &token.CreatedAt)
	if err != nil {
		return nil, err
	}
	if expiresAt.Valid {
		token.ExpiresAt = &expiresAt.Time
	}
	if revokedAt.Valid {
		token.RevokedAt = &revokedAt.Time
	}
	return &token, nil
}

func MemberTokenUsable(token *model.GroupMemberToken, now time.Time) bool {
	if token == nil || token.RevokedAt != nil {
		return false
	}
	if token.ExpiresAt != nil && !token.ExpiresAt.After(now) {
		return false
	}
	return true
}

func NewAccessToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func HashAccessToken(token string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(token)))
	return hex.EncodeToString(sum[:])
}

func nullableText(value string) interface{} {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return strings.TrimSpace(value)
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
	if err != nil {
		return err
	}
	stored, err := s.Get(e.ID)
	if err != nil || stored == nil {
		return err
	}
	*e = *stored
	return nil
}

func (s *GroupEventStore) Get(id int64) (*model.GroupEvent, error) {
	var e model.GroupEvent
	var metadata sql.NullString
	err := s.db.QueryRow(
		`SELECT id, group_id, event_type, sender_type, sender_id, content, metadata_json, created_at
		 FROM group_events WHERE id = ?`,
		id,
	).Scan(&e.ID, &e.GroupID, &e.EventType, &e.SenderType, &e.SenderID, &e.Content, &metadata, &e.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if metadata.Valid {
		e.MetadataJson = metadata.String
	}
	return &e, nil
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
	if err != nil {
		return err
	}
	stored, err := s.Get(a.ID)
	if err != nil || stored == nil {
		return err
	}
	*a = *stored
	return nil
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

func (s *GroupArtifactStore) GetByName(groupID, name string) (*model.GroupArtifact, error) {
	var a model.GroupArtifact
	var content, createdBy sql.NullString
	err := s.db.QueryRow(
		`SELECT id, group_id, name, artifact_type, version, content, status, created_by, created_at, updated_at
		 FROM group_artifacts WHERE group_id = ? AND name = ?`,
		groupID, name,
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

func (s *GroupArtifactStore) UpsertByName(a *model.GroupArtifact) error {
	if a.GroupID == "" {
		return fmt.Errorf("group id is required")
	}
	if strings.TrimSpace(a.Name) == "" {
		return fmt.Errorf("artifact name is required")
	}
	existing, err := s.GetByName(a.GroupID, a.Name)
	if err != nil {
		return err
	}
	if existing == nil {
		return s.Create(a)
	}
	a.ID = existing.ID
	if a.ArtifactType == "" {
		a.ArtifactType = existing.ArtifactType
	}
	if a.Status == "" {
		a.Status = existing.Status
	}
	if err := s.Update(a); err != nil {
		return err
	}
	stored, err := s.Get(a.ID)
	if err != nil || stored == nil {
		return err
	}
	*a = *stored
	return nil
}

func BuildGroupOrchestrationState(group *model.Group, members []*model.GroupMember) model.GroupOrchestrationState {
	state := model.GroupOrchestrationState{
		GroupID: group.ID,
		Mode:    group.OrchestrationMode,
	}

	switch group.OrchestrationMode {
	case model.GroupModeP2P:
		state.NextAction = "p2p_only"
		state.ContextPolicy = "members can discover each other and start direct agent-to-agent calls; group chat broadcast is disabled"
		state.TerminationPolicy = "no group orchestration is run for p2p network messages"
		state.EligibleSpeakers = groupSpeakers(members, false)
	case model.GroupModeFreeChat:
		state.NextAction = "agents_observe_and_optionally_reply"
		state.ContextPolicy = "each agent receives recent room messages, group rules, and the latest message; each decides whether to reply"
		state.TerminationPolicy = "stop after one bounded reaction wave or max_speakers is reached"
		state.EligibleSpeakers = groupSpeakers(members, false)
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
