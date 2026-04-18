# 四表进程内缓存设计（单实例 / 无 Redis）

**状态：** 已定稿（部署假设：长期单实例，不使用 Redis）  
**日期：** 2026-03-30  
**范围表：** `t_caichip_agent_script_auth`、`t_bom_platform_script`、`t_caichip_agent_installed_script`、`t_bom_manufacturer_alias`

## 1. 目标与非目标

**目标**

1. **定时预热/刷新**：后台任务按间隔从 MySQL 拉取数据写入进程内缓存，降低冷启动与漏删后的陈旧窗口。
2. **读路径**：优先读缓存；**miss** 时查库，将结果写回缓存（read-through）。
3. **写路径**：经 API/业务更新上述表并成功落库后，**删除**对应缓存项（含必要的聚合 key），避免脏读。

**非目标**

- 多实例间缓存一致性（明确不采用 Redis，接受单进程语义）。
- 跨服务共享缓存。
- 强一致「写库同时必可见于缓存」（允许极短窗口内下一次读触发读穿或等待定时刷新）。

## 2. 总体架构

```
                    ┌─────────────────┐
  定时任务 (ticker) │  LoadFromDB     │──► 进程内 Cache (mutex 保护)
                    └────────┬────────┘
                             │
  读路径  ──►  DecoratedRepo / Facade ──► cache.Get? ──yes──► 返回
                                    └──no──► DB ──► cache.Set ──► 返回

  写路径  ──►  原 Repo 写 DB 成功 ──► cache.Delete(精确 key 或前缀)
```

- **实现载体**：`sync.RWMutex` + `map[string]cacheEntry`（或按表分子 map），`cacheEntry` 可含 `[]byte`（JSON）或已反序列化的领域对象副本（注意不可变或深拷贝，避免并发改）。
- **可选依赖**：若希望自动 TTL/淘汰，可引入轻量库（如 `github.com/jellydator/ttlcache/v3`）；**不引入 Redis**。

## 3. 键空间与粒度

| 表 | Key 约定 | 值语义 | 备注 |
|----|-----------|--------|------|
| `t_caichip_agent_script_auth` | `asauth:agent:{agent_id}` | 该 agent 下凭据行列表（或等价结构） | `GetPlatformAuth(agent, script)` 可先取列表再选 script；单行更新删 `asauth:agent:{id}` 即可 |
| | `asauth:agent:{agent_id}:script:{script_id}` | 可选：单行缓存 | 若热点为单行，可双写；删除时删单行 + 父列表 key（若存在） |
| `t_bom_platform_script` | `bomplat:all` | 全表列表 | 行数少，单 key 简单；`Get(platform_id)` 可从切片派生或单独 `bomplat:{platform_id}` |
| `t_caichip_agent_installed_script` | `aginst:agent:{agent_id}` | 该 agent 已安装脚本列表 | 与按 agent 查询一致 |
| `t_bom_manufacturer_alias` | 依现有查询：如 `mfalias:lookup:{normalized_key}` 或 `mfalias:snapshot` | 点查或全表快照 | 以实现中 **实际 SQL 条件** 为准；若多为全表内存匹配，优先 `snapshot` + 定时刷新 |

**版本戳（可选简化多 key 删除）**

- 对 `bomplat` 可增加 `bomplat:ver` 整数；定时任务与写路径 `Inc`；读路径携带 ver 比对，不匹配则视为 miss。首版可不用，**精确删 key** 即可。

## 4. 定时任务

- **触发**：`time.Ticker` 或与现有调度框架一致；间隔建议 **可配置**（如 5m～15m），默认保守。
- **行为**：按表执行 `SELECT`（全量或带 `updated_at` 水印的增量，**增量为二期**；首版全量即可）。
- **失败**：打日志，**保留旧缓存**，不刷空。
- **并发**：刷新时建议 **双缓冲**或短暂持写锁替换指针，避免长时间 RLock 阻塞读路径。

## 5. 读穿（cache-aside）

- 统一入口：`GetX(ctx) → cache → DB → Set → return`。
- **nil 结果缓存（可选）**：防止穿透可对「明确不存在」短 TTL 缓存 negative；首版可不对敏感表做 negative，避免误缓存安全状态。
- **`agent_script_auth` 密文**：缓存中建议存 **与库一致的密文 + username**（或整行 DTO），解密仍在现有 cipher 路径；**避免在内存长期存明文密码**，除非单独威胁建模接受。

## 6. 写失效（API / 业务写库后）

顺序：**先提交 DB 成功，再删缓存**（避免 DB 失败导致缓存空窗风暴）。

| 操作 | 建议删除的 key |
|------|----------------|
| Upsert/Delete script auth（某 agent+script） | `asauth:agent:{agent_id}`；若存在单行 key 一并删 |
| Upsert/Delete BOM platform | `bomplat:all`、`bomplat:{platform_id}`（若使用） |
| 更新 installed script（通常 Agent 心跳写库） | `aginst:agent:{agent_id}` |
| 更新 manufacturer alias | 对应 lookup key 或整表 `mfalias:snapshot` |

**Admin HTTP**（Agent 凭据、BOM 平台）与 **Agent 上报**（installed script）等所有写表路径均需挂接失效，避免遗漏。

## 7. 与现有分层对齐（caichip / Kratos）

- **接口**：在 `internal/biz` 定义小型 `TableCache` 或按表 `XxxCache` 接口；**实现**放 `internal/data`，由 `New*Repo` 包装或 sidecar 注入。
- **业务规则**：仍只在 `biz`；`data` 层缓存装饰器只做「缓存 + 调下层 repo」，不新增业务分支逻辑。
- **Wire**：单例缓存实例 + 装饰器构造，与 `Data` 同生命周期。

## 8. 测试建议

- 单元：mock 底层 repo，验证 miss → DB → set；写成功后 `Delete` 被调用。
- 集成（可选）：写 API 后立即读，期望新数据；定时任务可通过短间隔配置在测试中触发一次 reload。

## 9. 后续扩展（不在首版）

- 增量同步（`updated_at > watermark`）。
- 多实例时切换为 Redis / Valkey，键空间与本设计保持一致。
- `bom_manufacturer_alias` 访问模式稳定后再固化「点查 key」 vs 「snapshot」。

## 10. 决策摘要

| 项 | 决策 |
|----|------|
| 部署 | 长期 **单实例** |
| 中间件 | **不用 Redis**，进程内缓存 |
| 模式 | 定时刷新 + read-through + 写后删键 |
| 一致性 | 单进程内最终一致；重启后依赖定时任务与读穿恢复 |
