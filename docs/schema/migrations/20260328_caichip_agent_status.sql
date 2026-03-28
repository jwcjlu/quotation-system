-- Agent 任务心跳在线状态（与 GORM CaichipAgent.agent_status 一致）
ALTER TABLE t_caichip_agent
    ADD COLUMN agent_status VARCHAR(16) NOT NULL DEFAULT 'unknown'
        COMMENT 'online | offline | unknown（任务心跳维度）'
        AFTER last_task_heartbeat_at;
