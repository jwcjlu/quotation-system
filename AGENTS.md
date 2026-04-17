# 仓库协作说明（Agent / 开发者）

## Kratos 工程结构约定

本服务遵循 Kratos 官方推荐的目录与分层，**新增或修改 Go 代码时请按此放置**：

| 区域 | 用途 |
|------|------|
| `api/` | Proto 与生成代码 |
| `cmd/server/` | 入口与 Wire 依赖注入 |
| `internal/biz/` | 领域逻辑、Repository **接口**（`biz/repo.go` 等）、调度与 BOM stdout 等领域代码 |
| `internal/data/` | Repo 实现、数据库（**MySQL 用 GORM**）与外部资源；**仅持久化，不写业务更新逻辑** |
| `internal/service/` | 应用层：实现 API、编排 biz |
| `internal/server/` | HTTP/gRPC 与中间件 |
| `internal/conf/` | 配置结构 |

依赖关系：`server` → `service` → `biz` ← `data`。

**持久化约定：** 操作 MySQL 请使用 **GORM**。状态机、就绪判定、合并调度等 **业务规则放在 `internal/biz`**，不要在 `data` 层实现「带业务决策的更新」。

### 数据库访问规则（新增）

- 所有数据库读写必须通过 GORM 完成。
- 禁止在业务代码中拼接 SQL 字符串执行查询或更新。
- 仅以下场景允许使用原生 SQL：
  - `docs/schema/migrations/` 下的 schema migration
  - 必要的只读运维/排障脚本
- 必须使用原生 SQL 时，必须参数化并在评审中说明原因。

### 单文件长度规则（新增）

- 默认要求：每个代码文件不超过 300 行。
- 超过 300 行时，优先按职责拆分为更小模块（函数、结构体、服务、仓储等）。
- 例外场景：
  - 自动生成文件（如 `*.pb.go`、`wire_gen.go`）
  - 数据库 migration SQL
  - 必要的第三方协议/数据映射大文件
- 发生例外时，需在评审中说明无法拆分的原因。

**详细规则与延伸阅读：** [docs/kratos-project-layout.md](docs/kratos-project-layout.md)（含官方文档链接与 §3 边界说明）。

Cursor 中已通过 `.cursor/rules/kratos-project-layout.mdc` 启用同一条约定。

## ICGOO 易盾滑块（复盘要点，浓缩）

调试入口：`scripts/icgoo_crawler_dev.py`；离线对齐缺口：`scripts/test_ddddocr_slide.py`；快照目录：`scripts/icgoo_captcha_snapshots/`（`attempts.tsv`、`00_detect.txt`、成对 PNG）。

- **`attempts.tsv` 里 `verified_ok`** 表示**端到端**是否通过（图内 x × `drag_scale_x` × `drag_boost` × 滑动系数 × 轨迹 × 风控），**不等于**单独的「缺口识别准确率」。
- 低通过率先拆四块再调参：**(A)** 图内缺口 x（`gap_backend`、`ddddocr_shim`、两路 `raw`、blend）；**(B)** 拖动换算（`drag_scale_x`、`--drag-boost`、`--gap-offset-image-px`）；**(C)** 失败后易盾是否**换新拼图**（须重 `get_images` + `detect_gap`，勿用旧 raw_x 连试系数）；**(D)** 风控与频率。
- **详细复盘表、快照聚合脚本、失败模式**：见项目 skill [`.cursor/skills/icgoo-yidun-captcha/SKILL.md`](.cursor/skills/icgoo-yidun-captcha/SKILL.md)。
