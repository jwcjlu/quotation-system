-- Agent 脚本包分发 — MySQL 8+（维度：**script_id**，与「平台」口径合一）
-- status: uploaded | published | archived
-- 同一 script_id 在业务上至多一行 published（由 SetPublished 事务保证）

CREATE TABLE IF NOT EXISTS t_agent_script_package (
    id                 BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
    script_id          VARCHAR(128) NOT NULL,
    version            VARCHAR(64)  NOT NULL,
    sha256             CHAR(64)     NOT NULL COMMENT 'lowercase hex',
    storage_rel_path   VARCHAR(512) NOT NULL COMMENT 'relative to script_store.root',
    filename           VARCHAR(255) NOT NULL,
    entry_file         VARCHAR(255) NOT NULL DEFAULT '' COMMENT 'entry python filename inside package',
    status             VARCHAR(32)  NOT NULL DEFAULT 'uploaded',
    release_notes      TEXT         NULL,
    created_at         DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    updated_at         DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
    UNIQUE KEY uk_pkg_script_version (script_id, version),
    KEY idx_pkg_script_status (script_id, status)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
