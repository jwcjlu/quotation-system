# Agent 调度表 `caichip_dispatch_task` 迁移说明

## 执行

1. 确认 **MySQL 8+**（需 `FOR UPDATE SKIP LOCKED`）。
2. 按顺序执行：
   - `docs/schema/agent_mysql.sql`（若尚未有 `caichip_agent` 等）
   - `docs/schema/agent_dispatch_task_mysql.sql`

## 验证

```sql
SHOW CREATE TABLE caichip_dispatch_task\G
SHOW INDEX FROM caichip_dispatch_task WHERE Key_name = 'idx_dispatch_claim';
```

## 回滚（仅未接生产流量）

```sql
DROP TABLE IF EXISTS caichip_dispatch_task;
```

生产环境请先用配置 **`agent.dispatch_store: memory`**，验证应用正常后再改为 `mysql`。

## 特性开关

见 `configs/config.yaml` 中 `agent.dispatch_store`：`memory`（默认）与 `mysql`。
