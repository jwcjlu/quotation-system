-- 为厂牌相关表补充 manufacturer_canonical_id（可空，规范厂牌 ID）
-- 目标表：
-- - t_bom_quote_item
-- - t_hs_model_mapping
-- - t_hs_model_features
-- - t_hs_model_recommendation
-- 说明：使用 INFORMATION_SCHEMA 防御式检查，避免重复执行失败。

SET @schema_name := DATABASE();

SET @sql := (
    SELECT IF(
        EXISTS (
            SELECT 1
            FROM INFORMATION_SCHEMA.COLUMNS
            WHERE TABLE_SCHEMA = @schema_name
              AND TABLE_NAME = 't_bom_quote_item'
              AND COLUMN_NAME = 'manufacturer_canonical_id'
        ),
        'SELECT 1',
        'ALTER TABLE t_bom_quote_item ADD COLUMN manufacturer_canonical_id VARCHAR(128) NULL AFTER manufacturer'
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
              AND TABLE_NAME = 't_bom_quote_item'
              AND COLUMN_NAME = 'manufacturer_canonical_id'
        ),
        'SELECT 1',
        'ALTER TABLE t_bom_quote_item ADD INDEX idx_bom_quote_item_mfr_canonical_id (manufacturer_canonical_id)'
    )
);
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @sql := (
    SELECT IF(
        EXISTS (
            SELECT 1
            FROM INFORMATION_SCHEMA.COLUMNS
            WHERE TABLE_SCHEMA = @schema_name
              AND TABLE_NAME = 't_hs_model_mapping'
              AND COLUMN_NAME = 'manufacturer_canonical_id'
        ),
        'SELECT 1',
        'ALTER TABLE t_hs_model_mapping ADD COLUMN manufacturer_canonical_id VARCHAR(128) NULL AFTER manufacturer'
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
              AND TABLE_NAME = 't_hs_model_mapping'
              AND COLUMN_NAME = 'manufacturer_canonical_id'
        ),
        'SELECT 1',
        'ALTER TABLE t_hs_model_mapping ADD INDEX idx_hs_model_mapping_mfr_canonical_id (manufacturer_canonical_id)'
    )
);
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @sql := (
    SELECT IF(
        EXISTS (
            SELECT 1
            FROM INFORMATION_SCHEMA.COLUMNS
            WHERE TABLE_SCHEMA = @schema_name
              AND TABLE_NAME = 't_hs_model_features'
              AND COLUMN_NAME = 'manufacturer_canonical_id'
        ),
        'SELECT 1',
        'ALTER TABLE t_hs_model_features ADD COLUMN manufacturer_canonical_id VARCHAR(128) NULL AFTER manufacturer'
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
              AND TABLE_NAME = 't_hs_model_features'
              AND COLUMN_NAME = 'manufacturer_canonical_id'
        ),
        'SELECT 1',
        'ALTER TABLE t_hs_model_features ADD INDEX idx_hs_model_features_mfr_canonical_id (manufacturer_canonical_id)'
    )
);
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @sql := (
    SELECT IF(
        EXISTS (
            SELECT 1
            FROM INFORMATION_SCHEMA.COLUMNS
            WHERE TABLE_SCHEMA = @schema_name
              AND TABLE_NAME = 't_hs_model_recommendation'
              AND COLUMN_NAME = 'manufacturer_canonical_id'
        ),
        'SELECT 1',
        'ALTER TABLE t_hs_model_recommendation ADD COLUMN manufacturer_canonical_id VARCHAR(128) NULL AFTER manufacturer'
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
              AND TABLE_NAME = 't_hs_model_recommendation'
              AND COLUMN_NAME = 'manufacturer_canonical_id'
        ),
        'SELECT 1',
        'ALTER TABLE t_hs_model_recommendation ADD INDEX idx_hs_model_recommendation_mfr_canonical_id (manufacturer_canonical_id)'
    )
);
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

-- 回滚指引（仅示例）：
-- 请先确认以下列/索引没有被查询、约束、代码路径或下游任务依赖，再执行 down SQL。
-- -- down:
-- ALTER TABLE t_bom_quote_item DROP INDEX idx_bom_quote_item_mfr_canonical_id;
-- ALTER TABLE t_bom_quote_item DROP COLUMN manufacturer_canonical_id;
-- ALTER TABLE t_hs_model_mapping DROP INDEX idx_hs_model_mapping_mfr_canonical_id;
-- ALTER TABLE t_hs_model_mapping DROP COLUMN manufacturer_canonical_id;
-- ALTER TABLE t_hs_model_features DROP INDEX idx_hs_model_features_mfr_canonical_id;
-- ALTER TABLE t_hs_model_features DROP COLUMN manufacturer_canonical_id;
-- ALTER TABLE t_hs_model_recommendation DROP INDEX idx_hs_model_recommendation_mfr_canonical_id;
-- ALTER TABLE t_hs_model_recommendation DROP COLUMN manufacturer_canonical_id;
