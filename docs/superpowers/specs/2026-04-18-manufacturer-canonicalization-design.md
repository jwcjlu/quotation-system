# manufacturer 标准化入库设计（四表 + 别名缓存）

## 1. 目标与范围

本设计用于统一以下四张表的厂牌标准化入库：

- `t_bom_quote_item`
- `t_hs_model_recommendation`
- `t_hs_model_features`
- `t_hs_model_mapping`

约束与目标如下：

- 保留原始 `manufacturer` 文本（审计/回显用途）。
- 新增 `manufacturer_canonical_id` 字段写入规范厂牌 ID。
- 规范 ID 来源于 `t_bom_manufacturer_alias.canonical_id`。
- 未命中别名时采用策略 D：不覆盖旧值（insert 时可为空，update 时保持原值）。
- 为提高吞吐与降低 DB 压力，维护 `t_bom_manufacturer_alias` 的进程内缓存。

## 2. 分层与职责

遵循当前 Kratos 分层约定：

- `biz`：标准化规则与写入决策。
- `data`：别名查询与持久化实现（GORM），不承载业务决策。

依赖方向保持：

- `service -> biz <- data`

## 3. 表结构变更

对四表新增字段（首期允许空）：

- `manufacturer_canonical_id VARCHAR(128) NULL`

建议索引：

- `idx_bom_quote_item_mfr_canonical_id(manufacturer_canonical_id)`
- `idx_hs_model_features_mfr_canonical_id(manufacturer_canonical_id)`
- `idx_hs_model_recommendation_mfr_canonical_id(manufacturer_canonical_id)`
- `idx_hs_model_mapping_mfr_canonical_id(manufacturer_canonical_id)`

说明：

- 首期不调整 `t_hs_model_mapping` 现有唯一键，先做兼容上线。
- 二期可评估由 `(model, manufacturer)` 迁移为 `(model, manufacturer_canonical_id)` 或联合约束。

## 4. 标准化规则

### 4.1 输入与查找

1. 对 `manufacturer` 执行现有规范化（与配单一致）：`biz.NormalizeMfrString`。
2. 使用 `alias_norm` 查询 `t_bom_manufacturer_alias`：
   - 命中：得到 `canonical_id`。
   - 未命中：返回未命中。

### 4.2 写入策略（D）

- 命中别名：
  - insert：写入 `manufacturer_canonical_id=canonical_id`
  - update：覆盖为 `canonical_id`
- 未命中别名：
  - insert：`manufacturer_canonical_id` 留空
  - update：不更新 `manufacturer_canonical_id` 字段（保留旧值）

## 5. 缓存设计（t_bom_manufacturer_alias）

## 5.1 缓存目标

- 降低热点 `alias_norm` 的重复查库。
- 提高批量写入、模型推荐链路下的标准化吞吐。

## 5.2 数据结构

- Key：`alias_norm`
- Value：
  - 命中：`canonical_id`
  - 未命中：负缓存标记（短 TTL）

建议实现：

- 进程内 map + `sync.RWMutex`
- `singleflight` 防击穿
- TTL 过期策略

## 5.3 TTL 与失效

- 正缓存 TTL：10-30 分钟（默认 15 分钟）。
- 负缓存 TTL：1-3 分钟（默认 2 分钟）。
- 失效方式：
  - 被动失效：TTL 到期重查。
  - 主动失效（可选）：alias 管理入口新增/修改后按 key 删除。

## 5.4 一致性取舍

- 缓存为性能优化，最终一致性由 TTL 保证。
- 对关键新增 alias 可通过主动失效立即生效。

## 6. 代码改造点

### 6.1 biz 层

新增标准化组件（示意名）：

- `ManufacturerCanonicalizer`
  - 输入：`manufacturer string`
  - 输出：`canonicalID string, matched bool, err error`

在四表写入用例入口调用该组件，生成写入参数。

### 6.2 data 层

复用并增强 `BomManufacturerAliasRepo`：

- 保留现有 `CanonicalID(ctx, aliasNorm)` 接口语义。
- 在 repo 内增设缓存与 singleflight。
- `DBOk=false` 时维持“未命中”语义，避免业务中断。

四个 repo 增加字段映射：

- `internal/data/models.go` 对应结构体新增 `ManufacturerCanonicalID`。
- `Create/Save/Upsert` 语句补齐字段，且 update 支持“未命中不覆盖”。

## 7. 存量回填方案

新增一次性回填任务（可做脚本或后台 job）：

1. 分批扫描四表，筛选 `manufacturer` 非空且 `manufacturer_canonical_id` 为空的记录。
2. 计算 `alias_norm`，查询别名（走同一缓存与 repo）。
3. 命中则更新 `manufacturer_canonical_id`。
4. 未命中跳过（符合策略 D）。

回填要求：

- 小批次提交（例如 500-2000 行/批）。
- 可重入（按主键游标推进）。
- 输出进度和命中率。

## 8. 可观测性与运维

建议新增指标：

- `mfr_alias_lookup_total{result=hit|miss|error}`
- `mfr_alias_cache_total{result=hit|miss}`
- `mfr_alias_lookup_latency_ms`
- `mfr_canonical_backfill_rows_total{result=updated|skipped|error}`

建议日志（采样）：

- 未命中：`manufacturer_raw`、`alias_norm`
- 错误：DB 错误类型、请求上下文标识

## 9. 测试计划

单元测试：

- `alias_norm` 规范化一致性。
- 缓存命中/过期/负缓存/singleflight 去重。
- D 策略：未命中 update 不覆盖旧值。

集成测试：

- 四表 insert/update 路径命中与未命中行为。
- 回填任务幂等与断点续跑。

回归测试：

- 现有依赖 `manufacturer` 原文的查询与展示不受影响。

## 10. 分阶段实施建议

阶段 1（兼容上线）：

- 增字段 + 索引
- 写入链路接入 canonical
- 缓存启用

阶段 2（数据完善）：

- 执行存量回填
- 观察 miss 率并补充 alias 表

阶段 3（约束强化，可选）：

- 评估唯一键或查询口径切换到 canonical 维度
- 评估字段非空约束（需 miss 率足够低后再执行）
