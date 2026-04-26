-- 为 /api/hs/items 查询补齐数据表（当环境关闭 AutoMigrate 时手工执行）
CREATE TABLE IF NOT EXISTS t_hs_item (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
  code_ts VARCHAR(16) NOT NULL,
  g_name VARCHAR(512) NOT NULL,
  unit_1 VARCHAR(16) NOT NULL DEFAULT '',
  unit_2 VARCHAR(16) NOT NULL DEFAULT '',
  control_mark VARCHAR(64) NOT NULL DEFAULT '',
  source_core_hs6 CHAR(6) NOT NULL DEFAULT '',
  raw_json JSON NULL,
  updated_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
  UNIQUE KEY uk_hs_item_code_ts (code_ts),
  KEY idx_hs_item_source_core_hs6 (source_core_hs6),
  KEY idx_hs_item_updated (updated_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
