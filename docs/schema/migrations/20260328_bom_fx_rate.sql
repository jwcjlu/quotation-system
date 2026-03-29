-- 配单换汇汇率表（设计 docs/superpowers/specs/2026-03-28-bom-match-currency-mfr-design.md §1.8）
-- 查表日期优先取 bom_quote_cache.biz_date；无效时退化为请求日并在审计中标记 fx_date_source。
-- rate 语义：1 单位 from_ccy = rate 单位 to_ccy（与实现约定一致即可，须在业务层写死并文档化）。
CREATE TABLE IF NOT EXISTS t_bom_fx_rate (
    id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
    from_ccy CHAR(3) NOT NULL COMMENT '原币种 ISO 4217',
    to_ccy CHAR(3) NOT NULL COMMENT '目标币种 ISO 4217',
    biz_date DATE NOT NULL COMMENT '业务日历日（与缓存 biz_date 对齐）',
    rate DECIMAL(24, 10) NOT NULL COMMENT '汇率：1 from_ccy = rate × to_ccy',
    source VARCHAR(64) NOT NULL DEFAULT 'manual' COMMENT '数据来源：manual/接口名等',
    table_version VARCHAR(64) NOT NULL DEFAULT '' COMMENT '批次或表版本号（审计 fx_table_version，空串表示默认批次）',
    created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    updated_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
    UNIQUE KEY uk_bom_fx_rate (from_ccy, to_ccy, biz_date, source, table_version),
    KEY idx_bom_fx_rate_lookup (from_ccy, to_ccy, biz_date)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
    COMMENT='BOM 配单汇率（按业务日与版本可追溯）';
