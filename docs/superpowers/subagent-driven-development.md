# Subagent-Driven Development（在本仓库中的用法）

本页说明如何用 **Superpowers 的 `subagent-driven-development` 技能**执行实现计划：每个任务派一个**全新上下文的子代理**，任务结束后做**两阶段审查**（先规格符合度，再代码质量），再进入下一任务。

完整技能说明见本机：`~/.cursor/skills/superpowers/skills/subagent-driven-development/SKILL.md`（随 Superpowers 插件更新）。

---

## 何时使用

- 已有 **writing-plans** 产出的分任务计划（如本目录下 `plans/*.md`）。
- 任务**相对独立**、希望在本会话内持续推进。
- **不要**并行派多个实现子代理改同一批文件（避免冲突）。

若更适合「单会话内自己逐步执行、带检查点」，用 **executing-plans**。

---

## 与本项目计划的衔接

| 文档 | 说明 |
|------|------|
| [plans/2025-03-24-bom-sourcing-implementation.md](./plans/2025-03-24-bom-sourcing-implementation.md) | BOM 货源搜索与配单落地计划（16 个 Task） |
| [../BOM货源搜索-技术设计方案.md](../BOM货源搜索-技术设计方案.md) | 设计约束 |
| [../BOM货源搜索-接口清单.md](../BOM货源搜索-接口清单.md) | HTTP/字段约定 |

---

## 推荐循环（每个 Task 重复）

1. **协调者（主会话）**  
   - 读出**当前 Task 全文**（含 Files / Steps），**不要**让子代理自己去翻整份计划。  
   - 补充场景：仓库根目录、`caichip` 模块布局、相关 spec 路径。

2. **实现子代理（implementer）**  
   - 使用技能包内模板：`superpowers/subagent-driven-development/implementer-prompt.md`（若插件内提供）。  
   - 产出：代码 + 测试 + 提交说明；状态可为 DONE / DONE_WITH_CONCERNS / NEEDS_CONTEXT / BLOCKED。

3. **规格审查子代理（spec reviewer）**  
   - 对照需求/设计/接口清单，确认无遗漏、无多写。  
   - 未通过则回到实现子代理修改 → **再审规格**。

4. **代码质量审查子代理（code quality reviewer）**  
   - 规格已通过后再做。  
   - 未通过则修改 → **再审质量**。

5. **主会话**  
   - 在 Todo 中勾选该 Task 完成，进入下一 Task。

全部 Task 完成后：可做**整体验收审查**，再按 **finishing-a-development-branch** 收尾（合并/PR 等）。

---

## 模型选择（摘自技能）

| 任务类型 | 建议 |
|----------|------|
| 机械实现、1～2 文件、规格清晰 | 较快/较省模型 |
| 多文件联调、排查 | 标准能力模型 |
| 架构/审查 | 较强模型 |

---

## 红线（摘自技能）

- 规格审查 **未通过** 前，不要进入代码质量审查。  
- 审查发现问题必须修完并**复审**，不能「差不多就行」。  
- 实现子代理标 **BLOCKED** 时，先补上下文或拆任务，不要硬重试同一上下文。  
- 不要在未约定的情况下在 `main` 上直接大改（按团队分支策略）。

---

## 相关 Superpowers 技能

| 技能 | 作用 |
|------|------|
| `using-git-worktrees` | 开发前隔离 worktree（技能要求先做这个）— 见 [using-git-worktrees.md](./using-git-worktrees.md) |
| `writing-plans` | 写本计划 |
| `test-driven-development` | 子代理实现时遵循 TDD |
| `requesting-code-review` | 审查时可作检查单参考 |
| `finishing-a-development-branch` | 全部任务完成后收尾 |

---

*本文档仅为项目内快速索引，以 Superpowers 插件内 SKILL.md 为准。*
