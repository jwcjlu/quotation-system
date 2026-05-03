-- 兜底补齐 BOM 报价缓存/明细来源追踪列。
-- 背景：旧库可能尚未执行 20260425_bom_gap_match_run_phase2.sql 中对
-- t_bom_quote_cache / t_bom_quote_item 的 source_type、session_id、line_id、created_by 扩展，
-- 会导致 GORM 写入 BomQuoteCache 时出现 Unknown column 'source_type'。

SET @__bom_db := DATABASE();

SET @__has_cache_source_type := (
    SELECT COUNT(*) FROM information_schema.COLUMNS
    WHERE TABLE_SCHEMA = @__bom_db AND TABLE_NAME = 't_bom_quote_cache' AND COLUMN_NAME = 'source_type'
);
SET @__sql_cache_source_type := IF(
    @__has_cache_source_type = 0,
    'ALTER TABLE t_bom_quote_cache ADD COLUMN source_type VARCHAR(32) NOT NULL DEFAULT ''platform''',
    'SELECT ''t_bom_quote_cache.source_type already exists'' AS bom_migration_msg'
);
PREPARE __stmt_cache_source_type FROM @__sql_cache_source_type;
EXECUTE __stmt_cache_source_type;
DEALLOCATE PREPARE __stmt_cache_source_type;

SET @__has_cache_session_id := (
    SELECT COUNT(*) FROM information_schema.COLUMNS
    WHERE TABLE_SCHEMA = @__bom_db AND TABLE_NAME = 't_bom_quote_cache' AND COLUMN_NAME = 'session_id'
);
SET @__sql_cache_session_id := IF(
    @__has_cache_session_id = 0,
    'ALTER TABLE t_bom_quote_cache ADD COLUMN session_id CHAR(36) NULL',
    'SELECT ''t_bom_quote_cache.session_id already exists'' AS bom_migration_msg'
);
PREPARE __stmt_cache_session_id FROM @__sql_cache_session_id;
EXECUTE __stmt_cache_session_id;
DEALLOCATE PREPARE __stmt_cache_session_id;

SET @__has_cache_line_id := (
    SELECT COUNT(*) FROM information_schema.COLUMNS
    WHERE TABLE_SCHEMA = @__bom_db AND TABLE_NAME = 't_bom_quote_cache' AND COLUMN_NAME = 'line_id'
);
SET @__sql_cache_line_id := IF(
    @__has_cache_line_id = 0,
    'ALTER TABLE t_bom_quote_cache ADD COLUMN line_id BIGINT NULL',
    'SELECT ''t_bom_quote_cache.line_id already exists'' AS bom_migration_msg'
);
PREPARE __stmt_cache_line_id FROM @__sql_cache_line_id;
EXECUTE __stmt_cache_line_id;
DEALLOCATE PREPARE __stmt_cache_line_id;

SET @__has_cache_created_by := (
    SELECT COUNT(*) FROM information_schema.COLUMNS
    WHERE TABLE_SCHEMA = @__bom_db AND TABLE_NAME = 't_bom_quote_cache' AND COLUMN_NAME = 'created_by'
);
SET @__sql_cache_created_by := IF(
    @__has_cache_created_by = 0,
    'ALTER TABLE t_bom_quote_cache ADD COLUMN created_by VARCHAR(128) NULL',
    'SELECT ''t_bom_quote_cache.created_by already exists'' AS bom_migration_msg'
);
PREPARE __stmt_cache_created_by FROM @__sql_cache_created_by;
EXECUTE __stmt_cache_created_by;
DEALLOCATE PREPARE __stmt_cache_created_by;

SET @__has_item_source_type := (
    SELECT COUNT(*) FROM information_schema.COLUMNS
    WHERE TABLE_SCHEMA = @__bom_db AND TABLE_NAME = 't_bom_quote_item' AND COLUMN_NAME = 'source_type'
);
SET @__sql_item_source_type := IF(
    @__has_item_source_type = 0,
    'ALTER TABLE t_bom_quote_item ADD COLUMN source_type VARCHAR(32) NOT NULL DEFAULT ''platform''',
    'SELECT ''t_bom_quote_item.source_type already exists'' AS bom_migration_msg'
);
PREPARE __stmt_item_source_type FROM @__sql_item_source_type;
EXECUTE __stmt_item_source_type;
DEALLOCATE PREPARE __stmt_item_source_type;

SET @__has_item_session_id := (
    SELECT COUNT(*) FROM information_schema.COLUMNS
    WHERE TABLE_SCHEMA = @__bom_db AND TABLE_NAME = 't_bom_quote_item' AND COLUMN_NAME = 'session_id'
);
SET @__sql_item_session_id := IF(
    @__has_item_session_id = 0,
    'ALTER TABLE t_bom_quote_item ADD COLUMN session_id CHAR(36) NULL',
    'SELECT ''t_bom_quote_item.session_id already exists'' AS bom_migration_msg'
);
PREPARE __stmt_item_session_id FROM @__sql_item_session_id;
EXECUTE __stmt_item_session_id;
DEALLOCATE PREPARE __stmt_item_session_id;

SET @__has_item_line_id := (
    SELECT COUNT(*) FROM information_schema.COLUMNS
    WHERE TABLE_SCHEMA = @__bom_db AND TABLE_NAME = 't_bom_quote_item' AND COLUMN_NAME = 'line_id'
);
SET @__sql_item_line_id := IF(
    @__has_item_line_id = 0,
    'ALTER TABLE t_bom_quote_item ADD COLUMN line_id BIGINT NULL',
    'SELECT ''t_bom_quote_item.line_id already exists'' AS bom_migration_msg'
);
PREPARE __stmt_item_line_id FROM @__sql_item_line_id;
EXECUTE __stmt_item_line_id;
DEALLOCATE PREPARE __stmt_item_line_id;

SET @__has_item_created_by := (
    SELECT COUNT(*) FROM information_schema.COLUMNS
    WHERE TABLE_SCHEMA = @__bom_db AND TABLE_NAME = 't_bom_quote_item' AND COLUMN_NAME = 'created_by'
);
SET @__sql_item_created_by := IF(
    @__has_item_created_by = 0,
    'ALTER TABLE t_bom_quote_item ADD COLUMN created_by VARCHAR(128) NULL',
    'SELECT ''t_bom_quote_item.created_by already exists'' AS bom_migration_msg'
);
PREPARE __stmt_item_created_by FROM @__sql_item_created_by;
EXECUTE __stmt_item_created_by;
DEALLOCATE PREPARE __stmt_item_created_by;
