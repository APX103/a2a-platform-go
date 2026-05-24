package svc

import (
	"database/sql"
	"fmt"
	"regexp"
	"strings"
	"time"

	"a2a-platform/internal/model"

	"github.com/google/uuid"
)

var handleRe = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]{2,63}$`)

type HumanUserStore struct {
	db *sql.DB
}

func NewHumanUserStore(db *sql.DB) *HumanUserStore {
	return &HumanUserStore{db: db}
}

func NormalizeHumanHandle(handle string) string {
	return strings.ToLower(strings.TrimSpace(handle))
}

func ValidateHumanHandle(handle string) error {
	if !handleRe.MatchString(handle) {
		return fmt.Errorf("handle must be 3-64 characters and contain only letters, numbers, dot, underscore, or dash")
	}
	return nil
}

func (s *HumanUserStore) Create(handle, displayName string) (*model.HumanUser, error) {
	handle = NormalizeHumanHandle(handle)
	if err := ValidateHumanHandle(handle); err != nil {
		return nil, err
	}
	displayName = strings.TrimSpace(displayName)
	if displayName == "" {
		displayName = handle
	}
	user := &model.HumanUser{
		ID:          uuid.NewString(),
		Handle:      handle,
		DisplayName: displayName,
	}
	_, err := s.db.Exec(
		`INSERT INTO human_users (id, handle, display_name, secret_hash, secret_salt)
		 VALUES (?, ?, ?, ?, ?)`,
		user.ID, user.Handle, user.DisplayName, user.SecretHash, user.SecretSalt,
	)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "duplicate") {
			return nil, fmt.Errorf("handle already exists")
		}
		return nil, err
	}
	return s.Get(user.ID)
}

func (s *HumanUserStore) Get(id string) (*model.HumanUser, error) {
	var user model.HumanUser
	var lastSeenAt sql.NullTime
	err := s.db.QueryRow(
		`SELECT id, handle, display_name, last_seen_at, secret_hash, secret_salt, created_at, updated_at
		 FROM human_users WHERE id = ?`,
		strings.TrimSpace(id),
	).Scan(&user.ID, &user.Handle, &user.DisplayName, &lastSeenAt, &user.SecretHash, &user.SecretSalt, &user.CreatedAt, &user.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if lastSeenAt.Valid {
		user.LastSeenAt = &lastSeenAt.Time
	}
	return &user, nil
}

func (s *HumanUserStore) GetByHandle(handle string) (*model.HumanUser, error) {
	var user model.HumanUser
	var lastSeenAt sql.NullTime
	err := s.db.QueryRow(
		`SELECT id, handle, display_name, last_seen_at, secret_hash, secret_salt, created_at, updated_at
		 FROM human_users WHERE handle = ?`,
		NormalizeHumanHandle(handle),
	).Scan(&user.ID, &user.Handle, &user.DisplayName, &lastSeenAt, &user.SecretHash, &user.SecretSalt, &user.CreatedAt, &user.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if lastSeenAt.Valid {
		user.LastSeenAt = &lastSeenAt.Time
	}
	return &user, nil
}

func (s *HumanUserStore) Update(id, handle, displayName string) (*model.HumanUser, error) {
	id = strings.TrimSpace(id)
	handle = NormalizeHumanHandle(handle)
	if err := ValidateHumanHandle(handle); err != nil {
		return nil, err
	}
	displayName = strings.TrimSpace(displayName)
	if displayName == "" {
		displayName = handle
	}
	res, err := s.db.Exec(
		`UPDATE human_users SET handle = ?, display_name = ?
		 WHERE id = ?`,
		handle, displayName, id,
	)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "duplicate") {
			return nil, fmt.Errorf("handle already exists")
		}
		return nil, err
	}
	if affected, affectedErr := res.RowsAffected(); affectedErr == nil && affected == 0 {
		return nil, nil
	}
	return s.Get(id)
}

func (s *HumanUserStore) Delete(id string) (bool, error) {
	id = strings.TrimSpace(id)
	tx, err := s.db.Begin()
	if err != nil {
		return false, err
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	if _, err := tx.Exec(
		`UPDATE human_sessions SET revoked_at = CURRENT_TIMESTAMP
		 WHERE human_id = ? AND revoked_at IS NULL`,
		id,
	); err != nil {
		return false, err
	}
	if _, err := tx.Exec(
		`UPDATE group_member_tokens SET revoked_at = CURRENT_TIMESTAMP
		 WHERE actor_type = ? AND actor_id = ? AND revoked_at IS NULL`,
		model.GroupActorHuman, id,
	); err != nil {
		return false, err
	}
	if _, err := tx.Exec(
		`DELETE FROM group_member_tokens WHERE actor_type = ? AND actor_id = ?`,
		model.GroupActorHuman, id,
	); err != nil {
		return false, err
	}
	if _, err := tx.Exec(
		`DELETE FROM group_members WHERE actor_type = ? AND actor_id = ?`,
		model.GroupActorHuman, id,
	); err != nil {
		return false, err
	}
	if _, err := tx.Exec(`DELETE FROM human_sessions WHERE human_id = ?`, id); err != nil {
		return false, err
	}
	res, err := tx.Exec(`DELETE FROM human_users WHERE id = ?`, id)
	if err != nil {
		return false, err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	committed = true
	return affected > 0, nil
}

func (s *HumanUserStore) TouchLastSeen(id string) error {
	_, err := s.db.Exec(
		`UPDATE human_users SET last_seen_at = CURRENT_TIMESTAMP
		 WHERE id = ?`,
		strings.TrimSpace(id),
	)
	return err
}

func (s *HumanUserStore) ListPresence(_ time.Time, onlineWindow time.Duration) ([]model.HumanPresence, error) {
	windowSeconds := int(onlineWindow.Seconds())
	if windowSeconds <= 0 {
		windowSeconds = 90
	}
	rows, err := s.db.Query(`
		SELECT
			u.id,
			u.handle,
			u.display_name,
			u.last_seen_at,
			u.created_at,
			u.updated_at,
			COUNT(s.id) AS active_sessions,
			CASE
			  WHEN u.last_seen_at IS NOT NULL
			   AND u.last_seen_at >= DATE_SUB(CURRENT_TIMESTAMP, INTERVAL ? SECOND)
			  THEN 1
			  ELSE 0
			END AS online
		FROM human_users u
		LEFT JOIN human_sessions s
		  ON s.human_id = u.id
		 AND s.revoked_at IS NULL
		 AND (s.expires_at IS NULL OR s.expires_at > CURRENT_TIMESTAMP)
		GROUP BY u.id, u.handle, u.display_name, u.last_seen_at, u.created_at, u.updated_at
		ORDER BY COALESCE(u.last_seen_at, u.updated_at, u.created_at) DESC, u.handle ASC`, windowSeconds)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make([]model.HumanPresence, 0)
	for rows.Next() {
		var item model.HumanPresence
		var lastSeenAt sql.NullTime
		var online int
		if err := rows.Scan(
			&item.ID,
			&item.Handle,
			&item.DisplayName,
			&lastSeenAt,
			&item.CreatedAt,
			&item.UpdatedAt,
			&item.ActiveSessions,
			&online,
		); err != nil {
			return nil, err
		}
		item.Status = "offline"
		if lastSeenAt.Valid {
			item.LastSeenAt = &lastSeenAt.Time
		}
		if online == 1 {
			item.Online = true
			item.Status = "online"
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

type HumanSessionStore struct {
	db *sql.DB
}

func NewHumanSessionStore(db *sql.DB) *HumanSessionStore {
	return &HumanSessionStore{db: db}
}

func (s *HumanSessionStore) Create(humanID string, ttl time.Duration) (*model.HumanSession, string, error) {
	plain, err := NewAccessToken()
	if err != nil {
		return nil, "", err
	}
	session := &model.HumanSession{
		HumanID:   strings.TrimSpace(humanID),
		TokenHash: HashAccessToken(plain),
	}
	if ttl > 0 {
		expiresAt := time.Now().Add(ttl).UTC()
		session.ExpiresAt = &expiresAt
	}
	res, err := s.db.Exec(
		`INSERT INTO human_sessions (human_id, token_hash, expires_at, revoked_at)
		 VALUES (?, ?, ?, ?)`,
		session.HumanID, session.TokenHash, session.ExpiresAt, session.RevokedAt,
	)
	if err != nil {
		return nil, "", err
	}
	if id, idErr := res.LastInsertId(); idErr == nil {
		session.ID = id
	}
	return session, plain, nil
}

func (s *HumanSessionStore) GetByToken(token string) (*model.HumanSession, error) {
	var session model.HumanSession
	var expiresAt, revokedAt sql.NullTime
	err := s.db.QueryRow(
		`SELECT id, human_id, token_hash, expires_at, revoked_at, created_at
		 FROM human_sessions WHERE token_hash = ?`,
		HashAccessToken(token),
	).Scan(&session.ID, &session.HumanID, &session.TokenHash, &expiresAt, &revokedAt, &session.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if expiresAt.Valid {
		session.ExpiresAt = &expiresAt.Time
	}
	if revokedAt.Valid {
		session.RevokedAt = &revokedAt.Time
	}
	return &session, nil
}

func (s *HumanSessionStore) Revoke(token string) error {
	_, err := s.db.Exec(
		`UPDATE human_sessions SET revoked_at = CURRENT_TIMESTAMP
		 WHERE token_hash = ? AND revoked_at IS NULL`,
		HashAccessToken(token),
	)
	return err
}

func HumanSessionUsable(session *model.HumanSession, now time.Time) bool {
	if session == nil || session.RevokedAt != nil {
		return false
	}
	if session.ExpiresAt != nil && !session.ExpiresAt.After(now) {
		return false
	}
	return true
}
