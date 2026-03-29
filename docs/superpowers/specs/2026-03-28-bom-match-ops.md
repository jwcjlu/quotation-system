# BOM 配单（币种 / 别名 / 汇率）— 运维备忘

> 对应实现计划 [../plans/2026-03-28-bom-match-currency-mfr-implementation.md](../plans/2026-03-28-bom-match-currency-mfr-implementation.md) 与规格 [2026-03-28-bom-match-currency-mfr-design.md](./2026-03-28-bom-match-currency-mfr-design.md) §4。

## 1. 配置（`configs/config.yaml`）

- **`bom_match.base_ccy`**：比价基准币，如 `CNY`、`USD`。  
- **`bom_match.parse_price_tier_strings`**：是否解析 `price_tiers` 字符串（默认建议 `true`）。  
- **`bom_match.rounding_mode`**：`decimal6` 或 `minor_unit`（与 §1.10 一致）。  
- **`bom_match.bom_qty_round`**：可选，如 `ceil`（BOM 需求数量取整，由实现读取）。

若未配置整块 `bom_match`，服务内对缺省字段有兜底（见 `internal/service/bom_service.go` 注释）。

## 2. 汇率表 `t_bom_fx_rate`

- 手工迁移：`docs/schema/migrations/20260328_bom_fx_rate.sql`。  
- 至少维护 **`base_ccy`** 与常见报价币（如 **USD→CNY**）在 **`biz_date`** 上的汇率；配单查表日以 **`bom_quote_cache.biz_date`** 为主（§1.8）。  
- 种子示例（需按真实汇率替换）：

```sql
INSERT INTO t_bom_fx_rate (from_ccy, to_ccy, biz_date, rate, source, table_version)
VALUES ('USD', 'CNY', '2026-03-28', 7.2000000000, 'manual', 'v1');
```

## 3. 厂牌别名表 `t_bom_manufacturer_alias`

- 手工迁移：`docs/schema/migrations/20260328_bom_manufacturer_alias.sql`。  
- **`alias_norm`** 必须与运行时 **`biz.NormalizeMfrString(alias)`** 一致后再写入，否则无法命中。  
- 示例：

```sql
INSERT INTO t_bom_manufacturer_alias (canonical_id, display_name, alias, alias_norm)
VALUES ('TI', 'Texas Instruments', 'TI', 'TI');
```

BOM 行或报价里出现该厂牌写法时，会先规范化再按 **`alias_norm`** 查 **`canonical_id`**；**BOM 填了厂牌但未命中别名**时，当前实现会 **无法自动配单**（严格策略 §2.3）。

## 4. API 约定

- **`bom_id`**（`SearchQuotes` / `AutoMatch` / `GetMatchResult`）**等于** `session_id`（UUID）。  
- **`SearchQuotes` / `AutoMatch` / `GetMatchResult`** 需要 **FX repo 与 alias repo 均 DBOk**，否则返回 **`DB_DISABLED`**。  
- 会话须 **`GetReadiness` 中 `can_enter_match`** 为真，否则返回 **`BOM_NOT_READY`**。

## 5. 验收建议

1. 准备会话 + 行 + 平台勾选，搜索任务成功写入 **`bom_quote_cache`**（含 §1.11.3 样式 `price_tiers` 更佳）。  
2. 插入上述汇率与别名种子。  
3. 调用 **`GET /api/v1/bom/{session_id}/match`**（`GetMatchResult`）核对选中平台与单价（基准币）。  
4. 调用 **`SearchQuotes`** 核对多平台报价列表。

---

| 日期 | 说明 |
|------|------|
| 2026-03-28 | 初稿：配置、FX/别名种子、API 与验收 |
