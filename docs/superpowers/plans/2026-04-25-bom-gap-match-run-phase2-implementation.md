# BOM 缺口处理与配单方案快照二期 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 实现 BOM 缺口落表、人工补录报价、替代料采集、版本化配单方案快照和按 run 导出闭环。

**Architecture:** `internal/biz` 定义缺口状态机、配单 run 汇总和 result item 来源判定；`internal/data` 只用 GORM 实现新增表的持久化；`internal/service` 编排缺口同步、人工报价入池、替代料搜索任务、保存配单方案与导出。前端只消费后端 API，不复制配单和缺口判定规则。

**Tech Stack:** Go + Kratos + GORM + MySQL migration、protobuf、React + TypeScript + Vitest、excelize。

---

## 文件结构

- 新建 `docs/schema/migrations/20260425_bom_gap_match_run_phase2.sql`：新增 `t_bom_line_gap`、`t_bom_match_run`、`t_bom_match_result_item`，并扩展报价来源字段。
- 修改 `internal/data/table_names.go`：新增三张表名常量。
- 修改 `internal/data/models.go`：新增 GORM model，扩展报价来源字段。
- 新建 `internal/biz/bom_gap.go`：缺口类型、状态、转换规则和同步输入输出。
- 新建 `internal/biz/bom_match_run.go`：run 状态、result item 来源类型、汇总计算。
- 修改 `internal/biz/repo.go`：新增 `BOMLineGapRepo`、`BOMMatchRunRepo` 接口，扩展人工报价所需 repo 方法。
- 新建 `internal/data/bom_line_gap_repo.go` 和测试：GORM 缺口 upsert、查询、状态更新。
- 新建 `internal/data/bom_match_run_repo.go` 和测试：GORM 创建 run、批量写 item、run_no 递增、supersede。
- 修改 `internal/data/bom_search_task_repo.go`：必要时扩展人工报价入池方法，保持报价读取仍走统一候选。
- 新建 `internal/service/bom_gap_service.go`：缺口同步、列表、人工补录、替代料选择。
- 新建 `internal/service/bom_match_run_service.go`：保存方案、查询方案、按 run 导出。
- 修改 `internal/service/bom_service.go`：接入新 service helper，扩展导出入口。
- 修改 `internal/service/provider.go`、`internal/data/provider.go`、`cmd/server/wire.go`、`cmd/server/wire_gen.go`：注入新增 repo。
- 修改 `api/bom/v1/bom.proto`：新增缺口和配单方案 API message/rpc。
- 重新生成 `api/bom/v1/bom.pb.go`、`api/bom/v1/bom_http.pb.go`、`api/bom/v1/bom_grpc.pb.go`。
- 修改 `web/src/api/types.ts`、`web/src/api/bomSession.ts`：新增 API 类型与客户端方法。
- 修改 `web/src/pages/SourcingSessionPage.tsx` 和测试：缺口面板、人工补录入口、替代料入口、方案保存入口、run 列表入口。

---

### Task 1: 新增二期表结构与 GORM model

**Files:**
- Create: `docs/schema/migrations/20260425_bom_gap_match_run_phase2.sql`
- Modify: `internal/data/table_names.go`
- Modify: `internal/data/models.go`

- [ ] **Step 1: 编写 migration**

创建 `docs/schema/migrations/20260425_bom_gap_match_run_phase2.sql`：

```sql
-- BOM 缺口处理与配单方案快照二期。

CREATE TABLE IF NOT EXISTS t_bom_line_gap (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  session_id CHAR(36) NOT NULL,
  line_id BIGINT NOT NULL,
  line_no INT NOT NULL,
  mpn VARCHAR(256) NOT NULL DEFAULT '',
  gap_type VARCHAR(64) NOT NULL,
  reason_code VARCHAR(64) NOT NULL DEFAULT '',
  reason_detail TEXT NULL,
  resolution_status VARCHAR(32) NOT NULL DEFAULT 'open',
  active_key VARCHAR(191) NOT NULL DEFAULT '',
  resolved_by VARCHAR(128) NULL,
  resolved_at DATETIME(3) NULL,
  resolution_note TEXT NULL,
  substitute_mpn VARCHAR(256) NULL,
  substitute_reason TEXT NULL,
  created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  updated_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
  PRIMARY KEY (id),
  UNIQUE KEY uk_bom_line_gap_active (active_key),
  KEY idx_bom_line_gap_session_status (session_id, resolution_status),
  KEY idx_bom_line_gap_line (session_id, line_id),
  KEY idx_bom_line_gap_updated (updated_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
COMMENT='BOM 行级数据缺口处理表';

CREATE TABLE IF NOT EXISTS t_bom_match_run (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  run_no INT NOT NULL,
  session_id CHAR(36) NOT NULL,
  selection_revision INT NOT NULL,
  status VARCHAR(32) NOT NULL DEFAULT 'saved',
  source VARCHAR(32) NOT NULL DEFAULT 'manual_save',
  line_total INT NOT NULL DEFAULT 0,
  matched_line_count INT NOT NULL DEFAULT 0,
  unresolved_line_count INT NOT NULL DEFAULT 0,
  total_amount DECIMAL(24,6) NOT NULL DEFAULT 0,
  currency VARCHAR(8) NOT NULL DEFAULT 'CNY',
  created_by VARCHAR(128) NULL,
  created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  saved_at DATETIME(3) NULL,
  superseded_at DATETIME(3) NULL,
  PRIMARY KEY (id),
  UNIQUE KEY uk_bom_match_run_session_no (session_id, run_no),
  KEY idx_bom_match_run_session_status (session_id, status),
  KEY idx_bom_match_run_created (created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
COMMENT='BOM 配单方案版本';

CREATE TABLE IF NOT EXISTS t_bom_match_result_item (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  run_id BIGINT UNSIGNED NOT NULL,
  session_id CHAR(36) NOT NULL,
  line_id BIGINT NOT NULL,
  line_no INT NOT NULL,
  source_type VARCHAR(32) NOT NULL,
  match_status VARCHAR(32) NOT NULL DEFAULT '',
  gap_id BIGINT UNSIGNED NULL,
  quote_item_id BIGINT UNSIGNED NULL,
  platform_id VARCHAR(32) NOT NULL DEFAULT '',
  demand_mpn VARCHAR(256) NOT NULL DEFAULT '',
  demand_mfr VARCHAR(256) NOT NULL DEFAULT '',
  demand_package VARCHAR(128) NOT NULL DEFAULT '',
  demand_qty DECIMAL(18,4) NULL,
  matched_mpn VARCHAR(256) NOT NULL DEFAULT '',
  matched_mfr VARCHAR(256) NOT NULL DEFAULT '',
  matched_package VARCHAR(128) NOT NULL DEFAULT '',
  stock BIGINT NULL,
  lead_time VARCHAR(128) NOT NULL DEFAULT '',
  unit_price DECIMAL(24,6) NULL,
  subtotal DECIMAL(24,6) NULL,
  currency VARCHAR(8) NOT NULL DEFAULT 'CNY',
  original_mpn VARCHAR(256) NULL,
  substitute_mpn VARCHAR(256) NULL,
  substitute_reason TEXT NULL,
  code_ts CHAR(10) NOT NULL DEFAULT '',
  control_mark VARCHAR(64) NOT NULL DEFAULT '',
  import_tax_imp_ordinary_rate VARCHAR(64) NOT NULL DEFAULT '',
  import_tax_imp_discount_rate VARCHAR(64) NOT NULL DEFAULT '',
  import_tax_imp_temp_rate VARCHAR(64) NOT NULL DEFAULT '',
  snapshot_json JSON NULL,
  created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  PRIMARY KEY (id),
  UNIQUE KEY uk_bom_match_result_run_line (run_id, line_id),
  KEY idx_bom_match_result_session_line (session_id, line_id),
  KEY idx_bom_match_result_gap (gap_id),
  KEY idx_bom_match_result_quote_item (quote_item_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
COMMENT='BOM 配单方案行结果快照';

ALTER TABLE t_bom_quote_cache
  ADD COLUMN source_type VARCHAR(32) NOT NULL DEFAULT 'platform',
  ADD COLUMN session_id CHAR(36) NULL,
  ADD COLUMN line_id BIGINT NULL,
  ADD COLUMN created_by VARCHAR(128) NULL;

ALTER TABLE t_bom_quote_item
  ADD COLUMN source_type VARCHAR(32) NOT NULL DEFAULT 'platform',
  ADD COLUMN session_id CHAR(36) NULL,
  ADD COLUMN line_id BIGINT NULL,
  ADD COLUMN created_by VARCHAR(128) NULL;
```

- [ ] **Step 2: 扩展表名常量**

在 `internal/data/table_names.go` 中加入：

```go
const (
	TableBomLineGap         = "t_bom_line_gap"
	TableBomMatchRun        = "t_bom_match_run"
	TableBomMatchResultItem = "t_bom_match_result_item"
)
```

如果该文件已有单个 const block，把三项放入现有 block，避免重复定义。

- [ ] **Step 3: 扩展 GORM model**

在 `internal/data/models.go` 中新增：

```go
type BomLineGap struct {
	ID               uint64     `gorm:"column:id;primaryKey;autoIncrement"`
	SessionID        string     `gorm:"column:session_id;size:36;not null;index:idx_bom_line_gap_session_status,priority:1;index:idx_bom_line_gap_line,priority:1"`
	LineID           int64      `gorm:"column:line_id;not null;index:idx_bom_line_gap_line,priority:2"`
	LineNo           int        `gorm:"column:line_no;not null"`
	Mpn              string     `gorm:"column:mpn;size:256;not null;default:''"`
	GapType          string     `gorm:"column:gap_type;size:64;not null"`
	ReasonCode       string     `gorm:"column:reason_code;size:64;not null;default:''"`
	ReasonDetail     *string    `gorm:"column:reason_detail;type:text"`
	ResolutionStatus string     `gorm:"column:resolution_status;size:32;not null;default:open;index:idx_bom_line_gap_session_status,priority:2"`
	ActiveKey        string     `gorm:"column:active_key;size:191;not null;default:'';uniqueIndex:uk_bom_line_gap_active"`
	ResolvedBy       *string    `gorm:"column:resolved_by;size:128"`
	ResolvedAt       *time.Time `gorm:"column:resolved_at;precision:3"`
	ResolutionNote   *string    `gorm:"column:resolution_note;type:text"`
	SubstituteMpn    *string    `gorm:"column:substitute_mpn;size:256"`
	SubstituteReason *string    `gorm:"column:substitute_reason;type:text"`
	CreatedAt        time.Time  `gorm:"column:created_at;precision:3;autoCreateTime"`
	UpdatedAt        time.Time  `gorm:"column:updated_at;precision:3;autoUpdateTime;index:idx_bom_line_gap_updated"`
}

func (BomLineGap) TableName() string { return TableBomLineGap }

type BomMatchRun struct {
	ID                  uint64     `gorm:"column:id;primaryKey;autoIncrement"`
	RunNo               int        `gorm:"column:run_no;not null;uniqueIndex:uk_bom_match_run_session_no,priority:2"`
	SessionID           string     `gorm:"column:session_id;size:36;not null;uniqueIndex:uk_bom_match_run_session_no,priority:1;index:idx_bom_match_run_session_status,priority:1"`
	SelectionRevision   int        `gorm:"column:selection_revision;not null"`
	Status              string     `gorm:"column:status;size:32;not null;default:saved;index:idx_bom_match_run_session_status,priority:2"`
	Source              string     `gorm:"column:source;size:32;not null;default:manual_save"`
	LineTotal           int        `gorm:"column:line_total;not null;default:0"`
	MatchedLineCount    int        `gorm:"column:matched_line_count;not null;default:0"`
	UnresolvedLineCount int        `gorm:"column:unresolved_line_count;not null;default:0"`
	TotalAmount         float64    `gorm:"column:total_amount;type:decimal(24,6);not null;default:0"`
	Currency            string     `gorm:"column:currency;size:8;not null;default:CNY"`
	CreatedBy           *string    `gorm:"column:created_by;size:128"`
	CreatedAt           time.Time  `gorm:"column:created_at;precision:3;autoCreateTime;index:idx_bom_match_run_created"`
	SavedAt             *time.Time `gorm:"column:saved_at;precision:3"`
	SupersededAt        *time.Time `gorm:"column:superseded_at;precision:3"`
}

func (BomMatchRun) TableName() string { return TableBomMatchRun }

type BomMatchResultItem struct {
	ID                           uint64     `gorm:"column:id;primaryKey;autoIncrement"`
	RunID                        uint64     `gorm:"column:run_id;not null;uniqueIndex:uk_bom_match_result_run_line,priority:1"`
	SessionID                    string     `gorm:"column:session_id;size:36;not null;index:idx_bom_match_result_session_line,priority:1"`
	LineID                       int64      `gorm:"column:line_id;not null;uniqueIndex:uk_bom_match_result_run_line,priority:2;index:idx_bom_match_result_session_line,priority:2"`
	LineNo                       int        `gorm:"column:line_no;not null"`
	SourceType                   string     `gorm:"column:source_type;size:32;not null"`
	MatchStatus                  string     `gorm:"column:match_status;size:32;not null;default:''"`
	GapID                        *uint64    `gorm:"column:gap_id;index:idx_bom_match_result_gap"`
	QuoteItemID                  *uint64    `gorm:"column:quote_item_id;index:idx_bom_match_result_quote_item"`
	PlatformID                   string     `gorm:"column:platform_id;size:32;not null;default:''"`
	DemandMpn                    string     `gorm:"column:demand_mpn;size:256;not null;default:''"`
	DemandMfr                    string     `gorm:"column:demand_mfr;size:256;not null;default:''"`
	DemandPackage                string     `gorm:"column:demand_package;size:128;not null;default:''"`
	DemandQty                    *float64   `gorm:"column:demand_qty;type:decimal(18,4)"`
	MatchedMpn                   string     `gorm:"column:matched_mpn;size:256;not null;default:''"`
	MatchedMfr                   string     `gorm:"column:matched_mfr;size:256;not null;default:''"`
	MatchedPackage               string     `gorm:"column:matched_package;size:128;not null;default:''"`
	Stock                        *int64     `gorm:"column:stock"`
	LeadTime                     string     `gorm:"column:lead_time;size:128;not null;default:''"`
	UnitPrice                    *float64   `gorm:"column:unit_price;type:decimal(24,6)"`
	Subtotal                     *float64   `gorm:"column:subtotal;type:decimal(24,6)"`
	Currency                     string     `gorm:"column:currency;size:8;not null;default:CNY"`
	OriginalMpn                  *string    `gorm:"column:original_mpn;size:256"`
	SubstituteMpn                *string    `gorm:"column:substitute_mpn;size:256"`
	SubstituteReason             *string    `gorm:"column:substitute_reason;type:text"`
	CodeTS                       string     `gorm:"column:code_ts;type:char(10);not null;default:''"`
	ControlMark                  string     `gorm:"column:control_mark;size:64;not null;default:''"`
	ImportTaxImpOrdinaryRate     string     `gorm:"column:import_tax_imp_ordinary_rate;size:64;not null;default:''"`
	ImportTaxImpDiscountRate     string     `gorm:"column:import_tax_imp_discount_rate;size:64;not null;default:''"`
	ImportTaxImpTempRate         string     `gorm:"column:import_tax_imp_temp_rate;size:64;not null;default:''"`
	SnapshotJSON                 []byte     `gorm:"column:snapshot_json;type:json"`
	CreatedAt                    time.Time  `gorm:"column:created_at;precision:3;autoCreateTime"`
}

func (BomMatchResultItem) TableName() string { return TableBomMatchResultItem }
```

同时扩展已有 `BomQuoteCache` 和 `BomQuoteItem`：

```go
SourceType string  `gorm:"column:source_type;size:32;not null;default:platform"`
SessionID  *string `gorm:"column:session_id;size:36"`
LineID     *int64  `gorm:"column:line_id"`
CreatedBy  *string `gorm:"column:created_by;size:128"`
```

- [ ] **Step 4: 运行编译检查**

Run:

```powershell
& 'C:\Program Files\Go\bin\go.exe' test ./internal/data -run '^$' -count=1
```

Expected: package 编译通过；如果已有测试依赖旧字段构造，补齐 `SourceType: "platform"` 或依赖默认零值不影响。

- [ ] **Step 5: Commit**

```powershell
git add -- 'docs/schema/migrations/20260425_bom_gap_match_run_phase2.sql' 'internal/data/table_names.go' 'internal/data/models.go'
git commit -m "feat(bom): add gap and match run tables"
```

---

### Task 2: 定义 biz 层缺口与配单快照规则

**Files:**
- Create: `internal/biz/bom_gap.go`
- Create: `internal/biz/bom_gap_test.go`
- Create: `internal/biz/bom_match_run.go`
- Create: `internal/biz/bom_match_run_test.go`
- Modify: `internal/biz/repo.go`

- [ ] **Step 1: 编写缺口状态测试**

Create `internal/biz/bom_gap_test.go`:

```go
package biz

import "testing"

func TestLineGapActiveKeyOnlyForOpen(t *testing.T) {
	g := BOMLineGap{
		SessionID: "sid",
		LineID:  10,
		GapType: LineGapNoData,
		Status:  LineGapOpen,
	}
	if got := g.ActiveKey(); got != "sid:10:NO_DATA" {
		t.Fatalf("active key=%q", got)
	}
	g.Status = LineGapResolved
	if got := g.ActiveKey(); got != "" {
		t.Fatalf("resolved active key=%q, want empty", got)
	}
}

func TestLineGapCanTransition(t *testing.T) {
	tests := []struct {
		from string
		to   string
		ok   bool
	}{
		{LineGapOpen, LineGapManualQuoteAdded, true},
		{LineGapOpen, LineGapSubstituteSelected, true},
		{LineGapManualQuoteAdded, LineGapResolved, true},
		{LineGapSubstituteSelected, LineGapResolved, true},
		{LineGapResolved, LineGapOpen, false},
	}
	for _, tt := range tests {
		if got := CanTransitionLineGap(tt.from, tt.to); got != tt.ok {
			t.Fatalf("%s -> %s got %v", tt.from, tt.to, got)
		}
	}
}
```

- [ ] **Step 2: 编写 run 汇总测试**

Create `internal/biz/bom_match_run_test.go`:

```go
package biz

import "testing"

func TestSummarizeMatchRunItems(t *testing.T) {
	total, matched, unresolved := SummarizeMatchRunItems([]BOMMatchResultItemDraft{
		{SourceType: MatchResultAutoMatch, Subtotal: 10},
		{SourceType: MatchResultManualQuote, Subtotal: 20},
		{SourceType: MatchResultUnresolved},
	})
	if total != 30 || matched != 2 || unresolved != 1 {
		t.Fatalf("summary total=%v matched=%d unresolved=%d", total, matched, unresolved)
	}
}

func TestMatchResultSourceFromItem(t *testing.T) {
	if got := MatchResultSourceFromMatchStatus("exact", false, false); got != MatchResultAutoMatch {
		t.Fatalf("source=%q", got)
	}
	if got := MatchResultSourceFromMatchStatus("exact", true, false); got != MatchResultManualQuote {
		t.Fatalf("source=%q", got)
	}
	if got := MatchResultSourceFromMatchStatus("exact", false, true); got != MatchResultSubstituteMatch {
		t.Fatalf("source=%q", got)
	}
	if got := MatchResultSourceFromMatchStatus("no_match", false, false); got != MatchResultUnresolved {
		t.Fatalf("source=%q", got)
	}
}
```

- [ ] **Step 3: Run failing tests**

Run:

```powershell
& 'C:\Program Files\Go\bin\go.exe' test ./internal/biz -run 'TestLineGap|TestSummarizeMatchRunItems|TestMatchResultSourceFromItem' -count=1
```

Expected: fails with undefined types/functions.

- [ ] **Step 4: 实现缺口规则**

Create `internal/biz/bom_gap.go`:

```go
package biz

import "fmt"

const (
	LineGapNoData                = "NO_DATA"
	LineGapCollectionUnavailable = "COLLECTION_UNAVAILABLE"
	LineGapNoMatchAfterFilter    = "NO_MATCH_AFTER_FILTER"

	LineGapOpen               = "open"
	LineGapManualQuoteAdded   = "manual_quote_added"
	LineGapSubstituteSelected = "substitute_selected"
	LineGapResolved           = "resolved"
	LineGapIgnored            = "ignored"
)

type BOMLineGap struct {
	ID               uint64
	SessionID        string
	LineID           int64
	LineNo           int
	Mpn              string
	GapType          string
	ReasonCode       string
	ReasonDetail     string
	Status           string
	ResolutionNote   string
	SubstituteMpn    string
	SubstituteReason string
}

func (g BOMLineGap) ActiveKey() string {
	if g.Status != LineGapOpen {
		return ""
	}
	return fmt.Sprintf("%s:%d:%s", g.SessionID, g.LineID, g.GapType)
}

func CanTransitionLineGap(from, to string) bool {
	if from == to {
		return true
	}
	switch from {
	case LineGapOpen:
		return to == LineGapManualQuoteAdded || to == LineGapSubstituteSelected || to == LineGapResolved || to == LineGapIgnored
	case LineGapManualQuoteAdded, LineGapSubstituteSelected:
		return to == LineGapResolved || to == LineGapIgnored
	default:
		return false
	}
}

func AvailabilityStatusToGapType(status string) string {
	switch status {
	case LineAvailabilityNoData:
		return LineGapNoData
	case LineAvailabilityCollectionUnavailable:
		return LineGapCollectionUnavailable
	case LineAvailabilityNoMatchAfterFilter:
		return LineGapNoMatchAfterFilter
	default:
		return ""
	}
}
```

- [ ] **Step 5: 实现 run 规则**

Create `internal/biz/bom_match_run.go`:

```go
package biz

const (
	MatchRunSaved      = "saved"
	MatchRunSuperseded = "superseded"
	MatchRunCanceled   = "canceled"

	MatchResultAutoMatch       = "auto_match"
	MatchResultManualQuote     = "manual_quote"
	MatchResultSubstituteMatch = "substitute_match"
	MatchResultUnresolved      = "unresolved"
)

type BOMMatchResultItemDraft struct {
	LineID        int64
	LineNo        int
	SourceType    string
	MatchStatus   string
	GapID         uint64
	QuoteItemID   uint64
	PlatformID    string
	DemandMpn     string
	DemandMfr     string
	DemandPackage string
	DemandQty     float64
	MatchedMpn    string
	MatchedMfr    string
	MatchedPackage string
	Stock         int64
	LeadTime      string
	UnitPrice     float64
	Subtotal      float64
	Currency      string
	OriginalMpn   string
	SubstituteMpn string
	SubstituteReason string
	CodeTS       string
	ControlMark  string
	ImportTaxImpOrdinaryRate string
	ImportTaxImpDiscountRate string
	ImportTaxImpTempRate     string
	SnapshotJSON []byte
}

func SummarizeMatchRunItems(items []BOMMatchResultItemDraft) (total float64, matched int, unresolved int) {
	for _, it := range items {
		if it.SourceType == MatchResultUnresolved {
			unresolved++
			continue
		}
		matched++
		total += it.Subtotal
	}
	return total, matched, unresolved
}

func MatchResultSourceFromMatchStatus(matchStatus string, manual bool, substitute bool) string {
	if matchStatus == "no_match" || matchStatus == "" {
		return MatchResultUnresolved
	}
	if substitute {
		return MatchResultSubstituteMatch
	}
	if manual {
		return MatchResultManualQuote
	}
	return MatchResultAutoMatch
}
```

- [ ] **Step 6: 扩展 repo 接口**

In `internal/biz/repo.go`, add focused interfaces:

```go
type BOMLineGapRepo interface {
	DBOk() bool
	UpsertOpenGaps(ctx context.Context, gaps []BOMLineGap) error
	ListLineGaps(ctx context.Context, sessionID string, statuses []string) ([]BOMLineGap, error)
	GetLineGap(ctx context.Context, gapID uint64) (*BOMLineGap, error)
	UpdateLineGapStatus(ctx context.Context, gapID uint64, fromStatus string, toStatus string, actor string, note string) error
	SelectLineGapSubstitute(ctx context.Context, gapID uint64, actor string, substituteMpn string, reason string) error
}

type BOMMatchRunRepo interface {
	DBOk() bool
	CreateMatchRun(ctx context.Context, sessionID string, selectionRevision int, currency string, createdBy string, items []BOMMatchResultItemDraft) (uint64, int, error)
	ListMatchRuns(ctx context.Context, sessionID string) ([]BOMMatchRunView, error)
	GetMatchRun(ctx context.Context, runID uint64) (*BOMMatchRunView, []BOMMatchResultItemDraft, error)
	SupersedePreviousRuns(ctx context.Context, sessionID string, keepRunID uint64) error
}

type BOMMatchRunView struct {
	ID                  uint64
	RunNo               int
	SessionID           string
	SelectionRevision   int
	Status              string
	LineTotal           int
	MatchedLineCount    int
	UnresolvedLineCount int
	TotalAmount         float64
	Currency            string
	CreatedBy           string
	CreatedAt           time.Time
	SavedAt             *time.Time
}
```

同时在现有 `BOMSearchTaskRepo` 接口中增加人工报价入池方法：

```go
UpsertManualQuote(ctx context.Context, gapID uint64, row AgentQuoteRow) error
```

data 层实现时必须通过 gap 找到 `session_id`、`line_id`、`mpn`、`biz_date` 和平台集合；人工报价写入 `source_type=manual`，并保证 `LoadQuoteCachesForKeys` 可以读到它。

Use existing imports in `repo.go`; if `time` is not imported, add it.

- [ ] **Step 7: Run biz tests**

Run:

```powershell
& 'C:\Program Files\Go\bin\go.exe' test ./internal/biz -run 'TestLineGap|TestSummarizeMatchRunItems|TestMatchResultSourceFromItem' -count=1
```

Expected: pass.

- [ ] **Step 8: Commit**

```powershell
git add -- 'internal/biz/bom_gap.go' 'internal/biz/bom_gap_test.go' 'internal/biz/bom_match_run.go' 'internal/biz/bom_match_run_test.go' 'internal/biz/repo.go'
git commit -m "feat(bom): define gap and match run domain rules"
```

---

### Task 3: 实现 data 层 GORM repo

**Files:**
- Create: `internal/data/bom_line_gap_repo.go`
- Create: `internal/data/bom_line_gap_repo_test.go`
- Create: `internal/data/bom_match_run_repo.go`
- Create: `internal/data/bom_match_run_repo_test.go`
- Modify: `internal/data/provider.go`

- [ ] **Step 1: 编写缺口 repo 测试**

Create `internal/data/bom_line_gap_repo_test.go`:

```go
package data

import (
	"context"
	"testing"

	"caichip/internal/biz"
)

func TestBOMLineGapRepo_UpsertOpenGapsDedupesActiveGap(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()
	repo := NewBOMLineGapRepo(&Data{db: db})
	gap := biz.BOMLineGap{
		SessionID: "sid",
		LineID:  1,
		LineNo:  1,
		Mpn:     "NO-DATA",
		GapType: biz.LineGapNoData,
		Status:  biz.LineGapOpen,
	}
	if err := repo.UpsertOpenGaps(context.Background(), []biz.BOMLineGap{gap, gap}); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	got, err := repo.ListLineGaps(context.Background(), "sid", []string{biz.LineGapOpen})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("gap count=%d, want 1", len(got))
	}
}
```

- [ ] **Step 2: 编写 run repo 测试**

Create `internal/data/bom_match_run_repo_test.go`:

```go
package data

import (
	"context"
	"testing"

	"caichip/internal/biz"
)

func TestBOMMatchRunRepo_CreateMatchRunIncrementsRunNoAndStoresCustomsFields(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()
	repo := NewBOMMatchRunRepo(&Data{db: db})
	item := biz.BOMMatchResultItemDraft{
		LineID: 1, LineNo: 1, SourceType: biz.MatchResultAutoMatch,
		MatchStatus: "exact", DemandMpn: "LM358", MatchedMpn: "LM358",
		Subtotal: 12.5, Currency: "CNY", CodeTS: "8542399000",
		ControlMark: "A", ImportTaxImpOrdinaryRate: "30%",
		ImportTaxImpDiscountRate: "0%", ImportTaxImpTempRate: "",
	}
	id1, no1, err := repo.CreateMatchRun(context.Background(), "sid", 1, "CNY", "tester", []biz.BOMMatchResultItemDraft{item})
	if err != nil {
		t.Fatalf("create run1: %v", err)
	}
	id2, no2, err := repo.CreateMatchRun(context.Background(), "sid", 1, "CNY", "tester", []biz.BOMMatchResultItemDraft{item})
	if err != nil {
		t.Fatalf("create run2: %v", err)
	}
	if id1 == id2 || no1 != 1 || no2 != 2 {
		t.Fatalf("ids/no = %d/%d %d/%d", id1, no1, id2, no2)
	}
	_, items, err := repo.GetMatchRun(context.Background(), id1)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if len(items) != 1 || items[0].CodeTS != "8542399000" || items[0].ControlMark != "A" {
		t.Fatalf("unexpected item: %+v", items)
	}
}
```

- [ ] **Step 3: Run failing repo tests**

Run:

```powershell
& 'C:\Program Files\Go\bin\go.exe' test ./internal/data -run 'TestBOMLineGapRepo|TestBOMMatchRunRepo' -count=1
```

Expected: fails with undefined constructors/repos.

- [ ] **Step 4: 实现缺口 repo**

Create `internal/data/bom_line_gap_repo.go` with GORM-only persistence:

```go
package data

import (
	"context"
	"time"

	"caichip/internal/biz"

	"gorm.io/gorm/clause"
)

type BOMLineGapRepo struct{ data *Data }

func NewBOMLineGapRepo(d *Data) *BOMLineGapRepo { return &BOMLineGapRepo{data: d} }
func (r *BOMLineGapRepo) DBOk() bool { return r != nil && r.data != nil && r.data.db != nil }

func (r *BOMLineGapRepo) UpsertOpenGaps(ctx context.Context, gaps []biz.BOMLineGap) error {
	if len(gaps) == 0 {
		return nil
	}
	rows := make([]BomLineGap, 0, len(gaps))
	for _, g := range gaps {
		g.Status = biz.LineGapOpen
		active := g.ActiveKey()
		if active == "" {
			continue
		}
		detail := nullableString(g.ReasonDetail)
		rows = append(rows, BomLineGap{
			SessionID: g.SessionID, LineID: g.LineID, LineNo: g.LineNo, Mpn: g.Mpn,
			GapType: g.GapType, ReasonCode: g.ReasonCode, ReasonDetail: detail,
			ResolutionStatus: biz.LineGapOpen, ActiveKey: active,
		})
	}
	return r.data.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "active_key"}},
		DoUpdates: clause.AssignmentColumns([]string{"line_no", "mpn", "reason_code", "reason_detail", "updated_at"}),
	}).Create(&rows).Error
}

func (r *BOMLineGapRepo) ListLineGaps(ctx context.Context, sessionID string, statuses []string) ([]biz.BOMLineGap, error) {
	var rows []BomLineGap
	q := r.data.db.WithContext(ctx).Where("session_id = ?", sessionID)
	if len(statuses) > 0 {
		q = q.Where("resolution_status IN ?", statuses)
	}
	if err := q.Order("line_no ASC, id ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]biz.BOMLineGap, 0, len(rows))
	for _, row := range rows {
		out = append(out, mapDataGapToBiz(row))
	}
	return out, nil
}

func (r *BOMLineGapRepo) GetLineGap(ctx context.Context, gapID uint64) (*biz.BOMLineGap, error) {
	var row BomLineGap
	if err := r.data.db.WithContext(ctx).Where("id = ?", gapID).First(&row).Error; err != nil {
		return nil, err
	}
	gap := mapDataGapToBiz(row)
	return &gap, nil
}

func (r *BOMLineGapRepo) UpdateLineGapStatus(ctx context.Context, gapID uint64, fromStatus string, toStatus string, actor string, note string) error {
	now := time.Now()
	updates := map[string]any{
		"resolution_status": toStatus,
		"active_key":        "",
		"resolved_by":       nullableString(actor),
		"resolved_at":       &now,
		"resolution_note":   nullableString(note),
	}
	return r.data.db.WithContext(ctx).Model(&BomLineGap{}).
		Where("id = ? AND resolution_status = ?", gapID, fromStatus).
		Updates(updates).Error
}

func (r *BOMLineGapRepo) SelectLineGapSubstitute(ctx context.Context, gapID uint64, actor string, substituteMpn string, reason string) error {
	now := time.Now()
	return r.data.db.WithContext(ctx).Model(&BomLineGap{}).
		Where("id = ? AND resolution_status = ?", gapID, biz.LineGapOpen).
		Updates(map[string]any{
			"resolution_status": biz.LineGapSubstituteSelected,
			"active_key":        "",
			"resolved_by":       nullableString(actor),
			"resolved_at":       &now,
			"substitute_mpn":    nullableString(substituteMpn),
			"substitute_reason": nullableString(reason),
		}).Error
}
```

If `nullableString` does not exist in data package, add a small helper in this file.

- [ ] **Step 5: 实现 run repo**

Create `internal/data/bom_match_run_repo.go` using `db.Transaction`:

```go
package data

import (
	"context"
	"time"

	"caichip/internal/biz"

	"gorm.io/gorm"
)

type BOMMatchRunRepo struct{ data *Data }

func NewBOMMatchRunRepo(d *Data) *BOMMatchRunRepo { return &BOMMatchRunRepo{data: d} }
func (r *BOMMatchRunRepo) DBOk() bool { return r != nil && r.data != nil && r.data.db != nil }

func (r *BOMMatchRunRepo) CreateMatchRun(ctx context.Context, sessionID string, selectionRevision int, currency string, createdBy string, items []biz.BOMMatchResultItemDraft) (uint64, int, error) {
	var runID uint64
	var runNo int
	err := r.data.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var maxNo int
		if err := tx.Model(&BomMatchRun{}).Where("session_id = ?", sessionID).Select("COALESCE(MAX(run_no), 0)").Scan(&maxNo).Error; err != nil {
			return err
		}
		runNo = maxNo + 1
		total, matched, unresolved := biz.SummarizeMatchRunItems(items)
		now := time.Now()
		run := BomMatchRun{
			RunNo: runNo, SessionID: sessionID, SelectionRevision: selectionRevision,
			Status: biz.MatchRunSaved, Source: "manual_save", LineTotal: len(items),
			MatchedLineCount: matched, UnresolvedLineCount: unresolved,
			TotalAmount: total, Currency: currency, CreatedBy: nullableString(createdBy),
			SavedAt: &now,
		}
		if err := tx.Create(&run).Error; err != nil {
			return err
		}
		runID = run.ID
		rows := make([]BomMatchResultItem, 0, len(items))
		for _, it := range items {
			rows = append(rows, mapBizMatchItemDraftToData(run.ID, sessionID, it))
		}
		if len(rows) > 0 {
			if err := tx.Create(&rows).Error; err != nil {
				return err
			}
		}
		return nil
	})
	return runID, runNo, err
}
```

Implement `ListMatchRuns`, `GetMatchRun`, `SupersedePreviousRuns`, and mapping helpers in the same file.

- [ ] **Step 6: 注入 provider**

In `internal/data/provider.go`, add constructors to the provider set:

```go
NewBOMLineGapRepo,
NewBOMMatchRunRepo,
```

- [ ] **Step 7: Run data tests**

Run:

```powershell
& 'C:\Program Files\Go\bin\go.exe' test ./internal/data -run 'TestBOMLineGapRepo|TestBOMMatchRunRepo' -count=1
```

Expected: pass.

- [ ] **Step 8: Commit**

```powershell
git add -- 'internal/data/bom_line_gap_repo.go' 'internal/data/bom_line_gap_repo_test.go' 'internal/data/bom_match_run_repo.go' 'internal/data/bom_match_run_repo_test.go' 'internal/data/provider.go'
git commit -m "feat(bom): persist line gaps and match runs"
```

---

### Task 4: 扩展 proto 与 service API 骨架

**Files:**
- Modify: `api/bom/v1/bom.proto`
- Regenerate: `api/bom/v1/bom.pb.go`
- Regenerate: `api/bom/v1/bom_http.pb.go`
- Regenerate: `api/bom/v1/bom_grpc.pb.go`
- Modify: `internal/service/provider.go`
- Modify: `cmd/server/wire.go`
- Regenerate: `cmd/server/wire_gen.go`

- [ ] **Step 1: 扩展 proto message**

In `api/bom/v1/bom.proto`, add:

```proto
message BOMLineGap {
  string gap_id = 1;
  string session_id = 2;
  string line_id = 3;
  int32 line_no = 4;
  string mpn = 5;
  string gap_type = 6;
  string reason_code = 7;
  string reason_detail = 8;
  string resolution_status = 9;
  string substitute_mpn = 10;
  string substitute_reason = 11;
  string updated_at = 12;
}

message ListLineGapsRequest {
  string session_id = 1;
  repeated string statuses = 2;
}

message ListLineGapsReply {
  repeated BOMLineGap gaps = 1;
}

message ResolveLineGapManualQuoteRequest {
  string gap_id = 1;
  string model = 2;
  string manufacturer = 3;
  string package = 4;
  string stock = 5;
  string lead_time = 6;
  string price_tiers = 7;
  string hk_price = 8;
  string mainland_price = 9;
  string note = 10;
}

message ResolveLineGapManualQuoteReply {
  bool accepted = 1;
}

message SelectLineGapSubstituteRequest {
  string gap_id = 1;
  string substitute_mpn = 2;
  string reason = 3;
}

message SelectLineGapSubstituteReply {
  bool accepted = 1;
}

message SaveMatchRunRequest {
  string session_id = 1;
  string strategy = 2;
}

message SaveMatchRunReply {
  string run_id = 1;
  int32 run_no = 2;
}

message MatchRunListItem {
  string run_id = 1;
  int32 run_no = 2;
  string session_id = 3;
  string status = 4;
  int32 line_total = 5;
  int32 matched_line_count = 6;
  int32 unresolved_line_count = 7;
  double total_amount = 8;
  string currency = 9;
  string created_at = 10;
  string saved_at = 11;
}

message ListMatchRunsRequest {
  string session_id = 1;
}

message ListMatchRunsReply {
  repeated MatchRunListItem runs = 1;
}

message MatchRunResultItem {
  string line_id = 1;
  int32 line_no = 2;
  string source_type = 3;
  string match_status = 4;
  string gap_id = 5;
  string quote_item_id = 6;
  string platform_id = 7;
  string demand_mpn = 8;
  string matched_mpn = 9;
  string matched_mfr = 10;
  string matched_package = 11;
  int64 stock = 12;
  string lead_time = 13;
  double unit_price = 14;
  double subtotal = 15;
  string currency = 16;
  string substitute_mpn = 17;
  string substitute_reason = 18;
  string code_ts = 19;
  string control_mark = 20;
  string import_tax_imp_ordinary_rate = 21;
  string import_tax_imp_discount_rate = 22;
  string import_tax_imp_temp_rate = 23;
}

message GetMatchRunRequest {
  string run_id = 1;
}

message GetMatchRunReply {
  MatchRunListItem run = 1;
  repeated MatchRunResultItem items = 2;
}
```

- [ ] **Step 2: 增加 service rpc**

In the BOM service definition:

```proto
rpc ListLineGaps (ListLineGapsRequest) returns (ListLineGapsReply) {
  option (google.api.http) = { get: "/api/bom/sessions/{session_id}/gaps" };
}
rpc ResolveLineGapManualQuote (ResolveLineGapManualQuoteRequest) returns (ResolveLineGapManualQuoteReply) {
  option (google.api.http) = { post: "/api/bom/gaps/{gap_id}/manual-quote" body: "*" };
}
rpc SelectLineGapSubstitute (SelectLineGapSubstituteRequest) returns (SelectLineGapSubstituteReply) {
  option (google.api.http) = { post: "/api/bom/gaps/{gap_id}/substitute" body: "*" };
}
rpc SaveMatchRun (SaveMatchRunRequest) returns (SaveMatchRunReply) {
  option (google.api.http) = { post: "/api/bom/sessions/{session_id}/match-runs" body: "*" };
}
rpc ListMatchRuns (ListMatchRunsRequest) returns (ListMatchRunsReply) {
  option (google.api.http) = { get: "/api/bom/sessions/{session_id}/match-runs" };
}
rpc GetMatchRun (GetMatchRunRequest) returns (GetMatchRunReply) {
  option (google.api.http) = { get: "/api/bom/match-runs/{run_id}" };
}
```

- [ ] **Step 3: Regenerate protobuf**

Run:

```powershell
protoc --proto_path=./api --proto_path=./third_party --go_out=paths=source_relative:./api --go-http_out=paths=source_relative:./api --go-grpc_out=paths=source_relative:./api api/bom/v1/bom.proto
```

Expected: exit code 0 and generated files updated.

- [ ] **Step 4: Wire新增 repo**

Update `internal/service/provider.go` constructor parameters for `NewBomService` only after Task 5 service implementation defines fields. Update `cmd/server/wire.go`, then run:

```powershell
wire ./cmd/server
```

Expected: `cmd/server/wire_gen.go` updated.

- [ ] **Step 5: Compile proto users**

Run:

```powershell
& 'C:\Program Files\Go\bin\go.exe' test ./api/bom/v1 ./internal/service -run '^$' -count=1
```

Expected: compile errors only if service methods are missing; Task 5 will add them. If generated interface requires methods immediately, add stub methods returning `BOM_LEGACY` and replace them in Task 5.

- [ ] **Step 6: Commit**

```powershell
git add -- 'api/bom/v1/bom.proto' 'api/bom/v1/bom.pb.go' 'api/bom/v1/bom_http.pb.go' 'api/bom/v1/bom_grpc.pb.go' 'internal/service/provider.go' 'cmd/server/wire.go' 'cmd/server/wire_gen.go'
git commit -m "feat(bom): add gap and match run APIs"
```

---

### Task 5: 实现缺口同步、人工补录和替代料选择

**Files:**
- Create: `internal/service/bom_gap_service.go`
- Create: `internal/service/bom_gap_service_test.go`
- Modify: `internal/service/bom_service.go`
- Modify: `internal/service/bom_service_test_helpers_test.go`
- Modify: `internal/data/bom_search_task_repo.go`

- [ ] **Step 1: 扩展 service test stubs**

In `internal/service/bom_service_test_helpers_test.go`, add fields to stubs:

```go
type bomLineGapRepoStub struct {
	gaps []biz.BOMLineGap
	updated []uint64
	substitutes []string
}

func (s *bomLineGapRepoStub) DBOk() bool { return true }
func (s *bomLineGapRepoStub) UpsertOpenGaps(ctx context.Context, gaps []biz.BOMLineGap) error {
	s.gaps = append(s.gaps, gaps...)
	return nil
}
func (s *bomLineGapRepoStub) ListLineGaps(ctx context.Context, sessionID string, statuses []string) ([]biz.BOMLineGap, error) {
	return append([]biz.BOMLineGap(nil), s.gaps...), nil
}
func (s *bomLineGapRepoStub) GetLineGap(ctx context.Context, gapID uint64) (*biz.BOMLineGap, error) {
	for _, gap := range s.gaps {
		if gap.ID == gapID {
			cp := gap
			return &cp, nil
		}
	}
	return nil, errors.New("gap not found")
}
func (s *bomLineGapRepoStub) UpdateLineGapStatus(ctx context.Context, gapID uint64, fromStatus string, toStatus string, actor string, note string) error {
	s.updated = append(s.updated, gapID)
	return nil
}
func (s *bomLineGapRepoStub) SelectLineGapSubstitute(ctx context.Context, gapID uint64, actor string, substituteMpn string, reason string) error {
	s.substitutes = append(s.substitutes, substituteMpn)
	return nil
}
```

- [ ] **Step 2: 编写 service 测试**

Create `internal/service/bom_gap_service_test.go`:

```go
package service

import (
	"context"
	"testing"
	"time"

	v1 "caichip/api/bom/v1"
	"caichip/internal/biz"
	"caichip/internal/data"
)

func TestListLineGapsSyncsAvailabilityGaps(t *testing.T) {
	view := &biz.BOMSessionView{SessionID: "sid", BizDate: time.Now(), PlatformIDs: []string{"icgoo"}}
	session := &bomSessionRepoStub{view: view, fullLines: []data.BomSessionLine{{ID: 1, LineNo: 1, Mpn: "NO-DATA"}}}
	search := &bomSearchTaskRepoStub{
		tasks: []biz.TaskReadinessSnapshot{{MpnNorm: "NO-DATA", PlatformID: "icgoo", State: "no_result"}},
		cacheMap: map[string]*biz.QuoteCacheSnapshot{
			testQuoteCacheKey("NO-DATA", "icgoo"): {Outcome: "no_mpn_match"},
		},
	}
	gaps := &bomLineGapRepoStub{}
	svc := NewBomService(session, search, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	svc.gaps = gaps

	_, err := svc.ListLineGaps(context.Background(), &v1.ListLineGapsRequest{SessionId: "sid", Statuses: []string{biz.LineGapOpen}})
	if err != nil {
		t.Fatalf("ListLineGaps: %v", err)
	}
	if len(gaps.gaps) != 1 || gaps.gaps[0].GapType != biz.LineGapNoData {
		t.Fatalf("synced gaps=%+v", gaps.gaps)
	}
}
```

- [ ] **Step 3: Run failing service tests**

Run:

```powershell
& 'C:\Program Files\Go\bin\go.exe' test ./internal/service -run 'TestListLineGapsSyncsAvailabilityGaps' -count=1
```

Expected: fails until service fields and methods exist.

- [ ] **Step 4: 扩展 BomService 依赖**

In `internal/service/bom_service.go`, add fields:

```go
gaps      biz.BOMLineGapRepo
matchRuns biz.BOMMatchRunRepo
```

Extend `NewBomService` signature with these dependencies after existing repos, and set fields.

- [ ] **Step 5: 实现缺口同步 helper**

Create `internal/service/bom_gap_service.go`:

```go
package service

import (
	"context"
	"strconv"

	v1 "caichip/api/bom/v1"
	"caichip/internal/biz"
)

func (s *BomService) syncLineGaps(ctx context.Context, view *biz.BOMSessionView) error {
	if s.gaps == nil || !s.gaps.DBOk() {
		return nil
	}
	lines, err := s.dataListLines(ctx, view.SessionID)
	if err != nil {
		return err
	}
	availability, _, err := s.computeLineAvailability(ctx, view, lines, view.PlatformIDs)
	if err != nil {
		return err
	}
	byLineID := make(map[int]int64, len(lines))
	for _, line := range lines {
		byLineID[line.LineNo] = line.ID
	}
	var gaps []biz.BOMLineGap
	for _, a := range availability {
		gt := biz.AvailabilityStatusToGapType(a.Status)
		if gt == "" {
			continue
		}
		gaps = append(gaps, biz.BOMLineGap{
			SessionID: view.SessionID, LineID: byLineID[a.LineNo], LineNo: a.LineNo,
			Mpn: a.MpnNorm, GapType: gt, ReasonCode: a.ReasonCode,
			ReasonDetail: a.Reason, Status: biz.LineGapOpen,
		})
	}
	return s.gaps.UpsertOpenGaps(ctx, gaps)
}
```

- [ ] **Step 6: 实现缺口 API**

In `internal/service/bom_gap_service.go`, add:

```go
func (s *BomService) ListLineGaps(ctx context.Context, req *v1.ListLineGapsRequest) (*v1.ListLineGapsReply, error) {
	view, err := s.session.GetSession(ctx, req.GetSessionId())
	if err != nil {
		return nil, err
	}
	if err := s.syncLineGaps(ctx, view); err != nil {
		s.log.Warnf("sync line gaps: %v", err)
	}
	gaps, err := s.gaps.ListLineGaps(ctx, req.GetSessionId(), req.GetStatuses())
	if err != nil {
		return nil, err
	}
	out := make([]*v1.BOMLineGap, 0, len(gaps))
	for _, g := range gaps {
		out = append(out, &v1.BOMLineGap{
			GapId: strconv.FormatUint(g.ID, 10), SessionId: g.SessionID,
			LineId: strconv.FormatInt(g.LineID, 10), LineNo: int32(g.LineNo),
			Mpn: g.Mpn, GapType: g.GapType, ReasonCode: g.ReasonCode,
			ReasonDetail: g.ReasonDetail, ResolutionStatus: g.Status,
			SubstituteMpn: g.SubstituteMpn, SubstituteReason: g.SubstituteReason,
		})
	}
	return &v1.ListLineGapsReply{Gaps: out}, nil
}
```

- [ ] **Step 7: 实现人工补录报价入口**

Add `ResolveLineGapManualQuote`:

```go
func (s *BomService) ResolveLineGapManualQuote(ctx context.Context, req *v1.ResolveLineGapManualQuoteRequest) (*v1.ResolveLineGapManualQuoteReply, error) {
	gapID, err := strconv.ParseUint(req.GetGapId(), 10, 64)
	if err != nil {
		return nil, kerrors.BadRequest("BAD_GAP_ID", "invalid gap_id")
	}
	if strings.TrimSpace(req.GetModel()) == "" {
		return nil, kerrors.BadRequest("BAD_MANUAL_QUOTE", "model required")
	}
	// Use a new search repo method to write source_type=manual quote cache/item for this gap line.
	if err := s.search.UpsertManualQuote(ctx, gapID, biz.AgentQuoteRow{
		Model: req.GetModel(), Manufacturer: req.GetManufacturer(), Package: req.GetPackage(),
		Stock: req.GetStock(), LeadTime: req.GetLeadTime(), PriceTiers: req.GetPriceTiers(),
		HKPrice: req.GetHkPrice(), MainlandPrice: req.GetMainlandPrice(),
	}); err != nil {
		return nil, err
	}
	if err := s.gaps.UpdateLineGapStatus(ctx, gapID, biz.LineGapOpen, biz.LineGapManualQuoteAdded, "", req.GetNote()); err != nil {
		return nil, err
	}
	return &v1.ResolveLineGapManualQuoteReply{Accepted: true}, nil
}
```

Implement `UpsertManualQuote` in `BOMSearchTaskRepo` and `internal/data/bom_search_task_repo.go` using GORM. It must write `source_type=manual` and keep the quotes readable by existing `LoadQuoteCachesForKeys`.

- [ ] **Step 8: 实现替代料入口**

Add `SelectLineGapSubstitute`:

```go
func (s *BomService) SelectLineGapSubstitute(ctx context.Context, req *v1.SelectLineGapSubstituteRequest) (*v1.SelectLineGapSubstituteReply, error) {
	gapID, err := strconv.ParseUint(req.GetGapId(), 10, 64)
	if err != nil {
		return nil, kerrors.BadRequest("BAD_GAP_ID", "invalid gap_id")
	}
	sub := strings.TrimSpace(req.GetSubstituteMpn())
	if sub == "" {
		return nil, kerrors.BadRequest("BAD_SUBSTITUTE", "substitute_mpn required")
	}
	gap, err := s.gaps.GetLineGap(ctx, gapID)
	if err != nil {
		return nil, kerrors.NotFound("GAP_NOT_FOUND", "gap not found")
	}
	if gap.Status != biz.LineGapOpen {
		return nil, kerrors.Conflict("GAP_NOT_OPEN", "gap is not open")
	}
	view, err := s.session.GetSession(ctx, gap.SessionID)
	if err != nil {
		return nil, err
	}
	var pairs []biz.MpnPlatformPair
	mn := biz.NormalizeMPNForBOMSearch(sub)
	for _, p := range view.PlatformIDs {
		pairs = append(pairs, biz.MpnPlatformPair{MpnNorm: mn, PlatformID: biz.NormalizePlatformID(p)})
	}
	if err := s.search.UpsertPendingTasks(ctx, gap.SessionID, view.BizDate, view.SelectionRevision, pairs); err != nil {
		return nil, err
	}
	if err := s.gaps.SelectLineGapSubstitute(ctx, gapID, "", sub, req.GetReason()); err != nil {
		return nil, err
	}
	s.tryMergeDispatchSession(ctx, gap.SessionID)
	return &v1.SelectLineGapSubstituteReply{Accepted: true}, nil
}
```

- [ ] **Step 9: Run service tests**

Run:

```powershell
& 'C:\Program Files\Go\bin\go.exe' test ./internal/service -run 'TestListLineGaps|TestResolveLineGapManualQuote|TestSelectLineGapSubstitute' -count=1
```

Expected: pass after adding the matching tests for manual quote and substitute.

- [ ] **Step 10: Commit**

```powershell
git add -- 'internal/service/bom_gap_service.go' 'internal/service/bom_gap_service_test.go' 'internal/service/bom_service.go' 'internal/service/bom_service_test_helpers_test.go' 'internal/data/bom_search_task_repo.go' 'internal/biz/repo.go'
git commit -m "feat(bom): close line gaps with manual quotes and substitutes"
```

---

### Task 6: 实现保存配单方案与按 run 查询导出

**Files:**
- Create: `internal/service/bom_match_run_service.go`
- Create: `internal/service/bom_match_run_service_test.go`
- Modify: `internal/service/bom_service.go`

- [ ] **Step 1: 编写保存 run 测试**

Create `internal/service/bom_match_run_service_test.go`:

```go
package service

import (
	"context"
	"testing"
	"time"

	v1 "caichip/api/bom/v1"
	"caichip/internal/biz"
	"caichip/internal/data"
)

func TestSaveMatchRunPersistsExactAndUnresolvedRows(t *testing.T) {
	view := &biz.BOMSessionView{SessionID: "sid", BizDate: time.Now(), PlatformIDs: []string{"icgoo"}, SelectionRevision: 1}
	session := &bomSessionRepoStub{view: view, fullLines: []data.BomSessionLine{
		{ID: 1, LineNo: 1, Mpn: "OK"},
		{ID: 2, LineNo: 2, Mpn: "NO-DATA"},
	}}
	runs := &bomMatchRunRepoStub{}
	gaps := &bomLineGapRepoStub{gaps: []biz.BOMLineGap{{ID: 99, SessionID: "sid", LineID: 2, LineNo: 2, Mpn: "NO-DATA", GapType: biz.LineGapNoData, Status: biz.LineGapOpen}}}
	search := &bomSearchTaskRepoStub{cacheMap: map[string]*biz.QuoteCacheSnapshot{}}
	svc := NewBomService(session, search, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	svc.gaps = gaps
	svc.matchRuns = runs

	resp, err := svc.SaveMatchRun(context.Background(), &v1.SaveMatchRunRequest{SessionId: "sid"})
	if err != nil {
		t.Fatalf("SaveMatchRun: %v", err)
	}
	if resp.GetRunNo() != 1 || len(runs.items) != 2 {
		t.Fatalf("run resp=%+v items=%+v", resp, runs.items)
	}
	if runs.items[1].SourceType != biz.MatchResultUnresolved || runs.items[1].GapID != 99 {
		t.Fatalf("unresolved item=%+v", runs.items[1])
	}
}
```

- [ ] **Step 2: Run failing test**

Run:

```powershell
& 'C:\Program Files\Go\bin\go.exe' test ./internal/service -run TestSaveMatchRunPersistsExactAndUnresolvedRows -count=1
```

Expected: fails until `SaveMatchRun` exists and stubs compile.

- [ ] **Step 3: 实现保存 run**

Create `internal/service/bom_match_run_service.go`:

```go
package service

import (
	"context"
	"encoding/json"
	"strconv"

	v1 "caichip/api/bom/v1"
	"caichip/internal/biz"
)

func (s *BomService) SaveMatchRun(ctx context.Context, req *v1.SaveMatchRunRequest) (*v1.SaveMatchRunReply, error) {
	view, lines, plats, err := s.loadSessionLinesAndPlatforms(ctx, req.GetSessionId(), nil)
	if err != nil {
		return nil, err
	}
	if err := s.matchReadinessError(ctx, req.GetSessionId(), view, lines); err != nil {
		return nil, err
	}
	items, _, err := s.computeMatchItems(ctx, view, lines, plats)
	if err != nil {
		return nil, err
	}
	openGaps, err := s.gaps.ListLineGaps(ctx, req.GetSessionId(), []string{biz.LineGapOpen, biz.LineGapManualQuoteAdded, biz.LineGapSubstituteSelected})
	if err != nil {
		return nil, err
	}
	gapByLine := make(map[int64]biz.BOMLineGap)
	for _, g := range openGaps {
		gapByLine[g.LineID] = g
	}
	drafts := make([]biz.BOMMatchResultItemDraft, 0, len(lines))
	for i, line := range lines {
		mi := items[i]
		raw, _ := json.Marshal(mi)
		source := biz.MatchResultSourceFromMatchStatus(mi.GetMatchStatus(), mi.GetPlatform() == "manual", false)
		gap := gapByLine[line.ID]
		if mi.GetMatchStatus() == "no_match" {
			source = biz.MatchResultUnresolved
		}
		drafts = append(drafts, matchItemToRunDraft(line, mi, source, gap, raw))
	}
	runID, runNo, err := s.matchRuns.CreateMatchRun(ctx, req.GetSessionId(), view.SelectionRevision, s.bomMatchBaseCCY(), "", drafts)
	if err != nil {
		return nil, err
	}
	_ = s.matchRuns.SupersedePreviousRuns(ctx, req.GetSessionId(), runID)
	return &v1.SaveMatchRunReply{RunId: strconv.FormatUint(runID, 10), RunNo: int32(runNo)}, nil
}
```

Implement `matchItemToRunDraft` in the same file and copy these fields from `MatchItem`: demand fields, matched fields, price fields, `CodeTs`, `ControlMark`, `ImportTaxImpOrdinaryRate`, `ImportTaxImpDiscountRate`, `ImportTaxImpTempRate`.

- [ ] **Step 4: 实现 run 查询**

Add `ListMatchRuns` and `GetMatchRun`:

```go
func (s *BomService) ListMatchRuns(ctx context.Context, req *v1.ListMatchRunsRequest) (*v1.ListMatchRunsReply, error) {
	runs, err := s.matchRuns.ListMatchRuns(ctx, req.GetSessionId())
	if err != nil {
		return nil, err
	}
	out := make([]*v1.MatchRunListItem, 0, len(runs))
	for _, r := range runs {
		out = append(out, matchRunViewToProto(r))
	}
	return &v1.ListMatchRunsReply{Runs: out}, nil
}

func (s *BomService) GetMatchRun(ctx context.Context, req *v1.GetMatchRunRequest) (*v1.GetMatchRunReply, error) {
	runID, err := strconv.ParseUint(req.GetRunId(), 10, 64)
	if err != nil {
		return nil, kerrors.BadRequest("BAD_RUN_ID", "invalid run_id")
	}
	run, items, err := s.matchRuns.GetMatchRun(ctx, runID)
	if err != nil {
		return nil, err
	}
	out := make([]*v1.MatchRunResultItem, 0, len(items))
	for _, item := range items {
		out = append(out, matchRunItemToProto(item))
	}
	return &v1.GetMatchRunReply{Run: matchRunViewToProto(*run), Items: out}, nil
}
```

- [ ] **Step 5: 扩展导出按 run 读取**

Extend `ExportSessionRequest` in proto with optional `run_id`. In `ExportSession`, if `run_id` is provided:

```go
runID, err := strconv.ParseUint(req.GetRunId(), 10, 64)
if err != nil {
	return nil, kerrors.BadRequest("BAD_RUN_ID", "invalid run_id")
}
_, items, err := s.matchRuns.GetMatchRun(ctx, runID)
if err != nil {
	return nil, err
}
return s.exportMatchRunItems(items, req.GetFormat())
```

`exportMatchRunItems` should include BOM demand fields, selected quote fields, source type, match status, subtotal, and the five customs/tax fields.

- [ ] **Step 6: Run service tests**

Run:

```powershell
& 'C:\Program Files\Go\bin\go.exe' test ./internal/service -run 'TestSaveMatchRun|TestListMatchRuns|TestGetMatchRun|TestExportSession' -count=1
```

Expected: pass after adding focused tests for list/get/export.

- [ ] **Step 7: Commit**

```powershell
git add -- 'internal/service/bom_match_run_service.go' 'internal/service/bom_match_run_service_test.go' 'internal/service/bom_service.go' 'api/bom/v1/bom.proto' 'api/bom/v1/bom.pb.go' 'api/bom/v1/bom_http.pb.go' 'api/bom/v1/bom_grpc.pb.go'
git commit -m "feat(bom): save and export match run snapshots"
```

---

### Task 7: 前端接入缺口处理与方案版本

**Files:**
- Modify: `web/src/api/types.ts`
- Modify: `web/src/api/bomSession.ts`
- Modify: `web/src/pages/SourcingSessionPage.tsx`
- Modify: `web/src/pages/SourcingSessionPage.test.tsx`

- [ ] **Step 1: 扩展 TypeScript 类型**

In `web/src/api/types.ts`, add:

```ts
export interface BOMLineGap {
  gap_id: string
  session_id: string
  line_id: string
  line_no: number
  mpn: string
  gap_type: string
  reason_code: string
  reason_detail: string
  resolution_status: string
  substitute_mpn: string
  substitute_reason: string
  updated_at: string
}

export interface MatchRunListItem {
  run_id: string
  run_no: number
  session_id: string
  status: string
  line_total: number
  matched_line_count: number
  unresolved_line_count: number
  total_amount: number
  currency: string
  created_at: string
  saved_at: string
}
```

- [ ] **Step 2: 增加 API client**

In `web/src/api/bomSession.ts`, add methods:

```ts
export async function listLineGaps(sessionId: string, statuses: string[] = []): Promise<{ gaps: BOMLineGap[] }> {
  const qs = statuses.map((s) => `statuses=${encodeURIComponent(s)}`).join('&')
  const json = await getJson(`/api/bom/sessions/${encodeURIComponent(sessionId)}/gaps${qs ? `?${qs}` : ''}`)
  return { gaps: arr(json.gaps).map(parseLineGap) }
}

export async function saveMatchRun(sessionId: string): Promise<{ run_id: string; run_no: number }> {
  const json = await postJson(`/api/bom/sessions/${encodeURIComponent(sessionId)}/match-runs`, {})
  return { run_id: str(json.run_id ?? json.runId), run_no: num(json.run_no ?? json.runNo, 0) }
}

export async function listMatchRuns(sessionId: string): Promise<{ runs: MatchRunListItem[] }> {
  const json = await getJson(`/api/bom/sessions/${encodeURIComponent(sessionId)}/match-runs`)
  return { runs: arr(json.runs).map(parseMatchRun) }
}
```

Add `resolveLineGapManualQuote` and `selectLineGapSubstitute` with `postJson`.

- [ ] **Step 3: 编写前端测试**

In `web/src/pages/SourcingSessionPage.test.tsx`, add:

```tsx
it('shows open gaps and saves a match run', async () => {
  listLineGaps.mockResolvedValue({
    gaps: [{ gap_id: '99', session_id: 's1', line_id: '2', line_no: 2, mpn: 'NO-DATA', gap_type: 'NO_DATA', reason_code: 'NO_DATA', reason_detail: 'all selected platforms returned no data', resolution_status: 'open', substitute_mpn: '', substitute_reason: '', updated_at: '' }],
  })
  listMatchRuns.mockResolvedValue({ runs: [] })
  saveMatchRun.mockResolvedValue({ run_id: '7', run_no: 1 })

  render(<SourcingSessionPage sessionId="s1" onEnterMatch={vi.fn()} />)
  await act(async () => { await flushAsyncWork() })

  expect(screen.getByText('NO-DATA')).toBeInTheDocument()
  fireEvent.click(screen.getByRole('button', { name: '保存配单方案' }))
  await waitFor(() => expect(saveMatchRun).toHaveBeenCalledWith('s1'))
  expect(await screen.findByText(/方案 V1/)).toBeInTheDocument()
})
```

- [ ] **Step 4: Run failing frontend test**

Run:

```powershell
& 'D:\Program Files\nodejs\npm.cmd' test -- src/pages/SourcingSessionPage.test.tsx
```

Expected: fails until UI and mocks are updated.

- [ ] **Step 5: 实现 UI**

In `SourcingSessionPage.tsx`:

- Load gaps with `listLineGaps(sessionId, ['open', 'manual_quote_added', 'substitute_selected'])`.
- Load runs with `listMatchRuns(sessionId)`.
- Add compact gap panel under readiness/incomplete warning.
- Add row actions:
  - `人工补录` opens a small form for model/manufacturer/package/stock/price.
  - `替代料` opens a small form for substitute MPN and reason.
- Add `保存配单方案` button near match/export actions.
- After save, refresh run list and show `方案 V{run_no}`.

Use existing page styling conventions. Keep text Chinese and avoid duplicating backend business rules.

- [ ] **Step 6: Run frontend test**

Run:

```powershell
& 'D:\Program Files\nodejs\npm.cmd' test -- src/pages/SourcingSessionPage.test.tsx
```

Expected: pass.

- [ ] **Step 7: Commit**

```powershell
git add -- 'web/src/api/types.ts' 'web/src/api/bomSession.ts' 'web/src/pages/SourcingSessionPage.tsx' 'web/src/pages/SourcingSessionPage.test.tsx'
git commit -m "feat(web): manage bom gaps and match runs"
```

---

### Task 8: 集成验证与收尾

**Files:**
- Modify only if verification exposes defects.

- [ ] **Step 1: Run focused backend tests**

Run:

```powershell
& 'C:\Program Files\Go\bin\go.exe' test ./internal/biz ./internal/data ./internal/service -run 'TestLineGap|TestBOMLineGapRepo|TestBOMMatchRunRepo|TestListLineGaps|TestSaveMatchRun|TestExportSession' -count=1
```

Expected: pass.

- [ ] **Step 2: Run broader backend tests**

Run:

```powershell
& 'C:\Program Files\Go\bin\go.exe' test ./internal/biz/... ./internal/data/... ./internal/service/... ./internal/server/... -count=1
```

Expected: pass. If unrelated existing failures appear, record package and error summary before deciding whether to fix.

- [ ] **Step 3: Run frontend tests**

Run:

```powershell
& 'D:\Program Files\nodejs\npm.cmd' test -- src/api/bomSession.test.ts src/pages/SourcingSessionPage.test.tsx
```

Expected: pass.

- [ ] **Step 4: Run frontend build**

Run:

```powershell
& 'D:\Program Files\nodejs\npm.cmd' run build
```

Expected: pass.

- [ ] **Step 5: Inspect file length rule**

Run:

```powershell
Get-ChildItem -Recurse -File -Include *.go -Path internal | Where-Object { (Get-Content -LiteralPath $_.FullName).Count -gt 300 } | Select-Object FullName
```

Expected: no newly created or modified non-generated Go file exceeds 300 lines. If a modified existing file already exceeds 300 lines, note it and keep new logic split into focused files.

- [ ] **Step 6: Commit verification fixes**

If any verification fix was needed:

```powershell
git status --short
git add -- docs/schema/migrations/20260425_bom_gap_match_run_phase2.sql internal/biz internal/data internal/service api/bom/v1 web/src
git commit -m "fix(bom): stabilize gap match run phase2"
```

If no fix was needed, do not create an empty commit.

---

## 自检

- Spec 覆盖：本计划覆盖缺口落表、人工补录报价、替代料选择、版本化配单快照、按 run 导出。
- 用户确认点：预览不落快照，显式保存创建 `bom_match_run`；人工补录进入统一报价池；替代料触发采集；`t_bom_match_result_item` 只结构化保存五个海关/税率字段。
- 分层：业务规则在 `internal/biz`，GORM 持久化在 `internal/data`，API 编排在 `internal/service`。
- 测试：每个任务先写失败测试，再实现，再运行聚焦测试。
- 自检扫描：计划中没有未定义范围的任务；可选接口已明确为可选，不阻塞核心闭环。
