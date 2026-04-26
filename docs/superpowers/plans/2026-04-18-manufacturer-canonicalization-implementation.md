# Manufacturer Canonicalization Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 为 `t_bom_quote_item`、`t_hs_model_recommendation`、`t_hs_model_features`、`t_hs_model_mapping` 增加并落地 `manufacturer_canonical_id` 标准化入库能力，同时保留原始 `manufacturer` 文本，未命中时遵循策略 D（不覆盖旧值）。

**Architecture:** 在 `biz` 层统一执行厂牌标准化决策（`NormalizeMfrString` + alias 查找）；`data` 层提供基于 GORM 的 alias 查询与进程内缓存能力；四表 repo 仅做字段持久化并支持“未命中不覆盖”。上线顺序采用“先兼容写入，再存量回填，再观察强化约束”三阶段。

**Tech Stack:** Go、Kratos、GORM、MySQL、`sync.RWMutex`、`singleflight`、仓库既有 `biz.NormalizeMfrString` 与 `BomManufacturerAliasRepo`。

**Spec:** [`docs/superpowers/specs/2026-04-18-manufacturer-canonicalization-design.md`](../specs/2026-04-18-manufacturer-canonicalization-design.md)

---

## 文件结构（创建 / 修改）

| 路径 | 职责 |
|------|------|
| `docs/schema/migrations/20260418_add_manufacturer_canonical_id.sql` | 四表新增 `manufacturer_canonical_id` 与索引 |
| `internal/data/models.go` | 四个 GORM 模型新增字段映射 |
| `internal/biz/repo.go` | 如需，补充 canonical 字段到相关 record DTO |
| `internal/biz/manufacturer_canonicalizer.go`（新） | 统一标准化逻辑（输入 manufacturer，输出 canonical 命中结果） |
| `internal/data/bom_manufacturer_alias_repo.go` | alias 查找增加进程内缓存（正负缓存 + TTL + singleflight） |
| `internal/data/hs_model_mapping_repo.go` | Save/Upsert 支持 `manufacturer_canonical_id`，未命中不覆盖（策略 D） |
| `internal/data/hs_model_features_repo.go` | Create 写入 `manufacturer_canonical_id` |
| `internal/data/hs_model_recommendation_repo.go` | SaveTopN 写入 `manufacturer_canonical_id` |
| `internal/data/bom_search_task_repo.go` | `t_bom_quote_item` 写入 canonical（命中写，未命中留空） |
| `cmd/tools/backfill_manufacturer_canonical_id/main.go`（新） | 存量回填任务（分批、可重入） |
| `internal/.../*_test.go` | 规则 D、缓存行为、四表落库行为测试 |

---

### Task 1: 迁移脚本与模型字段对齐

**Files:**
- Create: `docs/schema/migrations/20260418_add_manufacturer_canonical_id.sql`
- Modify: `internal/data/models.go`
- Test: （无专门测试文件，使用集成验证命令）

- [ ] **Step 1: 编写迁移脚本（四表新增列+索引）**
  - 使用可重复执行的防御式 SQL（存在则跳过）。
  - 字段类型统一：`VARCHAR(128) NULL`。

- [ ] **Step 2: 执行 migration（按项目既有流程）**
  - 在目标环境执行 `docs/schema/migrations/20260418_add_manufacturer_canonical_id.sql`。

- [ ] **Step 3: 校验列与索引存在**
  - 对四表执行结构检查：列 `manufacturer_canonical_id` 与索引 `idx_*_mfr_canonical_id` 均存在。

- [ ] **Step 4: 更新 GORM 模型字段**
  - 为 `BomQuoteItem`、`HsModelMapping`、`HsModelFeatures`、`HsModelRecommendation` 增加 `ManufacturerCanonicalID`。

- [ ] **Step 5: 编译验证**

Run: `go build ./cmd/server/...`  
Expected: PASS。

- [ ] **Step 6: Commit（仅本任务文件）**

```bash
git add docs/schema/migrations/20260418_add_manufacturer_canonical_id.sql internal/data/models.go
git commit -m "feat(schema): add manufacturer canonical id columns for four tables"
```

---

### Task 2: Biz 标准化组件落地

**Files:**
- Create: `internal/biz/manufacturer_canonicalizer.go`
- Modify: `internal/biz/repo.go`（如需扩展 record 字段）
- Test: `internal/biz/manufacturer_canonicalizer_test.go`（新）

- [ ] **Step 1: 先写失败测试（命中/未命中/空白输入）**
  - 命中：返回 canonical_id 与 matched=true。
  - 未命中：matched=false，不返回错误。
  - 空字符串：直接 unmatched。

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./internal/biz/... -run ManufacturerCanonicalizer -count=1`  
Expected: FAIL（实现尚未完成）。

- [ ] **Step 3: 最小实现通过测试**
  - 统一调用 `NormalizeMfrString`。
  - 仅依赖 alias lookup 接口，不引入 data 细节。

- [ ] **Step 4: 再跑测试确认通过**

Run: `go test ./internal/biz/... -run ManufacturerCanonicalizer -count=1`  
Expected: PASS。

- [ ] **Step 5: Commit**

```bash
git add internal/biz/manufacturer_canonicalizer.go internal/biz/manufacturer_canonicalizer_test.go internal/biz/repo.go
git commit -m "feat(biz): add manufacturer canonicalizer for normalized alias lookup"
```

---

### Task 3: Biz 写入入口接线（四条链路）

**Files:**
- Modify: `internal/biz/hs_model_resolver.go`（映射/特征/推荐）
- Modify: `internal/biz` 下触发 `t_bom_quote_item` 落库的用例文件
- Test: 对应 `internal/biz/*_test.go`

- [ ] **Step 1: 标出四表写入入口并补注释**
  - 明确每个入口都调用 canonicalizer。

- [ ] **Step 2: 先加失败测试（入口级策略 D）**
  - 模拟 alias 未命中时，update 场景不清空旧 canonical。

- [ ] **Step 3: 跑测试确认失败**

Run: `go test ./internal/biz/... -run CanonicalPolicyD -count=1`  
Expected: FAIL。

- [ ] **Step 4: 接线实现（最小改动）**
  - 命中写 canonical，未命中不传覆盖值。

- [ ] **Step 5: 跑测试确认通过**

Run: `go test ./internal/biz/... -run CanonicalPolicyD -count=1`  
Expected: PASS。

- [ ] **Step 6: Commit（仅本任务文件）**

```bash
git add internal/biz/hs_model_resolver.go internal/biz/*canonical*_test.go
git commit -m "feat(biz): wire canonicalizer into four write entrypoints"
```

---

### Task 4: Alias Repo 进程内缓存增强

**Files:**
- Modify: `internal/data/bom_manufacturer_alias_repo.go`
- Test: `internal/data/bom_manufacturer_alias_repo_test.go`（若不存在则新建）

- [ ] **Step 1: 先写失败测试（缓存命中、TTL、负缓存、并发单飞）**
  - 同 key 并发请求只落一次 DB 查询。
  - 正缓存过期后可重查。
  - 未命中可短 TTL 缓存。
  - `DBOk=false` 时保持未命中语义，不抛业务中断错误。

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./internal/data/... -run ManufacturerAliasRepoCache -count=1`  
Expected: FAIL（缓存能力尚未实现）。

- [ ] **Step 3: 实现缓存**
  - `map + RWMutex` 存储缓存条目。
  - `singleflight.Group` 控制并发查库。
  - 默认 TTL：正 15 分钟，负 2 分钟；保留可配置入口（可后续接配置）。

- [ ] **Step 4: 运行测试确认通过**

Run: `go test ./internal/data/... -run ManufacturerAliasRepoCache -count=1`  
Expected: PASS。

- [ ] **Step 5: Commit**

```bash
git add internal/data/bom_manufacturer_alias_repo.go internal/data/bom_manufacturer_alias_repo_test.go
git commit -m "feat(data): add in-process cache for manufacturer alias lookup"
```

---

### Task 5: 四表 Repo 落库支持 canonical 字段

**Files:**
- Modify: `internal/data/hs_model_mapping_repo.go`
- Modify: `internal/data/hs_model_features_repo.go`
- Modify: `internal/data/hs_model_recommendation_repo.go`
- Modify: `internal/data/bom_search_task_repo.go`
- Test: 对应 `*_repo_test.go` 与 `internal/biz` 入口测试

- [ ] **Step 1: 为写入 DTO 增加 canonical 字段透传**
  - 仅在命中时赋值。
  - 未命中保持空值（insert）或不覆盖（update）。

- [ ] **Step 2: 先补失败测试（策略 D）**
  - 已有 canonical 的记录在未命中更新时不被清空。
  - 命中时可覆盖为新 canonical。

- [ ] **Step 3: 运行测试确认失败**

Run: `go test ./internal/data/... -run CanonicalIDPolicyD -count=1`  
Expected: FAIL（行为尚未实现）。

- [ ] **Step 4: 实现最小代码让测试通过**
  - `t_hs_model_mapping` 的 OnConflict DoUpdates 使用条件赋值或动态构造 updates，确保未命中不覆盖。
  - 其余 create 路径命中写入，未命中留空。

- [ ] **Step 5: 运行测试确认通过**

Run: `go test ./internal/data/... -run CanonicalIDPolicyD -count=1`  
Expected: PASS。

- [ ] **Step 6: Commit**

```bash
git add internal/data/hs_model_mapping_repo.go internal/data/hs_model_features_repo.go internal/data/hs_model_recommendation_repo.go internal/data/bom_search_task_repo.go
git commit -m "feat(data): persist manufacturer canonical id in four table write paths"
```

---

### Task 6: 存量回填任务

**Files:**
- Create: `cmd/tools/backfill_manufacturer_canonical_id/main.go`
- Test: `cmd/tools/backfill_manufacturer_canonical_id/main_test.go`（至少覆盖核心函数）

- [ ] **Step 1: 定义回填参数**
  - 支持批大小、起始游标、目标表过滤、dry-run。

- [ ] **Step 2: 先写核心失败测试**
  - 仅更新空 canonical 且命中的行。
  - 未命中跳过，不报错中断。
  - 幂等重复执行无副作用。

- [ ] **Step 3: 运行测试确认失败**

Run: `go test ./cmd/tools/backfill_manufacturer_canonical_id -count=1`  
Expected: FAIL（实现尚未完成）。

- [ ] **Step 4: 实现回填逻辑并增加日志指标**
  - 分批查询 + 分批更新。
  - 输出 processed/updated/skipped/error 计数。

- [ ] **Step 5: 测试通过**

Run: `go test ./cmd/tools/backfill_manufacturer_canonical_id -count=1`  
Expected: PASS。

- [ ] **Step 6: Commit**

```bash
git add cmd/tools/backfill_manufacturer_canonical_id/main.go cmd/tools/backfill_manufacturer_canonical_id/main_test.go
git commit -m "feat(tools): add backfill tool for manufacturer canonical id"
```

---

### Task 7: 全量验证与发布前检查

**Files:**
- Modify: 必要时更新 `docs/superpowers/specs/2026-04-18-manufacturer-canonicalization-design.md`（若实现偏离设计）
- Test: 现有测试与新增测试

- [ ] **Step 1: 运行核心测试集**

Run: `go test ./internal/biz/... ./internal/data/... -count=1`  
Expected: PASS。

- [ ] **Step 2: 运行整仓编译验证**

Run: `go build ./cmd/server/...`  
Expected: PASS。

- [ ] **Step 3: 进行一次小批量 dry-run 回填**

Run: `go run ./cmd/tools/backfill_manufacturer_canonical_id --dry-run --limit=500`  
Expected: 输出统计并正常退出。

- [ ] **Step 4: 指标/日志可观测性验收**
  - 至少确认可输出：`lookup hit/miss`、`cache hit/miss`、`backfill updated/skipped/error`。

- [ ] **Step 5: 发布与回滚预案确认**
  - 发布顺序：迁移 -> 服务发布 -> 小流量验证 -> 全量回填。
  - 回滚：服务可回滚；新增列保留但不影响旧逻辑。

- [ ] **Step 6: Commit（仅本任务文件）**

```bash
git add docs/superpowers/specs/2026-04-18-manufacturer-canonicalization-design.md docs/superpowers/plans/2026-04-18-manufacturer-canonicalization-implementation.md
git commit -m "test: verify manufacturer canonicalization end-to-end rollout readiness"
```

---

## 风险与决策检查点

- 风险 1：`manufacturer` 原文质量差导致 miss 率偏高。  
  对策：未命中采样日志 + alias 补录流程。

- 风险 2：高并发下 alias 查库放大。  
  对策：singleflight + 正负缓存 TTL。

- 风险 3：update 语义误清空 canonical。  
  对策：策略 D 的测试先行，确保“未命中不覆盖”。

## 执行前置清单

- [ ] 研发、DBA 对迁移脚本评审通过。
- [ ] 明确回填执行窗口与监控看板。
- [ ] 确认 `manufacturer_canonical_id` 查询方（报表/检索）已准备好兼容空值。

## Execution Handoff

Plan complete and saved to `docs/superpowers/plans/2026-04-18-manufacturer-canonicalization-implementation.md`. Two execution options:

**1. Subagent-Driven (recommended)** - I dispatch a fresh subagent per task, review between tasks, fast iteration

**2. Inline Execution** - Execute tasks in this session using executing-plans, batch execution with checkpoints

Which approach?
