# BOM 表结构 — 执行说明

- **MySQL**：`mysql ... < docs/schema/bom_mysql.sql`

- **已有库追加客户字段**（2026-03-24）：`mysql ... < docs/schema/migrations/20260324_bom_session_customer.sql`

执行后检查表：`bom_session`, `bom_session_line`, `bom_quote_cache`, `bom_search_task`, `bom_platform_script`。

- **已存在库若含 `bom_match_result`**：配单历史功能已下线，可执行 `DROP TABLE IF EXISTS bom_match_result;` 清理（注意外键与备份）。
