package model

import "time"

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
	Id           int64     `db:"id" json:"-"`
	LocalTaskId  string    `db:"local_task_id" json:"local_task_id"`
	ServerTaskId *string   `db:"server_task_id" json:"server_task_id"`
	AgentName    string    `db:"agent_name" json:"agent_name"`
	ContextId    *string   `db:"context_id" json:"context_id"`
	State        string    `db:"state" json:"state"`
	CreatedAt    time.Time `db:"created_at" json:"created_at"`
	UpdatedAt    time.Time `db:"updated_at" json:"updated_at"`
}

// Message represents a chat message in a task.
type Message struct {
	Id        int64     `db:"id" json:"id"`
	TaskId    string    `db:"task_id" json:"task_id"`
	Role      string    `db:"role" json:"role"`
	Content   string    `db:"content" json:"content"`
	Timestamp time.Time `db:"timestamp" json:"timestamp"`
}

// TraceEvent represents a trace event for observability.
type TraceEvent struct {
	Id          int64     `db:"id" json:"id"`
	TaskId      string    `db:"task_id" json:"task_id"`
	ContextId   *string   `db:"context_id" json:"context_id"`
	Timestamp   time.Time `db:"timestamp" json:"timestamp"`
	EventType   string    `db:"event_type" json:"event_type"`
	AgentName   string    `db:"agent_name" json:"agent_name"`
	TargetAgent *string   `db:"target_agent" json:"target_agent"`
	DataJson    string    `db:"data_json" json:"data_json"`
	DurationMs  *int64    `db:"duration_ms" json:"duration_ms"`
}

// ===== API Request/Response types =====

type AgentInfo struct {
	Name          string   `json:"name"`
	Description   string   `json:"description"`
	Url           string   `json:"url"`
	Status        string   `json:"status"`
	Type          string   `json:"type"`
	Version       string   `json:"version"`
	Skills        []Skill  `json:"skills"`
	ErrorMessage  *string  `json:"error_message,omitempty"`
	AgentCardJson string   `json:"agent_card_json,omitempty"`
}

type Skill struct {
	Id          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Tags        []string `json:"tags"`
	Examples    []string `json:"examples"`
}

type RegisterAgentReq struct {
	Name   string   `json:"name"`
	Type   string   `json:"type"`
	Port   int      `json:"port"`
	Url    string   `json:"url"`
	Skills []Skill  `json:"skills"`
	Secret string   `json:"secret"`
}

type RegisterAgentResp struct {
	Ok            bool   `json:"ok"`
	Name          string `json:"name"`
	Url           string `json:"url"`
	Status        string `json:"status"`
	ReRegistered  bool   `json:"re_registered"`
}

type ListTasksResp struct {
	Items []TaskItem `json:"items"`
	Total int64      `json:"total"`
	Page  int        `json:"page"`
	Size  int        `json:"size"`
}

type TaskItem struct {
	LocalTaskId string  `json:"local_task_id"`
	DisplayId   string  `json:"display_id"`
	AgentName   string  `json:"agent_name"`
	State       string  `json:"state"`
	ContextId   *string `json:"context_id"`
	CreatedAt   string  `json:"created_at"`
	UpdatedAt   string  `json:"updated_at"`
}

type TraceResp struct {
	TaskId string `json:"task_id"`
	Trace  string `json:"trace"`
}

type ContextTraceResp struct {
	ContextId string `json:"context_id"`
	Trace     string `json:"trace"`
}
