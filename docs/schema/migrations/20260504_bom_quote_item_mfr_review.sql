-- 报价明细厂牌评审状态（两阶段清洗 — 阶段二）
-- 见 docs/superpowers/specs/2026-05-04-bom-mfr-cleaning-two-phase-design.md

SET @__bom_db := DATABASE();

SET @__has_mfr_review_status := (
  SELECT COUNT(*)
  FROM information_schema.COLUMNS
  WHERE TABLE_SCHEMA = @__bom_db
    AND TABLE_NAME = 't_bom_quote_item'
    AND COLUMN_NAME = 'manufacturer_review_status'
);
SET @__sql_add_status := IF(
  @__has_mfr_review_status = 0,
  'ALTER TABLE t_bom_quote_item ADD COLUMN manufacturer_review_status VARCHAR(16) NOT NULL DEFAULT ''pending'' COMMENT ''pending|accepted|rejected''',
  'SELECT ''t_bom_quote_item.manufacturer_review_status already exists'' AS bom_migration_msg'
);
PREPARE __stmt_add_status FROM @__sql_add_status;
EXECUTE __stmt_add_status;
DEALLOCATE PREPARE __stmt_add_status;

SET @__has_reason := (
  SELECT COUNT(*)
  FROM information_schema.COLUMNS
  WHERE TABLE_SCHEMA = @__bom_db
    AND TABLE_NAME = 't_bom_quote_item'
    AND COLUMN_NAME = 'manufacturer_review_reason'
);
SET @__sql_add_reason := IF(
  @__has_reason = 0,
  'ALTER TABLE t_bom_quote_item ADD COLUMN manufacturer_review_reason TEXT NULL COMMENT ''不通过原因''',
  'SELECT ''t_bom_quote_item.manufacturer_review_reason already exists'' AS bom_migration_msg'
);
PREPARE __stmt_add_reason FROM @__sql_add_reason;
EXECUTE __stmt_add_reason;
DEALLOCATE PREPARE __stmt_add_reason;

SET @__has_reviewed_at := (
  SELECT COUNT(*)
  FROM information_schema.COLUMNS
  WHERE TABLE_SCHEMA = @__bom_db
    AND TABLE_NAME = 't_bom_quote_item'
    AND COLUMN_NAME = 'manufacturer_reviewed_at'
);
SET @__sql_add_reviewed_at := IF(
  @__has_reviewed_at = 0,
  'ALTER TABLE t_bom_quote_item ADD COLUMN manufacturer_reviewed_at DATETIME(3) NULL COMMENT ''评审时间''',
  'SELECT ''t_bom_quote_item.manufacturer_reviewed_at already exists'' AS bom_migration_msg'
);
PREPARE __stmt_add_reviewed_at FROM @__sql_add_reviewed_at;
EXECUTE __stmt_add_reviewed_at;
DEALLOCATE PREPARE __stmt_add_reviewed_at;
