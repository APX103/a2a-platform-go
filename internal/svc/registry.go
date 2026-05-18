package svc

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"a2a-platform/internal/events"
	"a2a-platform/internal/model"
)

// AgentCard represents the A2A AgentCard fetched from /.well-known/agent.json
type AgentCard struct {
	Name        string      `json:"name"`
	Description string      `json:"description"`
	Version     string      `json:"version"`
	Url         string      `json:"url"`
	Skills      []CardSkill `json:"skills"`
	HealthUrl   string      `json:"health_url,omitempty"`
}

type CardSkill struct {
	Id          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Tags        []string `json:"tags"`
	Examples    []string `json:"examples"`
}

// AgentConnection wraps an agent's card and URL in memory.
type AgentConnection struct {
	mu   sync.RWMutex
	Card AgentCard
	Url  string
}

func (c *AgentConnection) Info() model.AgentInfo {
	c.mu.RLock()
	defer c.mu.RUnlock()
	skills := make([]model.Skill, 0, len(c.Card.Skills))
	for _, s := range c.Card.Skills {
		skills = append(skills, model.Skill{
			Id:          s.Id,
			Name:        s.Name,
			Description: s.Description,
			Tags:        s.Tags,
			Examples:    s.Examples,
		})
	}
	return model.AgentInfo{
		Name:        c.Card.Name,
		Description: c.Card.Description,
		Url:         "/agent/" + c.Card.Name,
		Version:     c.Card.Version,
		Skills:      skills,
	}
}

// AgentRegistry manages in-memory agent connections with DB persistence.
type AgentRegistry struct {
	store       *AgentStore
	connections map[string]*AgentConnection
	mu          sync.RWMutex
	failCounts  map[string]int
	failMu      sync.Mutex
	EventBus    *events.Broadcaster
}

func NewAgentRegistry(store *AgentStore) *AgentRegistry {
	return &AgentRegistry{
		store:       store,
		connections: make(map[string]*AgentConnection),
		failCounts:  make(map[string]int),
	}
}

// GetClient returns the in-memory connection for an agent.
func (r *AgentRegistry) GetClient(name string) *AgentConnection {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.connections[name]
}

// ListAgents returns all agents from DB, cross-referencing with live connections.
func (r *AgentRegistry) ListAgents() ([]model.AgentInfo, error) {
	records, err := r.store.List("")
	if err != nil {
		return nil, err
	}
	result := make([]model.AgentInfo, 0, len(records))
	for _, rec := range records {
		info := model.AgentInfo{
			Name:         rec.Name,
			Url:          "/agent/" + rec.Name,
			Status:       rec.Status,
			Type:         rec.Type,
			Skills:       ParseSkillsJson(rec.SkillsJson),
			ErrorMessage: rec.ErrorMessage,
		}
		conn := r.GetClient(rec.Name)
		if conn != nil {
			ci := conn.Info()
			info.Description = ci.Description
			info.Version = ci.Version
		}
		// If DB says connected but no live connection, fix status
		if rec.Status == "connected" && conn == nil {
			info.Status = "disconnected"
		}
		result = append(result, info)
	}
	return result, nil
}

// RegisterAgent performs full self-registration: validate → discover → ping → persist → connect.
func (r *AgentRegistry) RegisterAgent(name, agentType, url string, port int, skills []model.Skill, secret string) (*AgentConnection, error) {
	// 1. Check uniqueness / idempotency
	existing, err := r.store.Get(name)
	if err != nil {
		return nil, fmt.Errorf("DB error: %w", err)
	}
	if existing != nil {
		if secret != "" && existing.Secret == secret {
			// Idempotent re-registration — ok
		} else {
			return nil, fmt.Errorf("Agent '%s' already registered", name)
		}
	}

	// 2. Fetch AgentCard
	card, err := fetchAgentCard(url)
	if err != nil {
		return nil, fmt.Errorf("Cannot fetch agent card: %w", err)
	}

	// 3. Ping validation — send a lightweight A2A message
	if err := pingAgent(url); err != nil {
		return nil, fmt.Errorf("Agent unreachable: %w", err)
	}

	// 4. Update in-memory connection
	conn := &AgentConnection{Card: *card, Url: url}
	r.mu.Lock()
	r.connections[name] = conn
	r.mu.Unlock()

	// 5. Persist to DB
	now := time.Now().UTC().Format(time.RFC3339)
	skillsJson, _ := json.Marshal(skills)
	cardJson, _ := json.Marshal(card)
	dbRecord := &model.Agent{
		Name:          name,
		Type:          agentType,
		Url:           url,
		Port:          port,
		SkillsJson:    string(skillsJson),
		Status:        "connected",
		ConnectedAt:   &now,
		AgentCardJson: string(cardJson),
		Secret:        secret,
	}
	if err := r.store.Upsert(dbRecord); err != nil {
		return nil, fmt.Errorf("DB persist error: %w", err)
	}

	action := "Registered"
	if existing != nil {
		action = "Re-registered"
	}
	slog.Info("Agent registered", "action", action, "name", name, "url", url)
	if r.EventBus != nil {
		r.EventBus.AgentRegistered(name, "connected", agentType)
	}
	return conn, nil
}

// DisconnectAgent removes agent from connections and marks DB.
func (r *AgentRegistry) DisconnectAgent(name string) error {
	r.mu.Lock()
	delete(r.connections, name)
	r.mu.Unlock()
	r.failMu.Lock()
	delete(r.failCounts, name)
	r.failMu.Unlock()
	if r.EventBus != nil {
		r.EventBus.AgentStatus(name, "disconnected", "")
	}
	return r.store.UpdateStatus(name, "disconnected", nil)
}

// RestoreConnections reconnects agents from DB on startup.
func (r *AgentRegistry) RestoreConnections() {
	records, err := r.store.List("connected")
	if err != nil {
		slog.Error("RestoreConnections failed", "error", err)
		return
	}
	for _, rec := range records {
		card, err := fetchAgentCard(rec.Url)
		if err != nil {
			slog.Warn("Failed to restore agent", "name", rec.Name, "error", err)
			r.store.UpdateStatus(rec.Name, "disconnected", nil)
			continue
		}
		r.mu.Lock()
		r.connections[rec.Name] = &AgentConnection{Card: *card, Url: rec.Url}
		r.mu.Unlock()
		slog.Info("Restored connection to agent", "name", rec.Name, "url", rec.Url)
	}
}

// StartHealthCheck launches a background goroutine that periodically checks
// the health of all connected agents.
func (r *AgentRegistry) StartHealthCheck(interval time.Duration) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for range ticker.C {
			r.runHealthCheck()
		}
	}()
	slog.Info("Agent health check started", "interval", interval)
}

func (r *AgentRegistry) runHealthCheck() {
	r.mu.RLock()
	// Snapshot current connections
	type agentEntry struct {
		name string
		url  string
		card AgentCard
	}
	var agents []agentEntry
	for name, conn := range r.connections {
		agents = append(agents, agentEntry{name: name, url: conn.Url, card: conn.Card})
	}
	r.mu.RUnlock()

	client := &http.Client{Timeout: 5 * time.Second}

	for _, a := range agents {
		// Phase 1: check if bridge HTTP is reachable (agent card endpoint)
		cardURL := a.url + "/.well-known/agent.json"
		resp, err := client.Get(cardURL)
		if err != nil {
			// Bridge is completely unreachable → offline
			r.recordFailure(a.name, "offline", fmt.Sprintf("bridge unreachable: %v", err))
			continue
		}
		resp.Body.Close()
		if resp.StatusCode != 200 {
			r.recordFailure(a.name, "offline", fmt.Sprintf("agent card returned %d", resp.StatusCode))
			continue
		}

		// Phase 2: if bridge exposes a health_url, check downstream LLM health
		if a.card.HealthUrl != "" {
			healthResp, healthErr := client.Get(a.card.HealthUrl)
			if healthErr != nil {
				r.recordFailure(a.name, "unreachable", fmt.Sprintf("health check unreachable: %v", healthErr))
				continue
			}
			healthResp.Body.Close()
			if healthResp.StatusCode != 200 {
				r.recordFailure(a.name, "unreachable", fmt.Sprintf("health check returned %d", healthResp.StatusCode))
				continue
			}
		}

		// Success — reset failure count and ensure connected/online
		r.failMu.Lock()
		if r.failCounts[a.name] > 0 {
			slog.Info("Agent health check recovered", "name", a.name)
		}
		delete(r.failCounts, a.name)
		r.failMu.Unlock()
	}
}

func (r *AgentRegistry) recordFailure(name string, status string, reason string) {
	r.failMu.Lock()
	r.failCounts[name]++
	count := r.failCounts[name]
	r.failMu.Unlock()

	if count >= 3 {
		slog.Warn("Agent marked status after consecutive failures",
			"name", name, "status", status, "failures", count, "reason", reason)
		r.mu.Lock()
		delete(r.connections, name)
		r.mu.Unlock()
		r.store.UpdateStatus(name, status, &reason)
		if r.EventBus != nil {
			r.EventBus.AgentStatus(name, status, "")
		}
	}
}

// CountConnected returns the number of currently connected agents.
func (r *AgentRegistry) CountConnected() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.connections)
}

// CountTotal returns the total number of agents in DB.
func (r *AgentRegistry) CountTotal() (int, error) {
	records, err := r.store.List("")
	if err != nil {
		return 0, err
	}
	return len(records), nil
}

// ===== HTTP helpers =====

func fetchAgentCard(url string) (*AgentCard, error) {
	cardURL := url + "/.well-known/agent.json"
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(cardURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("agent card returned status %d", resp.StatusCode)
	}
	var card AgentCard
	if err := json.NewDecoder(resp.Body).Decode(&card); err != nil {
		return nil, err
	}
	return &card, nil
}

func pingAgent(url string) error {
	// Lightweight check: just verify the agent card endpoint is reachable
	cardURL := url + "/.well-known/agent.json"
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(cardURL)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("ping returned status %d", resp.StatusCode)
	}
	return nil
}
