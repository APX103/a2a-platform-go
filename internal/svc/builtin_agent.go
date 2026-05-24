package svc

import (
	"database/sql"
	"fmt"
	"time"

	"a2a-platform/internal/config"
)

// BuiltinAgentStore handles DB operations for builtin agents.
type BuiltinAgentStore struct {
	db *sql.DB
}

func NewBuiltinAgentStore(db *sql.DB) *BuiltinAgentStore {
	return &BuiltinAgentStore{db: db}
}

// BuiltinAgent represents a persisted builtin agent configuration.
type BuiltinAgent struct {
	ID            int64     `db:"id" json:"id"`
	Name          string    `db:"name" json:"name"`
	Provider      string    `db:"provider" json:"provider"`
	BaseURL       string    `db:"base_url" json:"base_url"`
	APIKey        string    `db:"api_key" json:"api_key"`
	Model         string    `db:"model" json:"model"`
	Description   string    `db:"description" json:"description"`
	SystemPrompt  string    `db:"system_prompt" json:"system_prompt"`
	MaxTokens     int       `db:"max_tokens" json:"max_tokens"`
	MaxToolRounds int       `db:"max_tool_rounds" json:"max_tool_rounds"`
	CreatedAt     time.Time `db:"created_at" json:"created_at"`
	UpdatedAt     time.Time `db:"updated_at" json:"updated_at"`
}

// ToConfig converts to config.BuiltinAgent.
func (b *BuiltinAgent) ToConfig() config.BuiltinAgent {
	return config.BuiltinAgent{
		Name:          b.Name,
		Provider:      b.Provider,
		BaseURL:       b.BaseURL,
		APIKey:        b.APIKey,
		Model:         b.Model,
		Description:   b.Description,
		SystemPrompt:  b.SystemPrompt,
		MaxTokens:     b.MaxTokens,
		MaxToolRounds: b.MaxToolRounds,
	}
}

// Create saves a new builtin agent.
func (s *BuiltinAgentStore) Create(cfg config.BuiltinAgent) (*BuiltinAgent, error) {
	query := `INSERT INTO builtin_agents (name, provider, base_url, api_key, model, description, system_prompt, max_tokens, max_tool_rounds)
			  VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`

	now := time.Now()
	res, err := s.db.Exec(query,
		cfg.Name, cfg.Provider, cfg.BaseURL, cfg.APIKey, cfg.Model,
		cfg.Description, cfg.SystemPrompt, cfg.MaxTokens, cfg.MaxToolRounds)
	if err != nil {
		return nil, fmt.Errorf("failed to create builtin agent: %w", err)
	}

	id, _ := res.LastInsertId()

	return &BuiltinAgent{
		ID:            id,
		Name:          cfg.Name,
		Provider:      cfg.Provider,
		BaseURL:       cfg.BaseURL,
		APIKey:        cfg.APIKey,
		Model:         cfg.Model,
		Description:   cfg.Description,
		SystemPrompt:  cfg.SystemPrompt,
		MaxTokens:     cfg.MaxTokens,
		MaxToolRounds: cfg.MaxToolRounds,
		CreatedAt:     now,
		UpdatedAt:     now,
	}, nil
}

// Get retrieves a builtin agent by name.
func (s *BuiltinAgentStore) Get(name string) (*BuiltinAgent, error) {
	var b BuiltinAgent
	var createdAt, updatedAt time.Time

	query := `SELECT id, name, provider, base_url, api_key, model, description, system_prompt, max_tokens, max_tool_rounds, created_at, updated_at
			  FROM builtin_agents WHERE name = ?`

	err := s.db.QueryRow(query, name).Scan(
		&b.ID, &b.Name, &b.Provider, &b.BaseURL, &b.APIKey, &b.Model, &b.Description,
		&b.SystemPrompt, &b.MaxTokens, &b.MaxToolRounds, &createdAt, &updatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	b.CreatedAt = createdAt
	b.UpdatedAt = updatedAt
	return &b, nil
}

// List retrieves all builtin agents.
func (s *BuiltinAgentStore) List() ([]*BuiltinAgent, error) {
	query := `SELECT id, name, provider, base_url, api_key, model, description, system_prompt, max_tokens, max_tool_rounds, created_at, updated_at
			  FROM builtin_agents ORDER BY created_at DESC`

	rows, err := s.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []*BuiltinAgent
	for rows.Next() {
		var b BuiltinAgent
		var createdAt, updatedAt time.Time
		if err := rows.Scan(
			&b.ID, &b.Name, &b.Provider, &b.BaseURL, &b.APIKey, &b.Model, &b.Description,
			&b.SystemPrompt, &b.MaxTokens, &b.MaxToolRounds, &createdAt, &updatedAt,
		); err != nil {
			return nil, err
		}
		b.CreatedAt = createdAt
		b.UpdatedAt = updatedAt
		result = append(result, &b)
	}

	return result, nil
}

// Delete removes a builtin agent by name.
func (s *BuiltinAgentStore) Delete(name string) error {
	_, err := s.db.Exec(`DELETE FROM builtin_agents WHERE name = ?`, name)
	return err
}

// Update updates an existing builtin agent.
func (s *BuiltinAgentStore) Update(cfg config.BuiltinAgent) error {
	query := `UPDATE builtin_agents SET provider = ?, base_url = ?, api_key = ?, model = ?, description = ?,
			  system_prompt = ?, max_tokens = ?, max_tool_rounds = ?, updated_at = ? WHERE name = ?`

	_, err := s.db.Exec(query,
		cfg.Provider, cfg.BaseURL, cfg.APIKey, cfg.Model, cfg.Description,
		cfg.SystemPrompt, cfg.MaxTokens, cfg.MaxToolRounds, time.Now(), cfg.Name)
	return err
}
