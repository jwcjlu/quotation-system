# Using Git Worktrees（在本仓库中的用法）

本页说明如何用 **Superpowers 的 `using-git-worktrees` 技能**在独立目录中开分支开发，避免与当前工作区互相干扰。完整技能见本机：

`~/.cursor/skills/superpowers/skills/using-git-worktrees/SKILL.md`

**在以下流程之前应完成：** [subagent-driven-development](./subagent-driven-development.md)、**executing-plans**（见 Cursor 内 Superpowers 技能）所要求的实现计划执行前。

---

## 核心原则

- **系统选目录** + **确认已 gitignore** + **基线测试通过** = 可靠隔离。

---

## 目录优先级（与技能一致）

1. 若已存在 **`.worktrees/`** → 使用（优先于 `worktrees/`）。
2. 若已存在 **`worktrees/`** → 使用。
3. 查项目根 **`CLAUDE.md` / `AGENTS.md`** 是否写明 worktree 路径偏好 → 按文档执行。
4. 否则在二者中选：**`.worktrees/`（项目内隐藏目录）** 或 **全局路径** `~/.config/superpowers/worktrees/<项目名>/`（不占仓库、无需 gitignore）。

**本仓库**：已在 `.gitignore` 中加入 `.worktrees/`，推荐使用 **项目内 `.worktrees/<分支名>`**。

---

## 安全：项目内目录必须被忽略

创建 worktree **之前**执行（若目录尚不存在可先 `mkdir .worktrees`，再检查路径）：

```bash
git check-ignore -v .worktrees
```

若**未**匹配到忽略规则：必须先改 `.gitignore` 再建 worktree（否则 worktree 内文件可能被误提交）。本仓库已包含 `.worktrees/` 规则。

---

## Windows / PowerShell 示例（caichip）

在仓库根目录 `d:\workspace\caichip`：

```powershell
cd d:\workspace\caichip
git worktree add .worktrees/agent-feature -b feature/agent-feature
cd .worktrees\agent-feature
go mod download
go test ./... -count=1
```

**期望：** `go test` 通过（若失败则先排查主分支或环境，再决定是否继续）。

---

## Go 项目落地后

```powershell
go build -o bin/server ./cmd/server/...
```

---

## 完成后

- 合并/PR 在**主工作区**或 CI 中处理；删除 worktree：

```powershell
cd d:\workspace\caichip
git worktree remove .worktrees/agent-feature
git branch -d feature/agent-feature
```

（分支是否删除视团队策略而定。）

更完整的收尾见 **finishing-a-development-branch** 技能。

---

## 与本仓库计划的关系

| 文档 | 说明 |
|------|------|
| [plans/2026-03-24-agent-dispatch-mysql-implementation.md](./plans/2026-03-24-agent-dispatch-mysql-implementation.md) | Agent MySQL 调度落地 |
| [plans/2026-03-24-agent-script-packages-implementation.md](./plans/2026-03-24-agent-script-packages-implementation.md) | 脚本包分发 |

---

## 常见错误（摘自技能）

| 问题 | 处理 |
|------|------|
| 未验证 gitignore | 先 `git check-ignore` / 补 `.gitignore` |
| 基线测试失败仍继续 | 先确认是环境问题还是仓库已有失败 |
| 随意选目录名 | 按上文优先级，或团队约定 |

---

*本文档仅为项目内快速索引，以 Superpowers 插件内 SKILL.md 为准。*
