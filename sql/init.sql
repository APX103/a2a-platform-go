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
