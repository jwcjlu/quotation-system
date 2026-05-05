# manufacturer 标准化入库设计（BOM 行 / 报价行 / HS 表 + 别名缓存）

## 演进说明（2026-05-04）

BOM 工作台「数据清洗」中的 **厂牌评审流程** 已单独沉淀为 **两阶段**（先需求行 canonical，再报价明细仅通过/不通过），并约定 **未上线、不保留旧混合接口**。请以以下文档为当前口径：

- 产品需求：`2026-05-04-bom-mfr-cleaning-two-phase-requirements.md`
- 技术方案：`2026-05-04-bom-mfr-cleaning-two-phase-design.md`

本文档其余章节仍可作为 **别名表、规范化、字段语义** 的参考；与「混合候选 + 单次双表回填」相关的交互与接口描述，以实现侧新文档为准。

## 1. 目标与范围

本设计用于统一以下表的厂牌标准化入库：

- `t_bom_session_line`
- `t_bom_quote_item`
- `t_hs_model_recommendation`
- `t_hs_model_features`
- `t_hs_model_mapping`

约束与目标如下：

- 保留原始 `manufacturer` 文本（审计/回显用途）。
- 新增 `manufacturer_canonical_id` 字段写入规范厂牌 ID。
- 规范 ID 来源于 `t_bom_manufacturer_alias.canonical_id`。
- 未命中别名时采用策略 D：不覆盖旧值（insert 时可为空，update 时保持原值）。
- BOM 需求侧与报价侧都尽早 canonical 化；搜索清洗页只处理“原始厂牌非空但 canonical 缺失”的待清洗数据。
- 为提高吞吐与降低 DB 压力，维护 `t_bom_manufacturer_alias` 的进程内缓存。

## 2. 分层与职责

遵循当前 Kratos 分层约定：

- `biz`：标准化规则与写入决策。
- `data`：别名查询与持久化实现（GORM），不承载业务决策。

依赖方向保持：

- `service -> biz <- data`

## 3. 表结构变更

对以下表新增字段（首期允许空）：

- `manufacturer_canonical_id VARCHAR(128) NULL`

建议索引：

- `idx_bom_session_line_mfr_canonical_id(manufacturer_canonical_id)`
- `idx_bom_quote_item_mfr_canonical_id(manufacturer_canonical_id)`
- `idx_hs_model_features_mfr_canonical_id(manufacturer_canonical_id)`
- `idx_hs_model_recommendation_mfr_canonical_id(manufacturer_canonical_id)`
- `idx_hs_model_mapping_mfr_canonical_id(manufacturer_canonical_id)`

说明：

- 首期不调整 `t_hs_model_mapping` 现有唯一键，先做兼容上线。
- 二期可评估由 `(model, manufacturer)` 迁移为 `(model, manufacturer_canonical_id)` 或联合约束。
- `t_bom_session_line.manufacturer_canonical_id` 表示 BOM 需求侧厂牌规范 ID；`manufacturer` 为空时该字段允许为空，语义是“不施加厂牌约束”，不是待清洗。
- `t_bom_quote_item.manufacturer_canonical_id` 表示平台报价侧厂牌规范 ID；`manufacturer` 非空且该字段为空时，语义是“报价侧厂牌待清洗”。

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

### 4.3 BOM 需求侧录入策略

`t_bom_session_line` 在导入或录入时同步执行厂牌标准化：

- `manufacturer` 为空或仅空白：`manufacturer_canonical_id=NULL`，表示该 BOM 行不施加厂牌约束，可继续参与搜索与配单。
- `manufacturer` 非空且命中别名：写入 `manufacturer_canonical_id`。
- `manufacturer` 非空但未命中别名：`manufacturer_canonical_id=NULL`，表示需求侧厂牌待清洗；该行应进入搜索清洗页，不应直接进入自动配单。

查询待清洗 BOM 行时必须使用双条件：

- `manufacturer IS NOT NULL AND TRIM(manufacturer) <> ''`
- `manufacturer_canonical_id IS NULL`

禁止仅凭 `manufacturer_canonical_id IS NULL` 判断待清洗，否则会误伤“未填写厂牌、无需约束”的 BOM 行。

### 4.4 报价侧清洗策略

`t_bom_quote_item` 入库时也尝试写入 `manufacturer_canonical_id`：

- `manufacturer` 为空：`manufacturer_canonical_id=NULL`，但该行没有可推荐的报价厂牌别名。
- `manufacturer` 非空且命中别名：写入 `manufacturer_canonical_id`，后续不再出现在厂牌清洗推荐中。
- `manufacturer` 非空但未命中别名：`manufacturer_canonical_id=NULL`，作为报价侧待清洗数据。

搜索清洗页查询报价侧待清洗候选时，只扫描：

- `t_bom_quote_item.manufacturer IS NOT NULL AND TRIM(manufacturer) <> ''`
- `t_bom_quote_item.manufacturer_canonical_id IS NULL`

推荐审核时再结合 `t_bom_session_line`：

1. 型号、封装等硬条件先对齐。
2. 若 BOM 行 `manufacturer_canonical_id` 已有值，则将其作为报价厂牌的推荐 canonical。
3. 若 BOM 行原始厂牌非空但 canonical 为空，则先提示“需求侧厂牌待清洗”，不自动推荐报价侧映射。
4. 若 BOM 行厂牌为空，则不能从该行推断报价厂牌 canonical，只能进入人工选择或跳过推荐。

### 4.5 搜索清洗页职责

搜索清洗页是平台报价数据进入自动配单前的治理入口，负责：

- 展示需求侧待清洗：`t_bom_session_line.manufacturer` 非空且 `manufacturer_canonical_id` 为空。
- 展示报价侧待清洗：`t_bom_quote_item.manufacturer` 非空且 `manufacturer_canonical_id` 为空。
- 基于同一 session 内“型号 + 封装”已对齐的数据，推荐 `报价厂牌 -> BOM 侧 canonical_id`。
- 审核通过后写入 `t_bom_manufacturer_alias`，并回填当前 session 命中的 `t_bom_session_line` 与 `t_bom_quote_item`。
- 提供“应用已有别名清洗”动作，将当前 session 中已能命中别名但 canonical 为空的数据补齐。

自动配单只消费清洗后的 canonical 结果：

- BOM 行厂牌为空：允许配单，不校验厂牌。
- BOM 行厂牌非空但 canonical 为空：阻止自动配单或标记“需先清洗”。
- 报价行厂牌非空但 canonical 为空：不参与带厂牌约束的自动配单。

### 4.6 平台维度与歧义处理

V1 仍以 `alias_norm` 全局唯一为默认约束。若业务确认“同一别名在不同平台可指向不同 canonical”，需要升级为以下两层模型之一：

- 方案 A：`platform_id + alias_norm` 唯一，所有别名都按平台隔离。
- 方案 B：保留全局别名表，新增平台覆盖表；查询时平台覆盖优先，全局别名兜底。

在未完成上述升级前，搜索清洗页遇到同一 `alias_norm` 试图指向不同 canonical 时必须拒绝自动写入，并提示人工处理冲突。

以下场景必须进入人工审核，不做自动映射：

- 同一报价厂牌候选可关联到多个不同 BOM canonical。
- 原始厂牌包含多个品牌，例如 `TI / ST`、`ADI; Maxim`。
- 厂牌涉及并购、旧品牌、新品牌或子品牌归属变化，且别名表中没有明确规则。
- 报价厂牌为空。

### 4.7 纠错与重新清洗

`manufacturer_canonical_id` 有值只表示“已清洗”，不保证永远正确。系统需支持纠错路径：

- 修改或删除错误别名后，可按 session、平台、厂牌或全局范围重新回填。
- 重新回填时允许覆盖旧 `manufacturer_canonical_id`，但必须记录来源、操作者与时间。
- 搜索清洗页需要能查看“已清洗但由某别名命中”的记录，便于排查错映射；默认列表仍只展示 canonical 为空的待清洗数据。

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

在相关写入用例入口调用该组件，生成写入参数。

### 6.2 data 层

复用并增强 `BomManufacturerAliasRepo`：

- 保留现有 `CanonicalID(ctx, aliasNorm)` 接口语义。
- 在 repo 内增设缓存与 singleflight。
- `DBOk=false` 时维持“未命中”语义，避免业务中断。

相关 repo 增加字段映射：

- `internal/data/models.go` 对应结构体新增 `ManufacturerCanonicalID`。
- `Create/Save/Upsert` 语句补齐字段，且 update 支持“未命中不覆盖”。

## 7. 存量回填方案

新增一次性或可重复执行的回填任务（可做脚本或后台 job）：

1. 分批扫描相关表，筛选 `manufacturer` 非空且 `manufacturer_canonical_id` 为空的记录。
2. 计算 `alias_norm`，查询别名（走同一缓存与 repo）。
3. 命中则更新 `manufacturer_canonical_id`。
4. 未命中跳过（符合策略 D）。

回填要求：

- 小批次提交（例如 500-2000 行/批）。
- 可重入（按主键游标推进）。
- 支持按当前 session 回填，供搜索清洗页审核通过后立即刷新清洗状态。
- 支持纠错重跑模式：仅在明确指定覆盖范围时，覆盖已有 `manufacturer_canonical_id`。
- 输出进度和命中率。

## 8. 可观测性与运维

建议新增指标：

- `mfr_alias_lookup_total{result=hit|miss|error}`
- `mfr_alias_cache_total{result=hit|miss}`
- `mfr_alias_lookup_latency_ms`
- `mfr_canonical_backfill_rows_total{result=updated|skipped|error}`
- `mfr_cleaning_pending_rows_total{side=demand|quote}`

建议日志（采样）：

- 未命中：`manufacturer_raw`、`alias_norm`
- 人工审核：`alias_norm`、`canonical_id`、`platform_id`、`session_id`、操作者、动作来源。
- 错误：DB 错误类型、请求上下文标识

## 9. 测试计划

单元测试：

- `alias_norm` 规范化一致性。
- 缓存命中/过期/负缓存/singleflight 去重。
- D 策略：未命中 update 不覆盖旧值。
- 空厂牌与待清洗的区别：`manufacturer` 为空不应被当作待清洗。
- 平台别名冲突：同一 `alias_norm` 指向不同 canonical 时拒绝自动写入。

集成测试：

- 相关表 insert/update 路径命中与未命中行为。
- `t_bom_session_line` 导入时命中、未命中、空厂牌三种行为。
- 搜索清洗页候选查询只返回 `manufacturer` 非空且 canonical 为空的数据。
- 审核通过后写别名并回填当前 session 的 BOM 行与报价行。
- 回填任务幂等与断点续跑。

回归测试：

- 现有依赖 `manufacturer` 原文的查询与展示不受影响。
- 自动配单仍允许 BOM 厂牌为空的行；但阻止 BOM 厂牌非空且 canonical 缺失的行直接配单。

## 10. 分阶段实施建议

阶段 1（兼容上线）：

- 增字段 + 索引
- BOM 行、报价行与 HS 写入链路接入 canonical
- 缓存启用
- 搜索清洗页按“原始厂牌非空 + canonical 为空”展示待清洗数据

阶段 2（数据完善）：

- 执行存量回填
- 观察 miss 率并补充 alias 表
- 支持当前 session 审核后即时回填

阶段 3（约束强化，可选）：

- 评估唯一键或查询口径切换到 canonical 维度
- 评估字段非空约束（需 miss 率足够低后再执行）
- 根据真实冲突情况评估是否升级为平台维度别名模型
