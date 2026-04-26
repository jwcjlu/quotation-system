-- 扩容 t_bom_quote_item 价格字段，避免 "Data too long"（历史库可能为 VARCHAR(64)）。

SET @__bom_db := DATABASE();

SET @__has_quote_item := (
    SELECT COUNT(*)
    FROM information_schema.TABLES
    WHERE TABLE_SCHEMA = @__bom_db
      AND TABLE_NAME = 't_bom_quote_item'
);

SET @__mainland_type := (
    SELECT COALESCE(LOWER(DATA_TYPE), '')
    FROM information_schema.COLUMNS
    WHERE TABLE_SCHEMA = @__bom_db
      AND TABLE_NAME = 't_bom_quote_item'
      AND COLUMN_NAME = 'mainland_price'
    LIMIT 1
);
SET @__sql_alter_mainland := IF(
    @__has_quote_item = 1 AND @__mainland_type <> 'text' AND @__mainland_type <> '',
    'ALTER TABLE t_bom_quote_item MODIFY COLUMN mainland_price TEXT NULL COMMENT ''大陆价原文''',
    'SELECT ''t_bom_quote_item.mainland_price already TEXT or missing'' AS bom_migration_msg'
);
PREPARE __stmt_alter_mainland FROM @__sql_alter_mainland;
EXECUTE __stmt_alter_mainland;
DEALLOCATE PREPARE __stmt_alter_mainland;

SET @__hk_type := (
    SELECT COALESCE(LOWER(DATA_TYPE), '')
    FROM information_schema.COLUMNS
    WHERE TABLE_SCHEMA = @__bom_db
      AND TABLE_NAME = 't_bom_quote_item'
      AND COLUMN_NAME = 'hk_price'
    LIMIT 1
);
SET @__sql_alter_hk := IF(
    @__has_quote_item = 1 AND @__hk_type <> 'text' AND @__hk_type <> '',
    'ALTER TABLE t_bom_quote_item MODIFY COLUMN hk_price TEXT NULL COMMENT ''香港价原文''',
    'SELECT ''t_bom_quote_item.hk_price already TEXT or missing'' AS bom_migration_msg'
);
PREPARE __stmt_alter_hk FROM @__sql_alter_hk;
EXECUTE __stmt_alter_hk;
DEALLOCATE PREPARE __stmt_alter_hk;
