package svc

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"a2a-platform/internal/events"
	"a2a-platform/internal/model"
)

type AgentCard = model.AgentCard
type CardSkill = model.CardSkill

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
		ContextMode: normalizeContextMode(c.Card.ContextMode),
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
		info := agentInfoFromRecord(rec)
		conn := r.GetClient(rec.Name)
		if conn != nil {
			ci := conn.Info()
			info.Description = ci.Description
			info.Version = ci.Version
			info.ContextMode = normalizeContextMode(conn.Card.ContextMode)
			info.Skills = ci.Skills
			card := hostedAgentCard(&conn.Card, rec.Name)
			cardJSON, _ := json.Marshal(card)
			info.AgentCardJson = string(cardJSON)
		}
		// If DB says connected but no live connection, fix status
		if rec.Status == "connected" && conn == nil {
			info.Status = "disconnected"
		}
		result = append(result, info)
	}
	return result, nil
}

func agentInfoFromRecord(rec *model.Agent) model.AgentInfo {
	info := model.AgentInfo{
		Name:         rec.Name,
		Url:          "/agent/" + rec.Name,
		Status:       rec.Status,
		Type:         rec.Type,
		ContextMode:  contextModeFromCardJson(rec.AgentCardJson),
		Skills:       ParseSkillsJson(rec.SkillsJson),
		ErrorMessage: rec.ErrorMessage,
	}
	if card, ok := parseAgentCard(rec.AgentCardJson); ok {
		hosted := hostedAgentCard(card, rec.Name)
		info.Description = hosted.Description
		info.Version = hosted.Version
		info.ContextMode = normalizeContextMode(hosted.ContextMode)
		info.Skills = skillsFromCard(hosted.Skills)
		cardJSON, _ := json.Marshal(hosted)
		info.AgentCardJson = string(cardJSON)
	}
	return info
}

// RegisterAgent performs full self-registration: validate → discover/static card → persist → connect.
func (r *AgentRegistry) RegisterAgent(name, agentType, url string, port int, skills []model.Skill, secret string, contextMode string, providedCard *model.AgentCard) (*AgentConnection, error) {
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
	if url == "" {
		return nil, fmt.Errorf("missing url")
	}
	if agentType == "" {
		agentType = "external"
	}

	// 2. Discover AgentCard unless the caller supplied one to be hosted by the platform.
	var card *model.AgentCard
	if providedCard != nil {
		c := *providedCard
		card = &c
		card.Static = true
		card.ContextMode = mergeContextMode(contextMode, card.ContextMode)
		normalizeAgentCard(card, name, url, skills)
		if card.HealthUrl != "" {
			if err := checkHealthURL(card.HealthUrl); err != nil {
				return nil, fmt.Errorf("health check failed: %w", err)
			}
		}
	} else {
		card, err = fetchAgentCard(url)
		if err != nil {
			return nil, fmt.Errorf("Cannot fetch agent card: %w", err)
		}
		card.ContextMode = mergeContextMode(contextMode, card.ContextMode)
		normalizeAgentCard(card, name, url, skills)
		if err := pingAgent(url); err != nil {
			return nil, fmt.Errorf("Agent unreachable: %w", err)
		}
	}

	// 3. Update in-memory connection
	conn := &AgentConnection{Card: *card, Url: url}
	r.mu.Lock()
	r.connections[name] = conn
	r.mu.Unlock()

	// 4. Persist to DB
	now := time.Now().UTC().Format(time.RFC3339)
	if len(skills) == 0 {
		skills = skillsFromCard(card.Skills)
	}
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

// UpdateAgentMetadata updates an external agent's upstream URL and hosted card.
func (r *AgentRegistry) UpdateAgentMetadata(name, url string, port int, skills []model.Skill, contextMode string, providedCard *model.AgentCard) (*AgentConnection, error) {
	existing, err := r.store.Get(name)
	if err != nil {
		return nil, fmt.Errorf("DB error: %w", err)
	}
	if existing == nil {
		return nil, fmt.Errorf("agent %q not found", name)
	}
	if existing.Type == "builtin" {
		return nil, fmt.Errorf("builtin agent metadata is managed by builtin agent config")
	}
	if strings.TrimSpace(url) == "" {
		url = existing.Url
	}
	if port == 0 {
		port = existing.Port
	}

	var card *model.AgentCard
	if providedCard != nil {
		c := *providedCard
		card = &c
	} else if stored, ok := parseAgentCard(existing.AgentCardJson); ok {
		card = stored
	} else {
		card = &model.AgentCard{Name: name}
	}
	card.Static = true
	card.ContextMode = mergeContextMode(contextMode, card.ContextMode)
	normalizeAgentCard(card, name, url, skills)
	if card.HealthUrl != "" {
		if err := checkHealthURL(card.HealthUrl); err != nil {
			return nil, fmt.Errorf("health check failed: %w", err)
		}
	}
	if len(skills) == 0 {
		skills = skillsFromCard(card.Skills)
	}
	skillsJson, _ := json.Marshal(skills)
	cardJson, _ := json.Marshal(card)
	now := time.Now().UTC().Format(time.RFC3339)
	record := &model.Agent{
		Name:          existing.Name,
		Type:          existing.Type,
		Url:           url,
		Port:          port,
		SkillsJson:    string(skillsJson),
		Status:        "connected",
		ConnectedAt:   &now,
		AgentCardJson: string(cardJson),
		Secret:        existing.Secret,
	}
	if err := r.store.Upsert(record); err != nil {
		return nil, fmt.Errorf("DB persist error: %w", err)
	}
	conn := &AgentConnection{Card: *card, Url: url}
	r.mu.Lock()
	r.connections[name] = conn
	r.mu.Unlock()
	if r.EventBus != nil {
		r.EventBus.AgentStatus(name, "connected", existing.Type)
	}
	return conn, nil
}

func (r *AgentRegistry) PublicAgentCard(name string) (*model.AgentCard, error) {
	rec, err := r.store.Get(name)
	if err != nil {
		return nil, err
	}
	if rec == nil {
		return nil, nil
	}
	if conn := r.GetClient(name); conn != nil {
		conn.mu.RLock()
		card := hostedAgentCard(&conn.Card, name)
		conn.mu.RUnlock()
		return card, nil
	}
	if card, ok := parseAgentCard(rec.AgentCardJson); ok {
		return hostedAgentCard(card, name), nil
	}
	return hostedAgentCard(&model.AgentCard{Name: name}, name), nil
}

// RegisterBuiltinAgent registers an in-process agent without HTTP discovery.
func (r *AgentRegistry) RegisterBuiltinAgent(name, description string, skills []model.Skill) error {
	now := time.Now().UTC().Format(time.RFC3339)
	skillsJson, _ := json.Marshal(skills)
	card := AgentCard{Name: name, Description: description, ContextMode: model.ContextModeContext}
	cardJson, _ := json.Marshal(card)

	dbRecord := &model.Agent{
		Name:          name,
		Type:          "builtin",
		Status:        "connected",
		ConnectedAt:   &now,
		SkillsJson:    string(skillsJson),
		AgentCardJson: string(cardJson),
	}
	if err := r.store.Upsert(dbRecord); err != nil {
		return err
	}

	conn := &AgentConnection{Card: card, Url: ""}
	r.mu.Lock()
	r.connections[name] = conn
	r.mu.Unlock()

	if r.EventBus != nil {
		r.EventBus.AgentRegistered(name, "connected", "builtin")
	}
	return nil
}

func (r *AgentRegistry) GetContextMode(name string) string {
	if conn := r.GetClient(name); conn != nil {
		return normalizeContextMode(conn.Card.ContextMode)
	}
	if r.store == nil {
		return model.ContextModeContext
	}
	rec, err := r.store.Get(name)
	if err != nil || rec == nil {
		return model.ContextModeContext
	}
	return contextModeFromCardJson(rec.AgentCardJson)
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
		if rec.Type == "builtin" || rec.Url == "" {
			continue
		}
		if storedCard, ok := parseStoredStaticCard(rec.AgentCardJson); ok {
			if storedCard.HealthUrl != "" {
				if healthErr := checkHealthURL(storedCard.HealthUrl); healthErr != nil {
					slog.Warn("Failed to restore static agent health check", "name", rec.Name, "error", healthErr)
					r.store.UpdateStatus(rec.Name, "disconnected", nil)
					continue
				}
			}
			r.mu.Lock()
			r.connections[rec.Name] = &AgentConnection{Card: *storedCard, Url: rec.Url}
			r.mu.Unlock()
			slog.Info("Restored static connection to agent", "name", rec.Name, "url", rec.Url)
			continue
		}
		card, err := fetchAgentCard(rec.Url)
		if err != nil {
			slog.Warn("Failed to restore agent", "name", rec.Name, "error", err)
			r.store.UpdateStatus(rec.Name, "disconnected", nil)
			continue
		}
		normalizeAgentCard(card, rec.Name, rec.Url, nil)
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
	client := &http.Client{Timeout: 5 * time.Second}

	// === Phase A: check currently connected agents ===
	r.mu.RLock()
	type agentEntry struct {
		name string
		url  string
		card AgentCard
	}
	var connectedAgents []agentEntry
	for name, conn := range r.connections {
		connectedAgents = append(connectedAgents, agentEntry{name: name, url: conn.Url, card: conn.Card})
	}
	r.mu.RUnlock()

	for _, a := range connectedAgents {
		// Skip builtin agents — they are in-process and always healthy
		if a.url == "" {
			continue
		}
		if a.card.Static {
			if a.card.HealthUrl != "" {
				if err := checkHealthURL(a.card.HealthUrl); err != nil {
					r.recordFailure(a.name, "unreachable", fmt.Sprintf("health check unreachable: %v", err))
					continue
				}
			}
			r.failMu.Lock()
			if r.failCounts[a.name] > 0 {
				slog.Info("Agent health check recovered", "name", a.name)
			}
			delete(r.failCounts, a.name)
			r.failMu.Unlock()
			continue
		}
		// Phase 1: check if bridge HTTP is reachable (agent card endpoint)
		cardURL := strings.TrimRight(a.url, "/") + "/.well-known/agent.json"
		resp, err := client.Get(cardURL)
		if err != nil {
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

		// Success — reset failure count
		r.failMu.Lock()
		if r.failCounts[a.name] > 0 {
			slog.Info("Agent health check recovered", "name", a.name)
		}
		delete(r.failCounts, a.name)
		r.failMu.Unlock()
	}

	// === Phase B: try to reconnect offline/disconnected agents ===
	records, err := r.store.List("")
	if err != nil {
		slog.Error("Health check reconnect: failed to list agents", "error", err)
		return
	}
	for _, rec := range records {
		if rec.Status == "connected" || rec.Status == "online" {
			continue // already handled above
		}
		if rec.Type == "builtin" {
			continue
		}
		if storedCard, ok := parseStoredStaticCard(rec.AgentCardJson); ok {
			if storedCard.HealthUrl != "" {
				if healthErr := checkHealthURL(storedCard.HealthUrl); healthErr != nil {
					continue
				}
			}
			r.mu.Lock()
			r.connections[rec.Name] = &AgentConnection{Card: *storedCard, Url: rec.Url}
			r.mu.Unlock()
			r.failMu.Lock()
			delete(r.failCounts, rec.Name)
			r.failMu.Unlock()
			r.store.UpdateStatus(rec.Name, "connected", nil)
			if r.EventBus != nil {
				r.EventBus.AgentStatus(rec.Name, "connected", rec.Type)
			}
			slog.Info("Static agent reconnected after recovery", "name", rec.Name, "url", rec.Url)
			continue
		}
		card, err := fetchAgentCard(rec.Url)
		if err != nil {
			continue // still unreachable
		}
		normalizeAgentCard(card, rec.Name, rec.Url, nil)
		// Reconnected — add back to in-memory map and update DB
		r.mu.Lock()
		r.connections[rec.Name] = &AgentConnection{Card: *card, Url: rec.Url}
		r.mu.Unlock()
		r.failMu.Lock()
		delete(r.failCounts, rec.Name)
		r.failMu.Unlock()
		r.store.UpdateStatus(rec.Name, "connected", nil)
		if r.EventBus != nil {
			r.EventBus.AgentStatus(rec.Name, "connected", rec.Type)
		}
		slog.Info("Agent reconnected after recovery", "name", rec.Name, "url", rec.Url)
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
	cardURL := strings.TrimRight(url, "/") + "/.well-known/agent.json"
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
	cardURL := strings.TrimRight(url, "/") + "/.well-known/agent.json"
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

func checkHealthURL(url string) error {
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("health returned status %d", resp.StatusCode)
	}
	return nil
}

func normalizeAgentCard(card *model.AgentCard, name, url string, skills []model.Skill) {
	if card.Name == "" {
		card.Name = name
	}
	card.Url = "/agent/" + name
	if card.Version == "" {
		card.Version = "1.0.0"
	}
	card.ContextMode = normalizeContextMode(card.ContextMode)
	if len(card.Skills) == 0 && len(skills) > 0 {
		card.Skills = cardSkillsFromSkills(skills)
	}
}

func normalizeContextMode(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case model.ContextModeStateless:
		return model.ContextModeStateless
	default:
		return model.ContextModeContext
	}
}

func mergeContextMode(primary, fallback string) string {
	if strings.TrimSpace(primary) != "" {
		return normalizeContextMode(primary)
	}
	return normalizeContextMode(fallback)
}

func contextModeFromCardJson(cardJson string) string {
	if cardJson == "" {
		return model.ContextModeContext
	}
	var card model.AgentCard
	if err := json.Unmarshal([]byte(cardJson), &card); err != nil {
		return model.ContextModeContext
	}
	return normalizeContextMode(card.ContextMode)
}

func parseStoredStaticCard(cardJson string) (*model.AgentCard, bool) {
	if cardJson == "" {
		return nil, false
	}
	var card model.AgentCard
	if err := json.Unmarshal([]byte(cardJson), &card); err != nil || !card.Static {
		return nil, false
	}
	return &card, true
}

func parseAgentCard(cardJson string) (*model.AgentCard, bool) {
	if cardJson == "" {
		return nil, false
	}
	var card model.AgentCard
	if err := json.Unmarshal([]byte(cardJson), &card); err != nil {
		return nil, false
	}
	return &card, true
}

func hostedAgentCard(card *model.AgentCard, name string) *model.AgentCard {
	if card == nil {
		card = &model.AgentCard{}
	}
	c := *card
	if c.Name == "" {
		c.Name = name
	}
	c.Url = "/agent/" + c.Name
	c.ContextMode = normalizeContextMode(c.ContextMode)
	return &c
}

func cardSkillsFromSkills(skills []model.Skill) []model.CardSkill {
	result := make([]model.CardSkill, 0, len(skills))
	for _, s := range skills {
		result = append(result, model.CardSkill{
			Id:          s.Id,
			Name:        s.Name,
			Description: s.Description,
			Tags:        s.Tags,
			Examples:    s.Examples,
		})
	}
	return result
}

func skillsFromCard(skills []model.CardSkill) []model.Skill {
	result := make([]model.Skill, 0, len(skills))
	for _, s := range skills {
		result = append(result, model.Skill{
			Id:          s.Id,
			Name:        s.Name,
			Description: s.Description,
			Tags:        s.Tags,
			Examples:    s.Examples,
		})
	}
	return result
}
