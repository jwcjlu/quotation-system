-- BOM 合并调度：合并键 (mpn_norm, platform_id, biz_date) 与在途 t_caichip_dispatch_task.task_id 映射
-- 设计：docs/superpowers/specs/2026-03-27-bom-sourcing-design.md §3.5
-- 调度终态后由应用删除行（见 FinishLeased 钩子）；亦可定期清理孤儿行。

CREATE TABLE IF NOT EXISTS t_bom_merge_inflight (
    mpn_norm    VARCHAR(256) NOT NULL,
    platform_id VARCHAR(32)  NOT NULL,
    biz_date    DATE         NOT NULL,
    task_id     VARCHAR(128) NOT NULL COMMENT 't_caichip_dispatch_task.task_id，在途期间唯一',
    created_at  DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    PRIMARY KEY (mpn_norm, platform_id, biz_date),
    KEY idx_bom_merge_inflight_task (task_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
COMMENT='合并键→在途调度 task_id；finished/cancelled 后删除以允许新一轮抓取';
