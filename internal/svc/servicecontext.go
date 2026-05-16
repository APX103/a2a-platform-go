package svc

import (
	"database/sql"
	"fmt"

	"a2a-platform/internal/config"

	_ "github.com/go-sql-driver/mysql"
)

type ServiceContext struct {
	Config  *config.Config
	DB      *sql.DB
	Agents  *AgentStore
	Tasks   *TaskStore
	Messages *MessageStore
	Traces  *TraceStore
	Registry *AgentRegistry
}

func NewServiceContext(c *config.Config) *ServiceContext {
	db, err := sql.Open("mysql", c.MySQL.DSN())
	if err != nil {
		panic(err)
	}
	db.SetMaxIdleConns(c.MySQL.MaxIdle)
	db.SetMaxOpenConns(c.MySQL.MaxOpen)

	if err := db.Ping(); err != nil {
		panic(fmt.Sprintf("Failed to connect to MySQL: %v", err))
	}

	// Auto-migrate tables
	migrate(db)

	agents := NewAgentStore(db)
	tasks := NewTaskStore(db)
	messages := NewMessageStore(db)
	traces := NewTraceStore(db)
	registry := NewAgentRegistry(agents)

	return &ServiceContext{
		Config:   c,
		DB:       db,
		Agents:   agents,
		Tasks:    tasks,
		Messages: messages,
		Traces:   traces,
		Registry: registry,
	}
}

func migrate(db *sql.DB) {
	schema := `
	CREATE TABLE IF NOT EXISTS agents (
		id BIGINT AUTO_INCREMENT PRIMARY KEY,
		name VARCHAR(255) NOT NULL UNIQUE,
		type VARCHAR(64) NOT NULL DEFAULT '',
		url VARCHAR(512) NOT NULL DEFAULT '',
		port INT NOT NULL DEFAULT 0,
		skills_json TEXT,
		status VARCHAR(32) NOT NULL DEFAULT 'disconnected',
		connected_at VARCHAR(64),
		agent_card_json TEXT,
		error_message TEXT,
		secret VARCHAR(255) NOT NULL DEFAULT '',
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

	CREATE TABLE IF NOT EXISTS tasks (
		id BIGINT AUTO_INCREMENT PRIMARY KEY,
		local_task_id VARCHAR(64) NOT NULL UNIQUE,
		server_task_id VARCHAR(64),
		agent_name VARCHAR(255) NOT NULL,
		context_id VARCHAR(64),
		state VARCHAR(32) NOT NULL DEFAULT 'PENDING',
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
		INDEX idx_agent_name (agent_name),
		INDEX idx_context_id (context_id),
		INDEX idx_state (state)
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

	CREATE TABLE IF NOT EXISTS messages (
		id BIGINT AUTO_INCREMENT PRIMARY KEY,
		task_id VARCHAR(64) NOT NULL,
		role VARCHAR(16) NOT NULL,
		content TEXT,
		timestamp TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		INDEX idx_task_id (task_id)
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

	CREATE TABLE IF NOT EXISTS traces (
		id BIGINT AUTO_INCREMENT PRIMARY KEY,
		task_id VARCHAR(64) NOT NULL,
		context_id VARCHAR(64),
		timestamp TIMESTAMP(3) DEFAULT CURRENT_TIMESTAMP(3),
		event_type VARCHAR(32) NOT NULL,
		agent_name VARCHAR(255) NOT NULL,
		target_agent VARCHAR(255),
		data_json TEXT,
		duration_ms BIGINT,
		INDEX idx_task_id (task_id),
		INDEX idx_agent_name (agent_name)
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
	`
	// Execute each statement separately
	statements := splitStatements(schema)
	for _, stmt := range statements {
		if stmt == "" {
			continue
		}
		if _, err := db.Exec(stmt); err != nil {
			// Table/index already exists is fine
			fmt.Printf("Migration note: %v\n", err)
		}
	}
}

func splitStatements(schema string) []string {
	var result []string
	var current []byte
	for i := 0; i < len(schema); i++ {
		if schema[i] == ';' {
			stmt := string(current)
			if len(stmt) > 2 {
				result = append(result, stmt)
			}
			current = current[:0]
		} else {
			current = append(current, schema[i])
		}
	}
	if len(current) > 2 {
		result = append(result, string(current))
	}
	return result
}
