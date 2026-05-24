package svc

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"a2a-platform/internal/bridge"
	"a2a-platform/internal/config"
	"a2a-platform/internal/engine"
	"a2a-platform/internal/events"
	"a2a-platform/internal/llm"
	"a2a-platform/internal/model"
	"a2a-platform/internal/tools"

	_ "github.com/go-sql-driver/mysql"
)

type ServiceContext struct {
	Config         *config.Config
	DB             *sql.DB
	Agents         *AgentStore
	Tasks          *TaskStore
	Messages       *MessageStore
	Traces         *TraceStore
	Contexts       *ContextStore
	Subagents      *SubagentStore
	Humans         *HumanUserStore
	HumanSessions  *HumanSessionStore
	TaskItems      *TaskItemStore
	BuiltinAgents  *BuiltinAgentStore
	Groups         *GroupStore
	GroupMembers   *GroupMemberStore
	GroupInvites   *GroupInviteStore
	GroupTokens    *GroupMemberTokenStore
	GroupEvents    *GroupEventStore
	GroupArtifacts *GroupArtifactStore
	Registry       *AgentRegistry
	EventBus       *events.Broadcaster
	Engine         *engine.Engine
	BridgeRegistry *bridge.BridgeRegistry
}

func (s *ServiceContext) ConfigureAuxiliaryAgentTools(cfg config.BuiltinAgent) {
	if s == nil || s.Engine == nil {
		return
	}
	var provider llm.Provider
	switch cfg.Provider {
	case "openai":
		provider = llm.NewOpenAIProvider(cfg.BaseURL, cfg.APIKey)
	case "anthropic":
		provider = llm.NewAnthropicProvider(cfg.BaseURL, cfg.APIKey)
	}
	if provider == nil {
		return
	}
	chatReq := llm.ChatRequest{
		Model:     cfg.Model,
		MaxTokens: cfg.MaxTokens,
	}
	se := tools.NewSubagentEngine(s.Subagents, provider, cfg.Name, chatReq)
	s.Engine.SetSubagentEngine(se)
	tools.RegisterDynamicTools([]model.BuiltinTool{tools.NewSpawnAgentTool(se)})
	slog.Info("Registered spawn_agent tool", "agent", cfg.Name)
	tools.RegisterDynamicTools(tools.NewTaskTools(s.TaskItems))
	slog.Info("Registered task system tools", "agent", cfg.Name)
}

func NewServiceContext(c *config.Config) (*ServiceContext, error) {
	db, err := openMySQL(c)
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
	humans := NewHumanUserStore(db)
	humanSessions := NewHumanSessionStore(db)
	taskItems := NewTaskItemStore(db)
	builtinAgents := NewBuiltinAgentStore(db)
	groups := NewGroupStore(db)
	groupMembers := NewGroupMemberStore(db)
	groupInvites := NewGroupInviteStore(db)
	groupTokens := NewGroupMemberTokenStore(db)
	groupEvents := NewGroupEventStore(db)
	groupArtifacts := NewGroupArtifactStore(db)
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
		Humans:         humans,
		HumanSessions:  humanSessions,
		TaskItems:      taskItems,
		BuiltinAgents:  builtinAgents,
		Groups:         groups,
		GroupMembers:   groupMembers,
		GroupInvites:   groupInvites,
		GroupTokens:    groupTokens,
		GroupEvents:    groupEvents,
		GroupArtifacts: groupArtifacts,
		Registry:       registry,
		EventBus:       eventBus,
		Engine:         eng,
		BridgeRegistry: bridgeReg,
	}, nil
}

func openMySQL(c *config.Config) (*sql.DB, error) {
	if c == nil || c.MySQL == nil {
		return nil, fmt.Errorf("mysql configuration is required")
	}
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

func migrate(db *sql.DB) {
	statements := splitStatements(mysqlSchema)
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
	ensureHumanPresenceColumns(db)
}

func ensureTaskDirectionColumns(db *sql.DB) {
	statements := []string{
		"ALTER TABLE tasks ADD COLUMN source_agent VARCHAR(255)",
		"ALTER TABLE tasks ADD COLUMN target_agent VARCHAR(255)",
		"UPDATE tasks SET target_agent = agent_name WHERE target_agent IS NULL OR target_agent = ''",
		"CREATE INDEX idx_tasks_source_agent ON tasks(source_agent)",
		"CREATE INDEX idx_tasks_target_agent ON tasks(target_agent)",
	}
	for _, stmt := range statements {
		if _, err := db.Exec(stmt); err != nil {
			slog.Debug("Migration note", "statement", stmt, "error", err)
		}
	}
}

func ensureMessageDirectionColumns(db *sql.DB) {
	statements := []string{
		"ALTER TABLE messages ADD COLUMN sender_agent VARCHAR(255)",
		"ALTER TABLE messages ADD COLUMN recipient_agent VARCHAR(255)",
		"CREATE INDEX idx_messages_sender_agent ON messages(sender_agent)",
		"CREATE INDEX idx_messages_recipient_agent ON messages(recipient_agent)",
	}
	for _, stmt := range statements {
		if _, err := db.Exec(stmt); err != nil {
			slog.Debug("Migration note", "statement", stmt, "error", err)
		}
	}
}

func ensureHumanPresenceColumns(db *sql.DB) {
	statements := []string{
		"ALTER TABLE human_users ADD COLUMN last_seen_at TIMESTAMP NULL",
		"CREATE INDEX idx_human_users_last_seen ON human_users(last_seen_at)",
	}
	for _, stmt := range statements {
		if _, err := db.Exec(stmt); err != nil {
			slog.Debug("Migration note", "statement", stmt, "error", err)
		}
	}
}

func ensureContextLineageColumns(db *sql.DB) {
	statements := []string{
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

CREATE TABLE IF NOT EXISTS human_users (
	id VARCHAR(36) PRIMARY KEY,
	handle VARCHAR(128) NOT NULL UNIQUE,
	display_name VARCHAR(255) NOT NULL,
	last_seen_at TIMESTAMP NULL,
	secret_hash VARCHAR(128) NOT NULL DEFAULT '',
	secret_salt VARCHAR(64) NOT NULL DEFAULT '',
	created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
	INDEX idx_human_users_handle (handle),
	INDEX idx_human_users_last_seen (last_seen_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS human_sessions (
	id BIGINT AUTO_INCREMENT PRIMARY KEY,
	human_id VARCHAR(36) NOT NULL,
	token_hash VARCHAR(64) NOT NULL UNIQUE,
	expires_at TIMESTAMP NULL,
	revoked_at TIMESTAMP NULL,
	created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	INDEX idx_human_sessions_human (human_id),
	INDEX idx_human_sessions_token (token_hash)
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

CREATE TABLE IF NOT EXISTS a2a_groups (
	id VARCHAR(36) PRIMARY KEY,
	name VARCHAR(255) NOT NULL,
	description TEXT,
	orchestration_mode VARCHAR(64) NOT NULL DEFAULT 'leader_led',
	rules_json TEXT,
	memory_policy_json TEXT,
	status VARCHAR(32) NOT NULL DEFAULT 'active',
	created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
	INDEX idx_groups_status (status),
	INDEX idx_groups_mode (orchestration_mode)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS group_members (
	id BIGINT AUTO_INCREMENT PRIMARY KEY,
	group_id VARCHAR(36) NOT NULL,
	actor_type VARCHAR(32) NOT NULL,
	actor_id VARCHAR(255) NOT NULL,
	role VARCHAR(64) NOT NULL DEFAULT 'member',
	capabilities_json TEXT,
	joined_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	UNIQUE KEY uniq_group_actor (group_id, actor_type, actor_id),
	INDEX idx_group_members_group (group_id),
	INDEX idx_group_members_actor (actor_type, actor_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS group_invites (
	id BIGINT AUTO_INCREMENT PRIMARY KEY,
	group_id VARCHAR(36) NOT NULL,
	token_hash VARCHAR(64) NOT NULL UNIQUE,
	actor_type_allowed VARCHAR(32),
	role VARCHAR(64) NOT NULL DEFAULT 'member',
	max_uses INT NOT NULL DEFAULT 1,
	used_count INT NOT NULL DEFAULT 0,
	expires_at TIMESTAMP NULL,
	status VARCHAR(32) NOT NULL DEFAULT 'active',
	created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	INDEX idx_group_invites_group (group_id),
	INDEX idx_group_invites_status (status)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS group_member_tokens (
	id BIGINT AUTO_INCREMENT PRIMARY KEY,
	group_id VARCHAR(36) NOT NULL,
	actor_type VARCHAR(32) NOT NULL,
	actor_id VARCHAR(255) NOT NULL,
	token_hash VARCHAR(64) NOT NULL UNIQUE,
	expires_at TIMESTAMP NULL,
	revoked_at TIMESTAMP NULL,
	created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	INDEX idx_group_member_tokens_group (group_id),
	INDEX idx_group_member_tokens_actor (actor_type, actor_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS group_events (
	id BIGINT AUTO_INCREMENT PRIMARY KEY,
	group_id VARCHAR(36) NOT NULL,
	event_type VARCHAR(64) NOT NULL,
	sender_type VARCHAR(32) NOT NULL,
	sender_id VARCHAR(255) NOT NULL,
	content TEXT,
	metadata_json TEXT,
	created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	INDEX idx_group_events_group (group_id, created_at),
	INDEX idx_group_events_type (event_type)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS group_artifacts (
	id VARCHAR(36) PRIMARY KEY,
	group_id VARCHAR(36) NOT NULL,
	name VARCHAR(255) NOT NULL,
	artifact_type VARCHAR(64) NOT NULL DEFAULT 'document',
	version INT NOT NULL DEFAULT 1,
	content MEDIUMTEXT,
	status VARCHAR(32) NOT NULL DEFAULT 'draft',
	created_by VARCHAR(255),
	created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
	INDEX idx_group_artifacts_group (group_id),
	INDEX idx_group_artifacts_status (status)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
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
