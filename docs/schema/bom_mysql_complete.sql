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
    id                      BIGINT UNSIGNED NOT NULL AUTO_INCREMENT COMMENT '报价缓存主键',
    mpn_norm                VARCHAR(256) NOT NULL COMMENT '规范化型号，与搜索任务一致',
    platform_id             VARCHAR(32)  NOT NULL COMMENT '平台 ID，见 bom_platform_script',
    biz_date                DATE         NOT NULL COMMENT '业务日，与任务/报价批次对齐',
    outcome                 VARCHAR(32)  NOT NULL COMMENT '结果概要：有报价/无结果/失败等，枚举以应用为准',
    no_mpn_detail           JSON NULL COMMENT '无型号或无结果时的详情',
    raw_ref                 VARCHAR(512) NULL COMMENT '原始抓取引用（URL、快照键等）',
    created_at              DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) COMMENT '首次写入时间',
    updated_at              DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3) COMMENT '最后更新时间',
    PRIMARY KEY (id),
    UNIQUE KEY uk_bom_quote_cache_merge (mpn_norm, platform_id, biz_date),
    KEY idx_bom_quote_cache_updated (updated_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
COMMENT='按 (型号,平台,业务日) 缓存报价；任务作废后历史行可保留供审计';

CREATE TABLE IF NOT EXISTS t_bom_quote_item (
    id                      BIGINT UNSIGNED NOT NULL AUTO_INCREMENT COMMENT '报价明细主键',
    quote_id                BIGINT UNSIGNED NOT NULL COMMENT '关联 t_bom_quote_cache.id',
    model                   VARCHAR(255) NULL COMMENT '报价型号（平台返回）',
    manufacturer            VARCHAR(255) NULL COMMENT '厂牌原文',
    stock                   VARCHAR(64)  NULL COMMENT '库存原文，允许 N/A',
    package                 VARCHAR(255) NULL COMMENT '封装原文',
    `desc`                  TEXT NULL COMMENT '描述原文',
    datasheet_url           TEXT NULL COMMENT '数据手册 URL',
    moq                     VARCHAR(64)  NULL COMMENT '起订量原文，允许 N/A',
    lead_time               VARCHAR(128) NULL COMMENT '交期原文，如 1工作日',
    price_tiers             TEXT NULL COMMENT '阶梯价原文',
    hk_price                TEXT NULL COMMENT '香港价原文',
    mainland_price          TEXT NULL COMMENT '大陆价原文',
    query_model             VARCHAR(255) NULL COMMENT '查询型号（用于回溯）',
    created_at              DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) COMMENT '创建时间',
    updated_at              DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3) COMMENT '最后更新时间',
    PRIMARY KEY (id),
    KEY idx_bom_quote_item_quote_id (quote_id),
    KEY idx_bom_quote_item_query_model (query_model),
    CONSTRAINT fk_bom_quote_item_cache FOREIGN KEY (quote_id) REFERENCES t_bom_quote_cache (id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
COMMENT='BOM 报价明细：由缓存主表一对多展开';

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

CREATE TABLE IF NOT EXISTS t_hs_item (
    id              BIGINT UNSIGNED NOT NULL AUTO_INCREMENT COMMENT '主键',
    code_ts         VARCHAR(16) NOT NULL COMMENT '税则编码',
    g_name          VARCHAR(512) NOT NULL COMMENT '商品名称',
    unit_1          VARCHAR(16) NOT NULL DEFAULT '' COMMENT '第一计量单位',
    unit_2          VARCHAR(16) NOT NULL DEFAULT '' COMMENT '第二计量单位',
    control_mark    VARCHAR(64) NOT NULL DEFAULT '' COMMENT '监管条件',
    source_core_hs6 CHAR(6) NOT NULL DEFAULT '' COMMENT '来源 hs6',
    raw_json        JSON NULL COMMENT '原始返回',
    updated_at      DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3) COMMENT '更新时间',
    PRIMARY KEY (id),
    UNIQUE KEY uk_hs_item_code_ts (code_ts),
    KEY idx_hs_item_source_core_hs6 (source_core_hs6),
    KEY idx_hs_item_updated (updated_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
COMMENT='HS 条目缓存表（按 code_ts 唯一，支持 upsert）';

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

-- t_bom_quote_cache.id（无则 ADD，已有则跳过）
SET @__bom_has_cache_id := (
    SELECT COUNT(*) FROM information_schema.COLUMNS
    WHERE TABLE_SCHEMA = @__bom_db AND TABLE_NAME = 't_bom_quote_cache' AND COLUMN_NAME = 'id'
);
SET @__bom_sql_add_cache_id := IF(@__bom_has_cache_id = 0,
    'ALTER TABLE t_bom_quote_cache ADD COLUMN id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT FIRST',
    'SELECT ''t_bom_quote_cache.id already exists'' AS bom_migration_msg'
);
PREPARE __bom_stmt_add_cache_id FROM @__bom_sql_add_cache_id;
EXECUTE __bom_stmt_add_cache_id;
DEALLOCATE PREPARE __bom_stmt_add_cache_id;

-- t_bom_quote_cache 主键切换到 id（若尚未切换）
SET @__bom_cache_pk_is_id := (
    SELECT COUNT(*) FROM information_schema.KEY_COLUMN_USAGE
    WHERE TABLE_SCHEMA = @__bom_db
      AND TABLE_NAME = 't_bom_quote_cache'
      AND CONSTRAINT_NAME = 'PRIMARY'
      AND COLUMN_NAME = 'id'
);
SET @__bom_sql_switch_cache_pk := IF(@__bom_cache_pk_is_id = 0,
    'ALTER TABLE t_bom_quote_cache DROP PRIMARY KEY, ADD PRIMARY KEY (id)',
    'SELECT ''t_bom_quote_cache primary key already uses id'' AS bom_migration_msg'
);
PREPARE __bom_stmt_switch_cache_pk FROM @__bom_sql_switch_cache_pk;
EXECUTE __bom_stmt_switch_cache_pk;
DEALLOCATE PREPARE __bom_stmt_switch_cache_pk;

-- t_bom_quote_cache 合并键唯一索引（无则补）
SET @__bom_has_cache_merge_uk := (
    SELECT COUNT(*) FROM information_schema.STATISTICS
    WHERE TABLE_SCHEMA = @__bom_db
      AND TABLE_NAME = 't_bom_quote_cache'
      AND INDEX_NAME = 'uk_bom_quote_cache_merge'
);
SET @__bom_sql_add_cache_merge_uk := IF(@__bom_has_cache_merge_uk = 0,
    'ALTER TABLE t_bom_quote_cache ADD UNIQUE KEY uk_bom_quote_cache_merge (mpn_norm, platform_id, biz_date)',
    'SELECT ''uk_bom_quote_cache_merge already exists'' AS bom_migration_msg'
);
PREPARE __bom_stmt_add_cache_merge_uk FROM @__bom_sql_add_cache_merge_uk;
EXECUTE __bom_stmt_add_cache_merge_uk;
DEALLOCATE PREPARE __bom_stmt_add_cache_merge_uk;

-- t_hs_item.uk_hs_item_code_ts（无则补）
SET @__bom_has_hs_item_uk := (
    SELECT COUNT(*) FROM information_schema.STATISTICS
    WHERE TABLE_SCHEMA = @__bom_db
      AND TABLE_NAME = 't_hs_item'
      AND INDEX_NAME = 'uk_hs_item_code_ts'
);
SET @__bom_sql_add_hs_item_uk := IF(@__bom_has_hs_item_uk = 0,
    'ALTER TABLE t_hs_item ADD UNIQUE KEY uk_hs_item_code_ts (code_ts)',
    'SELECT ''uk_hs_item_code_ts already exists'' AS bom_migration_msg'
);
PREPARE __bom_stmt_add_hs_item_uk FROM @__bom_sql_add_hs_item_uk;
EXECUTE __bom_stmt_add_hs_item_uk;
DEALLOCATE PREPARE __bom_stmt_add_hs_item_uk;

-- t_bom_quote_item（无则创建）
CREATE TABLE IF NOT EXISTS t_bom_quote_item (
    id              BIGINT UNSIGNED NOT NULL AUTO_INCREMENT COMMENT '报价明细主键',
    quote_id        BIGINT UNSIGNED NOT NULL COMMENT '关联 t_bom_quote_cache.id',
    model           VARCHAR(255) NULL COMMENT '报价型号（平台返回）',
    manufacturer    VARCHAR(255) NULL COMMENT '厂牌原文',
    stock           VARCHAR(64)  NULL COMMENT '库存原文，允许 N/A',
    package         VARCHAR(255) NULL COMMENT '封装原文',
    `desc`          TEXT NULL COMMENT '描述原文',
    datasheet_url   TEXT NULL COMMENT '数据手册 URL',
    moq             VARCHAR(64)  NULL COMMENT '起订量原文，允许 N/A',
    lead_time       VARCHAR(128) NULL COMMENT '交期原文，如 1工作日',
    price_tiers     TEXT NULL COMMENT '阶梯价原文',
    hk_price        TEXT NULL COMMENT '香港价原文',
    mainland_price  TEXT NULL COMMENT '大陆价原文',
    query_model     VARCHAR(255) NULL COMMENT '查询型号（用于回溯）',
    created_at      DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) COMMENT '创建时间',
    updated_at      DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3) COMMENT '最后更新时间',
    PRIMARY KEY (id),
    KEY idx_bom_quote_item_quote_id (quote_id),
    KEY idx_bom_quote_item_query_model (query_model),
    CONSTRAINT fk_bom_quote_item_cache
        FOREIGN KEY (quote_id) REFERENCES t_bom_quote_cache (id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
COMMENT='BOM 报价明细：t_bom_quote_cache 一对多子表';

CREATE TABLE IF NOT EXISTS t_hs_datasheet_asset (
    id              BIGINT UNSIGNED NOT NULL AUTO_INCREMENT COMMENT '主键',
    model           VARCHAR(128) NOT NULL COMMENT '型号',
    manufacturer    VARCHAR(128) NOT NULL COMMENT '厂牌',
    datasheet_url   VARCHAR(1024) NOT NULL DEFAULT '' COMMENT '数据手册 URL',
    local_path      VARCHAR(512) NOT NULL DEFAULT '' COMMENT '本地落盘路径',
    sha256          CHAR(64) NOT NULL DEFAULT '' COMMENT '文件 SHA256',
    download_status ENUM('ok','failed') NOT NULL DEFAULT 'failed' COMMENT '下载状态',
    error_msg       VARCHAR(512) NOT NULL DEFAULT '' COMMENT '失败原因',
    updated_at      DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3) COMMENT '更新时间',
    PRIMARY KEY (id),
    KEY idx_hs_datasheet_asset_model_mfr (model, manufacturer)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
COMMENT='datasheet 资产事实表';

CREATE TABLE IF NOT EXISTS t_hs_model_mapping (
    id                      BIGINT UNSIGNED NOT NULL AUTO_INCREMENT COMMENT '主键',
    model                   VARCHAR(128) NOT NULL COMMENT '型号',
    manufacturer            VARCHAR(128) NOT NULL COMMENT '厂牌',
    code_ts                 CHAR(10) NOT NULL COMMENT '10 位数字编码（保留前导 0）',
    source                  ENUM('manual','llm_auto') NOT NULL DEFAULT 'llm_auto' COMMENT '映射来源',
    confidence              DECIMAL(5,4) NULL COMMENT '置信度',
    status                  ENUM('confirmed','pending_review','rejected') NOT NULL DEFAULT 'pending_review' COMMENT '结果状态',
    features_version        VARCHAR(64) NOT NULL DEFAULT '' COMMENT '抽取版本',
    recommendation_version  VARCHAR(64) NOT NULL DEFAULT '' COMMENT '推荐版本',
    created_at              DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) COMMENT '创建时间',
    updated_at              DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3) COMMENT '更新时间',
    PRIMARY KEY (id),
    UNIQUE KEY uk_hs_model_mapping_model_mfr (model, manufacturer),
    KEY idx_hs_model_mapping_code_ts (code_ts),
    KEY idx_hs_model_mapping_status (status),
    CONSTRAINT chk_hs_model_mapping_code_ts CHECK (code_ts REGEXP '^[0-9]{10}$')
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
COMMENT='型号 + 厂牌 到 HS 编码最终映射';

CREATE TABLE IF NOT EXISTS t_hs_model_features (
    id               BIGINT UNSIGNED NOT NULL AUTO_INCREMENT COMMENT '主键',
    model            VARCHAR(128) NOT NULL COMMENT '型号',
    manufacturer     VARCHAR(128) NOT NULL COMMENT '厂牌',
    asset_id         BIGINT UNSIGNED NOT NULL COMMENT '关联 datasheet 资产',
    tech_category    VARCHAR(64) NOT NULL DEFAULT '' COMMENT '技术类别',
    component_name   VARCHAR(128) NOT NULL DEFAULT '' COMMENT '元器件名称',
    package_form     VARCHAR(64) NOT NULL DEFAULT '' COMMENT '封装形式',
    key_specs_json   JSON NULL COMMENT '关键参数 JSON',
    raw_extract_json JSON NULL COMMENT '原始抽取 JSON',
    extract_model    VARCHAR(64) NOT NULL DEFAULT '' COMMENT '抽取模型名',
    extract_version  VARCHAR(64) NOT NULL DEFAULT '' COMMENT '抽取版本',
    created_at       DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) COMMENT '创建时间',
    PRIMARY KEY (id),
    KEY idx_hs_model_features_model_mfr (model, manufacturer),
    KEY idx_hs_model_features_asset_id (asset_id),
    CONSTRAINT fk_hs_model_features_asset_id FOREIGN KEY (asset_id) REFERENCES t_hs_datasheet_asset(id) ON DELETE RESTRICT
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
COMMENT='datasheet 结构化特征';

CREATE TABLE IF NOT EXISTS t_hs_model_recommendation (
    id                  BIGINT UNSIGNED NOT NULL AUTO_INCREMENT COMMENT '主键',
    model               VARCHAR(128) NOT NULL COMMENT '型号',
    manufacturer        VARCHAR(128) NOT NULL COMMENT '厂牌',
    run_id              CHAR(36) NOT NULL COMMENT '推荐批次 ID',
    candidate_rank      TINYINT UNSIGNED NOT NULL COMMENT '候选排序位次',
    code_ts             CHAR(10) NOT NULL COMMENT '候选 10 位 code_ts',
    g_name              VARCHAR(512) NOT NULL DEFAULT '' COMMENT '候选商品名称',
    score               DECIMAL(5,4) NULL COMMENT '分值',
    reason              VARCHAR(1024) NOT NULL DEFAULT '' COMMENT '推荐理由',
    input_snapshot_json JSON NULL COMMENT '输入快照',
    recommend_model     VARCHAR(64) NOT NULL DEFAULT '' COMMENT '推荐模型名',
    recommend_version   VARCHAR(64) NOT NULL DEFAULT '' COMMENT '推荐版本',
    created_at          DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) COMMENT '创建时间',
    PRIMARY KEY (id),
    UNIQUE KEY uk_hs_model_reco_run_rank (run_id, candidate_rank),
    KEY idx_hs_model_reco_model_mfr_created (model, manufacturer, created_at),
    KEY idx_hs_model_reco_run_id (run_id),
    CONSTRAINT chk_hs_model_reco_code_ts CHECK (code_ts REGEXP '^[0-9]{10}$')
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
COMMENT='候选推荐审计轨迹';
