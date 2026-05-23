-- A2A Platform MySQL initialization

CREATE DATABASE IF NOT EXISTS a2a_platform CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;

USE a2a_platform;

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
