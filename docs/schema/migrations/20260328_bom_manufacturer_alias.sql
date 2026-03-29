-- 厂牌别名表（设计 docs/superpowers/specs/2026-03-28-bom-match-currency-mfr-design.md §2.7）
-- 比较用 canonical_id；展示用 display_name；命中规则与 alias_norm 一致。
-- UNIQUE(alias_norm)：规范化后的别名全局唯一。alias_norm 由应用层写入，须与匹配路径使用的规范化规则
-- （trim、大小写、全半角等与需求 §6「同一规范化规则」一致）相同，否则 UNIQUE 无法表达设计语义。
CREATE TABLE IF NOT EXISTS t_bom_manufacturer_alias (
    id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
    canonical_id VARCHAR(128) NOT NULL COMMENT '稳定规范厂牌 ID（比较用）',
    display_name VARCHAR(512) NOT NULL COMMENT '规范厂牌展示名',
    alias VARCHAR(512) NOT NULL COMMENT '别名原文（审计/展示）',
    alias_norm VARCHAR(512) NOT NULL COMMENT '规范化别名：应用写入，与命中比较键一致；全局唯一',
    created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    updated_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
    UNIQUE KEY uk_bom_mfr_alias_norm (alias_norm),
    KEY idx_bom_mfr_canonical_id (canonical_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
    COMMENT='BOM/报价厂牌别名 → 规范 ID（§2.7，alias_norm 应用层规范化）';
