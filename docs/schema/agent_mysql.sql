-- Agent 信息表（MySQL 8+，InnoDB，utf8mb4）
-- 与 docs/数据库设计-Agent信息.md 一致

CREATE TABLE IF NOT EXISTS t_caichip_agent (
    id                          BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
    agent_id                    VARCHAR(64) NOT NULL,
    queue                       VARCHAR(128) NOT NULL DEFAULT 'default',
    hostname                    VARCHAR(256) NULL,
    protocol_version            VARCHAR(32) NULL,
    python_version              VARCHAR(64) NULL,
    agent_version               VARCHAR(64) NULL,
    last_reported_at          DATETIME(3) NULL,
    last_task_heartbeat_at    DATETIME(3) NULL,
    agent_status                VARCHAR(16) NOT NULL DEFAULT 'unknown' COMMENT 'online | offline | unknown（任务心跳）',
    last_script_sync_heartbeat_at DATETIME(3) NULL,
    created_at                  DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    updated_at                  DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
    UNIQUE KEY uk_caichip_agent_agent_id (agent_id),
    KEY idx_queue (queue),
    KEY idx_last_task_hb (last_task_heartbeat_at),
    KEY idx_updated (updated_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS t_caichip_agent_tag (
    agent_id VARCHAR(64) NOT NULL,
    tag      VARCHAR(256) NOT NULL,
    PRIMARY KEY (agent_id, tag),
    CONSTRAINT fk_agent_tag_agent FOREIGN KEY (agent_id) REFERENCES t_caichip_agent (agent_id) ON DELETE CASCADE,
    KEY idx_tag (tag)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS t_caichip_agent_installed_script (
    agent_id         VARCHAR(64) NOT NULL,
    script_id        VARCHAR(128) NOT NULL,
    version          VARCHAR(64)  NOT NULL,
    env_status       VARCHAR(32)  NOT NULL,
    package_sha256   VARCHAR(64) NULL,
    message          TEXT NULL,
    updated_at       DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
    PRIMARY KEY (agent_id, script_id),
    CONSTRAINT fk_agent_script_agent FOREIGN KEY (agent_id) REFERENCES t_caichip_agent (agent_id) ON DELETE CASCADE,
    KEY idx_env_status (env_status)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
