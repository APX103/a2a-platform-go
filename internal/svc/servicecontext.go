package svc

import (
	"database/sql"
	"encoding/json"
	"fmt"
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
	Contexts       *ContextStore
	Subagents      *SubagentStore
	TaskItems      *TaskItemStore
	BuiltinAgents  *BuiltinAgentStore
	Registry       *AgentRegistry
	EventBus       *events.Broadcaster
	Engine         *engine.Engine
	BridgeRegistry *bridge.BridgeRegistry
}

func NewServiceContext(c *config.Config) (*ServiceContext, error) {
	db, err := openDB(c)
	if err != nil {
		return nil, err
	}
	migrate(db)

	agents := NewAgentStore(db)
	tasks := NewTaskStore(db)
	messages := NewMessageStore(db)
	traces := NewTraceStore(db)
	contexts := NewContextStore(db)
	subagents := NewSubagentStore(db)
	taskItems := NewTaskItemStore(db)
	builtinAgents := NewBuiltinAgentStore(db)
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
		Contexts:       contexts,
		Subagents:      subagents,
		TaskItems:      taskItems,
		BuiltinAgents:  builtinAgents,
		Registry:       registry,
		EventBus:       eventBus,
		Engine:         eng,
		BridgeRegistry: bridgeReg,
	}, nil
}

func openDB(c *config.Config) (*sql.DB, error) {
	if c.IsMySQL() {
		return openMySQL(c)
	}
	return openSQLite()
}

func openMySQL(c *config.Config) (*sql.DB, error) {
	DBDriver = "mysql"
	var db *sql.DB
	var err error

	retryIntervals := []time.Duration{2 * time.Second, 4 * time.Second, 8 * time.Second}
	for attempt := 0; attempt <= len(retryIntervals); attempt++ {
		db, err = sql.Open("mysql", c.MySQL.DSN())
		if err != nil {
			return nil, err
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
		return nil, fmt.Errorf("failed to connect to MySQL after retries: %w", err)
	}

	slog.Info("Connected to MySQL successfully")
	return db, nil
}

func openSQLite() (*sql.DB, error) {
	DBDriver = "sqlite"
	dbPath := filepath.Join(".", "data", "a2a.db")
	if err := os.MkdirAll(filepath.Dir(dbPath), 0755); err != nil {
		return nil, fmt.Errorf("failed to create data directory: %w", err)
	}

	db, err := sql.Open("sqlite", dbPath+"?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)")
	if err != nil {
		return nil, fmt.Errorf("failed to open SQLite: %w", err)
	}
	db.SetMaxOpenConns(1)

	slog.Info("Connected to SQLite", "path", dbPath)
	return db, nil
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
	ensureTaskDirectionColumns(db)
	ensureContextLineageColumns(db)
	repairLegacyTaskSourcesFromToolCalls(db)
	repairLegacyContextLineageFromToolCalls(db)
	ensureMessageDirectionColumns(db)
	backfillMessageDirections(db)
}

func ensureTaskDirectionColumns(db *sql.DB) {
	statements := []string{}
	if DBDriver == "mysql" {
		statements = []string{
			"ALTER TABLE tasks ADD COLUMN source_agent VARCHAR(255)",
			"ALTER TABLE tasks ADD COLUMN target_agent VARCHAR(255)",
			"UPDATE tasks SET target_agent = agent_name WHERE target_agent IS NULL OR target_agent = ''",
			"CREATE INDEX idx_tasks_source_agent ON tasks(source_agent)",
			"CREATE INDEX idx_tasks_target_agent ON tasks(target_agent)",
		}
	} else {
		statements = []string{
			"ALTER TABLE tasks ADD COLUMN source_agent TEXT",
			"ALTER TABLE tasks ADD COLUMN target_agent TEXT",
			"UPDATE tasks SET target_agent = agent_name WHERE target_agent IS NULL OR target_agent = ''",
			"CREATE INDEX IF NOT EXISTS idx_tasks_source_agent ON tasks(source_agent)",
			"CREATE INDEX IF NOT EXISTS idx_tasks_target_agent ON tasks(target_agent)",
		}
	}
	for _, stmt := range statements {
		if _, err := db.Exec(stmt); err != nil {
			slog.Debug("Migration note", "statement", stmt, "error", err)
		}
	}
}

func ensureMessageDirectionColumns(db *sql.DB) {
	statements := []string{}
	if DBDriver == "mysql" {
		statements = []string{
			"ALTER TABLE messages ADD COLUMN sender_agent VARCHAR(255)",
			"ALTER TABLE messages ADD COLUMN recipient_agent VARCHAR(255)",
			"CREATE INDEX idx_messages_sender_agent ON messages(sender_agent)",
			"CREATE INDEX idx_messages_recipient_agent ON messages(recipient_agent)",
		}
	} else {
		statements = []string{
			"ALTER TABLE messages ADD COLUMN sender_agent TEXT",
			"ALTER TABLE messages ADD COLUMN recipient_agent TEXT",
			"CREATE INDEX IF NOT EXISTS idx_messages_sender_agent ON messages(sender_agent)",
			"CREATE INDEX IF NOT EXISTS idx_messages_recipient_agent ON messages(recipient_agent)",
		}
	}
	for _, stmt := range statements {
		if _, err := db.Exec(stmt); err != nil {
			slog.Debug("Migration note", "statement", stmt, "error", err)
		}
	}
}

func ensureContextLineageColumns(db *sql.DB) {
	statements := []string{}
	if DBDriver == "mysql" {
		statements = []string{
			"ALTER TABLE tasks ADD COLUMN root_context_id VARCHAR(64)",
			"ALTER TABLE tasks ADD COLUMN parent_task_id VARCHAR(64)",
			"ALTER TABLE tasks ADD COLUMN parent_tool_call_id VARCHAR(128)",
			"UPDATE tasks SET root_context_id = context_id WHERE root_context_id IS NULL AND context_id IS NOT NULL",
			"CREATE INDEX idx_tasks_root_context_id ON tasks(root_context_id)",
			"CREATE INDEX idx_tasks_parent_task_id ON tasks(parent_task_id)",
			"ALTER TABLE traces ADD COLUMN root_context_id VARCHAR(64)",
			"ALTER TABLE traces ADD COLUMN parent_task_id VARCHAR(64)",
			"UPDATE traces SET root_context_id = context_id WHERE root_context_id IS NULL AND context_id IS NOT NULL",
			"CREATE INDEX idx_traces_root_context_id ON traces(root_context_id)",
		}
	} else {
		statements = []string{
			"ALTER TABLE tasks ADD COLUMN root_context_id TEXT",
			"ALTER TABLE tasks ADD COLUMN parent_task_id TEXT",
			"ALTER TABLE tasks ADD COLUMN parent_tool_call_id TEXT",
			"UPDATE tasks SET root_context_id = context_id WHERE root_context_id IS NULL AND context_id IS NOT NULL",
			"CREATE INDEX IF NOT EXISTS idx_tasks_root_context_id ON tasks(root_context_id)",
			"CREATE INDEX IF NOT EXISTS idx_tasks_parent_task_id ON tasks(parent_task_id)",
			"ALTER TABLE traces ADD COLUMN root_context_id TEXT",
			"ALTER TABLE traces ADD COLUMN parent_task_id TEXT",
			"UPDATE traces SET root_context_id = context_id WHERE root_context_id IS NULL AND context_id IS NOT NULL",
			"CREATE INDEX IF NOT EXISTS idx_traces_root_context_id ON traces(root_context_id)",
		}
	}
	for _, stmt := range statements {
		if _, err := db.Exec(stmt); err != nil {
			slog.Debug("Migration note", "statement", stmt, "error", err)
		}
	}
}

func repairLegacyTaskSourcesFromToolCalls(db *sql.DB) {
	type legacyTask struct {
		id          string
		targetAgent string
		userText    string
	}

	rows, err := db.Query(`
		SELECT t.local_task_id, COALESCE(NULLIF(t.target_agent, ''), t.agent_name), COALESCE(m.content, '')
		FROM tasks t
		LEFT JOIN messages m ON m.task_id = t.local_task_id AND m.role = 'user'
		WHERE (t.source_agent IS NULL OR t.source_agent = '')
		  AND COALESCE(NULLIF(t.target_agent, ''), t.agent_name) <> ''
		ORDER BY t.created_at, m.id`)
	if err != nil {
		slog.Debug("Legacy task source repair skipped", "error", err)
		return
	}
	defer rows.Close()

	tasks := map[string]legacyTask{}
	for rows.Next() {
		var t legacyTask
		if err := rows.Scan(&t.id, &t.targetAgent, &t.userText); err != nil {
			slog.Debug("Legacy task source repair scan skipped", "error", err)
			return
		}
		if _, exists := tasks[t.id]; !exists {
			tasks[t.id] = t
		}
	}
	if len(tasks) == 0 {
		return
	}

	traceRows, err := db.Query("SELECT agent_name, data_json FROM traces WHERE event_type = 'tool_call' ORDER BY timestamp, id")
	if err != nil {
		slog.Debug("Legacy task source repair trace scan skipped", "error", err)
		return
	}
	defer traceRows.Close()

	type toolCall struct {
		sourceAgent string
		targetAgent string
		message     string
	}
	var calls []toolCall
	for traceRows.Next() {
		var sourceAgent string
		var dataJSON sql.NullString
		if err := traceRows.Scan(&sourceAgent, &dataJSON); err != nil {
			slog.Debug("Legacy task source repair trace row skipped", "error", err)
			continue
		}
		if !dataJSON.Valid {
			continue
		}
		targetAgent, message, ok := parseSendToAgentTrace(dataJSON.String)
		if !ok || sourceAgent == "" || targetAgent == "" || message == "" {
			continue
		}
		calls = append(calls, toolCall{sourceAgent: sourceAgent, targetAgent: targetAgent, message: message})
	}

	for _, task := range tasks {
		for _, call := range calls {
			if call.targetAgent != task.targetAgent || call.message != task.userText {
				continue
			}
			_, err := db.Exec("UPDATE tasks SET source_agent=? WHERE local_task_id=? AND (source_agent IS NULL OR source_agent='')", call.sourceAgent, task.id)
			if err != nil {
				slog.Debug("Legacy task source repair update skipped", "task", task.id, "error", err)
				break
			}
			_, _ = db.Exec("UPDATE traces SET agent_name=? WHERE task_id=? AND event_type='send' AND agent_name='host'", call.sourceAgent, task.id)
			break
		}
	}
}

func repairLegacyContextLineageFromToolCalls(db *sql.DB) {
	for i := 0; i < 10; i++ {
		updated := repairLegacyContextLineagePass(db)
		updated += propagateLegacyContextLineageRoots(db)
		if updated == 0 {
			return
		}
	}
}

func repairLegacyContextLineagePass(db *sql.DB) int {
	rows, err := db.Query(`
		SELECT task_id, COALESCE(root_context_id, context_id, ''), data_json, strftime('%Y-%m-%d %H:%M:%S', timestamp)
		FROM traces
		WHERE event_type='tool_call'
		ORDER BY timestamp`)
	if err != nil {
		slog.Debug("Legacy context lineage repair trace query skipped", "error", err)
		return 0
	}
	defer rows.Close()

	type call struct {
		parentTaskId  string
		rootContextId string
		targetAgent   string
		message       string
		timestamp     string
	}
	var calls []call
	for rows.Next() {
		var c call
		var dataJSON sql.NullString
		if err := rows.Scan(&c.parentTaskId, &c.rootContextId, &dataJSON, &c.timestamp); err != nil {
			slog.Debug("Legacy context lineage repair trace row skipped", "error", err)
			continue
		}
		if !dataJSON.Valid {
			continue
		}
		targetAgent, message, ok := parseSendToAgentTrace(dataJSON.String)
		if !ok || c.parentTaskId == "" || c.rootContextId == "" || targetAgent == "" || message == "" {
			continue
		}
		c.targetAgent = targetAgent
		c.message = message
		calls = append(calls, c)
	}

	updated := 0
	for _, call := range calls {
		var childTaskId string
		err := db.QueryRow(`
			SELECT t.local_task_id
			FROM tasks t
			JOIN messages m ON m.task_id = t.local_task_id AND m.role='user'
			WHERE COALESCE(NULLIF(t.target_agent, ''), t.agent_name)=?
			  AND m.content=?
			  AND (t.parent_task_id IS NULL OR t.parent_task_id='')
			  AND t.local_task_id <> ?
			  AND datetime(t.created_at) >= datetime(?)
			ORDER BY t.created_at
			LIMIT 1`,
			call.targetAgent, call.message, call.parentTaskId, call.timestamp,
		).Scan(&childTaskId)
		if err == sql.ErrNoRows {
			continue
		}
		if err != nil {
			slog.Debug("Legacy context lineage repair child lookup skipped", "error", err)
			continue
		}

		res, err := db.Exec(
			"UPDATE tasks SET parent_task_id=?, root_context_id=? WHERE local_task_id=? AND (parent_task_id IS NULL OR parent_task_id='')",
			call.parentTaskId, call.rootContextId, childTaskId,
		)
		if err != nil {
			slog.Debug("Legacy context lineage repair task update skipped", "task", childTaskId, "error", err)
			continue
		}
		affected, _ := res.RowsAffected()
		if affected == 0 {
			continue
		}
		_, _ = db.Exec(
			"UPDATE traces SET parent_task_id=?, root_context_id=? WHERE task_id=?",
			call.parentTaskId, call.rootContextId, childTaskId,
		)
		updated++
	}
	return updated
}

func propagateLegacyContextLineageRoots(db *sql.DB) int {
	res, err := db.Exec(`
		UPDATE tasks
		SET root_context_id = (
			SELECT p.root_context_id
			FROM tasks p
			WHERE p.local_task_id = tasks.parent_task_id
		)
		WHERE parent_task_id IS NOT NULL
		  AND parent_task_id <> ''
		  AND EXISTS (
			SELECT 1
			FROM tasks p
			WHERE p.local_task_id = tasks.parent_task_id
			  AND p.root_context_id IS NOT NULL
			  AND p.root_context_id <> ''
			  AND COALESCE(tasks.root_context_id, '') <> p.root_context_id
		  )`)
	if err != nil {
		slog.Debug("Legacy context lineage root propagation skipped", "error", err)
		return 0
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		return 0
	}
	_, _ = db.Exec(`
		UPDATE traces
		SET root_context_id = (
			SELECT t.root_context_id
			FROM tasks t
			WHERE t.local_task_id = traces.task_id
		)
		WHERE EXISTS (
			SELECT 1
			FROM tasks t
			WHERE t.local_task_id = traces.task_id
			  AND t.root_context_id IS NOT NULL
			  AND t.root_context_id <> ''
			  AND COALESCE(traces.root_context_id, '') <> t.root_context_id
		)`)
	return int(affected)
}

func parseSendToAgentTrace(dataJSON string) (string, string, bool) {
	var outer struct {
		Tool      string `json:"tool"`
		Arguments string `json:"arguments"`
	}
	if err := json.Unmarshal([]byte(dataJSON), &outer); err != nil {
		return "", "", false
	}
	if outer.Tool != "send_to_agent" || outer.Arguments == "" {
		return "", "", false
	}
	var args struct {
		Agent     string `json:"agent"`
		AgentName string `json:"agent_name"`
		Message   string `json:"message"`
	}
	if err := json.Unmarshal([]byte(outer.Arguments), &args); err != nil {
		return "", "", false
	}
	target := args.Agent
	if target == "" {
		target = args.AgentName
	}
	return target, args.Message, target != "" && args.Message != ""
}

func backfillMessageDirections(db *sql.DB) {
	rows, err := db.Query(`
		SELECT m.id, m.role, t.source_agent, COALESCE(NULLIF(t.target_agent, ''), t.agent_name)
		FROM messages m
		LEFT JOIN tasks t ON t.local_task_id = m.task_id
		WHERE m.sender_agent IS NULL OR m.sender_agent = '' OR m.recipient_agent IS NULL OR m.recipient_agent = ''`)
	if err != nil {
		slog.Debug("Message direction backfill skipped", "error", err)
		return
	}
	defer rows.Close()

	type update struct {
		id        int64
		sender    *string
		recipient *string
	}
	var updates []update
	for rows.Next() {
		var id int64
		var role string
		var sourceAgent, targetAgent sql.NullString
		if err := rows.Scan(&id, &role, &sourceAgent, &targetAgent); err != nil {
			slog.Debug("Message direction backfill scan skipped", "error", err)
			return
		}

		var sender, recipient *string
		source := nullableString(sourceAgent)
		target := nullableString(targetAgent)
		switch role {
		case "user":
			sender = source
			recipient = target
		case "agent":
			sender = target
			recipient = source
		case "tool":
			sender = target
			recipient = target
		default:
			sender = source
			recipient = target
		}
		updates = append(updates, update{id: id, sender: sender, recipient: recipient})
	}
	for _, u := range updates {
		_, err := db.Exec("UPDATE messages SET sender_agent=?, recipient_agent=? WHERE id=?", u.sender, u.recipient, u.id)
		if err != nil {
			slog.Debug("Message direction backfill update skipped", "message", u.id, "error", err)
		}
	}
}

func nullableString(ns sql.NullString) *string {
	if !ns.Valid || ns.String == "" {
		return nil
	}
	value := ns.String
	return &value
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
	source_agent VARCHAR(255),
	target_agent VARCHAR(255),
	agent_name VARCHAR(255) NOT NULL,
	context_id VARCHAR(64),
	root_context_id VARCHAR(64),
	parent_task_id VARCHAR(64),
	parent_tool_call_id VARCHAR(128),
	state VARCHAR(32) NOT NULL DEFAULT 'PENDING',
	created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
	INDEX idx_agent_name (agent_name),
	INDEX idx_tasks_source_agent (source_agent),
	INDEX idx_tasks_target_agent (target_agent),
	INDEX idx_context_id (context_id),
	INDEX idx_tasks_root_context_id (root_context_id),
	INDEX idx_tasks_parent_task_id (parent_task_id),
	INDEX idx_state (state)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS messages (
	id BIGINT AUTO_INCREMENT PRIMARY KEY,
	task_id VARCHAR(64) NOT NULL,
	context_id VARCHAR(64),
	role VARCHAR(16) NOT NULL,
	sender_agent VARCHAR(255),
	recipient_agent VARCHAR(255),
	content TEXT,
	reasoning_content TEXT,
	tool_calls JSON,
	tool_call_id VARCHAR(64),
	thinking_blocks JSON,
	timestamp TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	INDEX idx_task_id (task_id),
	INDEX idx_context_id (context_id),
	INDEX idx_messages_sender_agent (sender_agent),
	INDEX idx_messages_recipient_agent (recipient_agent)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS traces (
	id BIGINT AUTO_INCREMENT PRIMARY KEY,
	task_id VARCHAR(64) NOT NULL,
	context_id VARCHAR(64),
	root_context_id VARCHAR(64),
	parent_task_id VARCHAR(64),
	timestamp TIMESTAMP(3) DEFAULT CURRENT_TIMESTAMP(3),
	event_type VARCHAR(32) NOT NULL,
	agent_name VARCHAR(255) NOT NULL,
	target_agent VARCHAR(255),
	data_json TEXT,
	duration_ms BIGINT,
	INDEX idx_task_id (task_id),
	INDEX idx_traces_root_context_id (root_context_id),
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

CREATE TABLE IF NOT EXISTS task_items (
	id VARCHAR(36) PRIMARY KEY,
	context_id VARCHAR(36) NOT NULL,
	subject TEXT NOT NULL,
	description TEXT,
	status VARCHAR(16) NOT NULL DEFAULT 'pending',
	owner VARCHAR(128),
	blocked_by TEXT,
	result TEXT,
	error TEXT,
	created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	completed_at TIMESTAMP,
	INDEX idx_task_items_context (context_id),
	INDEX idx_task_items_status (status)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS builtin_agents (
	id INT AUTO_INCREMENT PRIMARY KEY,
	name VARCHAR(255) NOT NULL UNIQUE,
	provider VARCHAR(64) NOT NULL,
	base_url VARCHAR(512),
	api_key VARCHAR(512),
	model VARCHAR(255) NOT NULL,
	description TEXT,
	system_prompt TEXT,
	max_tokens INT DEFAULT 4096,
	max_tool_rounds INT DEFAULT 10,
	created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
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
	source_agent TEXT,
	target_agent TEXT,
	agent_name TEXT NOT NULL,
	context_id TEXT,
	root_context_id TEXT,
	parent_task_id TEXT,
	parent_tool_call_id TEXT,
	state TEXT NOT NULL DEFAULT 'PENDING',
	created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_tasks_agent_name ON tasks(agent_name);
CREATE INDEX IF NOT EXISTS idx_tasks_source_agent ON tasks(source_agent);
CREATE INDEX IF NOT EXISTS idx_tasks_target_agent ON tasks(target_agent);
CREATE INDEX IF NOT EXISTS idx_tasks_context_id ON tasks(context_id);
CREATE INDEX IF NOT EXISTS idx_tasks_root_context_id ON tasks(root_context_id);
CREATE INDEX IF NOT EXISTS idx_tasks_parent_task_id ON tasks(parent_task_id);
CREATE INDEX IF NOT EXISTS idx_tasks_state ON tasks(state);

CREATE TABLE IF NOT EXISTS messages (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	task_id TEXT NOT NULL,
	context_id TEXT,
	role TEXT NOT NULL,
	sender_agent TEXT,
	recipient_agent TEXT,
	content TEXT,
	reasoning_content TEXT,
	tool_calls TEXT,
	tool_call_id TEXT,
	thinking_blocks TEXT,
	timestamp TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_messages_task_id ON messages(task_id);
CREATE INDEX IF NOT EXISTS idx_messages_context_id ON messages(context_id);
CREATE INDEX IF NOT EXISTS idx_messages_sender_agent ON messages(sender_agent);
CREATE INDEX IF NOT EXISTS idx_messages_recipient_agent ON messages(recipient_agent);

CREATE TABLE IF NOT EXISTS traces (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	task_id TEXT NOT NULL,
	context_id TEXT,
	root_context_id TEXT,
	parent_task_id TEXT,
	timestamp TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	event_type TEXT NOT NULL,
	agent_name TEXT NOT NULL,
	target_agent TEXT,
	data_json TEXT,
	duration_ms INTEGER
);

CREATE INDEX IF NOT EXISTS idx_traces_task_id ON traces(task_id);
CREATE INDEX IF NOT EXISTS idx_traces_root_context_id ON traces(root_context_id);
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

CREATE TABLE IF NOT EXISTS task_items (
	id TEXT PRIMARY KEY,
	context_id TEXT NOT NULL,
	subject TEXT NOT NULL,
	description TEXT,
	status TEXT NOT NULL DEFAULT 'pending',
	owner TEXT,
	blocked_by TEXT,
	result TEXT,
	error TEXT,
	created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	completed_at TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_task_items_context ON task_items(context_id);
CREATE INDEX IF NOT EXISTS idx_task_items_status ON task_items(status);

CREATE TABLE IF NOT EXISTS builtin_agents (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	name TEXT NOT NULL UNIQUE,
	provider TEXT NOT NULL,
	base_url TEXT,
	api_key TEXT,
	model TEXT NOT NULL,
	description TEXT,
	system_prompt TEXT,
	max_tokens INTEGER DEFAULT 4096,
	max_tool_rounds INTEGER DEFAULT 10,
	created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
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
