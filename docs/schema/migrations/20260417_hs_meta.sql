-- HS 元数据配置表（与 docs/superpowers/specs/2026-04-15-hs-meta-management-and-query-design.md §4.1 一致）
CREATE TABLE IF NOT EXISTS t_hs_meta (
    id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    category VARCHAR(64) NOT NULL DEFAULT '',
    component_name VARCHAR(128) NOT NULL DEFAULT '',
    core_hs6 CHAR(6) NOT NULL,
    description VARCHAR(512) NOT NULL DEFAULT '',
    enabled TINYINT(1) NOT NULL DEFAULT 1,
    sort_order INT NOT NULL DEFAULT 0,
    created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    updated_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
    PRIMARY KEY (id),
    UNIQUE KEY uk_hs_meta_core_component (core_hs6, component_name),
    KEY idx_hs_meta_category (category),
    KEY idx_hs_meta_core_hs6 (core_hs6),
    KEY idx_hs_meta_enabled (enabled)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
