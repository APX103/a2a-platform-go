package svc

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"a2a-platform/internal/model"
	"a2a-platform/internal/testutil"
)

func setupRegistryTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db := testutil.TempMySQLDB(t)
	if err := migrate(db); err != nil {
		t.Fatalf("migrate test db: %v", err)
	}
	return db
}

func TestFetchAgentCardPrefersStandardAgentCardJSON(t *testing.T) {
	var legacyHit bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/.well-known/agent-card.json":
			_ = json.NewEncoder(w).Encode(model.AgentCard{
				Name:        "standard-card-agent",
				Description: "standard",
				Version:     "1.0.0",
				Skills: []model.CardSkill{
					{Id: "chat", Name: "Chat", Description: "Chat"},
				},
			})
		case "/.well-known/agent.json":
			legacyHit = true
			w.WriteHeader(http.StatusOK)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	card, err := fetchAgentCard(server.URL)
	if err != nil {
		t.Fatalf("fetchAgentCard: %v", err)
	}
	if card.Name != "standard-card-agent" {
		t.Fatalf("card name = %q, want standard-card-agent", card.Name)
	}
	if legacyHit {
		t.Fatal("legacy /.well-known/agent.json was hit even though standard card existed")
	}
}

func TestFetchAgentCardFallsBackToLegacyAgentJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/.well-known/agent-card.json":
			http.NotFound(w, r)
		case "/.well-known/agent.json":
			_ = json.NewEncoder(w).Encode(model.AgentCard{
				Name:        "legacy-card-agent",
				Description: "legacy",
				Version:     "1.0.0",
				Skills: []model.CardSkill{
					{Id: "chat", Name: "Chat", Description: "Chat"},
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	card, err := fetchAgentCard(server.URL)
	if err != nil {
		t.Fatalf("fetchAgentCard: %v", err)
	}
	if card.Name != "legacy-card-agent" {
		t.Fatalf("card name = %q, want legacy-card-agent", card.Name)
	}
}

func TestRegisterAgent_WithStaticAgentCard_DoesNotRequireDiscoveryEndpoint(t *testing.T) {
	db := setupRegistryTestDB(t)
	registry := NewAgentRegistry(NewAgentStore(db))

	card := &model.AgentCard{
		Description: "Existing agent served behind a custom endpoint",
		Skills: []model.CardSkill{
			{Id: "chat", Name: "Chat", Description: "General chat"},
		},
	}
	conn, err := registry.RegisterAgent(
		"static-agent",
		"external",
		"http://127.0.0.1:1/custom-message-endpoint",
		0,
		nil,
		"",
		model.ContextModeStateless,
		card,
	)
	if err != nil {
		t.Fatalf("RegisterAgent failed: %v", err)
	}
	if conn.Card.Name != "static-agent" {
		t.Fatalf("card name = %q, want static-agent", conn.Card.Name)
	}
	if !conn.Card.Static {
		t.Fatal("static card marker was not persisted in connection")
	}
	if conn.Card.ContextMode != model.ContextModeStateless {
		t.Fatalf("context mode = %q, want stateless", conn.Card.ContextMode)
	}

	record, err := registry.store.Get("static-agent")
	if err != nil {
		t.Fatalf("get agent: %v", err)
	}
	if record == nil {
		t.Fatal("agent was not persisted")
	}

	var stored model.AgentCard
	if err := json.Unmarshal([]byte(record.AgentCardJson), &stored); err != nil {
		t.Fatalf("decode stored card: %v", err)
	}
	if !stored.Static {
		t.Fatal("stored card is not marked static")
	}

	agents, err := registry.ListAgents()
	if err != nil {
		t.Fatalf("list agents: %v", err)
	}
	if len(agents) != 1 {
		t.Fatalf("agents len = %d, want 1", len(agents))
	}
	if got := agents[0].Skills[0].Id; got != "chat" {
		t.Fatalf("skill id = %q, want chat", got)
	}

	restored := NewAgentRegistry(NewAgentStore(db))
	restored.RestoreConnections()
	if restored.GetClient("static-agent") == nil {
		t.Fatal("static agent was not restored from stored AgentCard")
	}
}

func TestRegistryStopHealthCheckIsIdempotent(t *testing.T) {
	db := setupRegistryTestDB(t)
	registry := NewAgentRegistry(NewAgentStore(db))

	registry.StartHealthCheck(time.Hour)
	registry.StopHealthCheck()
	registry.StopHealthCheck()
}

func TestRegistryStopHealthCheckCancelsActiveHealthPass(t *testing.T) {
	db := setupRegistryTestDB(t)
	registry := NewAgentRegistry(NewAgentStore(db))
	started := make(chan struct{})
	release := make(chan struct{})
	var hits int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&hits, 1) == 1 {
			close(started)
		}
		select {
		case <-r.Context().Done():
		case <-release:
		}
	}))
	defer server.Close()
	defer close(release)

	registry.mu.Lock()
	registry.connections["blocked"] = &AgentConnection{
		Card: AgentCard{
			Name:      "blocked",
			Static:    true,
			HealthUrl: server.URL,
		},
		Url: server.URL,
	}
	registry.mu.Unlock()

	registry.StartHealthCheck(time.Nanosecond)
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("health check request did not start")
	}

	done := make(chan struct{})
	go func() {
		registry.StopHealthCheck()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("StopHealthCheck did not return while a health pass was active")
	}
}
