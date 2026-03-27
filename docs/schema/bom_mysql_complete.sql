-- =============================================================================
-- BOM 货源搜索与配单 — MySQL 完整脚本（MySQL 8+，InnoDB，utf8mb4）
-- =============================================================================
-- 合并了：docs/schema/bom_mysql.sql 全量 DDL
--        + docs/schema/migrations/20260327_bom_readiness.sql（逻辑）
--        + docs/schema/migrations/20260327_bom_merge_inflight.sql（逻辑）
--
-- 使用方式：
--   · 全新库：执行「第一部分」即可（或继续执行第二部分，增量段为幂等，不会破坏新库）。
--   · 旧库（缺列/缺表）：可只执行「第二部分」做补齐；亦可执行全文。
--
-- 物理表名统一前缀 t_（与 internal/data/table_names.go 一致）。
-- 调度队列表（t_caichip_dispatch_task 等）见 docs/schema/agent_dispatch_task_mysql.sql，
-- 与 t_bom_search_task.caichip_task_id、t_bom_merge_inflight.task_id 关联。
--
-- bom_search_task.state 允许值：pending, running, succeeded, no_result,
--   failed_retryable, failed_terminal, cancelled, skipped
-- =============================================================================

-- -----------------------------------------------------------------------------
-- 第一部分：全量建表（与 bom_mysql.sql 保持一致，便于单一文件交付）
-- -----------------------------------------------------------------------------

CREATE TABLE IF NOT EXISTS t_bom_session (
    id                      CHAR(36) NOT NULL COMMENT '会话/母单 UUID，主键',
    title                   VARCHAR(256) NULL COMMENT '标题或内部简称',
    customer_name           VARCHAR(256) NULL COMMENT '客户/公司名称',
    contact_phone           VARCHAR(64)  NULL COMMENT '联系电话',
    contact_email           VARCHAR(256) NULL COMMENT '联系邮箱',
    contact_extra           VARCHAR(512) NULL COMMENT '扩展联系信息（JSON 文本或备注摘要；联系人/备注/内部单号亦可后续拆独立列）',
    status                  VARCHAR(32)  NOT NULL DEFAULT 'draft' COMMENT '会话状态：draft/searching/data_ready/blocked/cancelled 等',
    readiness_mode          VARCHAR(16)  NOT NULL DEFAULT 'lenient' COMMENT '数据已准备判定：lenient=仅要求平台终态齐全；strict=另要求每行至少一平台 succeeded',
    biz_date                DATE         NOT NULL COMMENT '业务日：报价缓存与搜索任务去重维度之一',
    selection_revision      INT          NOT NULL DEFAULT 1 COMMENT '选配/修订版本号，行或平台变更时可递增',
    platform_ids            JSON         NOT NULL COMMENT '本轮参与搜索的平台 ID 列表，如 ["ickey","szlcsc"]',
    parse_mode              VARCHAR(32)  NULL COMMENT 'Excel/导入解析模式标识',
    storage_file_key        VARCHAR(512) NULL COMMENT '原始上传文件在对象存储中的键',
    created_at              DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3) COMMENT '创建时间',
    updated_at              DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3) COMMENT '最后更新时间',
    PRIMARY KEY (id),
    KEY idx_bom_session_biz_date (biz_date),
    KEY idx_bom_session_status (status),
    KEY idx_bom_session_updated (updated_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
COMMENT='BOM 配单母单（会话）：客户维度 + 平台勾选 + 生命周期状态';

CREATE TABLE IF NOT EXISTS t_bom_session_line (
    id                      BIGINT UNSIGNED NOT NULL AUTO_INCREMENT COMMENT '行主键',
    session_id              CHAR(36) NOT NULL COMMENT '所属 bom_session.id',
    line_no                 INT  NOT NULL COMMENT '行号，会话内唯一，用于展示与导入对齐',
    raw_text                TEXT NULL COMMENT '原始行文本备份（导入溯源）',
    mpn                     VARCHAR(256) NOT NULL COMMENT '型号（原始/展示用）；规范化见应用层 mpn_norm',
    mfr                     VARCHAR(256) NULL COMMENT '厂牌/制造商',
    package                 VARCHAR(128) NULL COMMENT '封装',
    qty                     DECIMAL(18, 4) NULL COMMENT '数量',
    extra_json              JSON NULL COMMENT '扩展字段：参数、备注等',
    created_at              DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) COMMENT '创建时间',
    PRIMARY KEY (id),
    UNIQUE KEY uk_session_line (session_id, line_no),
    KEY idx_bom_line_session (session_id),
    KEY idx_bom_line_mpn (session_id, mpn),
    CONSTRAINT fk_bom_line_session FOREIGN KEY (session_id) REFERENCES t_bom_session (id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
COMMENT='BOM 会话行明细：一行对应多平台搜索任务的物料维度';

CREATE TABLE IF NOT EXISTS t_bom_quote_cache (
    mpn_norm                VARCHAR(256) NOT NULL COMMENT '规范化型号，与搜索任务一致',
    platform_id             VARCHAR(32)  NOT NULL COMMENT '平台 ID，见 bom_platform_script',
    biz_date                DATE         NOT NULL COMMENT '业务日，与任务/报价批次对齐',
    outcome                 VARCHAR(32)  NOT NULL COMMENT '结果概要：有报价/无结果/失败等，枚举以应用为准',
    quotes_json             JSON NULL COMMENT '结构化报价列表（成功时）',
    no_mpn_detail           JSON NULL COMMENT '无型号或无结果时的详情',
    raw_ref                 VARCHAR(512) NULL COMMENT '原始抓取引用（URL、快照键等）',
    created_at              DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) COMMENT '首次写入时间',
    updated_at              DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3) COMMENT '最后更新时间',
    PRIMARY KEY (mpn_norm, platform_id, biz_date),
    KEY idx_bom_quote_cache_updated (updated_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
COMMENT='按 (型号,平台,业务日) 缓存报价；任务作废后历史行可保留供审计';

CREATE TABLE IF NOT EXISTS t_bom_search_task (
    id                      BIGINT UNSIGNED NOT NULL AUTO_INCREMENT COMMENT '搜索任务主键',
    session_id              CHAR(36) NOT NULL COMMENT '所属 bom_session.id',
    mpn_norm                VARCHAR(256) NOT NULL COMMENT '规范化型号，与缓存主键一致',
    platform_id             VARCHAR(32)  NOT NULL COMMENT '目标平台 ID',
    biz_date                DATE         NOT NULL COMMENT '业务日',
    state                   VARCHAR(32)  NOT NULL DEFAULT 'pending' COMMENT '任务状态：见脚本头部 bom_search_task.state 允许值列表',
    auto_attempt            INT          NOT NULL DEFAULT 0 COMMENT '自动重试次数',
    manual_attempt          INT          NOT NULL DEFAULT 0 COMMENT '用户手动重试次数',
    selection_revision      INT          NOT NULL COMMENT '创建时的会话修订号，用于与行变更对齐',
    caichip_task_id         VARCHAR(128) NULL COMMENT 'caichip_dispatch_task.task_id；多条业务行可共用同一 ID（同 mpn_norm+platform+biz_date 合并调度），勿对列做 UNIQUE',
    last_error              TEXT NULL COMMENT '最后一次失败错误信息',
    created_at              DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) COMMENT '创建时间',
    updated_at              DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3) COMMENT '最后更新时间',
    PRIMARY KEY (id),
    UNIQUE KEY uk_bom_search (session_id, mpn_norm, platform_id, biz_date),
    KEY idx_bom_search_session_state (session_id, state),
    KEY idx_bom_search_caichip_task (caichip_task_id),
    KEY idx_bom_search_mpn (mpn_norm, platform_id, biz_date),
    CONSTRAINT fk_bom_search_session FOREIGN KEY (session_id) REFERENCES t_bom_session (id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
COMMENT='平台×型号×业务日×会话 粒度搜索任务；多会话同键可共用一条 caichip_dispatch_task（合并调度）';

CREATE TABLE IF NOT EXISTS t_bom_merge_inflight (
    mpn_norm    VARCHAR(256) NOT NULL COMMENT '规范化型号',
    platform_id VARCHAR(32)  NOT NULL COMMENT '平台 ID',
    biz_date    DATE         NOT NULL COMMENT '业务日',
    task_id     VARCHAR(128) NOT NULL COMMENT '在途 caichip_dispatch_task.task_id；调度终态后由应用删除',
    created_at  DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3) COMMENT '创建时间',
    PRIMARY KEY (mpn_norm, platform_id, biz_date),
    KEY idx_bom_merge_inflight_task (task_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
COMMENT='合并键→在途调度；finished/cancelled 后删除以允许新一轮抓取';

CREATE TABLE IF NOT EXISTS t_bom_platform_script (
    platform_id             VARCHAR(32) NOT NULL COMMENT '平台唯一标识，与任务/缓存 platform_id 一致',
    script_id                 VARCHAR(128) NOT NULL COMMENT 'Agent 侧脚本 ID',
    display_name              VARCHAR(128) NULL COMMENT '展示名称',
    enabled                   TINYINT(1) NOT NULL DEFAULT 1 COMMENT '是否启用该平台',
    updated_at                DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3) COMMENT '配置更新时间',
    PRIMARY KEY (platform_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
COMMENT='平台与采集脚本的映射，可配置扩展';

INSERT INTO t_bom_platform_script (platform_id, script_id, display_name) VALUES
    ('find_chips', 'find_chips', 'FindChips'),
    ('hqchip', 'hqchip', 'HQChip'),
    ('icgoo', 'icgoo', 'ICGOO'),
    ('ickey', 'ickey', '云汉芯城'),
    ('szlcsc', 'szlcsc', '立创商城')
ON DUPLICATE KEY UPDATE script_id = VALUES(script_id);

-- -----------------------------------------------------------------------------
-- 第二部分：旧库增量（幂等；新库执行第一部分后此处多为 no-op）
-- 设计：readiness — specs 2026-03-27-bom-sourcing-design §2.3
--       merge_inflight — §3.5
-- -----------------------------------------------------------------------------

SET @__bom_db := DATABASE();

-- bom_session.readiness_mode（无则 ADD，已有则跳过）
SET @__bom_rdy := (
    SELECT COUNT(*) FROM information_schema.COLUMNS
    WHERE TABLE_SCHEMA = @__bom_db AND TABLE_NAME = 't_bom_session' AND COLUMN_NAME = 'readiness_mode'
);
SET @__bom_sql_rdy := IF(@__bom_rdy = 0,
    'ALTER TABLE t_bom_session ADD COLUMN readiness_mode VARCHAR(16) NOT NULL DEFAULT ''lenient'' COMMENT ''数据已准备判定：lenient=仅要求平台终态齐全；strict=另要求每行至少一平台 succeeded'' AFTER status',
    'SELECT ''t_bom_session.readiness_mode already exists'' AS bom_migration_msg'
);
PREPARE __bom_stmt_rdy FROM @__bom_sql_rdy;
EXECUTE __bom_stmt_rdy;
DEALLOCATE PREPARE __bom_stmt_rdy;

-- bom_merge_inflight（与第一部分同结构，CREATE IF NOT EXISTS）
CREATE TABLE IF NOT EXISTS t_bom_merge_inflight (
    mpn_norm    VARCHAR(256) NOT NULL,
    platform_id VARCHAR(32)  NOT NULL,
    biz_date    DATE         NOT NULL,
    task_id     VARCHAR(128) NOT NULL COMMENT 'caichip_dispatch_task.task_id，在途期间唯一',
    created_at  DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    PRIMARY KEY (mpn_norm, platform_id, biz_date),
    KEY idx_bom_merge_inflight_task (task_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
COMMENT='合并键→在途调度 task_id；finished/cancelled 后删除以允许新一轮抓取';
