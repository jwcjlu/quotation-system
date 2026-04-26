-- 关税税率接口（TaxRate001）按 code_ts + 自然日缓存一行；列与响应 data.data[] 单条对齐（见 docs/tax_rate_api）。
CREATE TABLE IF NOT EXISTS t_hs_tax_rate_daily (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
  code_ts CHAR(10) NOT NULL,
  biz_date DATE NOT NULL,
  g_name VARCHAR(512) NOT NULL DEFAULT '',
  imp_discount_rate VARCHAR(32) NOT NULL DEFAULT '',
  imp_temp_rate VARCHAR(32) NOT NULL DEFAULT '',
  imp_ordinary_rate VARCHAR(32) NOT NULL DEFAULT '',
  created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  updated_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
  UNIQUE KEY uk_hs_tax_rate_daily (code_ts, biz_date),
  KEY idx_hs_tax_rate_daily_biz_date (biz_date)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
