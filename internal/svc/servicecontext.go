package svc

import (
	"database/sql"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"a2a-platform/internal/bridge"
	"a2a-platform/internal/config"
	"a2a-platform/internal/engine"
	"a2a-platform/internal/events"

	_ "github.com/go-sql-driver/mysql"
	_ "modernc.org/sqlite"
)

// DBDriver is set at init time: "mysql" or "sqlite".
var DBDriver string

type ServiceContext struct {
	Config         *config.Config
	DB             *sql.DB
	Agents         *AgentStore
	Tasks          *TaskStore
	Messages       *MessageStore
	Traces         *TraceStore
	Registry       *AgentRegistry
	EventBus       *events.Broadcaster
	Engine         *engine.Engine
	BridgeRegistry *bridge.BridgeRegistry
}

func NewServiceContext(c *config.Config) *ServiceContext {
	db := openDB(c)
	migrate(db)

	agents := NewAgentStore(db)
	tasks := NewTaskStore(db)
	messages := NewMessageStore(db)
	traces := NewTraceStore(db)
	registry := NewAgentRegistry(agents)
	eventBus := events.NewBroadcaster()
	eng := engine.New()
	bridgeReg := bridge.NewRegistry()

	return &ServiceContext{
		Config:         c,
		DB:             db,
		Agents:         agents,
		Tasks:          tasks,
		Messages:       messages,
		Traces:         traces,
		Registry:       registry,
		EventBus:       eventBus,
		Engine:         eng,
		BridgeRegistry: bridgeReg,
	}
}

func openDB(c *config.Config) *sql.DB {
	if c.IsMySQL() {
		return openMySQL(c)
	}
	return openSQLite()
}

func openMySQL(c *config.Config) *sql.DB {
	DBDriver = "mysql"
	var db *sql.DB
	var err error

	retryIntervals := []time.Duration{2 * time.Second, 4 * time.Second, 8 * time.Second}
	for attempt := 0; attempt <= len(retryIntervals); attempt++ {
		db, err = sql.Open("mysql", c.MySQL.DSN())
		if err != nil {
			panic(err)
		}
		db.SetMaxIdleConns(c.MySQL.MaxIdle)
		db.SetMaxOpenConns(c.MySQL.MaxOpen)
		db.SetConnMaxLifetime(5 * time.Minute)
		db.SetConnMaxIdleTime(2 * time.Minute)

		if err = db.Ping(); err == nil {
			break
		}
		if attempt < len(retryIntervals) {
			slog.Warn("DB connection failed, retrying", "attempt", attempt+1, "error", err, "wait", retryIntervals[attempt])
			time.Sleep(retryIntervals[attempt])
			db.Close()
		}
	}
	if err != nil {
		panic("Failed to connect to MySQL after retries: " + err.Error())
	}

	slog.Info("Connected to MySQL successfully")
	return db
}

func openSQLite() *sql.DB {
	DBDriver = "sqlite"
	dbPath := filepath.Join(".", "data", "a2a.db")
	if err := os.MkdirAll(filepath.Dir(dbPath), 0755); err != nil {
		panic("Failed to create data directory: " + err.Error())
	}

	db, err := sql.Open("sqlite", dbPath+"?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)")
	if err != nil {
		panic("Failed to open SQLite: " + err.Error())
	}
	db.SetMaxOpenConns(1)

	slog.Info("Connected to SQLite", "path", dbPath)
	return db
}

func migrate(db *sql.DB) {
	var schema string
	if DBDriver == "mysql" {
		schema = mysqlSchema
	} else {
		schema = sqliteSchema
	}
	statements := splitStatements(schema)
	for _, stmt := range statements {
		if stmt == "" {
			continue
		}
		if _, err := db.Exec(stmt); err != nil {
			slog.Warn("Migration note", "error", err)
		}
	}
}

const mysqlSchema = `
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
	context_id VARCHAR(64),
	role VARCHAR(16) NOT NULL,
	content TEXT,
	reasoning_content TEXT,
	tool_calls JSON,
	tool_call_id VARCHAR(64),
	thinking_blocks JSON,
	timestamp TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	INDEX idx_task_id (task_id),
	INDEX idx_context_id (context_id)
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

CREATE TABLE IF NOT EXISTS contexts (
	id VARCHAR(36) PRIMARY KEY,
	agent_name VARCHAR(128) NOT NULL,
	title VARCHAR(256),
	message_count INT DEFAULT 0,
	created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
	INDEX idx_agent_name (agent_name),
	INDEX idx_updated_at (updated_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS subagent_sessions (
	id VARCHAR(36) PRIMARY KEY,
	parent_context_id VARCHAR(36) NOT NULL,
	parent_tool_call_id VARCHAR(64),
	task TEXT,
	context TEXT,
	status VARCHAR(16) NOT NULL DEFAULT 'running',
	messages JSON,
	result TEXT,
	error TEXT,
	created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	completed_at TIMESTAMP,
	INDEX idx_parent_context (parent_context_id),
	INDEX idx_status (status)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
`

const sqliteSchema = `
CREATE TABLE IF NOT EXISTS agents (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	name TEXT NOT NULL UNIQUE,
	type TEXT NOT NULL DEFAULT '',
	url TEXT NOT NULL DEFAULT '',
	port INTEGER NOT NULL DEFAULT 0,
	skills_json TEXT,
	status TEXT NOT NULL DEFAULT 'disconnected',
	connected_at TEXT,
	agent_card_json TEXT,
	error_message TEXT,
	secret TEXT NOT NULL DEFAULT '',
	created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS tasks (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	local_task_id TEXT NOT NULL UNIQUE,
	server_task_id TEXT,
	agent_name TEXT NOT NULL,
	context_id TEXT,
	state TEXT NOT NULL DEFAULT 'PENDING',
	created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_tasks_agent_name ON tasks(agent_name);
CREATE INDEX IF NOT EXISTS idx_tasks_context_id ON tasks(context_id);
CREATE INDEX IF NOT EXISTS idx_tasks_state ON tasks(state);

CREATE TABLE IF NOT EXISTS messages (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	task_id TEXT NOT NULL,
	context_id TEXT,
	role TEXT NOT NULL,
	content TEXT,
	reasoning_content TEXT,
	tool_calls TEXT,
	tool_call_id TEXT,
	thinking_blocks TEXT,
	timestamp TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_messages_task_id ON messages(task_id);
CREATE INDEX IF NOT EXISTS idx_messages_context_id ON messages(context_id);

CREATE TABLE IF NOT EXISTS traces (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	task_id TEXT NOT NULL,
	context_id TEXT,
	timestamp TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	event_type TEXT NOT NULL,
	agent_name TEXT NOT NULL,
	target_agent TEXT,
	data_json TEXT,
	duration_ms INTEGER
);

CREATE INDEX IF NOT EXISTS idx_traces_task_id ON traces(task_id);
CREATE INDEX IF NOT EXISTS idx_traces_agent_name ON traces(agent_name);

CREATE TABLE IF NOT EXISTS contexts (
	id TEXT PRIMARY KEY,
	agent_name TEXT NOT NULL,
	title TEXT,
	message_count INTEGER DEFAULT 0,
	created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_contexts_agent_name ON contexts(agent_name);
CREATE INDEX IF NOT EXISTS idx_contexts_updated_at ON contexts(updated_at);

CREATE TABLE IF NOT EXISTS subagent_sessions (
	id TEXT PRIMARY KEY,
	parent_context_id TEXT NOT NULL,
	parent_tool_call_id TEXT,
	task TEXT,
	context TEXT,
	status TEXT NOT NULL DEFAULT 'running',
	messages TEXT,
	result TEXT,
	error TEXT,
	created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	completed_at TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_subagent_parent ON subagent_sessions(parent_context_id);
CREATE INDEX IF NOT EXISTS idx_subagent_status ON subagent_sessions(status);
`

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
