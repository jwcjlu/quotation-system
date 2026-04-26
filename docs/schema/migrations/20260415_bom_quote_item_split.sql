-- BOM 报价缓存拆表：t_bom_quote_cache 增加 id 主键，quotes_json 拆分至 t_bom_quote_item
-- 直接切换迁移：本次不保留拆分前后兼容路径（完成后删除 t_bom_quote_cache.quotes_json）

SET @__bom_db := DATABASE();

-- 1) t_bom_quote_cache 增加 id（若不存在）
SET @__has_cache_id := (
    SELECT COUNT(*)
    FROM information_schema.COLUMNS
    WHERE TABLE_SCHEMA = @__bom_db
      AND TABLE_NAME = 't_bom_quote_cache'
      AND COLUMN_NAME = 'id'
);
SET @__sql_add_cache_id := IF(
    @__has_cache_id = 0,
    'ALTER TABLE t_bom_quote_cache ADD COLUMN id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT FIRST',
    'SELECT ''t_bom_quote_cache.id already exists'' AS bom_migration_msg'
);
PREPARE __stmt_add_cache_id FROM @__sql_add_cache_id;
EXECUTE __stmt_add_cache_id;
DEALLOCATE PREPARE __stmt_add_cache_id;

-- 2) 主键切换为 id（若当前主键不是 id）
SET @__pk_is_id := (
    SELECT COUNT(*)
    FROM information_schema.KEY_COLUMN_USAGE
    WHERE TABLE_SCHEMA = @__bom_db
      AND TABLE_NAME = 't_bom_quote_cache'
      AND CONSTRAINT_NAME = 'PRIMARY'
      AND COLUMN_NAME = 'id'
);
SET @__sql_switch_pk := IF(
    @__pk_is_id = 0,
    'ALTER TABLE t_bom_quote_cache DROP PRIMARY KEY, ADD PRIMARY KEY (id)',
    'SELECT ''t_bom_quote_cache primary key already uses id'' AS bom_migration_msg'
);
PREPARE __stmt_switch_pk FROM @__sql_switch_pk;
EXECUTE __stmt_switch_pk;
DEALLOCATE PREPARE __stmt_switch_pk;

-- 3) 保留合并键唯一约束（若不存在则补齐）
SET @__has_merge_uk := (
    SELECT COUNT(*)
    FROM information_schema.STATISTICS
    WHERE TABLE_SCHEMA = @__bom_db
      AND TABLE_NAME = 't_bom_quote_cache'
      AND INDEX_NAME = 'uk_bom_quote_cache_merge'
);
SET @__sql_add_merge_uk := IF(
    @__has_merge_uk = 0,
    'ALTER TABLE t_bom_quote_cache ADD UNIQUE KEY uk_bom_quote_cache_merge (mpn_norm, platform_id, biz_date)',
    'SELECT ''uk_bom_quote_cache_merge already exists'' AS bom_migration_msg'
);
PREPARE __stmt_add_merge_uk FROM @__sql_add_merge_uk;
EXECUTE __stmt_add_merge_uk;
DEALLOCATE PREPARE __stmt_add_merge_uk;

-- 4) 新建报价明细表
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

-- 5) 删除旧列 quotes_json（若存在）
SET @__has_quotes_json := (
    SELECT COUNT(*)
    FROM information_schema.COLUMNS
    WHERE TABLE_SCHEMA = @__bom_db
      AND TABLE_NAME = 't_bom_quote_cache'
      AND COLUMN_NAME = 'quotes_json'
);
SET @__sql_drop_quotes_json := IF(
    @__has_quotes_json = 1,
    'ALTER TABLE t_bom_quote_cache DROP COLUMN quotes_json',
    'SELECT ''t_bom_quote_cache.quotes_json already dropped'' AS bom_migration_msg'
);
PREPARE __stmt_drop_quotes_json FROM @__sql_drop_quotes_json;
EXECUTE __stmt_drop_quotes_json;
DEALLOCATE PREPARE __stmt_drop_quotes_json;
