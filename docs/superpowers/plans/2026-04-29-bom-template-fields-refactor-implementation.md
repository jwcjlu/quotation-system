# BOM 模板字段统一化改造 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 按“标准BOM收集模板”新增 `统一型号`、`位号`、`替代型号`、`备注`、`描述` 等字段，完成“正式列 + `extra_json` 追溯”双轨落库，并在导入/展示/导出/匹单链路可用。

**Architecture:** 在 `internal/biz` 扩展导入解析模型和表头映射；在 `internal/data` 为 `t_bom_session_line` 增加正式列并打通 repo 读写；在 `api` 与 `internal/service` 扩展会话行返回结构。导入时新字段写正式列，同时将原始文本补入 `extra_json` 供追溯。匹单逻辑新增兜底策略：主匹配（客户原型号）失败或不满足时，自动尝试 `替代型号`，并在结果中标识命中来源。

**Tech Stack:** Go, Kratos, GORM, protobuf, MySQL migration SQL, excelize

---

### Task 1: 扩展数据库与数据模型（正式列）

**Files:**
- Create: `docs/schema/migrations/20260429_bom_session_line_template_fields.sql`
- Modify: `internal/data/models.go`
- Modify: `internal/data/migrate.go`
- Test: `internal/data/bom_session_repo_test.go`

- [ ] **Step 1: 写失败测试（模型字段可读写）**

在 `internal/data/bom_session_repo_test.go` 新增用例：构造 `BomSessionLine` 并断言新字段可持久化/读取（`UnifiedMpn`、`ReferenceDesignator`、`SubstituteMpn`、`Remark`、`Description`）。

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./internal/data -run TestBomSessionLineTemplateFields -v`
Expected: FAIL（字段不存在或未映射）

- [ ] **Step 3: 增加 migration SQL**

在 `docs/schema/migrations/20260429_bom_session_line_template_fields.sql` 中为 `t_bom_session_line` 增加：
- `unified_mpn`（varchar）
- `reference_designator`（text/varchar，按现有表风格）
- `substitute_mpn`（varchar）
- `remark`（text）
- `description`（text）

并确保幂等（`ADD COLUMN IF NOT EXISTS` 或项目既有写法）。

- [ ] **Step 4: 更新 GORM 模型**

在 `internal/data/models.go` 的 `BomSessionLine` 增加对应字段与 `gorm:"column:..."` tag。

- [ ] **Step 5: 运行测试确认通过**

Run: `go test ./internal/data -run TestBomSessionLineTemplateFields -v`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add docs/schema/migrations/20260429_bom_session_line_template_fields.sql internal/data/models.go internal/data/migrate.go internal/data/bom_session_repo_test.go
git commit -m "feat: add bom session line template columns for canonical import/display"
```

### Task 2: 扩展导入解析（模板字段映射 + 必填规则）

**Files:**
- Modify: `internal/biz/bom_import_excel.go`
- Modify: `internal/biz/bom_import_qty.go`（仅当数量解析需兼容位号分隔文本时）
- Test: `internal/biz/bom_import_excel_test.go`

- [ ] **Step 1: 写失败测试（模板字段解析）**

在 `internal/biz/bom_import_excel_test.go` 新增用例：
- 表头包含 `客户原型号/统一型号/位号/替代型号/备注/描述/规格` 的解析
- 仅 `mpn` 必填；其余字段空值允许
- 新字段同时进入结构体字段与 `extra_json`（用于追溯）

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./internal/biz -run TestParseBomImportRows_TemplateFields -v`
Expected: FAIL（字段未映射或断言不满足）

- [ ] **Step 3: 扩展导入结构与表头别名**

在 `BomImportLine` 增加：
- `UnifiedMpn`
- `ReferenceDesignator`
- `SubstituteMpn`
- `Remark`
- `Description`

并扩展 `headerAliases`，覆盖模板字段中文名与常见别名（如 `描述/规格`）。

- [ ] **Step 4: 在 `parseDataRow` 中落地赋值与追溯**

- 正式字段：填入新增结构体字段
- 追溯：将原始文本放入 `extra_json`（仅非空键）
- 校验保持：仅 `mpn` 必填

- [ ] **Step 5: 运行测试确认通过**

Run: `go test ./internal/biz -run TestParseBomImportRows_TemplateFields -v`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/biz/bom_import_excel.go internal/biz/bom_import_excel_test.go internal/biz/bom_import_qty.go
git commit -m "feat: parse template fields with canonical columns and extra_json trace"
```

### Task 3: 打通 repo 持久化与读取

**Files:**
- Modify: `internal/data/bom_session_repo.go`
- Modify: `internal/biz/repo.go`（若仓储接口需新增参数）
- Test: `internal/data/bom_session_repo_test.go`

- [ ] **Step 1: 写失败测试（Replace/Create/Patch 覆盖新字段）**

新增 repo 层测试：
- `ReplaceSessionLines` 新字段写入
- `CreateSessionLine`/`UpdateSessionLine` 新字段写入与更新
- `ListSessionLinesFull` 读出新字段

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./internal/data -run TestBomSessionRepo_TemplateFields -v`
Expected: FAIL

- [ ] **Step 3: 更新 repo 写入逻辑**

在 `ReplaceSessionLines`、`CreateSessionLine`、`UpdateSessionLine` 完整映射新增字段，保持现有 revision 与任务重建逻辑不变。

- [ ] **Step 4: 更新 repo 接口签名（如需要）**

若 `biz.BOMSessionRepo` 接口参数不足，先在 `internal/biz/repo.go` 扩展，再同步实现。

- [ ] **Step 5: 运行测试确认通过**

Run: `go test ./internal/data -run TestBomSessionRepo_TemplateFields -v`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/data/bom_session_repo.go internal/data/bom_session_repo_test.go internal/biz/repo.go
git commit -m "feat: persist and query bom template fields in session repo"
```

### Task 4: 扩展 API 展示模型（GetBOMLines / 上传回显）

**Files:**
- Modify: `api/bom/v1/bom.proto`
- Modify: `api/bom/v1/bom.pb.go`（生成）
- Modify: `api/bom/v1/bom_grpc.pb.go`（生成）
- Modify: `api/bom/v1/bom_http.pb.go`（生成）
- Modify: `internal/service/bom_service.go`
- Modify: `internal/service/bom_import_async.go`
- Test: `internal/service/bom_service_upload_test.go`

- [ ] **Step 1: 写失败测试（返回结构包含新字段）**

在 service 测试中新增断言：
- `GetBOMLinesReply.BOMLineRow` 返回新增字段
- `UploadBOMReply.ParsedItem`（如果需要回显）返回新增字段

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./internal/service -run TestBOMTemplateFieldsInReplies -v`
Expected: FAIL

- [ ] **Step 3: 修改 proto 并生成代码**

在 `BOMLineRow` 增加字段：
- `customer_mpn`（可选，或复用 `mpn` 并新增注释）
- `unified_mpn`
- `reference_designator`
- `substitute_mpn`
- `remark`
- `description`

必要时在 `ParsedItem` 同步新增（用于上传同步模式回显）。

Run: `make api`（或项目内既有 proto 生成命令）
Expected: 生成文件更新成功

- [ ] **Step 4: 修改 service 映射**

在 `GetBOMLines` / `UploadBOM` 构建 reply 时填充新字段。

- [ ] **Step 5: 运行测试确认通过**

Run: `go test ./internal/service -run TestBOMTemplateFieldsInReplies -v`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add api/bom/v1/bom.proto api/bom/v1/bom.pb.go api/bom/v1/bom_grpc.pb.go api/bom/v1/bom_http.pb.go internal/service/bom_service.go internal/service/bom_import_async.go internal/service/bom_service_upload_test.go
git commit -m "feat: expose bom template fields in session line and upload replies"
```

### Task 5: 导出模板与导出文件对齐（展示闭环）

**Files:**
- Modify: `internal/service/bom_service.go`
- Test: `internal/service/bom_service_upload_test.go`（或新增导出测试）

- [ ] **Step 1: 写失败测试（导出列包含新字段）**

新增导出断言：
- `ExportSession` 头部列包含新增字段
- 行内容与 `bom_session_line` 新列一致

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./internal/service -run TestExportSessionIncludesTemplateFields -v`
Expected: FAIL

- [ ] **Step 3: 修改导出列与写值逻辑**

更新 `ExportSession` 的表头和逐行写入，列顺序建议与模板一致：
`序号/客户原型号/统一型号/品牌/用量/描述/规格/封装/位号/替代型号/备注`（按最终产品确认）。

- [ ] **Step 4: 运行测试确认通过**

Run: `go test ./internal/service -run TestExportSessionIncludesTemplateFields -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/service/bom_service.go internal/service/bom_service_upload_test.go
git commit -m "feat: align session export columns with standard bom template fields"
```

### Task 6: 匹单兜底策略（原型号失败后匹配替代型号）

**Files:**
- Modify: `internal/biz/bom_line_match.go`
- Modify: `internal/biz/bom_line_match_test.go`
- Modify: `internal/service/bom_service.go`
- Modify: `api/bom/v1/bom.proto`
- Modify: `api/bom/v1/bom.pb.go`（生成）
- Modify: `api/bom/v1/bom_grpc.pb.go`（生成）
- Modify: `api/bom/v1/bom_http.pb.go`（生成）
- Test: `internal/service/bom_service_availability_api_test.go`（或匹单相关测试）

- [ ] **Step 1: 写失败测试（替代型号兜底与标识）**

新增测试覆盖：
- 原型号无可用报价时，若 `substitute_mpn` 存在，则按替代型号检索并可命中
- 原型号有可用报价时，不走替代型号
- 响应中有“匹配来源”字段（`original` / `substitute`）可区分

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./internal/biz ./internal/service -run TestMatchWithSubstituteFallback -v`
Expected: FAIL

- [ ] **Step 3: 实现匹单 fallback 策略**

在匹单主链路中实现：
- 第一阶段：按客户原型号（`mpn`）执行现有逻辑
- 第二阶段：仅当第一阶段“找不到或不满足过滤条件”且替代型号非空时，按 `substitute_mpn` 重试
- 记录最终命中来源（原型号/替代型号）

- [ ] **Step 4: 扩展返回结构并生成代码**

在匹单结果结构（`MatchItem` 或会话行扩展字段）新增：
- `matched_by`（建议枚举值：`original`、`substitute`）
- （可选）`matched_query_mpn`（实际用于命中的查询型号）

Run: `make api`
Expected: proto 生成成功

- [ ] **Step 5: service 层映射与展示标识**

在 `internal/service/bom_service.go` 将 biz 结果映射到 API 字段，保证前端/表格能直接看到“替代型号匹配”标识。

- [ ] **Step 6: 运行测试确认通过**

Run: `go test ./internal/biz ./internal/service -run TestMatchWithSubstituteFallback -v`
Expected: PASS

- [ ] **Step 7: Commit**

```bash
git add internal/biz/bom_line_match.go internal/biz/bom_line_match_test.go internal/service/bom_service.go api/bom/v1/bom.proto api/bom/v1/bom.pb.go api/bom/v1/bom_grpc.pb.go api/bom/v1/bom_http.pb.go
git commit -m "feat: fallback to substitute mpn when original mpn cannot match"
```

### Task 7: 全量回归与文档同步

**Files:**
- Modify: `docs/kratos-project-layout.md`（仅必要补充）
- Modify: `docs/superpowers/plans/2026-04-29-bom-template-fields-refactor-implementation.md`（打勾执行记录）

- [ ] **Step 1: 运行核心测试集**

Run: `go test ./internal/biz ./internal/data ./internal/service`
Expected: PASS

- [ ] **Step 2: 运行 lint 检查改动文件**

Run: `golangci-lint run ./internal/biz ./internal/data ./internal/service`
Expected: PASS（或仅存量问题）

- [ ] **Step 3: 手工联调验证**

- 上传模板样例：确认导入成功且仅 `mpn` 必填
- 查看会话行：确认新增字段显示
- 导出会话：确认列头与内容一致

- [ ] **Step 4: 最终 Commit（若前面未分批提交）**

```bash
git add .
git commit -m "feat: refactor bom import/display for standard template canonical fields"
```
