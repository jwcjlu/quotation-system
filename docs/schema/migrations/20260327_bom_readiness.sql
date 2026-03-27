-- BOM 会话：就绪判定模式（已有库增量迁移）
-- 设计：docs/superpowers/specs/2026-03-27-bom-sourcing-design.md §2.3
-- 物理表名前缀 t_；若库已由新版 docs/schema/bom_mysql.sql 建表且含 readiness_mode，请勿重复执行。

ALTER TABLE t_bom_session
    ADD COLUMN readiness_mode VARCHAR(16) NOT NULL DEFAULT 'lenient'
        COMMENT '数据已准备判定：lenient=仅要求平台终态齐全；strict=另要求每行至少一平台 succeeded'
        AFTER status;
