-- 分布式采集 — 调度任务队列表（MySQL 8+，InnoDB，utf8mb4）
-- 多实例 server 共用：pending 行可被任意节点 FOR UPDATE SKIP LOCKED 原子认领并转为 leased。
-- 与协议对齐字段见 docs/分布式采集Agent-API协议.md；与 BOM 对齐：task_id 可与 bom_search_task.caichip_task_id 一致。
-- 执行前请已存在 docs/schema/agent_mysql.sql 中的 caichip_agent（若使用下方可选外键）。

CREATE TABLE IF NOT EXISTS caichip_dispatch_task (
    id                      BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
    task_id                 VARCHAR(128) NOT NULL,
    queue                   VARCHAR(128) NOT NULL DEFAULT 'default',
    script_id               VARCHAR(128) NOT NULL,
    version                 VARCHAR(64)  NOT NULL COMMENT '与包 version.txt 规范化后一致',
    required_tags           JSON         NULL COMMENT 'string[]，非空则 Agent 须含全部 tag',
    entry_file              VARCHAR(512)  NULL,
    timeout_sec             INT          NOT NULL DEFAULT 300,
    params_json             JSON         NULL COMMENT '下发参数，脚本约定读取方式',
    argv_json               JSON         NULL COMMENT 'string[]，追加到入口脚本后的命令行参数',
    attempt                 INT          NOT NULL DEFAULT 1 COMMENT '当前执行世代，成功认领后重派会 +1',
    state                   VARCHAR(32)  NOT NULL DEFAULT 'pending'
        COMMENT 'pending=待认领 leased=已派发 finished=终态 cancelled=取消',
    lease_id                VARCHAR(64)  NULL,
    leased_to_agent_id      VARCHAR(64)  NULL,
    leased_at               DATETIME(3)   NULL,
    lease_deadline_at       DATETIME(3)   NULL COMMENT '可选：lease 最长有效至该时刻，供回收任务扫描',
    finished_at             DATETIME(3)   NULL,
    result_status           VARCHAR(32)   NULL COMMENT 'success|timeout|error|...，仅 finished 时有意义',
    last_error              TEXT         NULL,
    created_at              DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    updated_at              DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
    UNIQUE KEY uk_dispatch_task_id (task_id),
    KEY idx_dispatch_claim (queue, state, id),
    KEY idx_dispatch_leased_agent (leased_to_agent_id, state),
    KEY idx_dispatch_state_updated (state, updated_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- 可选：与 caichip_agent 绑定外键（若认领前保证 agent 已注册；否则注释掉本段）
-- ALTER TABLE caichip_dispatch_task
--     ADD CONSTRAINT fk_dispatch_leased_agent
--     FOREIGN KEY (leased_to_agent_id) REFERENCES caichip_agent (agent_id)
--     ON DELETE SET NULL;

-- 可选：仅审计「每次派发/换租约」，便于排障与对账（主表仍保留当前 lease 快照）
-- CREATE TABLE IF NOT EXISTS caichip_dispatch_task_lease_log (
--     id                      BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
--     task_id                 VARCHAR(128) NOT NULL,
--     lease_id                VARCHAR(64)  NOT NULL,
--     agent_id                VARCHAR(64)  NOT NULL,
--     attempt                 INT          NOT NULL,
--     action                  VARCHAR(32)  NOT NULL COMMENT 'dispatch|reclaim|finish',
--     created_at              DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
--     KEY idx_lease_log_task (task_id, id),
--     KEY idx_lease_log_lease (lease_id)
-- ) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
