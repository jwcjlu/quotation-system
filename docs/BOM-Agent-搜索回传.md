# BOM 搜索任务结果回传（Agent）

Agent 在完成本地爬取后，应在更新 `caichip_dispatch_task` 终态**之前或之后**调用主站 **`SubmitBomSearchResult`**，但不要对同一任务并发双写；推荐顺序：

1. 调用 `POST /api/v1/bom-sessions/{session_id}/search-results` 落库 `bom_search_task` + `bom_quote_cache`；
2. 再回报派发队列终态（或按你方统一错误处理顺序，保证幂等与可追溯）。

## 鉴权

使用配置项 **`bom_search_callback.api_keys`**（与 `agent.api_keys` 独立）。请求头任选其一：

- `X-API-Key: <key>`
- `Authorization: Bearer <key>`

## curl 示例

```bash
curl -sS -X POST "http://127.0.0.1:18080/api/v1/bom-sessions/00000000-0000-0000-0000-000000000001/search-results" \
  -H "Content-Type: application/json" \
  -H "X-API-Key: change-me-bom-search-callback" \
  -d '{
    "session_id": "00000000-0000-0000-0000-000000000001",
    "mpn_norm": "LM358",
    "platform_id": "ickey",
    "caichip_task_id": "cloud-task-id-from-dispatch",
    "status": "succeeded_quotes",
    "quotes_json": "[{\"matched_model\":\"LM358\",\"manufacturer\":\"TI\",\"unit_price\":0.32,\"stock\":5000}]"
  }'
```

无匹配型号：

```bash
-d '{
  "session_id": "...",
  "mpn_norm": "RarePart",
  "platform_id": "szlcsc",
  "caichip_task_id": "...",
  "status": "succeeded_no_mpn",
  "no_mpn_detail_json": "{\"hint\":\"no result\"}"
}'
```

失败：

```bash
-d '{
  "session_id": "...",
  "mpn_norm": "LM358",
  "platform_id": "ickey",
  "caichip_task_id": "...",
  "status": "failed",
  "error_message": "timeout after 120s"
}'
```

## 错误码摘要

| HTTP | reason | 说明 |
|------|--------|------|
| 401 | `UNAUTHORIZED` | Key 缺失或错误 |
| 404 | `SEARCH_TASK_NOT_FOUND` | 无对应 `bom_search_task` 行 |
| 409 | `SEARCH_TASK_ID_MISMATCH` | `caichip_task_id` 与库内不一致 |
| 503 | `SEARCH_CALLBACK_NOT_CONFIGURED` | 未配置 `bom_search_callback.api_keys` |
| 503 | `DB_UNAVAILABLE` | 未连接 BOM 搜索任务库 |

更完整的字段与状态机见：`docs/BOM货源搜索-任务回写与配单衔接.md`。
