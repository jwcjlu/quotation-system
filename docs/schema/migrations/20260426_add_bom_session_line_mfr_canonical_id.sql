-- 为 BOM 会话行补充厂牌规范 ID，用于把需求行的原始厂牌与厂牌别名 canonical 结果对齐。
-- 说明：使用 INFORMATION_SCHEMA 防御式检查，避免重复执行失败。

SET @schema_name := DATABASE();

SET @sql := (
    SELECT IF(
        EXISTS (
            SELECT 1
            FROM INFORMATION_SCHEMA.COLUMNS
            WHERE TABLE_SCHEMA = @schema_name
              AND TABLE_NAME = 't_bom_session_line'
              AND COLUMN_NAME = 'manufacturer_canonical_id'
        ),
        'SELECT 1',
        'ALTER TABLE t_bom_session_line ADD COLUMN manufacturer_canonical_id VARCHAR(128) NULL COMMENT ''BOM 需求行厂牌规范 ID，对应 t_bom_manufacturer_alias.canonical_id'' AFTER mfr'
    )
);
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @sql := (
    SELECT IF(
        EXISTS (
            SELECT 1
            FROM INFORMATION_SCHEMA.STATISTICS
            WHERE TABLE_SCHEMA = @schema_name
              AND TABLE_NAME = 't_bom_session_line'
              AND INDEX_NAME = 'idx_bom_session_line_mfr_canonical_id'
        ),
        'SELECT 1',
        'ALTER TABLE t_bom_session_line ADD INDEX idx_bom_session_line_mfr_canonical_id (manufacturer_canonical_id)'
    )
);
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;
