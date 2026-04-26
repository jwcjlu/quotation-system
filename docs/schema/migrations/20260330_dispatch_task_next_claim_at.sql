-- 调度任务延迟认领（通用 pending；BOM 合并策略 B 成功路径不设此列）
ALTER TABLE t_caichip_dispatch_task
  ADD COLUMN next_claim_at DATETIME(3) NULL DEFAULT NULL
    COMMENT '到达该时间前 Agent 不可认领；NULL 表示立即可认领'
    AFTER lease_deadline_at;

CREATE INDEX idx_dispatch_next_claim ON t_caichip_dispatch_task (queue, state, next_claim_at);
