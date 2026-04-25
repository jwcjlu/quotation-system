-- BOM 缺口处理与配单方案快照（二期）
CREATE TABLE IF NOT EXISTS t_bom_line_gap (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  session_id CHAR(36) NOT NULL,
  line_id BIGINT NOT NULL,
  line_no INT NOT NULL,
  mpn VARCHAR(256) NOT NULL DEFAULT '',
  gap_type VARCHAR(64) NOT NULL,
  reason_code VARCHAR(64) NOT NULL DEFAULT '',
  reason_detail TEXT NULL,
  resolution_status VARCHAR(32) NOT NULL DEFAULT 'open',
  active_key VARCHAR(191) NOT NULL DEFAULT '',
  resolved_by VARCHAR(128) NULL,
  resolved_at DATETIME(3) NULL,
  resolution_note TEXT NULL,
  substitute_mpn VARCHAR(256) NULL,
  substitute_reason TEXT NULL,
  created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  updated_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
  PRIMARY KEY (id),
  UNIQUE KEY uk_bom_line_gap_active (active_key),
  KEY idx_bom_line_gap_session_status (session_id, resolution_status),
  KEY idx_bom_line_gap_line (session_id, line_id),
  KEY idx_bom_line_gap_updated (updated_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
COMMENT='BOM 行缺口处理记录';

CREATE TABLE IF NOT EXISTS t_bom_match_run (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  run_no INT NOT NULL,
  session_id CHAR(36) NOT NULL,
  selection_revision INT NOT NULL,
  status VARCHAR(32) NOT NULL DEFAULT 'saved',
  source VARCHAR(32) NOT NULL DEFAULT 'manual_save',
  line_total INT NOT NULL DEFAULT 0,
  matched_line_count INT NOT NULL DEFAULT 0,
  unresolved_line_count INT NOT NULL DEFAULT 0,
  total_amount DECIMAL(24,6) NOT NULL DEFAULT 0,
  currency VARCHAR(8) NOT NULL DEFAULT 'CNY',
  created_by VARCHAR(128) NULL,
  created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  saved_at DATETIME(3) NULL,
  superseded_at DATETIME(3) NULL,
  PRIMARY KEY (id),
  UNIQUE KEY uk_bom_match_run_session_no (session_id, run_no),
  KEY idx_bom_match_run_session_status (session_id, status),
  KEY idx_bom_match_run_created (created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
COMMENT='BOM 配单方案保存记录';

CREATE TABLE IF NOT EXISTS t_bom_match_result_item (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  run_id BIGINT UNSIGNED NOT NULL,
  session_id CHAR(36) NOT NULL,
  line_id BIGINT NOT NULL,
  line_no INT NOT NULL,
  source_type VARCHAR(32) NOT NULL,
  match_status VARCHAR(32) NOT NULL DEFAULT '',
  gap_id BIGINT UNSIGNED NULL,
  quote_item_id BIGINT UNSIGNED NULL,
  platform_id VARCHAR(32) NOT NULL DEFAULT '',
  demand_mpn VARCHAR(256) NOT NULL DEFAULT '',
  demand_mfr VARCHAR(256) NOT NULL DEFAULT '',
  demand_package VARCHAR(128) NOT NULL DEFAULT '',
  demand_qty DECIMAL(18,4) NULL,
  matched_mpn VARCHAR(256) NOT NULL DEFAULT '',
  matched_mfr VARCHAR(256) NOT NULL DEFAULT '',
  matched_package VARCHAR(128) NOT NULL DEFAULT '',
  stock BIGINT NULL,
  lead_time VARCHAR(128) NOT NULL DEFAULT '',
  unit_price DECIMAL(24,6) NULL,
  subtotal DECIMAL(24,6) NULL,
  currency VARCHAR(8) NOT NULL DEFAULT 'CNY',
  original_mpn VARCHAR(256) NULL,
  substitute_mpn VARCHAR(256) NULL,
  substitute_reason TEXT NULL,
  code_ts CHAR(10) NOT NULL DEFAULT '',
  control_mark VARCHAR(64) NOT NULL DEFAULT '',
  import_tax_imp_ordinary_rate VARCHAR(64) NOT NULL DEFAULT '',
  import_tax_imp_discount_rate VARCHAR(64) NOT NULL DEFAULT '',
  import_tax_imp_temp_rate VARCHAR(64) NOT NULL DEFAULT '',
  snapshot_json JSON NULL,
  created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  PRIMARY KEY (id),
  UNIQUE KEY uk_bom_match_result_run_line (run_id, line_id),
  KEY idx_bom_match_result_session_line (session_id, line_id),
  KEY idx_bom_match_result_gap (gap_id),
  KEY idx_bom_match_result_quote_item (quote_item_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
COMMENT='BOM 配单方案行结果快照';

ALTER TABLE t_bom_quote_cache
  ADD COLUMN source_type VARCHAR(32) NOT NULL DEFAULT 'platform',
  ADD COLUMN session_id CHAR(36) NULL,
  ADD COLUMN line_id BIGINT NULL,
  ADD COLUMN created_by VARCHAR(128) NULL;

ALTER TABLE t_bom_quote_item
  ADD COLUMN source_type VARCHAR(32) NOT NULL DEFAULT 'platform',
  ADD COLUMN session_id CHAR(36) NULL,
  ADD COLUMN line_id BIGINT NULL,
  ADD COLUMN created_by VARCHAR(128) NULL;
