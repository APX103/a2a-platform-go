package model

import "time"

const (
	ContextModeContext   = "context"
	ContextModeStateless = "stateless"
)

// Agent represents a registered A2A agent.
type Agent struct {
	Id            int64     `db:"id" json:"-"`
	Name          string    `db:"name" json:"name"`
	Type          string    `db:"type" json:"type"`
	Url           string    `db:"url" json:"url"`
	Port          int       `db:"port" json:"port"`
	SkillsJson    string    `db:"skills_json" json:"skills_json"`
	Status        string    `db:"status" json:"status"`
	ConnectedAt   *string   `db:"connected_at" json:"connected_at"`
	AgentCardJson string    `db:"agent_card_json" json:"agent_card_json"`
	ErrorMessage  *string   `db:"error_message" json:"error_message"`
	Secret        string    `db:"secret" json:"-"`
	CreatedAt     time.Time `db:"created_at" json:"-"`
	UpdatedAt     time.Time `db:"updated_at" json:"-"`
}

// Task represents an A2A task.
type Task struct {
	Id               int64     `db:"id" json:"-"`
	LocalTaskId      string    `db:"local_task_id" json:"local_task_id"`
	ServerTaskId     *string   `db:"server_task_id" json:"server_task_id"`
	SourceAgent      *string   `db:"source_agent" json:"source_agent,omitempty"`
	TargetAgent      string    `db:"target_agent" json:"target_agent"`
	AgentName        string    `db:"agent_name" json:"agent_name"`
	ContextId        *string   `db:"context_id" json:"context_id"`
	RootContextId    *string   `db:"root_context_id" json:"root_context_id,omitempty"`
	ParentTaskId     *string   `db:"parent_task_id" json:"parent_task_id,omitempty"`
	ParentToolCallId *string   `db:"parent_tool_call_id" json:"parent_tool_call_id,omitempty"`
	State            string    `db:"state" json:"state"`
	CreatedAt        time.Time `db:"created_at" json:"created_at"`
	UpdatedAt        time.Time `db:"updated_at" json:"updated_at"`
}

// Message represents a chat message in a task.
type Message struct {
	Id               int64     `db:"id" json:"id"`
	TaskId           string    `db:"task_id" json:"task_id"`
	ContextId        *string   `db:"context_id" json:"context_id,omitempty"`
	Role             string    `db:"role" json:"role"`
	SenderAgent      *string   `db:"sender_agent" json:"sender_agent,omitempty"`
	RecipientAgent   *string   `db:"recipient_agent" json:"recipient_agent,omitempty"`
	Content          string    `db:"content" json:"content"`
	ReasoningContent *string   `db:"reasoning_content" json:"reasoning_content,omitempty"`
	ToolCalls        string    `db:"tool_calls" json:"tool_calls,omitempty"`
	ToolCallId       *string   `db:"tool_call_id" json:"tool_call_id,omitempty"`
	ThinkingBlocks   string    `db:"thinking_blocks" json:"thinking_blocks,omitempty"`
	Timestamp        time.Time `db:"timestamp" json:"timestamp"`
}

// ToolCall represents a single tool invocation.
type ToolCall struct {
	ID        string                 `json:"id"`
	Name      string                 `json:"name"`
	Arguments string                 `json:"arguments"`
	Result    string                 `json:"result,omitempty"`
	Status    string                 `json:"status,omitempty"` // "started", "completed", "error"
	StartTime *time.Time             `json:"start_time,omitempty"`
	EndTime   *time.Time             `json:"end_time,omitempty"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
}

// ThinkingBlock represents a time-bounded thinking segment.
type ThinkingBlock struct {
	ID        string    `json:"id"`
	Timestamp time.Time `json:"timestamp"`
	Content   string    `json:"content"`
	Duration  int64     `json:"duration_ms,omitempty"` // milliseconds since previous block
}

// Context represents a chat session/conversation context.
type Context struct {
	ID           string    `db:"id" json:"id"`
	AgentName    string    `db:"agent_name" json:"agent_name"`
	Title        string    `db:"title" json:"title"`
	MessageCount int       `db:"message_count" json:"message_count"`
	CreatedAt    time.Time `db:"created_at" json:"created_at"`
	UpdatedAt    time.Time `db:"updated_at" json:"updated_at"`
}

// SubagentSession represents a spawned subagent execution.
type SubagentSession struct {
	ID               string     `db:"id" json:"id"`
	ParentContextId  string     `db:"parent_context_id" json:"parent_context_id"`
	ParentToolCallId string     `db:"parent_tool_call_id" json:"parent_tool_call_id"`
	Task             string     `db:"task" json:"task"`
	Context          string     `db:"context" json:"context"`
	Status           string     `db:"status" json:"status"`     // "running", "completed", "failed", "timeout"
	Messages         string     `db:"messages" json:"messages"` // JSON array
	Result           string     `db:"result" json:"result,omitempty"`
	Error            string     `db:"error" json:"error,omitempty"`
	CreatedAt        time.Time  `db:"created_at" json:"created_at"`
	CompletedAt      *time.Time `db:"completed_at" json:"completed_at,omitempty"`
}

// TraceEvent represents a trace event for observability.
type TraceEvent struct {
	Id            int64     `db:"id" json:"id"`
	TaskId        string    `db:"task_id" json:"task_id"`
	ContextId     *string   `db:"context_id" json:"context_id"`
	RootContextId *string   `db:"root_context_id" json:"root_context_id,omitempty"`
	ParentTaskId  *string   `db:"parent_task_id" json:"parent_task_id,omitempty"`
	Timestamp     time.Time `db:"timestamp" json:"timestamp"`
	EventType     string    `db:"event_type" json:"event_type"`
	AgentName     string    `db:"agent_name" json:"agent_name"`
	TargetAgent   *string   `db:"target_agent" json:"target_agent"`
	DataJson      string    `db:"data_json" json:"data_json"`
	DurationMs    *int64    `db:"duration_ms" json:"duration_ms"`
}

// ===== API Request/Response types =====

type AgentInfo struct {
	Name          string  `json:"name"`
	Description   string  `json:"description"`
	Url           string  `json:"url"`
	Status        string  `json:"status"`
	Type          string  `json:"type"`
	Version       string  `json:"version"`
	ContextMode   string  `json:"context_mode,omitempty"`
	Skills        []Skill `json:"skills"`
	ErrorMessage  *string `json:"error_message,omitempty"`
	AgentCardJson string  `json:"agent_card_json,omitempty"`
}

// AgentCard describes an external A2A agent's identity and capabilities.
type AgentCard struct {
	Name        string      `json:"name"`
	Description string      `json:"description"`
	Version     string      `json:"version"`
	Url         string      `json:"url"`
	Skills      []CardSkill `json:"skills"`
	HealthUrl   string      `json:"health_url,omitempty"`
	Static      bool        `json:"x_static,omitempty"`
	ContextMode string      `json:"x_context_mode,omitempty"`
}

type CardSkill struct {
	Id          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Tags        []string `json:"tags"`
	Examples    []string `json:"examples"`
}

type Skill struct {
	Id          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Tags        []string `json:"tags"`
	Examples    []string `json:"examples"`
}

const (
	DefaultP2PGroupID = "default-p2p"

	GroupModeP2P             = "p2p"
	GroupModeLeaderLed       = "leader_led"
	GroupModeFreeChat        = "free_chat"
	GroupModeRoundtable      = "roundtable"
	GroupModeStateflow       = "stateflow"
	GroupModeResearchLongRun = "research_long_horizon"

	GroupStatusActive   = "active"
	GroupStatusArchived = "archived"

	GroupActorAgent  = "agent"
	GroupActorHuman  = "human"
	GroupActorSystem = "system"
)

// Group is the native A2A collaboration boundary for discovery, context, permissions, and orchestration.
type Group struct {
	ID                string    `db:"id" json:"id"`
	Name              string    `db:"name" json:"name"`
	Description       string    `db:"description" json:"description,omitempty"`
	OrchestrationMode string    `db:"orchestration_mode" json:"orchestration_mode"`
	RulesJson         string    `db:"rules_json" json:"rules_json,omitempty"`
	MemoryPolicyJson  string    `db:"memory_policy_json" json:"memory_policy_json,omitempty"`
	Status            string    `db:"status" json:"status"`
	CreatedAt         time.Time `db:"created_at" json:"created_at"`
	UpdatedAt         time.Time `db:"updated_at" json:"updated_at"`
}

type GroupMember struct {
	ID               int64     `db:"id" json:"id"`
	GroupID          string    `db:"group_id" json:"group_id"`
	ActorType        string    `db:"actor_type" json:"actor_type"`
	ActorID          string    `db:"actor_id" json:"actor_id"`
	Role             string    `db:"role" json:"role"`
	CapabilitiesJson string    `db:"capabilities_json" json:"capabilities_json,omitempty"`
	JoinedAt         time.Time `db:"joined_at" json:"joined_at"`
}

type GroupInvite struct {
	ID               int64      `db:"id" json:"id"`
	GroupID          string     `db:"group_id" json:"group_id"`
	TokenHash        string     `db:"token_hash" json:"-"`
	ActorTypeAllowed string     `db:"actor_type_allowed" json:"actor_type_allowed,omitempty"`
	Role             string     `db:"role" json:"role"`
	MaxUses          int        `db:"max_uses" json:"max_uses"`
	UsedCount        int        `db:"used_count" json:"used_count"`
	ExpiresAt        *time.Time `db:"expires_at" json:"expires_at,omitempty"`
	Status           string     `db:"status" json:"status"`
	CreatedAt        time.Time  `db:"created_at" json:"created_at"`
}

type GroupMemberToken struct {
	ID        int64      `db:"id" json:"id"`
	GroupID   string     `db:"group_id" json:"group_id"`
	ActorType string     `db:"actor_type" json:"actor_type"`
	ActorID   string     `db:"actor_id" json:"actor_id"`
	TokenHash string     `db:"token_hash" json:"-"`
	ExpiresAt *time.Time `db:"expires_at" json:"expires_at,omitempty"`
	RevokedAt *time.Time `db:"revoked_at" json:"revoked_at,omitempty"`
	CreatedAt time.Time  `db:"created_at" json:"created_at"`
}

type GroupEvent struct {
	ID           int64     `db:"id" json:"id"`
	GroupID      string    `db:"group_id" json:"group_id"`
	EventType    string    `db:"event_type" json:"event_type"`
	SenderType   string    `db:"sender_type" json:"sender_type"`
	SenderID     string    `db:"sender_id" json:"sender_id"`
	Content      string    `db:"content" json:"content"`
	MetadataJson string    `db:"metadata_json" json:"metadata_json,omitempty"`
	CreatedAt    time.Time `db:"created_at" json:"created_at"`
}

type GroupArtifact struct {
	ID           string    `db:"id" json:"id"`
	GroupID      string    `db:"group_id" json:"group_id"`
	Name         string    `db:"name" json:"name"`
	ArtifactType string    `db:"artifact_type" json:"artifact_type"`
	Version      int       `db:"version" json:"version"`
	Content      string    `db:"content" json:"content"`
	Status       string    `db:"status" json:"status"`
	CreatedBy    string    `db:"created_by" json:"created_by,omitempty"`
	CreatedAt    time.Time `db:"created_at" json:"created_at"`
	UpdatedAt    time.Time `db:"updated_at" json:"updated_at"`
}

type GroupOrchestrationState struct {
	GroupID           string   `json:"group_id"`
	Mode              string   `json:"mode"`
	NextAction        string   `json:"next_action"`
	EligibleSpeakers  []string `json:"eligible_speakers"`
	ContextPolicy     string   `json:"context_policy"`
	TerminationPolicy string   `json:"termination_policy"`
}

type RegisterAgentReq struct {
	Name        string     `json:"name"`
	Type        string     `json:"type"`
	Port        int        `json:"port"`
	Url         string     `json:"url"`
	Skills      []Skill    `json:"skills"`
	Secret      string     `json:"secret"`
	ContextMode string     `json:"context_mode,omitempty"`
	AgentCard   *AgentCard `json:"agent_card,omitempty"`
	SimpleMode  bool       `json:"simple_mode,omitempty"`
}

type RegisterAgentResp struct {
	Ok             bool   `json:"ok"`
	Name           string `json:"name"`
	Url            string `json:"url"`
	Status         string `json:"status"`
	ReRegistered   bool   `json:"re_registered"`
	SimpleMode     bool   `json:"simple_mode,omitempty"`
	DefaultGroupID string `json:"default_group_id,omitempty"`
}

type ListTasksResp struct {
	Items []TaskListItem `json:"items"`
	Total int64          `json:"total"`
	Page  int            `json:"page"`
	Size  int            `json:"size"`
}

type TaskListItem struct {
	LocalTaskId      string  `json:"local_task_id"`
	DisplayId        string  `json:"display_id"`
	SourceAgent      *string `json:"source_agent,omitempty"`
	TargetAgent      string  `json:"target_agent"`
	AgentName        string  `json:"agent_name"`
	State            string  `json:"state"`
	ContextId        *string `json:"context_id"`
	RootContextId    *string `json:"root_context_id,omitempty"`
	ParentTaskId     *string `json:"parent_task_id,omitempty"`
	ParentToolCallId *string `json:"parent_tool_call_id,omitempty"`
	CreatedAt        string  `json:"created_at"`
	UpdatedAt        string  `json:"updated_at"`
}

type TraceContextSummary struct {
	ContextId  string    `json:"context_id"`
	TraceCount int       `json:"trace_count"`
	LastActive time.Time `json:"last_active"`
	Agents     []string  `json:"agents"`
}

type TraceResp struct {
	TaskId string `json:"task_id"`
	Trace  string `json:"trace"`
}

type ContextTraceResp struct {
	ContextId string `json:"context_id"`
	Trace     string `json:"trace"`
}

// CreateContextReq creates a new context/session.
type CreateContextReq struct {
	AgentName string `json:"agent_name"`
	Title     string `json:"title,omitempty"`
}

// ContextListItem represents an item in the context list.
type ContextListItem struct {
	ID           string `json:"id"`
	AgentName    string `json:"agent_name"`
	Title        string `json:"title"`
	MessageCount int    `json:"message_count"`
	CreatedAt    string `json:"created_at"`
	UpdatedAt    string `json:"updated_at"`
}

// ContextDetailResp includes the context with its messages.
type ContextDetailResp struct {
	Context  *Context  `json:"context"`
	Messages []Message `json:"messages"`
}

// ListContextsResp paginated context list.
type ListContextsResp struct {
	Items []ContextListItem `json:"items"`
	Total int64             `json:"total"`
	Page  int               `json:"page"`
	Size  int               `json:"size"`
}

// ChatSSEEvent represents an SSE event for chat streaming.
type ChatSSEEvent struct {
	Type      string      `json:"type"` // text.delta, thinking.delta, tool.call_start, etc.
	TaskId    string      `json:"taskId,omitempty"`
	ContextId string      `json:"contextId,omitempty"`
	Text      string      `json:"text,omitempty"`
	Thinking  string      `json:"thinking,omitempty"`
	Tool      ToolCall    `json:"tool,omitempty"`
	Error     string      `json:"error,omitempty"`
	Metadata  interface{} `json:"metadata,omitempty"`
}

// SubagentStreamEvent represents a subagent execution event for SSE streaming.
type SubagentStreamEvent struct {
	Type       string `json:"type"` // subagent_started, subagent_tool_call, subagent_tool_result, subagent_completed, subagent_error
	SubagentId string `json:"subagent_id"`
	Task       string `json:"task,omitempty"`
	ToolName   string `json:"tool_name,omitempty"`
	Arguments  string `json:"arguments,omitempty"`
	Result     string `json:"result,omitempty"`
	Error      string `json:"error,omitempty"`
}
