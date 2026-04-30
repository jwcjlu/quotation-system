# BOM 厂牌清洗闭环 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 按 `docs/superpowers/specs/2026-04-18-manufacturer-canonicalization-design.md` 实现 BOM 需求侧与报价侧厂牌 canonical 化、搜索清洗页推荐审核、写入别名后回填当前 session 的闭环。

**Architecture:** `biz` 保持厂牌规范化与 canonical 解析规则；`service` 负责编排导入/编辑时的需求侧 canonical 化、候选推荐与审核动作；`data` 只通过 GORM 做字段持久化、候选读取和回填更新。前端把“厂牌别名审核”作为搜索清洗页的一部分，默认只展示原始厂牌非空但 `manufacturer_canonical_id` 为空的数据。

**Tech Stack:** Go 1.25+、Kratos、GORM、MySQL 8+、protobuf、React + TypeScript + Vitest。

---

## 文件结构

| 路径 | 职责 |
|------|------|
| Create `docs/schema/migrations/20260426_add_bom_session_line_mfr_canonical_id.sql` | 给 `t_bom_session_line` 新增 `manufacturer_canonical_id` 与索引 |
| Modify `internal/data/models.go` | `BomSessionLine` 增加 `ManufacturerCanonicalID` 字段 |
| Modify `internal/biz/bom_import_excel.go` | `BomImportLine` 增加 `ManufacturerCanonicalID`，供 service 清洗后传给 data |
| Modify `internal/biz/repo.go` | `BOMSessionLineView` 与 `BOMSessionRepo` 接口增加 canonical 字段/参数 |
| Modify `internal/data/bom_session_repo.go` | `ReplaceSessionLines`、`CreateSessionLine`、`UpdateSessionLine` 持久化 BOM 行 canonical |
| Test `internal/data/bom_session_repo_test.go` | 验证 BOM 行 canonical 字段落库、空厂牌不误判 |
| Create `internal/service/bom_session_line_canonical.go` | service 层统一解析 BOM 行厂牌 canonical |
| Modify `internal/service/bom_import_async.go` | 导入完成前先 canonical 化 BOM 行 |
| Modify `internal/service/bom_service.go` | 手工新增/编辑 BOM 行时同步 canonical 化；自动配单前拦截未清洗需求侧厂牌 |
| Test `internal/service/bom_session_line_canonical_test.go` | 验证导入/新增/编辑三条路径的需求侧 canonical 语义 |
| Modify `internal/data/bom_search_task_alias_candidates.go` | 报价侧候选只读取 `manufacturer_canonical_id IS NULL` 且 `manufacturer` 非空的 `t_bom_quote_item` |
| Create `internal/data/bom_manufacturer_cleaning_repo.go` | 当前 session 厂牌清洗候选读取与回填更新 |
| Modify `internal/biz/repo.go` | 增加清洗候选/回填 repo 接口 DTO |
| Modify `internal/service/bom_manufacturer_alias_candidates.go` | 候选按“需求侧待清洗 / 报价侧待清洗”输出，报价侧优先推荐 BOM canonical |
| Modify `api/bom/v1/bom.proto` | 增加审核/应用清洗 RPC 与候选字段 |
| Modify `internal/service/bom_manufacturer_alias_api.go` | 写别名 + 当前 session 回填，处理同 canonical 幂等与不同 canonical 冲突 |
| Test `internal/service/bom_manufacturer_alias_candidates_test.go` | 覆盖候选推荐、空厂牌、需求侧未清洗、报价侧 canonical 已有等场景 |
| Test `internal/service/bom_manufacturer_alias_api_test.go` | 覆盖审核写入、回填、冲突 |
| Modify `web/src/api/bomMatchExtras.ts` | 增加候选字段、审核/应用已有别名 API |
| Modify `web/src/pages/bom-workbench/ManufacturerAliasReviewPanel.tsx` | 在搜索清洗页内完成候选选择 canonical、提交审核、应用已有别名 |
| Modify `web/src/pages/bom-workbench/SessionSearchCleanPanel.tsx` | 加载新候选、提交后刷新搜索清洗状态 |
| Test `web/src/api/bomMatchExtras.test.ts` | API wrapper 测试 |
| Test `web/src/pages/BomWorkbenchPage.test.tsx` 或 `ManufacturerAliasReviewPanel.test.tsx` | 页面交互测试 |

---

### Task 1: Schema 与模型字段

**Files:**
- Create: `docs/schema/migrations/20260426_add_bom_session_line_mfr_canonical_id.sql`
- Modify: `internal/data/models.go`
- Modify: `internal/biz/bom_import_excel.go`
- Modify: `internal/biz/repo.go`
- Test: `internal/data/bom_session_repo_test.go`

- [ ] **Step 1: 写失败测试，证明 `BomSessionLine` 需要保存 canonical**

在 `internal/data/bom_session_repo_test.go` 增加：

```go
func TestBomSessionRepo_ReplaceSessionLinesPersistsManufacturerCanonicalID(t *testing.T) {
	ctx := context.Background()
	db := openBomSessionRepoTestDB(t)
	repo := NewBomSessionRepo(&Data{DB: db})
	sessionID, _, _, err := repo.CreateSession(ctx, "清洗测试", []string{"find_chips"}, nil, nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	canon := "MFR_TEXAS_INSTRUMENTS"
	_, err = repo.ReplaceSessionLines(ctx, sessionID, []biz.BomImportLine{
		{LineNo: 1, Mpn: "SN74HC595DR", Mfr: "TI", ManufacturerCanonicalID: &canon},
		{LineNo: 2, Mpn: "EMPTY-MFR"},
	}, nil)
	if err != nil {
		t.Fatalf("ReplaceSessionLines() error = %v", err)
	}
	rows, err := repo.ListSessionLinesFull(ctx, sessionID)
	if err != nil {
		t.Fatalf("ListSessionLinesFull() error = %v", err)
	}
	if rows[0].ManufacturerCanonicalID == nil || *rows[0].ManufacturerCanonicalID != canon {
		t.Fatalf("line 1 canonical = %v, want %s", rows[0].ManufacturerCanonicalID, canon)
	}
	if rows[1].ManufacturerCanonicalID != nil {
		t.Fatalf("empty manufacturer line canonical = %v, want nil", *rows[1].ManufacturerCanonicalID)
	}
}
```

- [ ] **Step 2: 运行测试确认失败**

Run:

```powershell
$env:GOCACHE='D:\workspace\caichip\.gocache'
& 'C:\Program Files\Go\bin\go.exe' test ./internal/data -run BomSessionRepo_ReplaceSessionLinesPersistsManufacturerCanonicalID -count=1
```

Expected: FAIL，报 `ManufacturerCanonicalID` 字段或 `BomImportLine.ManufacturerCanonicalID` 不存在。

- [ ] **Step 3: 编写 migration**

Create `docs/schema/migrations/20260426_add_bom_session_line_mfr_canonical_id.sql`：

```sql
-- BOM 需求侧厂牌规范 ID：空厂牌允许为空；原始厂牌非空且为空表示待清洗。
ALTER TABLE t_bom_session_line
    ADD COLUMN manufacturer_canonical_id VARCHAR(128) NULL COMMENT 'BOM 需求侧厂牌规范 ID，来源 t_bom_manufacturer_alias.canonical_id';

CREATE INDEX idx_bom_session_line_mfr_canonical_id
    ON t_bom_session_line (manufacturer_canonical_id);
```

如果目标 MySQL 环境不支持重复执行 `ADD COLUMN`，上线时由 DBA 包装成幂等脚本；代码仓库保留清晰 DDL。

- [ ] **Step 4: 更新 Go 模型与 DTO**

在 `internal/data/models.go` 的 `BomSessionLine` 中加入：

```go
ManufacturerCanonicalID *string `gorm:"column:manufacturer_canonical_id;size:128"`
```

在 `internal/biz/bom_import_excel.go` 的 `BomImportLine` 中加入：

```go
ManufacturerCanonicalID *string
```

在 `internal/biz/repo.go` 的 `BOMSessionLineView` 中加入：

```go
Mfr                     string
ManufacturerCanonicalID *string
```

- [ ] **Step 5: 运行测试确认通过**

Run:

```powershell
$env:GOCACHE='D:\workspace\caichip\.gocache'
& 'C:\Program Files\Go\bin\go.exe' test ./internal/data -run BomSessionRepo_ReplaceSessionLinesPersistsManufacturerCanonicalID -count=1
```

Expected: PASS。

- [ ] **Step 6: Commit**

```bash
git add docs/schema/migrations/20260426_add_bom_session_line_mfr_canonical_id.sql internal/data/models.go internal/biz/bom_import_excel.go internal/biz/repo.go internal/data/bom_session_repo_test.go
git commit -m "feat(bom): add manufacturer canonical id to session lines"
```

---

### Task 2: BOM 行导入 / 新增 / 编辑时 canonical 化

**Files:**
- Create: `internal/service/bom_session_line_canonical.go`
- Modify: `internal/service/bom_import_async.go`
- Modify: `internal/service/bom_service.go`
- Modify: `internal/data/bom_session_repo.go`
- Modify: `internal/biz/repo.go`
- Test: `internal/service/bom_session_line_canonical_test.go`
- Test: `internal/service/bom_service_upload_test.go`

- [ ] **Step 1: 写失败测试，导入时厂牌命中写 canonical，空厂牌不写**

Create `internal/service/bom_session_line_canonical_test.go`：

```go
func TestCanonicalizeBomImportLines(t *testing.T) {
	svc := &BomService{alias: manufacturerAliasRepoStub{
		"TI": "MFR_TI",
	}}
	lines := []biz.BomImportLine{
		{LineNo: 1, Mpn: "A", Mfr: "TI"},
		{LineNo: 2, Mpn: "B", Mfr: ""},
		{LineNo: 3, Mpn: "C", Mfr: "Unknown"},
	}
	got, err := svc.canonicalizeBomImportLines(context.Background(), lines)
	if err != nil {
		t.Fatalf("canonicalizeBomImportLines() error = %v", err)
	}
	if got[0].ManufacturerCanonicalID == nil || *got[0].ManufacturerCanonicalID != "MFR_TI" {
		t.Fatalf("line 1 canonical = %v, want MFR_TI", got[0].ManufacturerCanonicalID)
	}
	if got[1].ManufacturerCanonicalID != nil {
		t.Fatalf("empty mfr canonical = %v, want nil", got[1].ManufacturerCanonicalID)
	}
	if got[2].ManufacturerCanonicalID != nil {
		t.Fatalf("unknown mfr canonical = %v, want nil pending cleaning", got[2].ManufacturerCanonicalID)
	}
}
```

- [ ] **Step 2: 运行测试确认失败**

Run:

```powershell
$env:GOCACHE='D:\workspace\caichip\.gocache'
& 'C:\Program Files\Go\bin\go.exe' test ./internal/service -run CanonicalizeBomImportLines -count=1
```

Expected: FAIL，`canonicalizeBomImportLines` 不存在。

- [ ] **Step 3: 实现 service helper**

Create `internal/service/bom_session_line_canonical.go`：

```go
package service

import (
	"context"
	"strings"

	"caichip/internal/biz"
)

func (s *BomService) canonicalPtrForManufacturer(ctx context.Context, raw string) (*string, error) {
	if strings.TrimSpace(raw) == "" || s == nil || s.alias == nil {
		return nil, nil
	}
	id, hit, err := biz.ResolveManufacturerCanonical(ctx, raw, s.alias)
	if err != nil {
		return nil, err
	}
	if !hit {
		return nil, nil
	}
	return &id, nil
}

func (s *BomService) canonicalizeBomImportLines(ctx context.Context, lines []biz.BomImportLine) ([]biz.BomImportLine, error) {
	out := make([]biz.BomImportLine, len(lines))
	copy(out, lines)
	for i := range out {
		canon, err := s.canonicalPtrForManufacturer(ctx, out[i].Mfr)
		if err != nil {
			return nil, err
		}
		out[i].ManufacturerCanonicalID = canon
	}
	return out, nil
}
```

- [ ] **Step 4: 接入导入完成路径**

在 `internal/service/bom_import_async.go` 的 `finishBomImport` 调用 `ReplaceSessionLines` 前加入：

```go
cleanedLines, err := s.canonicalizeBomImportLines(ctx, lines)
if err != nil {
	return "BOM_IMPORT_MFR_CANONICALIZE_FAILED", err
}
lines = cleanedLines
```

- [ ] **Step 5: 扩展 repo 接口与 data 持久化**

在 `internal/biz/repo.go` 调整签名：

```go
CreateSessionLine(ctx context.Context, sessionID, mpn, mfr, pkg string, manufacturerCanonicalID *string, qty *float64, rawText, extraJSON *string) (lineID int64, lineNo int32, newRevision int, err error)
UpdateSessionLine(ctx context.Context, sessionID string, lineID int64, mpn, mfr, pkg *string, manufacturerCanonicalID **string, qty *float64, rawText, extraJSON *string) (newRevision int, err error)
```

说明：`UpdateSessionLine` 使用 `**string` 区分“不修改厂牌字段”和“厂牌被修改后 canonical 结果为 nil”。若实现者不喜欢双指针，可定义：

```go
type OptionalStringPtr struct {
	Set   bool
	Value *string
}
```

在 `internal/data/bom_session_repo.go`：

```go
if ln.ManufacturerCanonicalID != nil {
	line.ManufacturerCanonicalID = ln.ManufacturerCanonicalID
}
```

`CreateSessionLine` 创建 `BomSessionLine` 时赋值：

```go
ManufacturerCanonicalID: manufacturerCanonicalID,
```

`UpdateSessionLine` 在 `mfr != nil` 时同时设置：

```go
up["mfr"] = normalizeOptionalStringPtr(mfr)
up["manufacturer_canonical_id"] = manufacturerCanonicalIDValue
```

- [ ] **Step 6: 接入手工新增 / 编辑 BOM 行**

在 `internal/service/bom_service.go` 的 `CreateSessionLine` 调用 repo 前：

```go
canon, err := s.canonicalPtrForManufacturer(ctx, req.GetMfr())
if err != nil {
	return nil, err
}
id, lineNo, rev, err := s.session.CreateSessionLine(ctx, req.GetSessionId(), req.GetMpn(), req.GetMfr(), req.GetPackage(), canon, qty, raw, extra)
```

在 `PatchSessionLine` 中，仅当 `req.Mfr != nil` 时解析：

```go
var mfrCanon **string
if req.Mfr != nil {
	canon, err := s.canonicalPtrForManufacturer(ctx, *req.Mfr)
	if err != nil {
		return nil, err
	}
	mfrCanon = &canon
}
rev, err := s.session.UpdateSessionLine(ctx, sid, lid, mpn, mfr, pkg, mfrCanon, qty, raw, extra)
```

- [ ] **Step 7: 运行 service 测试**

Run:

```powershell
$env:GOCACHE='D:\workspace\caichip\.gocache'
& 'C:\Program Files\Go\bin\go.exe' test ./internal/service -run "CanonicalizeBomImportLines|UploadBOM|CreateSessionLine|PatchSessionLine" -count=1
```

Expected: PASS。

- [ ] **Step 8: Commit**

```bash
git add internal/service/bom_session_line_canonical.go internal/service/bom_import_async.go internal/service/bom_service.go internal/data/bom_session_repo.go internal/biz/repo.go internal/service/bom_session_line_canonical_test.go internal/service/bom_service_upload_test.go
git commit -m "feat(bom): canonicalize session line manufacturers on write"
```

---

### Task 3: 报价侧待清洗候选只读 canonical 为空的数据

**Files:**
- Modify: `internal/data/bom_search_task_alias_candidates.go`
- Test: `internal/data/bom_manufacturer_cleaning_repo_test.go` 或 `internal/data/bom_search_task_repo_batch_test.go`
- Modify: `internal/service/bom_manufacturer_alias_candidates.go`
- Test: `internal/service/bom_manufacturer_alias_candidates_test.go`

- [ ] **Step 1: 写失败测试，报价侧 canonical 已有值不再推荐**

在 `internal/service/bom_manufacturer_alias_candidates_test.go` 增加：

```go
func TestListManufacturerAliasCandidatesSkipsCleanedQuoteManufacturers(t *testing.T) {
	view := &biz.BOMSessionView{
		SessionID:   "session-1",
		BizDate:     time.Date(2026, 4, 25, 0, 0, 0, 0, time.UTC),
		PlatformIDs: []string{"find_chips"},
	}
	canonTI := "MFR_TI"
	lines := []data.BomSessionLine{
		{LineNo: 1, Mpn: "PART-A", Mfr: strPtr("TI"), ManufacturerCanonicalID: &canonTI, Package: strPtr("QFN")},
	}
	mpnNorm := biz.NormalizeMPNForBOMSearch("PART-A")
	search := &bomSearchTaskRepoStub{candidateRows: map[string][]biz.AgentQuoteRow{
		quoteCachePairKey(mpnNorm, "find_chips"): {
			{Model: "PART-A", Package: "QFN", Manufacturer: "ADI"},
		},
	}}
	search.cleanedQuoteManufacturers = map[string]struct{}{"ADI": {}}
	svc := &BomService{
		session: &bomSessionRepoStub{view: view, fullLines: lines},
		search:  search,
		alias:   manufacturerAliasRepoStub{"TI": "MFR_TI", "ADI": "MFR_ADI"},
	}
	reply, err := svc.ListManufacturerAliasCandidates(context.Background(), "session-1")
	if err != nil {
		t.Fatalf("ListManufacturerAliasCandidates() error = %v", err)
	}
	if len(reply.Items) != 0 {
		t.Fatalf("cleaned quote manufacturers should not be recommended: %+v", reply.Items)
	}
}
```

如果当前 stub 不支持 `cleanedQuoteManufacturers`，先在测试 stub 上加字段，让测试表达“data 层已经过滤 canonical 不为空的报价行”。

- [ ] **Step 2: 运行测试确认失败**

Run:

```powershell
$env:GOCACHE='D:\workspace\caichip\.gocache'
& 'C:\Program Files\Go\bin\go.exe' test ./internal/service -run ListManufacturerAliasCandidatesSkipsCleanedQuoteManufacturers -count=1
```

Expected: FAIL。

- [ ] **Step 3: 修改 data 轻量报价读取**

在 `internal/data/bom_search_task_alias_candidates.go` 的 `BomQuoteItem` 查询中增加字段并过滤：

```go
Select("quote_id, model, manufacturer, package, manufacturer_canonical_id").
Where("quote_id IN ?", quoteIDs).
Where("manufacturer <> ''").
Where("manufacturer_canonical_id IS NULL")
```

`quoteItemRow` 增加：

```go
ManufacturerCanonicalID *string `gorm:"column:manufacturer_canonical_id"`
```

说明：这是 data 层 GORM 查询，业务规则仍由 service 决定；这里的过滤只表达“候选列表只需要待清洗报价行”。

- [ ] **Step 4: service 候选逻辑优先使用 BOM 行 canonical 字段**

在 `collectLineManufacturerAliasCandidates` 中：

```go
if strings.TrimSpace(derefStrPtr(line.Mfr)) != "" && line.ManufacturerCanonicalID == nil {
	addDemandManufacturerCleaningCandidate(groups, line)
	return nil
}
if line.ManufacturerCanonicalID != nil {
	demandCanon = *line.ManufacturerCanonicalID
	hit = true
} else {
	return nil // BOM 厂牌为空，不推断报价厂牌 canonical
}
```

候选 DTO 增加 `RecommendedCanonicalID`、`Kind`、`PlatformIDs`：

```go
type ManufacturerAliasCandidate struct {
	Kind                   string   `json:"kind"` // demand | quote
	Alias                  string   `json:"alias"`
	RecommendedCanonicalID string   `json:"recommended_canonical_id"`
	LineNos                []int    `json:"line_nos"`
	PlatformIDs            []string `json:"platform_ids"`
	DemandHint             string   `json:"demand_hint"`
}
```

- [ ] **Step 5: 运行候选测试**

Run:

```powershell
$env:GOCACHE='D:\workspace\caichip\.gocache'
& 'C:\Program Files\Go\bin\go.exe' test ./internal/service -run ListManufacturerAliasCandidates -count=1
```

Expected: PASS。

- [ ] **Step 6: Commit**

```bash
git add internal/data/bom_search_task_alias_candidates.go internal/service/bom_manufacturer_alias_candidates.go internal/service/bom_manufacturer_alias_candidates_test.go
git commit -m "feat(bom): recommend only uncleansed manufacturer aliases"
```

---

### Task 4: 当前 session 审核写别名并回填

**Files:**
- Modify: `api/bom/v1/bom.proto`
- Create: `internal/data/bom_manufacturer_cleaning_repo.go`
- Modify: `internal/data/provider.go`
- Modify: `internal/biz/repo.go`
- Modify: `internal/service/bom_manufacturer_alias_api.go`
- Test: `internal/service/bom_manufacturer_alias_api_test.go`

- [ ] **Step 1: 写失败测试，审核后写 alias 并回填当前 session**

在 `internal/service/bom_manufacturer_alias_api_test.go` 增加：

```go
func TestApproveManufacturerAliasCleaningCreatesAliasAndBackfillsSession(t *testing.T) {
	aliasRepo := &manufacturerAliasRepoRecorder{}
	cleanRepo := &manufacturerCleaningRepoRecorder{}
	svc := &BomService{
		alias:        aliasRepo,
		mfrCleaning:  cleanRepo,
	}
	_, err := svc.ApproveManufacturerAliasCleaning(context.Background(), &v1.ApproveManufacturerAliasCleaningRequest{
		SessionId:   "session-1",
		Alias:       "TI",
		CanonicalId: "MFR_TI",
		DisplayName: "Texas Instruments",
	})
	if err != nil {
		t.Fatalf("ApproveManufacturerAliasCleaning() error = %v", err)
	}
	if aliasRepo.createdAlias != "TI" || aliasRepo.createdCanonicalID != "MFR_TI" {
		t.Fatalf("created alias = %q/%q", aliasRepo.createdAlias, aliasRepo.createdCanonicalID)
	}
	if cleanRepo.sessionID != "session-1" || cleanRepo.aliasNorm != "TI" || cleanRepo.canonicalID != "MFR_TI" {
		t.Fatalf("backfill args = %+v", cleanRepo)
	}
}
```

- [ ] **Step 2: 运行测试确认失败**

Run:

```powershell
$env:GOCACHE='D:\workspace\caichip\.gocache'
& 'C:\Program Files\Go\bin\go.exe' test ./internal/service -run ApproveManufacturerAliasCleaningCreatesAliasAndBackfillsSession -count=1
```

Expected: FAIL，RPC/service/repo 不存在。

- [ ] **Step 3: 扩展 proto**

在 `api/bom/v1/bom.proto` 增加：

```proto
message ApproveManufacturerAliasCleaningRequest {
  string session_id = 1;
  string alias = 2;
  string canonical_id = 3;
  string display_name = 4;
}

message ApproveManufacturerAliasCleaningReply {
  int32 session_line_updated = 1;
  int32 quote_item_updated = 2;
}

message ApplyManufacturerAliasesToSessionRequest {
  string session_id = 1;
}

message ApplyManufacturerAliasesToSessionReply {
  int32 session_line_updated = 1;
  int32 quote_item_updated = 2;
}
```

在 service 中增加：

```proto
rpc ApproveManufacturerAliasCleaning(ApproveManufacturerAliasCleaningRequest) returns (ApproveManufacturerAliasCleaningReply) {
  option (google.api.http) = {
    post: "/api/v1/bom-sessions/{session_id}/manufacturer-alias-approvals"
    body: "*"
  };
}

rpc ApplyManufacturerAliasesToSession(ApplyManufacturerAliasesToSessionRequest) returns (ApplyManufacturerAliasesToSessionReply) {
  option (google.api.http) = {
    post: "/api/v1/bom-sessions/{session_id}/manufacturer-aliases/apply"
    body: "*"
  };
}
```

运行项目既有 proto 生成命令；若本地没有封装命令，按仓库现有 `make api` 或 `buf` 配置执行。

- [ ] **Step 4: 定义 data repo 接口**

在 `internal/biz/repo.go` 增加：

```go
type ManufacturerCleaningBackfillResult struct {
	SessionLineUpdated int64
	QuoteItemUpdated   int64
}

type ManufacturerCleaningRepo interface {
	DBOk() bool
	BackfillSessionManufacturerCanonical(ctx context.Context, sessionID, aliasNorm, canonicalID string, overwrite bool) (ManufacturerCleaningBackfillResult, error)
	ApplyKnownAliasesToSession(ctx context.Context, sessionID string) (ManufacturerCleaningBackfillResult, error)
}
```

- [ ] **Step 5: 实现当前 session 回填**

Create `internal/data/bom_manufacturer_cleaning_repo.go`：

```go
type ManufacturerCleaningRepo struct {
	db    *gorm.DB
	alias biz.AliasLookup
}

func NewManufacturerCleaningRepo(d *Data, alias biz.AliasLookup) *ManufacturerCleaningRepo {
	if d == nil || d.DB == nil {
		return &ManufacturerCleaningRepo{alias: alias}
	}
	return &ManufacturerCleaningRepo{db: d.DB, alias: alias}
}

func (r *ManufacturerCleaningRepo) DBOk() bool {
	return r != nil && r.db != nil
}
```

`BackfillSessionManufacturerCanonical` 使用 GORM 分两段更新：

```go
// 需求侧：同 session、mfr 规范化后等于 aliasNorm、canonical 为空时更新。
// 报价侧：当前 session 的搜索任务对应 quote_cache -> quote_item，manufacturer 规范化后等于 aliasNorm、canonical 为空时更新。
```

实现时不要在业务代码拼 SQL；复杂筛选分批读出 ID，再用 `Where("id IN ?", ids).Updates(...)` 更新。`aliasNorm` 比较用 Go 侧 `biz.NormalizeMfrString(row.Manufacturer)` 过滤，避免依赖数据库函数。

- [ ] **Step 6: 实现 service API**

在 `internal/service/bom_manufacturer_alias_api.go` 增加：

```go
func (s *BomService) ApproveManufacturerAliasCleaning(ctx context.Context, req *v1.ApproveManufacturerAliasCleaningRequest) (*v1.ApproveManufacturerAliasCleaningReply, error) {
	alias := strings.TrimSpace(req.GetAlias())
	canonicalID := strings.TrimSpace(req.GetCanonicalId())
	displayName := strings.TrimSpace(req.GetDisplayName())
	sessionID := strings.TrimSpace(req.GetSessionId())
	if alias == "" || canonicalID == "" || displayName == "" || sessionID == "" {
		return nil, kerrors.BadRequest("BAD_INPUT", "session_id, alias, canonical_id, display_name required")
	}
	aliasNorm := biz.NormalizeMfrString(alias)
	err := s.alias.CreateRow(ctx, canonicalID, displayName, alias, aliasNorm)
	if err != nil && !isSameAliasCanonicalConflict(ctx, s.alias, aliasNorm, canonicalID, err) {
		return nil, err
	}
	res, err := s.mfrCleaning.BackfillSessionManufacturerCanonical(ctx, sessionID, aliasNorm, canonicalID, false)
	if err != nil {
		return nil, err
	}
	return &v1.ApproveManufacturerAliasCleaningReply{
		SessionLineUpdated: int32(res.SessionLineUpdated),
		QuoteItemUpdated: int32(res.QuoteItemUpdated),
	}, nil
}
```

同 canonical 已存在时视为幂等；不同 canonical 冲突返回 `ALIAS_EXISTS`。

- [ ] **Step 7: 运行测试**

Run:

```powershell
$env:GOCACHE='D:\workspace\caichip\.gocache'
& 'C:\Program Files\Go\bin\go.exe' test ./internal/service ./internal/data -run "ApproveManufacturerAliasCleaning|ApplyKnownAliasesToSession|ManufacturerCleaning" -count=1
```

Expected: PASS。

- [ ] **Step 8: Commit**

```bash
git add api/bom/v1/bom.proto api/bom/v1/*.pb.go internal/biz/repo.go internal/data/bom_manufacturer_cleaning_repo.go internal/data/provider.go internal/service/bom_manufacturer_alias_api.go internal/service/bom_manufacturer_alias_api_test.go
git commit -m "feat(bom): approve manufacturer alias cleaning and backfill session"
```

---

### Task 5: 自动配单前拦截需求侧未清洗厂牌

**Files:**
- Modify: `internal/service/bom_match_parallel.go`
- Modify: `internal/service/bom_service.go`
- Test: `internal/service/bom_match_readiness_availability_test.go`
- Test: `internal/service/bom_manufacturer_alias_candidates_test.go`

- [ ] **Step 1: 写失败测试，BOM 厂牌非空但 canonical 为空时不自动配单**

在 `internal/service/bom_match_readiness_availability_test.go` 增加：

```go
func TestAutoMatchBlocksUncleanedDemandManufacturer(t *testing.T) {
	lines := []data.BomSessionLine{
		{ID: 1, LineNo: 1, Mpn: "PART-A", Mfr: strPtr("UnknownMfr"), Package: strPtr("QFN")},
	}
	svc := &BomService{
		session: &bomSessionRepoStub{
			view:      &biz.BOMSessionView{SessionID: "s1", PlatformIDs: []string{"find_chips"}},
			fullLines: lines,
		},
		search: &bomSearchTaskRepoStub{},
		alias:  manufacturerAliasRepoStub{},
	}
	_, err := svc.AutoMatch(context.Background(), &v1.AutoMatchRequest{BomId: "s1"})
	if err == nil || !strings.Contains(err.Error(), "MFR_CLEANING_REQUIRED") {
		t.Fatalf("AutoMatch error = %v, want MFR_CLEANING_REQUIRED", err)
	}
}
```

- [ ] **Step 2: 运行测试确认失败**

Run:

```powershell
$env:GOCACHE='D:\workspace\caichip\.gocache'
& 'C:\Program Files\Go\bin\go.exe' test ./internal/service -run AutoMatchBlocksUncleanedDemandManufacturer -count=1
```

Expected: FAIL。

- [ ] **Step 3: 实现拦截函数**

在 `internal/service/bom_service.go` 增加：

```go
func demandManufacturerCleaningRequired(lines []data.BomSessionLine) []int {
	var out []int
	for _, line := range lines {
		if strings.TrimSpace(derefStrPtr(line.Mfr)) != "" && line.ManufacturerCanonicalID == nil {
			out = append(out, line.LineNo)
		}
	}
	return out
}
```

在自动配单读取 lines 后：

```go
if pending := demandManufacturerCleaningRequired(lines); len(pending) > 0 {
	return nil, kerrors.FailedPrecondition("MFR_CLEANING_REQUIRED", fmt.Sprintf("manufacturer cleaning required for lines: %v", pending))
}
```

BOM 厂牌为空仍允许进入配单。

- [ ] **Step 4: 调整匹配使用 BOM 行 canonical**

在 `matchOneLine` 中，如果 `line.ManufacturerCanonicalID != nil`，直接构造：

```go
mfrHint = &biz.BomManufacturerResolveHint{CanonID: *line.ManufacturerCanonicalID, Hit: true}
```

只有历史数据没有 canonical 但有 mfr 时才走 `ResolveManufacturerCanonical` 兼容路径。

- [ ] **Step 5: 运行测试**

Run:

```powershell
$env:GOCACHE='D:\workspace\caichip\.gocache'
& 'C:\Program Files\Go\bin\go.exe' test ./internal/service -run "AutoMatchBlocksUncleanedDemandManufacturer|MatchReadiness|ListManufacturerAliasCandidates" -count=1
```

Expected: PASS。

- [ ] **Step 6: Commit**

```bash
git add internal/service/bom_service.go internal/service/bom_match_parallel.go internal/service/bom_match_readiness_availability_test.go
git commit -m "feat(bom): require manufacturer cleaning before auto match"
```

---

### Task 6: 搜索清洗页前端审核交互

**Files:**
- Modify: `web/src/api/bomMatchExtras.ts`
- Test: `web/src/api/bomMatchExtras.test.ts`
- Modify: `web/src/pages/bom-workbench/ManufacturerAliasReviewPanel.tsx`
- Modify: `web/src/pages/bom-workbench/SessionSearchCleanPanel.tsx`
- Test: `web/src/pages/bom-workbench/ManufacturerAliasReviewPanel.test.tsx`
- Test: `web/src/pages/BomWorkbenchPage.test.tsx`

- [ ] **Step 1: 写 API wrapper 失败测试**

在 `web/src/api/bomMatchExtras.test.ts` 增加：

```ts
it('approves manufacturer alias cleaning for current session', async () => {
  const { approveManufacturerAliasCleaning } = await import('./bomMatchExtras')
  await approveManufacturerAliasCleaning('session-1', {
    alias: 'TI',
    canonical_id: 'MFR_TI',
    display_name: 'Texas Instruments',
  })
  expect(fetchJsonMock).toHaveBeenCalledWith(
    '/api/v1/bom-sessions/session-1/manufacturer-alias-approvals',
    expect.objectContaining({
      method: 'POST',
      body: JSON.stringify({
        alias: 'TI',
        canonical_id: 'MFR_TI',
        display_name: 'Texas Instruments',
      }),
    })
  )
})
```

- [ ] **Step 2: 运行前端 API 测试确认失败**

Run:

```powershell
& 'D:\Program Files\nodejs\npm.cmd' test -- src/api/bomMatchExtras.test.ts
```

Expected: FAIL。

- [ ] **Step 3: 实现 API wrapper**

在 `web/src/api/bomMatchExtras.ts` 增加字段：

```ts
export interface ManufacturerAliasCandidate {
  kind: 'demand' | 'quote'
  alias: string
  recommended_canonical_id: string
  line_nos: number[]
  platform_ids: string[]
  demand_hint: string
}

export async function approveManufacturerAliasCleaning(
  sessionId: string,
  input: { alias: string; canonical_id: string; display_name: string }
): Promise<{ session_line_updated: number; quote_item_updated: number }> {
  return fetchJson(`/api/v1/bom-sessions/${encodeURIComponent(sessionId)}/manufacturer-alias-approvals`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(input),
  })
}

export async function applyManufacturerAliasesToSession(sessionId: string): Promise<{ session_line_updated: number; quote_item_updated: number }> {
  return fetchJson(`/api/v1/bom-sessions/${encodeURIComponent(sessionId)}/manufacturer-aliases/apply`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({}),
  })
}
```

- [ ] **Step 4: 写面板交互测试**

Create `web/src/pages/bom-workbench/ManufacturerAliasReviewPanel.test.tsx`：

```tsx
it('submits selected canonical for pending quote manufacturer', async () => {
  const onApprove = vi.fn().mockResolvedValue(undefined)
  render(
    <ManufacturerAliasReviewPanel
      pendingRows={[{ kind: 'quote', alias: 'TI', lineIndexes: [1], platformIds: ['find_chips'], demandHint: 'Texas Instruments', recommendedCanonicalId: 'MFR_TI' }]}
      canonicalRows={[{ canonical_id: 'MFR_TI', display_name: 'Texas Instruments' }]}
      onApprove={onApprove}
      onApplyExisting={vi.fn()}
    />
  )
  await userEvent.click(screen.getByRole('button', { name: /审核通过/ }))
  expect(onApprove).toHaveBeenCalledWith({
    alias: 'TI',
    canonical_id: 'MFR_TI',
    display_name: 'Texas Instruments',
  })
})
```

- [ ] **Step 5: 实现 UI 控件**

在 `ManufacturerAliasReviewPanel.tsx`：

- 每条候选显示 `kind`、`alias`、影响行、平台、推荐 canonical。
- canonical 有推荐值时默认选中推荐值。
- 提供下拉选择 canonical。
- 提供“审核通过”按钮调用 `onApprove`。
- 提供“应用已有别名清洗”按钮调用 `onApplyExisting`。
- 提交中禁用按钮并显示错误。

Props 调整为：

```ts
interface ManufacturerAliasReviewPanelProps {
  pendingRows: PendingMfrRow[]
  canonicalRows: ManufacturerCanonicalRow[]
  onApprove: (input: { alias: string; canonical_id: string; display_name: string }) => Promise<void>
  onApplyExisting: () => Promise<void>
}
```

- [ ] **Step 6: 接入搜索清洗页**

在 `SessionSearchCleanPanel.tsx`：

```ts
async function handleApproveManufacturerAlias(input: { alias: string; canonical_id: string; display_name: string }) {
  await approveManufacturerAliasCleaning(sessionId, input)
  await loadSearchTasks()
}

async function handleApplyExistingAliases() {
  await applyManufacturerAliasesToSession(sessionId)
  await loadSearchTasks()
}
```

提交成功后刷新候选和搜索任务状态。

- [ ] **Step 7: 运行前端测试**

Run:

```powershell
& 'D:\Program Files\nodejs\npm.cmd' test -- src/api/bomMatchExtras.test.ts src/pages/bom-workbench/ManufacturerAliasReviewPanel.test.tsx src/pages/BomWorkbenchPage.test.tsx
```

Expected: PASS。

- [ ] **Step 8: Commit**

```bash
git add web/src/api/bomMatchExtras.ts web/src/api/bomMatchExtras.test.ts web/src/pages/bom-workbench/ManufacturerAliasReviewPanel.tsx web/src/pages/bom-workbench/ManufacturerAliasReviewPanel.test.tsx web/src/pages/bom-workbench/SessionSearchCleanPanel.tsx web/src/pages/BomWorkbenchPage.test.tsx
git commit -m "feat(web): review manufacturer aliases in search cleaning"
```

---

### Task 7: 回填工具与纠错入口

**Files:**
- Modify: `cmd/tools/backfill_manufacturer_canonical_id/main.go`
- Test: `cmd/tools/backfill_manufacturer_canonical_id/main_test.go`
- Modify: `docs/superpowers/specs/2026-04-18-manufacturer-canonicalization-design.md`（仅当实现偏离当前设计）

- [ ] **Step 1: 写失败测试，支持 session scope 与 overwrite**

在 `cmd/tools/backfill_manufacturer_canonical_id/main_test.go` 增加核心函数测试：

```go
func TestBackfillOptionsParseSessionAndOverwrite(t *testing.T) {
	opts, err := parseBackfillOptions([]string{"--session-id", "session-1", "--overwrite", "--dry-run"})
	if err != nil {
		t.Fatalf("parseBackfillOptions() error = %v", err)
	}
	if opts.SessionID != "session-1" || !opts.Overwrite || !opts.DryRun {
		t.Fatalf("opts = %+v", opts)
	}
}
```

- [ ] **Step 2: 运行测试确认失败**

Run:

```powershell
$env:GOCACHE='D:\workspace\caichip\.gocache'
& 'C:\Program Files\Go\bin\go.exe' test ./cmd/tools/backfill_manufacturer_canonical_id -count=1
```

Expected: FAIL。

- [ ] **Step 3: 扩展回填参数**

在工具中支持：

```text
--session-id <id>      只回填当前 BOM session 的 t_bom_session_line 与 t_bom_quote_item
--overwrite            允许覆盖已有 manufacturer_canonical_id，用于纠错
--dry-run              只输出将更新数量
--limit <n>            单批扫描数量
```

回填核心调用 `ManufacturerCleaningRepo.BackfillSessionManufacturerCanonical` 或 `ApplyKnownAliasesToSession`，不要复制一份规则。

- [ ] **Step 4: 运行工具测试**

Run:

```powershell
$env:GOCACHE='D:\workspace\caichip\.gocache'
& 'C:\Program Files\Go\bin\go.exe' test ./cmd/tools/backfill_manufacturer_canonical_id -count=1
```

Expected: PASS。

- [ ] **Step 5: Commit**

```bash
git add cmd/tools/backfill_manufacturer_canonical_id/main.go cmd/tools/backfill_manufacturer_canonical_id/main_test.go
git commit -m "feat(tools): support session scoped manufacturer backfill"
```

---

### Task 8: 全量验证与发布检查

**Files:**
- No code files expected unless previous tasks reveal a defect.

- [ ] **Step 1: 后端目标测试**

Run:

```powershell
$env:GOCACHE='D:\workspace\caichip\.gocache'
& 'C:\Program Files\Go\bin\go.exe' test ./internal/biz/... ./internal/data/... ./internal/service/... ./cmd/tools/backfill_manufacturer_canonical_id -run "Manufacturer|Mfr|Alias|Canonical|Cleaning|AutoMatchBlocksUncleanedDemandManufacturer" -count=1
```

Expected: PASS。

- [ ] **Step 2: 后端编译**

Run:

```powershell
$env:GOCACHE='D:\workspace\caichip\.gocache'
& 'C:\Program Files\Go\bin\go.exe' build ./cmd/server/...
```

Expected: PASS。

- [ ] **Step 3: 前端目标测试**

Run:

```powershell
& 'D:\Program Files\nodejs\npm.cmd' test -- src/api/bomMatchExtras.test.ts src/pages/bom-workbench/ManufacturerAliasReviewPanel.test.tsx src/pages/BomWorkbenchPage.test.tsx
```

Expected: PASS。

- [ ] **Step 4: 前端构建**

Run:

```powershell
& 'D:\Program Files\nodejs\npm.cmd' run build
```

Expected: PASS。

- [ ] **Step 5: 手工验收**

准备一个 session：

1. BOM 行 1：`mpn=PART-A`、`manufacturer=TI`，别名表已有 `TI -> MFR_TI`。
2. BOM 行 2：`mpn=PART-B`、`manufacturer=UnknownDemand`，别名表未命中。
3. 报价行：`PART-A` 同封装，平台厂牌 `Texas Instruments`，`manufacturer_canonical_id=NULL`。

验收：

- 搜索清洗页显示 `Texas Instruments` 推荐映射到 `MFR_TI`。
- 搜索清洗页显示 `UnknownDemand` 为需求侧待清洗。
- 审核 `Texas Instruments -> MFR_TI` 后，当前 session 对应 `t_bom_quote_item.manufacturer_canonical_id` 被回填。
- BOM 行 2 未清洗前，自动配单返回 `MFR_CLEANING_REQUIRED`。
- BOM 厂牌为空的行仍可自动配单。

- [ ] **Step 6: Commit 验证修补**

如本任务只修补测试或小问题：

```bash
git add <changed-files>
git commit -m "test(bom): verify manufacturer cleaning workflow"
```

---

## 自审结果

- Spec 覆盖：已覆盖 `t_bom_session_line` 字段、报价侧只查 canonical 为空、搜索清洗页职责、审核回填、空厂牌区别、平台歧义、纠错回填、配单前拦截。
- 占位扫描：未发现禁用占位表达。
- 类型一致性：后端统一使用 `manufacturer_canonical_id` / `ManufacturerCanonicalID`；前端使用 JSON 字段 `recommended_canonical_id`、`platform_ids`、`line_nos`。

## Execution Handoff

Plan complete and saved to `docs/superpowers/plans/2026-04-26-bom-manufacturer-cleaning-implementation.md`. Two execution options:

**1. Subagent-Driven (recommended)** - I dispatch a fresh subagent per task, review between tasks, fast iteration

**2. Inline Execution** - Execute tasks in this session using executing-plans, batch execution with checkpoints

Which approach?
