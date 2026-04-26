-- 为旧环境补齐 t_hs_item.code_ts 唯一键（供 upsert 冲突目标使用）
SET @__hs_db := DATABASE();
SET @__hs_has_table := (
    SELECT COUNT(*) FROM information_schema.TABLES
    WHERE TABLE_SCHEMA = @__hs_db
      AND TABLE_NAME = 't_hs_item'
);
SET @__hs_has_uk := (
    SELECT COUNT(*) FROM information_schema.STATISTICS
    WHERE TABLE_SCHEMA = @__hs_db
      AND TABLE_NAME = 't_hs_item'
      AND INDEX_NAME = 'uk_hs_item_code_ts'
);
SET @__hs_sql_add_uk := IF(@__hs_has_table = 0,
    'SELECT ''t_hs_item not exists, skip'' AS hs_migration_msg',
    IF(@__hs_has_uk = 0,
        'ALTER TABLE t_hs_item ADD UNIQUE KEY uk_hs_item_code_ts (code_ts)',
        'SELECT ''uk_hs_item_code_ts already exists'' AS hs_migration_msg'
    )
);
PREPARE __hs_stmt_add_uk FROM @__hs_sql_add_uk;
EXECUTE __hs_stmt_add_uk;
DEALLOCATE PREPARE __hs_stmt_add_uk;
