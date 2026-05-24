package svc

import (
	"database/sql"
	"encoding/json"
	"testing"

	"a2a-platform/internal/model"
	"a2a-platform/internal/testutil"
)

func setupRegistryTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db := testutil.TempMySQLDB(t)
	migrate(db)
	return db
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
