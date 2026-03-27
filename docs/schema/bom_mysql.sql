-- BOM 货源搜索与配单 — 表结构草案（MySQL 8+，InnoDB，utf8mb4）
-- 项目 BOM 域表结构以本文件为准（MySQL）

CREATE TABLE IF NOT EXISTS bom_session (
    id                      CHAR(36) NOT NULL PRIMARY KEY,
    title                   VARCHAR(256) NULL,
    customer_name           VARCHAR(256) NULL,
    contact_phone           VARCHAR(64)  NULL,
    contact_email           VARCHAR(256) NULL,
    contact_extra           VARCHAR(512) NULL,
    status                  VARCHAR(32)  NOT NULL DEFAULT 'draft',
    biz_date                DATE         NOT NULL,
    selection_revision      INT          NOT NULL DEFAULT 1,
    platform_ids            JSON         NOT NULL,
    parse_mode              VARCHAR(32)  NULL,
    storage_file_key        VARCHAR(512) NULL,
    created_at              DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    updated_at              DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
    KEY idx_bom_session_biz_date (biz_date),
    KEY idx_bom_session_status (status),
    KEY idx_bom_session_updated (updated_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS bom_session_line (
    id                      BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
    session_id              CHAR(36) NOT NULL,
    line_no                 INT  NOT NULL,
    raw_text                TEXT NULL,
    mpn                     VARCHAR(256) NOT NULL,
    mfr                     VARCHAR(256) NULL,
    package                 VARCHAR(128) NULL,
    qty                     DECIMAL(18, 4) NULL,
    extra_json              JSON NULL,
    created_at              DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    UNIQUE KEY uk_session_line (session_id, line_no),
    KEY idx_bom_line_session (session_id),
    KEY idx_bom_line_mpn (session_id, mpn),
    CONSTRAINT fk_bom_line_session FOREIGN KEY (session_id) REFERENCES bom_session (id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS bom_quote_cache (
    mpn_norm                VARCHAR(256) NOT NULL,
    platform_id             VARCHAR(32)  NOT NULL,
    biz_date                DATE         NOT NULL,
    outcome                 VARCHAR(32)  NOT NULL,
    quotes_json             JSON NULL,
    no_mpn_detail           JSON NULL,
    raw_ref                 VARCHAR(512) NULL,
    created_at              DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    updated_at              DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
    PRIMARY KEY (mpn_norm, platform_id, biz_date),
    KEY idx_bom_quote_cache_updated (updated_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS bom_search_task (
    id                      BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
    session_id              CHAR(36) NOT NULL,
    mpn_norm                VARCHAR(256) NOT NULL,
    platform_id             VARCHAR(32)  NOT NULL,
    biz_date                DATE         NOT NULL,
    state                   VARCHAR(32)  NOT NULL DEFAULT 'pending',
    auto_attempt            INT          NOT NULL DEFAULT 0,
    manual_attempt          INT          NOT NULL DEFAULT 0,
    selection_revision      INT          NOT NULL,
    caichip_task_id         VARCHAR(128) NULL,
    last_error              TEXT NULL,
    created_at              DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    updated_at              DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
    UNIQUE KEY uk_bom_search (session_id, mpn_norm, platform_id, biz_date),
    KEY idx_bom_search_session_state (session_id, state),
    KEY idx_bom_search_mpn (mpn_norm, platform_id, biz_date),
    CONSTRAINT fk_bom_search_session FOREIGN KEY (session_id) REFERENCES bom_session (id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS bom_platform_script (
    platform_id             VARCHAR(32) PRIMARY KEY,
    script_id                 VARCHAR(128) NOT NULL,
    display_name              VARCHAR(128) NULL,
    enabled                   TINYINT(1) NOT NULL DEFAULT 1,
    updated_at                DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

INSERT INTO bom_platform_script (platform_id, script_id, display_name) VALUES
    ('find_chips', 'find_chips', 'FindChips'),
    ('hqchip', 'hqchip', 'HQChip'),
    ('icgoo', 'icgoo', 'ICGOO'),
    ('ickey', 'ickey', '云汉芯城'),
    ('szlcsc', 'szlcsc', '立创商城')
ON DUPLICATE KEY UPDATE script_id = VALUES(script_id);
