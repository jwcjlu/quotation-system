# BOM 行数据可用性实施计划

> **给 agentic workers：** 必须使用子技能：执行本计划时使用 `superpowers:subagent-driven-development`（推荐）或 `superpowers:executing-plans`。所有步骤使用 checkbox（`- [ ]`）语法跟踪进度。

**目标：** 增加 BOM 行级数据可用性能力，让 `lenient` 会话可以生成“不完整配单/导出”，让 `strict` 会话在存在缺口时阻断并给出明确原因。

**架构：** 业务判定放在 `internal/biz`，以纯函数汇总采集任务、报价缓存和配单筛选事实。`internal/service` 负责从现有 repo 收集事实、调用匹配能力并组装 API 返回。前端只做 code 到文案的映射，不复制后端判定逻辑。

**技术栈：** Go + Kratos + GORM、protobuf 生成的 Go/HTTP 绑定、React + TypeScript + Vitest、`excelize` 导出 Excel。

---

## 文件结构

- 新建 `internal/biz/bom_line_availability.go`：定义 availability 常量、平台事实模型、行级汇总模型和纯分类器。
- 新建 `internal/biz/bom_line_availability_test.go`：覆盖每一种 availability 状态和汇总统计。
- 修改 `internal/biz/bom_readiness.go`：保留现有 `ReadinessFromTasks`，必要时增加 availability 汇总辅助函数。
- 修改 `internal/biz/bom_session_ready.go`：任务终态后继续设置 `data_ready` 或 `blocked`，但不把报价缓存业务判断塞进 data 层。
- 修改 `internal/service/bom_availability.go`：新增 service helper，加载任务、报价缓存，运行单行报价筛选并返回 availability 汇总。
- 修改 `internal/service/bom_service.go`：把 availability 接入 `GetReadiness`、`GetBOMLines`、`matchReadinessError` 和 `ExportSession`。
- 修改 `internal/service/bom_match_parallel.go`：复用同一套行级匹配能力，避免配单结果和 availability 判断漂移。
- 修改 `api/bom/v1/bom.proto`：增加 readiness 统计字段和 BOM 行 availability 字段。
- 重新生成 `api/bom/v1/bom.pb.go`、`api/bom/v1/bom_http.pb.go`、`api/bom/v1/bom_grpc.pb.go`。
- 修改 `web/src/api/types.ts`：增加 TypeScript 字段。
- 修改 `web/src/api/bomSession.ts`：解析 snake_case 和 camelCase 的 availability 字段。
- 修改 `web/src/pages/SourcingSessionPage.tsx`：展示数据状态列和不完整配单提示。
- 修改 `web/src/pages/SourcingSessionPage.test.tsx`：覆盖提示和行状态渲染。

---

### 任务 1：增加纯 availability 分类器

**文件：**
- 新建：`internal/biz/bom_line_availability.go`
- 新建：`internal/biz/bom_line_availability_test.go`

- [ ] **步骤 1：编写失败测试**

新建 `internal/biz/bom_line_availability_test.go`：

```go
package biz

import "testing"

func TestClassifyLineAvailability_ReadyWhenAnyPlatformHasUsableQuote(t *testing.T) {
	got := ClassifyLineAvailability(LineAvailabilityInput{
		LineNo:  1,
		MpnNorm: "LM358",
		Platforms: []PlatformAvailabilityFact{
			{PlatformID: "icgoo", TaskState: "no_result", NoData: true},
			{PlatformID: "szlcsc", TaskState: "succeeded", HasRawQuote: true, HasUsableQuote: true},
		},
	})
	if got.Status != LineAvailabilityReady {
		t.Fatalf("status=%q, want %q", got.Status, LineAvailabilityReady)
	}
	if !got.HasUsableQuote || got.UsableQuotePlatformCount != 1 || got.RawQuotePlatformCount != 1 {
		t.Fatalf("unexpected quote counters: %+v", got)
	}
}

func TestClassifyLineAvailability_NoDataWhenAllPlatformsExplicitlyNoData(t *testing.T) {
	got := ClassifyLineAvailability(LineAvailabilityInput{
		LineNo:  2,
		MpnNorm: "NO-MPN",
		Platforms: []PlatformAvailabilityFact{
			{PlatformID: "icgoo", TaskState: "no_result", NoData: true, ReasonCode: "NO_MPN"},
			{PlatformID: "szlcsc", TaskState: "no_result", NoData: true, ReasonCode: "NO_QUOTES"},
		},
	})
	if got.Status != LineAvailabilityNoData {
		t.Fatalf("status=%q, want %q", got.Status, LineAvailabilityNoData)
	}
	if got.ReasonCode != "NO_DATA" {
		t.Fatalf("reason=%q, want NO_DATA", got.ReasonCode)
	}
}

func TestClassifyLineAvailability_CollectionUnavailableWhenTerminalFailurePresent(t *testing.T) {
	got := ClassifyLineAvailability(LineAvailabilityInput{
		LineNo:  3,
		MpnNorm: "FETCH-FAIL",
		Platforms: []PlatformAvailabilityFact{
			{PlatformID: "icgoo", TaskState: "failed_terminal", CollectionUnavailable: true, ReasonCode: "FETCH_FAILED"},
			{PlatformID: "szlcsc", TaskState: "no_result", NoData: true, ReasonCode: "NO_MPN"},
		},
	})
	if got.Status != LineAvailabilityCollectionUnavailable {
		t.Fatalf("status=%q, want %q", got.Status, LineAvailabilityCollectionUnavailable)
	}
	if got.ReasonCode != "COLLECTION_UNAVAILABLE" {
		t.Fatalf("reason=%q, want COLLECTION_UNAVAILABLE", got.ReasonCode)
	}
}

func TestClassifyLineAvailability_NoMatchAfterFilterWhenRawQuoteExistsWithoutUsableQuote(t *testing.T) {
	got := ClassifyLineAvailability(LineAvailabilityInput{
		LineNo:  4,
		MpnNorm: "MFR-FILTER",
		Platforms: []PlatformAvailabilityFact{
			{PlatformID: "icgoo", TaskState: "succeeded", HasRawQuote: true, HasUsableQuote: false, ReasonCode: "FILTERED_BY_MFR"},
			{PlatformID: "szlcsc", TaskState: "no_result", NoData: true, ReasonCode: "NO_MPN"},
		},
	})
	if got.Status != LineAvailabilityNoMatchAfterFilter {
		t.Fatalf("status=%q, want %q", got.Status, LineAvailabilityNoMatchAfterFilter)
	}
	if got.RawQuotePlatformCount != 1 || got.UsableQuotePlatformCount != 0 {
		t.Fatalf("unexpected counters: %+v", got)
	}
}

func TestClassifyLineAvailability_CollectingWhenAnyPlatformNonTerminal(t *testing.T) {
	got := ClassifyLineAvailability(LineAvailabilityInput{
		LineNo:  5,
		MpnNorm: "PENDING",
		Platforms: []PlatformAvailabilityFact{
			{PlatformID: "icgoo", TaskState: "pending"},
			{PlatformID: "szlcsc", TaskState: "no_result", NoData: true},
		},
	})
	if got.Status != LineAvailabilityCollecting {
		t.Fatalf("status=%q, want %q", got.Status, LineAvailabilityCollecting)
	}
}

func TestSummarizeLineAvailability_StrictBlocksOnAnyGap(t *testing.T) {
	summary := SummarizeLineAvailability([]LineAvailability{
		{Status: LineAvailabilityReady},
		{Status: LineAvailabilityNoData},
		{Status: LineAvailabilityCollectionUnavailable},
	})
	if summary.LineTotal != 3 || summary.ReadyLineCount != 1 || summary.GapLineCount != 2 {
		t.Fatalf("unexpected summary: %+v", summary)
	}
	if !summary.HasStrictBlockingGap() {
		t.Fatalf("expected strict blocking gap")
	}
}
```

- [ ] **步骤 2：运行测试并确认失败**

运行：

```powershell
& 'C:\Program Files\Go\bin\go.exe' test ./internal/biz -run 'TestClassifyLineAvailability|TestSummarizeLineAvailability' -count=1
```

预期：失败，出现 `ClassifyLineAvailability` 等未定义标识符。

- [ ] **步骤 3：实现分类器**

新建 `internal/biz/bom_line_availability.go`：

```go
package biz

import "strings"

const (
	LineAvailabilityReady                 = "ready"
	LineAvailabilityNoData                = "no_data"
	LineAvailabilityCollectionUnavailable = "collection_unavailable"
	LineAvailabilityNoMatchAfterFilter    = "no_match_after_filter"
	LineAvailabilityCollecting            = "collecting"
)

type PlatformAvailabilityFact struct {
	PlatformID            string
	TaskState             string
	HasRawQuote           bool
	HasUsableQuote        bool
	NoData                bool
	CollectionUnavailable bool
	ReasonCode            string
	Message               string
	AutoAttempt           int
	ManualAttempt         int
}

type LineAvailabilityInput struct {
	LineNo    int
	MpnNorm   string
	Platforms []PlatformAvailabilityFact
}

type LineAvailability struct {
	LineNo                   int
	MpnNorm                  string
	Status                   string
	ReasonCode               string
	Reason                   string
	HasUsableQuote           bool
	RawQuotePlatformCount    int
	UsableQuotePlatformCount int
	ResolutionStatus         string
	PlatformFacts            []PlatformAvailabilityFact
}

type LineAvailabilitySummary struct {
	LineTotal                      int
	ReadyLineCount                 int
	GapLineCount                   int
	NoDataLineCount                int
	CollectionUnavailableLineCount int
	NoMatchAfterFilterLineCount    int
	CollectingLineCount            int
}

func ClassifyLineAvailability(in LineAvailabilityInput) LineAvailability {
	out := LineAvailability{
		LineNo:           in.LineNo,
		MpnNorm:          strings.TrimSpace(in.MpnNorm),
		Status:           LineAvailabilityCollecting,
		ReasonCode:       "COLLECTING",
		ResolutionStatus: "open",
		PlatformFacts:    append([]PlatformAvailabilityFact(nil), in.Platforms...),
	}

	hasCollecting := false
	hasRaw := false
	hasUsable := false
	hasUnavailable := false
	allExplicitNoData := len(in.Platforms) > 0

	for _, p := range in.Platforms {
		state := strings.ToLower(strings.TrimSpace(p.TaskState))
		if !isPlatformTerminal(state) {
			hasCollecting = true
		}
		if p.HasRawQuote {
			hasRaw = true
			out.RawQuotePlatformCount++
		}
		if p.HasUsableQuote {
			hasUsable = true
			out.UsableQuotePlatformCount++
		}
		if p.CollectionUnavailable {
			hasUnavailable = true
		}
		if !p.NoData {
			allExplicitNoData = false
		}
	}

	out.HasUsableQuote = hasUsable
	switch {
	case hasCollecting:
		out.Status = LineAvailabilityCollecting
		out.ReasonCode = "COLLECTING"
	case hasUsable:
		out.Status = LineAvailabilityReady
		out.ReasonCode = "READY"
	case hasRaw:
		out.Status = LineAvailabilityNoMatchAfterFilter
		out.ReasonCode = "NO_MATCH_AFTER_FILTER"
	case hasUnavailable:
		out.Status = LineAvailabilityCollectionUnavailable
		out.ReasonCode = "COLLECTION_UNAVAILABLE"
	case allExplicitNoData:
		out.Status = LineAvailabilityNoData
		out.ReasonCode = "NO_DATA"
	default:
		out.Status = LineAvailabilityCollectionUnavailable
		out.ReasonCode = "COLLECTION_UNAVAILABLE"
	}
	out.Reason = lineAvailabilityReason(out.Status)
	return out
}

func lineAvailabilityReason(status string) string {
	switch status {
	case LineAvailabilityReady:
		return "at least one platform has a usable quote"
	case LineAvailabilityNoData:
		return "all selected platforms returned no data"
	case LineAvailabilityCollectionUnavailable:
		return "collection finished without usable quotes and at least one platform was unavailable"
	case LineAvailabilityNoMatchAfterFilter:
		return "raw quotes exist but no quote passed matching filters"
	case LineAvailabilityCollecting:
		return "one or more platform searches are still running"
	default:
		return "availability unknown"
	}
}

func SummarizeLineAvailability(lines []LineAvailability) LineAvailabilitySummary {
	var s LineAvailabilitySummary
	s.LineTotal = len(lines)
	for _, line := range lines {
		switch line.Status {
		case LineAvailabilityReady:
			s.ReadyLineCount++
		case LineAvailabilityNoData:
			s.GapLineCount++
			s.NoDataLineCount++
		case LineAvailabilityCollectionUnavailable:
			s.GapLineCount++
			s.CollectionUnavailableLineCount++
		case LineAvailabilityNoMatchAfterFilter:
			s.GapLineCount++
			s.NoMatchAfterFilterLineCount++
		case LineAvailabilityCollecting:
			s.CollectingLineCount++
		}
	}
	return s
}

func (s LineAvailabilitySummary) HasStrictBlockingGap() bool {
	return s.NoDataLineCount > 0 || s.CollectionUnavailableLineCount > 0 || s.NoMatchAfterFilterLineCount > 0
}
```

- [ ] **步骤 4：运行 biz 测试**

运行：

```powershell
& 'C:\Program Files\Go\bin\go.exe' test ./internal/biz -run 'TestClassifyLineAvailability|TestSummarizeLineAvailability|TestReadiness' -count=1
```

预期：通过。

- [ ] **步骤 5：提交任务 1**

```powershell
git add -- 'internal/biz/bom_line_availability.go' 'internal/biz/bom_line_availability_test.go'
git commit -m "feat(bom): add line availability classifier"
```

---

### 任务 2：在 service 层汇总 availability 事实

**文件：**
- 新建：`internal/service/bom_availability.go`
- 新建：`internal/service/bom_availability_test.go`
- 修改：`internal/service/bom_service_test_helpers_test.go`

- [ ] **步骤 1：扩展 service 测试桩**

修改 `internal/service/bom_service_test_helpers_test.go` 中的 `bomSessionRepoStub`：

```go
type bomSessionRepoStub struct {
	mu                sync.Mutex
	view              *biz.BOMSessionView
	fullLines         []data.BomSessionLine
	patches           []biz.BOMImportStatePatch
	replaced          bool
	replacedNum       int
	replacedLineNos   []int
	tryStartCalls     int
	tryStartSuccesses int
	sessionExists     bool
}

func (s *bomSessionRepoStub) ListSessionLines(ctx context.Context, sessionID string) ([]biz.BOMSessionLineView, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]biz.BOMSessionLineView, 0, len(s.fullLines))
	for _, ln := range s.fullLines {
		out = append(out, biz.BOMSessionLineView{ID: ln.ID, LineNo: ln.LineNo, Mpn: ln.Mpn})
	}
	return out, nil
}

func (s *bomSessionRepoStub) ListSessionLinesFull(ctx context.Context, sessionID string) ([]data.BomSessionLine, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]data.BomSessionLine(nil), s.fullLines...), nil
}
```

修改 `bomSearchTaskRepoStub`：

```go
type bomSearchTaskRepoStub struct {
	mu          sync.Mutex
	tasks       []biz.TaskReadinessSnapshot
	cacheMap    map[string]*biz.QuoteCacheSnapshot
	cancelAll   bool
	upsertTasks bool
}

func (s *bomSearchTaskRepoStub) ListTasksForSession(ctx context.Context, sessionID string) ([]biz.TaskReadinessSnapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]biz.TaskReadinessSnapshot(nil), s.tasks...), nil
}

func (s *bomSearchTaskRepoStub) LoadQuoteCachesForKeys(ctx context.Context, bizDate time.Time, pairs []biz.MpnPlatformPair) (map[string]*biz.QuoteCacheSnapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make(map[string]*biz.QuoteCacheSnapshot, len(s.cacheMap))
	for k, v := range s.cacheMap {
		cp := *v
		out[k] = &cp
	}
	return out, nil
}
```

- [ ] **步骤 2：编写失败测试**

新建 `internal/service/bom_availability_test.go`：

```go
package service

import (
	"context"
	"testing"
	"time"

	"caichip/internal/biz"
	"caichip/internal/data"
)

func testQuoteCacheKey(mpn, platform string) string {
	return biz.NormalizeMPNForBOMSearch(mpn) + "\x00" + biz.NormalizePlatformID(platform)
}

func f64ptr(v float64) *float64 { return &v }

func TestComputeLineAvailability_NoDataAndCollectionUnavailable(t *testing.T) {
	view := &biz.BOMSessionView{
		SessionID:     "sid",
		BizDate:       time.Date(2026, 4, 25, 0, 0, 0, 0, time.Local),
		PlatformIDs:   []string{"icgoo", "szlcsc"},
		ReadinessMode: biz.ReadinessLenient,
	}
	session := &bomSessionRepoStub{
		view: view,
		fullLines: []data.BomSessionLine{
			{ID: 1, LineNo: 1, Mpn: "NO-DATA", Qty: f64ptr(1)},
			{ID: 2, LineNo: 2, Mpn: "BROKEN", Qty: f64ptr(1)},
		},
	}
	search := &bomSearchTaskRepoStub{
		tasks: []biz.TaskReadinessSnapshot{
			{MpnNorm: "NO-DATA", PlatformID: "icgoo", State: "no_result"},
			{MpnNorm: "NO-DATA", PlatformID: "szlcsc", State: "no_result"},
			{MpnNorm: "BROKEN", PlatformID: "icgoo", State: "failed_terminal"},
			{MpnNorm: "BROKEN", PlatformID: "szlcsc", State: "no_result"},
		},
		cacheMap: map[string]*biz.QuoteCacheSnapshot{
			testQuoteCacheKey("NO-DATA", "icgoo"):  {Outcome: "no_mpn_match"},
			testQuoteCacheKey("NO-DATA", "szlcsc"): {Outcome: "no_mpn_match"},
			testQuoteCacheKey("BROKEN", "szlcsc"):  {Outcome: "no_mpn_match"},
		},
	}
	svc := NewBomService(session, search, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)

	lines, summary, err := svc.computeLineAvailability(context.Background(), view, session.fullLines, view.PlatformIDs)
	if err != nil {
		t.Fatalf("computeLineAvailability: %v", err)
	}
	if lines[0].Status != biz.LineAvailabilityNoData {
		t.Fatalf("line 1 status=%q", lines[0].Status)
	}
	if lines[1].Status != biz.LineAvailabilityCollectionUnavailable {
		t.Fatalf("line 2 status=%q", lines[1].Status)
	}
	if summary.NoDataLineCount != 1 || summary.CollectionUnavailableLineCount != 1 {
		t.Fatalf("unexpected summary: %+v", summary)
	}
}
```

- [ ] **步骤 3：运行测试并确认失败**

运行：

```powershell
& 'C:\Program Files\Go\bin\go.exe' test ./internal/service -run TestComputeLineAvailability -count=1
```

预期：失败，出现 `svc.computeLineAvailability undefined`。

- [ ] **步骤 4：实现 service 事实收集**

新建 `internal/service/bom_availability.go`：

```go
package service

import (
	"context"
	"strings"

	"caichip/internal/biz"
	"caichip/internal/data"
)

func (s *BomService) computeLineAvailability(
	ctx context.Context,
	view *biz.BOMSessionView,
	lines []data.BomSessionLine,
	plats []string,
) ([]biz.LineAvailability, biz.LineAvailabilitySummary, error) {
	if view == nil || len(lines) == 0 {
		return nil, biz.LineAvailabilitySummary{}, nil
	}
	tasks, err := s.search.ListTasksForSession(ctx, view.SessionID)
	if err != nil {
		return nil, biz.LineAvailabilitySummary{}, err
	}
	pairList := dedupeQuoteCachePairs(lines, plats)
	cacheMap, err := s.search.LoadQuoteCachesForKeys(ctx, view.BizDate, pairList)
	if err != nil {
		return nil, biz.LineAvailabilitySummary{}, err
	}
	taskMap := make(map[string]string, len(tasks))
	for _, t := range tasks {
		taskMap[quoteCachePairKey(t.MpnNorm, t.PlatformID)] = strings.ToLower(strings.TrimSpace(t.State))
	}

	out := make([]biz.LineAvailability, 0, len(lines))
	for _, line := range lines {
		mn := biz.NormalizeMPNForBOMSearch(line.Mpn)
		facts := make([]biz.PlatformAvailabilityFact, 0, len(plats))
		for _, rawPID := range plats {
			pid := biz.NormalizePlatformID(rawPID)
			key := quoteCachePairKey(mn, pid)
			state := taskMap[key]
			snap := cacheMap[key]
			fact := s.platformAvailabilityFact(ctx, line, pid, state, snap, view)
			facts = append(facts, fact)
		}
		out = append(out, biz.ClassifyLineAvailability(biz.LineAvailabilityInput{
			LineNo:    line.LineNo,
			MpnNorm:   mn,
			Platforms: facts,
		}))
	}
	return out, biz.SummarizeLineAvailability(out), nil
}

func (s *BomService) platformAvailabilityFact(
	ctx context.Context,
	line data.BomSessionLine,
	pid, state string,
	snap *biz.QuoteCacheSnapshot,
	view *biz.BOMSessionView,
) biz.PlatformAvailabilityFact {
	fact := biz.PlatformAvailabilityFact{PlatformID: pid, TaskState: state}
	st := strings.ToLower(strings.TrimSpace(state))
	switch st {
	case "no_result":
		fact.NoData = true
		fact.ReasonCode = "NO_MPN"
	case "failed_terminal":
		fact.CollectionUnavailable = true
		fact.ReasonCode = "FETCH_FAILED"
	case "cancelled", "skipped":
		fact.CollectionUnavailable = true
		fact.ReasonCode = "PLATFORM_SKIPPED"
	}
	if snap == nil {
		return fact
	}
	if quoteCacheUsable(snap) {
		fact.HasRawQuote = true
		if s.platformHasUsableQuote(ctx, line, pid, snap, view) {
			fact.HasUsableQuote = true
			fact.ReasonCode = "READY"
		} else if fact.ReasonCode == "" {
			fact.ReasonCode = "FILTERED_BY_MFR"
		}
		return fact
	}
	switch strings.ToLower(strings.TrimSpace(snap.Outcome)) {
	case "no_mpn_match", "no_result":
		fact.NoData = true
		fact.ReasonCode = "NO_MPN"
	}
	return fact
}

func (s *BomService) platformHasUsableQuote(ctx context.Context, line data.BomSessionLine, pid string, snap *biz.QuoteCacheSnapshot, view *biz.BOMSessionView) bool {
	rows, ok := parseQuoteRowsForMatch(snap.QuotesJSON)
	if !ok || s.fx == nil || s.alias == nil {
		return false
	}
	pick, err := biz.PickBestQuoteForLine(ctx, biz.LineMatchInput{
		BomMpn:           line.Mpn,
		BomPackage:       derefStrPtr(line.Package),
		BomMfr:           derefStrPtr(line.Mfr),
		BomQty:           bomLineQtyInt(line.Qty),
		PlatformID:       pid,
		QuoteRows:        rows,
		BizDate:          view.BizDate,
		RequestDay:       view.BizDate,
		BaseCCY:          s.bomMatchBaseCCY(),
		RoundingMode:     s.bomMatchRoundingMode(),
		ParseTierStrings: s.bomMatchParseTiers(),
	}, s.fx, s.alias)
	return err == nil && pick.Ok
}
```

- [ ] **步骤 5：运行 service availability 测试**

运行：

```powershell
& 'C:\Program Files\Go\bin\go.exe' test ./internal/service -run TestComputeLineAvailability -count=1
```

预期：通过。

- [ ] **步骤 6：提交任务 2**

```powershell
git add -- 'internal/service/bom_availability.go' 'internal/service/bom_availability_test.go' 'internal/service/bom_service_test_helpers_test.go'
git commit -m "feat(bom): compute session line availability"
```

---

### 任务 3：把 availability 接入 readiness 和配单入口

**文件：**
- 修改：`internal/biz/bom_session_ready.go`
- 修改：`internal/service/bom_service.go`
- 新建：`internal/service/bom_readiness_availability_test.go`

- [ ] **步骤 1：编写 readiness 失败测试**

新建 `internal/service/bom_readiness_availability_test.go`：

```go
package service

import (
	"context"
	"testing"
	"time"

	v1 "caichip/api/bom/v1"
	"caichip/internal/biz"
	"caichip/internal/data"

	kerrors "github.com/go-kratos/kratos/v2/errors"
)

func TestGetReadiness_LenientAllowsIncompleteWithStats(t *testing.T) {
	view := &biz.BOMSessionView{
		SessionID:         "sid",
		Status:            "draft",
		BizDate:           time.Date(2026, 4, 25, 0, 0, 0, 0, time.Local),
		PlatformIDs:       []string{"icgoo"},
		ReadinessMode:     biz.ReadinessLenient,
		SelectionRevision: 7,
	}
	session := &bomSessionRepoStub{
		view:      view,
		fullLines: []data.BomSessionLine{{ID: 1, LineNo: 1, Mpn: "NO-DATA"}},
	}
	search := &bomSearchTaskRepoStub{
		tasks: []biz.TaskReadinessSnapshot{{MpnNorm: "NO-DATA", PlatformID: "icgoo", State: "no_result"}},
		cacheMap: map[string]*biz.QuoteCacheSnapshot{
			testQuoteCacheKey("NO-DATA", "icgoo"): {Outcome: "no_mpn_match"},
		},
	}
	svc := NewBomService(session, search, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)

	resp, err := svc.GetReadiness(context.Background(), &v1.GetReadinessRequest{SessionId: "sid"})
	if err != nil {
		t.Fatalf("GetReadiness: %v", err)
	}
	if !resp.GetCanEnterMatch() || resp.GetPhase() != "data_ready" {
		t.Fatalf("expected lenient data_ready, got phase=%q can=%v", resp.GetPhase(), resp.GetCanEnterMatch())
	}
	if resp.GetGapLineCount() != 1 || resp.GetNoDataLineCount() != 1 {
		t.Fatalf("missing gap stats: %+v", resp)
	}
}

func TestGetReadiness_StrictBlocksIncompleteWithStats(t *testing.T) {
	view := &biz.BOMSessionView{
		SessionID:         "sid",
		Status:            "draft",
		BizDate:           time.Date(2026, 4, 25, 0, 0, 0, 0, time.Local),
		PlatformIDs:       []string{"icgoo"},
		ReadinessMode:     biz.ReadinessStrict,
		SelectionRevision: 7,
	}
	session := &bomSessionRepoStub{
		view:      view,
		fullLines: []data.BomSessionLine{{ID: 1, LineNo: 1, Mpn: "NO-DATA"}},
	}
	search := &bomSearchTaskRepoStub{
		tasks: []biz.TaskReadinessSnapshot{{MpnNorm: "NO-DATA", PlatformID: "icgoo", State: "no_result"}},
		cacheMap: map[string]*biz.QuoteCacheSnapshot{
			testQuoteCacheKey("NO-DATA", "icgoo"): {Outcome: "no_mpn_match"},
		},
	}
	svc := NewBomService(session, search, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)

	resp, err := svc.GetReadiness(context.Background(), &v1.GetReadinessRequest{SessionId: "sid"})
	if err != nil {
		t.Fatalf("GetReadiness: %v", err)
	}
	if resp.GetCanEnterMatch() || resp.GetPhase() != "blocked" {
		t.Fatalf("expected strict blocked, got phase=%q can=%v", resp.GetPhase(), resp.GetCanEnterMatch())
	}
	if resp.GetBlockReason() != "strict_mode_line_availability_gap" {
		t.Fatalf("block_reason=%q", resp.GetBlockReason())
	}
}

func TestMatchReadinessError_StrictBlockedReturnsNotReady(t *testing.T) {
	view := &biz.BOMSessionView{
		SessionID:     "sid",
		Status:        "blocked",
		BizDate:       time.Date(2026, 4, 25, 0, 0, 0, 0, time.Local),
		PlatformIDs:   []string{"icgoo"},
		ReadinessMode: biz.ReadinessStrict,
	}
	session := &bomSessionRepoStub{view: view}
	search := &bomSearchTaskRepoStub{}
	svc := NewBomService(session, search, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)

	err := svc.matchReadinessError(context.Background(), "sid", view, []data.BomSessionLine{{ID: 1, LineNo: 1, Mpn: "NO-DATA"}})
	if err == nil {
		t.Fatalf("expected BOM_NOT_READY")
	}
	se := kerrors.FromError(err)
	if se == nil || se.Reason != "BOM_NOT_READY" {
		t.Fatalf("unexpected error: %+v", se)
	}
}
```

- [ ] **步骤 2：运行测试并确认失败**

运行：

```powershell
& 'C:\Program Files\Go\bin\go.exe' test ./internal/service -run 'TestGetReadiness_.*Availability|TestMatchReadinessError_StrictBlocked' -count=1
```

预期：在任务 4 增加 proto 字段前，可能因 `GapLineCount` 等字段不存在而编译失败。若是编译失败，先执行任务 4 的 proto 扩展，再回到本任务。

- [ ] **步骤 3：更新 `GetReadiness`**

在 `GetReadiness` 中，用 availability 替换现有 lenient/strict 判断：

```go
availability, summary, err := s.computeLineAvailability(ctx, view, lines, view.PlatformIDs)
if err != nil {
	return nil, err
}
_ = availability
phase := "searching"
can := false
block := ""
if summary.CollectingLineCount > 0 {
	phase = "searching"
} else if strings.TrimSpace(view.ReadinessMode) == biz.ReadinessStrict && summary.HasStrictBlockingGap() {
	phase = "blocked"
	block = "strict_mode_line_availability_gap"
} else {
	phase = "data_ready"
	can = true
}
if view.Status == "blocked" {
	phase = "blocked"
	can = false
	if block == "" {
		block = "strict_mode_line_availability_gap"
	}
}
```

返回中补齐：

```go
LineTotal:                      int32(summary.LineTotal),
ReadyLineCount:                 int32(summary.ReadyLineCount),
GapLineCount:                   int32(summary.GapLineCount),
NoDataLineCount:                int32(summary.NoDataLineCount),
CollectionUnavailableLineCount: int32(summary.CollectionUnavailableLineCount),
NoMatchAfterFilterLineCount:    int32(summary.NoMatchAfterFilterLineCount),
```

- [ ] **步骤 4：更新配单入口判断**

在 `matchReadinessError` 中保留 import parsing guard，然后调用 `computeLineAvailability`。当所有行都不是 `collecting`，并且：

- `ReadinessMode != strict`：允许进入配单；
- `ReadinessMode == strict` 且 `summary.HasStrictBlockingGap() == false`：允许进入配单；
- 其他情况返回 `BOM_NOT_READY`。

- [ ] **步骤 5：更新 session 终态注释**

在 `internal/biz/bom_session_ready.go` 保留现有 `ReadinessFromTasks` 作为任务终态层判断，并加注释说明：

```go
// TryMarkSessionDataReady only has task snapshots. Service endpoints compute richer line availability
// from quote caches and may report strict blocking reasons without moving data-layer logic into data.
```

不要从 `biz` 反向导入 `service` 或 `data`。

- [ ] **步骤 6：运行 readiness 测试**

运行：

```powershell
& 'C:\Program Files\Go\bin\go.exe' test ./internal/service -run 'TestGetReadiness_|TestMatchReadinessError_' -count=1
```

预期：任务 4 完成后通过。

- [ ] **步骤 7：提交任务 3**

```powershell
git add -- 'internal/service/bom_service.go' 'internal/biz/bom_session_ready.go' 'internal/service/bom_readiness_availability_test.go'
git commit -m "feat(bom): apply line availability to readiness"
```

---

### 任务 4：扩展 proto 与前端 API 类型

**文件：**
- 修改：`api/bom/v1/bom.proto`
- 重新生成：`api/bom/v1/bom.pb.go`
- 重新生成：`api/bom/v1/bom_http.pb.go`
- 重新生成：`api/bom/v1/bom_grpc.pb.go`
- 修改：`web/src/api/types.ts`
- 修改：`web/src/api/bomSession.ts`

- [ ] **步骤 1：增加 proto 字段**

在 `GetReadinessReply` 的 `block_reason = 6` 后追加：

```proto
  int32 line_total = 7;
  int32 ready_line_count = 8;
  int32 gap_line_count = 9;
  int32 no_data_line_count = 10;
  int32 collection_unavailable_line_count = 11;
  int32 no_match_after_filter_line_count = 12;
```

在 `BOMLineRow` 的 `platform_gaps = 8` 后追加：

```proto
  string availability_status = 9;
  string availability_reason = 10;
  bool has_usable_quote = 11;
  int32 raw_quote_platform_count = 12;
  int32 usable_quote_platform_count = 13;
  string resolution_status = 14;
```

- [ ] **步骤 2：重新生成 protobuf 绑定**

运行：

```powershell
protoc --proto_path=./api --proto_path=./third_party --go_out=paths=source_relative:./api --go-http_out=paths=source_relative:./api --go-grpc_out=paths=source_relative:./api api/bom/v1/bom.proto
```

预期：退出码为 0，生成文件发生更新。

- [ ] **步骤 3：更新 TypeScript 类型**

在 `web/src/api/types.ts` 中扩展 `GetReadinessReply`：

```ts
export interface GetReadinessReply {
  session_id: string
  biz_date: string
  selection_revision: number
  phase: string
  can_enter_match: boolean
  block_reason: string
  line_total: number
  ready_line_count: number
  gap_line_count: number
  no_data_line_count: number
  collection_unavailable_line_count: number
  no_match_after_filter_line_count: number
}
```

扩展 `BOMLineRow`：

```ts
export interface BOMLineRow {
  line_id: string
  line_no: number
  mpn: string
  mfr: string
  package: string
  qty: number
  match_status: string
  platform_gaps: PlatformGap[]
  availability_status?: string
  availability_reason?: string
  has_usable_quote?: boolean
  raw_quote_platform_count?: number
  usable_quote_platform_count?: number
  resolution_status?: string
}
```

更新 `PlatformGap` 注释：

```ts
/** pending | searching | succeeded | no_data | failed | skipped */
search_ui_state?: string
```

- [ ] **步骤 4：解析新字段**

在 `web/src/api/bomSession.ts` 的 `getReadiness` 返回中增加：

```ts
line_total: num(json.line_total ?? json.lineTotal, 0),
ready_line_count: num(json.ready_line_count ?? json.readyLineCount, 0),
gap_line_count: num(json.gap_line_count ?? json.gapLineCount, 0),
no_data_line_count: num(json.no_data_line_count ?? json.noDataLineCount, 0),
collection_unavailable_line_count: num(
  json.collection_unavailable_line_count ?? json.collectionUnavailableLineCount,
  0
),
no_match_after_filter_line_count: num(
  json.no_match_after_filter_line_count ?? json.noMatchAfterFilterLineCount,
  0
),
```

在 `getBOMLines` 的每行返回中增加：

```ts
availability_status: str(row.availability_status ?? row.availabilityStatus) || undefined,
availability_reason: str(row.availability_reason ?? row.availabilityReason) || undefined,
has_usable_quote: Boolean(row.has_usable_quote ?? row.hasUsableQuote),
raw_quote_platform_count: num(row.raw_quote_platform_count ?? row.rawQuotePlatformCount, 0),
usable_quote_platform_count: num(
  row.usable_quote_platform_count ?? row.usableQuotePlatformCount,
  0
),
resolution_status: str(row.resolution_status ?? row.resolutionStatus) || undefined,
```

- [ ] **步骤 5：运行 proto/API 相关检查**

运行：

```powershell
& 'C:\Program Files\Go\bin\go.exe' test ./internal/service -run 'TestGetReadiness_|TestComputeLineAvailability' -count=1
```

运行：

```powershell
& 'D:\Program Files\nodejs\npm.cmd' test -- src/api/bomSession.test.ts
```

预期：均通过。若 `bomSession.test.ts` 尚未覆盖新字段，先增加一个小的 parser 测试再运行。

- [ ] **步骤 6：提交任务 4**

```powershell
git add -- 'api/bom/v1/bom.proto' 'api/bom/v1/bom.pb.go' 'api/bom/v1/bom_http.pb.go' 'api/bom/v1/bom_grpc.pb.go' 'web/src/api/types.ts' 'web/src/api/bomSession.ts'
git commit -m "feat(bom): expose line availability fields"
```

---

### 任务 5：把 availability 接入行接口、配单结果和导出

**文件：**
- 修改：`internal/service/bom_service.go`
- 修改：`internal/service/bom_match_parallel.go`
- 新建：`internal/service/bom_export_availability_test.go`

- [ ] **步骤 1：编写行接口和导出失败测试**

新建 `internal/service/bom_export_availability_test.go`：

```go
package service

import (
	"bytes"
	"context"
	"testing"
	"time"

	v1 "caichip/api/bom/v1"
	"caichip/internal/biz"
	"caichip/internal/data"

	"github.com/xuri/excelize/v2"
)

func TestGetBOMLines_IncludesAvailabilityFields(t *testing.T) {
	view := &biz.BOMSessionView{
		SessionID:   "sid",
		BizDate:     time.Date(2026, 4, 25, 0, 0, 0, 0, time.Local),
		PlatformIDs: []string{"icgoo"},
	}
	session := &bomSessionRepoStub{
		view:      view,
		fullLines: []data.BomSessionLine{{ID: 1, LineNo: 1, Mpn: "NO-DATA"}},
	}
	search := &bomSearchTaskRepoStub{
		tasks: []biz.TaskReadinessSnapshot{{MpnNorm: "NO-DATA", PlatformID: "icgoo", State: "no_result"}},
		cacheMap: map[string]*biz.QuoteCacheSnapshot{
			testQuoteCacheKey("NO-DATA", "icgoo"): {Outcome: "no_mpn_match"},
		},
	}
	svc := NewBomService(session, search, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)

	resp, err := svc.GetBOMLines(context.Background(), &v1.GetBOMLinesRequest{SessionId: "sid"})
	if err != nil {
		t.Fatalf("GetBOMLines: %v", err)
	}
	line := resp.GetLines()[0]
	if line.GetAvailabilityStatus() != biz.LineAvailabilityNoData {
		t.Fatalf("availability=%q", line.GetAvailabilityStatus())
	}
	if line.GetRawQuotePlatformCount() != 0 || line.GetHasUsableQuote() {
		t.Fatalf("unexpected quote fields: %+v", line)
	}
}

func TestExportSession_IncludesAvailabilityReasonColumn(t *testing.T) {
	view := &biz.BOMSessionView{
		SessionID:   "sid",
		BizDate:     time.Date(2026, 4, 25, 0, 0, 0, 0, time.Local),
		PlatformIDs: []string{"icgoo"},
	}
	session := &bomSessionRepoStub{
		view:      view,
		fullLines: []data.BomSessionLine{{ID: 1, LineNo: 1, Mpn: "NO-DATA"}},
	}
	search := &bomSearchTaskRepoStub{
		tasks: []biz.TaskReadinessSnapshot{{MpnNorm: "NO-DATA", PlatformID: "icgoo", State: "no_result"}},
		cacheMap: map[string]*biz.QuoteCacheSnapshot{
			testQuoteCacheKey("NO-DATA", "icgoo"): {Outcome: "no_mpn_match"},
		},
	}
	svc := NewBomService(session, search, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)

	resp, err := svc.ExportSession(context.Background(), &v1.ExportSessionRequest{SessionId: "sid", Format: "xlsx"})
	if err != nil {
		t.Fatalf("ExportSession: %v", err)
	}
	f, err := excelize.OpenReader(bytes.NewReader(resp.GetFile()))
	if err != nil {
		t.Fatalf("open export: %v", err)
	}
	defer f.Close()
	sheet := f.GetSheetName(0)
	status, _ := f.GetCellValue(sheet, "F2")
	reason, _ := f.GetCellValue(sheet, "G2")
	if status != biz.LineAvailabilityNoData {
		t.Fatalf("status cell=%q", status)
	}
	if reason == "" {
		t.Fatalf("expected reason cell")
	}
}
```

- [ ] **步骤 2：运行测试并确认失败**

运行：

```powershell
& 'C:\Program Files\Go\bin\go.exe' test ./internal/service -run 'TestGetBOMLines_IncludesAvailabilityFields|TestExportSession_IncludesAvailabilityReasonColumn' -count=1
```

预期：失败，直到 `GetBOMLines` 和 `ExportSession` 填充 availability 字段。

- [ ] **步骤 3：填充行接口 availability**

在 `GetBOMLines` 中先加载 session view，然后计算 availability 并按行号索引：

```go
view, err := s.session.GetSession(ctx, req.GetSessionId())
if err != nil {
	return nil, err
}
availability, _, err := s.computeLineAvailability(ctx, view, rows, view.PlatformIDs)
if err != nil {
	return nil, err
}
byLine := make(map[int]biz.LineAvailability, len(availability))
for _, a := range availability {
	byLine[a.LineNo] = a
}
```

在每个 `BOMLineRow` 中填入：

```go
a := byLine[row.LineNo]
AvailabilityStatus:       a.Status,
AvailabilityReason:       a.Reason,
HasUsableQuote:           a.HasUsableQuote,
RawQuotePlatformCount:    int32(a.RawQuotePlatformCount),
UsableQuotePlatformCount: int32(a.UsableQuotePlatformCount),
ResolutionStatus:         a.ResolutionStatus,
```

- [ ] **步骤 4：保持配单结果兼容**

在 `matchOneLine` 中，当 `len(cand) == 0` 时继续返回 `MatchStatus: "no_match"`，保持兼容。本期不向 `MatchItem` 增加新字段；结果页需要缺口状态时从 `GetBOMLines` 获取。

- [ ] **步骤 5：增加导出列**

在 `ExportSession` 中读取 session view、计算 availability，然后把表头改为：

```go
_ = f.SetSheetRow(sheet, "A1", &[]any{"Line No", "Model", "Manufacturer", "Package", "Quantity", "Availability", "Availability Reason"})
```

写入每行时增加：

```go
a := byLine[row.LineNo]
_ = f.SetSheetRow(sheet, cell, &[]any{
	row.LineNo,
	row.Mpn,
	derefStrPtr(row.Mfr),
	derefStrPtr(row.Package),
	qty,
	a.Status,
	a.Reason,
})
```

CSV 仍复用当前 response 结构；本任务通过同一 workbook 写入逻辑保持内容一致。

- [ ] **步骤 6：运行 service 测试**

运行：

```powershell
& 'C:\Program Files\Go\bin\go.exe' test ./internal/service -run 'TestGetBOMLines_|TestExportSession_|TestGetReadiness_|TestComputeLineAvailability' -count=1
```

预期：通过。

- [ ] **步骤 7：提交任务 5**

```powershell
git add -- 'internal/service/bom_service.go' 'internal/service/bom_match_parallel.go' 'internal/service/bom_export_availability_test.go'
git commit -m "feat(bom): surface availability in lines and export"
```

---

### 任务 6：增加前端 availability 展示

**文件：**
- 修改：`web/src/pages/SourcingSessionPage.tsx`
- 修改：`web/src/pages/SourcingSessionPage.test.tsx`

- [ ] **步骤 1：编写前端失败测试**

在 `web/src/pages/SourcingSessionPage.test.tsx` 中增加：

```tsx
it('shows line availability status and incomplete warning', async () => {
  getSession.mockResolvedValue({
    ...baseSession,
    status: 'data_ready',
    import_status: 'ready',
    import_progress: 100,
  })
  getBOMLines.mockResolvedValue({
    lines: [
      {
        line_id: '1',
        line_no: 1,
        mpn: 'NO-DATA',
        mfr: '',
        package: '',
        qty: 1,
        match_status: '',
        platform_gaps: [],
        availability_status: 'no_data',
        availability_reason: 'all selected platforms returned no data',
        has_usable_quote: false,
        raw_quote_platform_count: 0,
        usable_quote_platform_count: 0,
        resolution_status: 'open',
      },
    ],
  })

  render(<SourcingSessionPage sessionId="session-1" onEnterMatch={vi.fn()} />)

  await act(async () => {
    await flushAsyncWork()
  })

  expect(screen.getByText('无平台数据')).toBeInTheDocument()
  expect(screen.getByText(/本 BOM 有 1 行无法自动配单/)).toBeInTheDocument()
})
```

- [ ] **步骤 2：运行前端测试并确认失败**

运行：

```powershell
& 'D:\Program Files\nodejs\npm.cmd' test -- src/pages/SourcingSessionPage.test.tsx
```

预期：失败，因为页面尚未渲染新文案。

- [ ] **步骤 3：增加前端状态映射**

在 `SourcingSessionPage.tsx` 组件附近增加：

```tsx
function availabilityLabel(status?: string): string {
  switch ((status || '').trim()) {
    case 'ready':
      return '可配单'
    case 'no_data':
      return '无平台数据'
    case 'collection_unavailable':
      return '采集不可用'
    case 'no_match_after_filter':
      return '有报价但未匹配'
    case 'collecting':
      return '采集中'
    default:
      return '未判定'
  }
}

function availabilityTone(status?: string): string {
  switch ((status || '').trim()) {
    case 'ready':
      return 'bg-emerald-50 text-emerald-700 border-emerald-200'
    case 'no_data':
      return 'bg-amber-50 text-amber-800 border-amber-200'
    case 'collection_unavailable':
      return 'bg-red-50 text-red-700 border-red-200'
    case 'no_match_after_filter':
      return 'bg-violet-50 text-violet-700 border-violet-200'
    case 'collecting':
      return 'bg-blue-50 text-blue-700 border-blue-200'
    default:
      return 'bg-slate-50 text-slate-600 border-slate-200'
  }
}
```

- [ ] **步骤 4：增加不完整提示**

在 BOM 行表渲染前计算：

```tsx
const gapLines = lines.filter((line) =>
  ['no_data', 'collection_unavailable', 'no_match_after_filter'].includes(
    (line.availability_status || '').trim()
  )
)
const noDataCount = gapLines.filter((line) => line.availability_status === 'no_data').length
const unavailableCount = gapLines.filter((line) => line.availability_status === 'collection_unavailable').length
const filteredCount = gapLines.filter((line) => line.availability_status === 'no_match_after_filter').length
```

渲染：

```tsx
{gapLines.length > 0 && (
  <div className="mb-3 rounded-lg border border-amber-200 bg-amber-50 px-3 py-2 text-sm text-amber-950">
    本 BOM 有 {gapLines.length} 行无法自动配单：{noDataCount} 行无数据，{unavailableCount} 行采集不可用，{filteredCount} 行有报价但未通过筛选。
  </div>
)}
```

- [ ] **步骤 5：增加表格状态列**

在 `match_status` 后增加表头：

```tsx
<th className="py-2 px-2">数据状态</th>
```

在每行 `match_status` 后增加：

```tsx
<td className="py-2 px-2">
  <span className={`inline-flex rounded border px-2 py-0.5 text-xs font-medium ${availabilityTone(row.availability_status)}`}>
    {availabilityLabel(row.availability_status)}
  </span>
  {row.availability_reason && (
    <div className="mt-1 max-w-48 text-xs text-slate-500">{row.availability_reason}</div>
  )}
</td>
```

把空状态行的 `colSpan` 加 1。

- [ ] **步骤 6：运行前端测试**

运行：

```powershell
& 'D:\Program Files\nodejs\npm.cmd' test -- src/pages/SourcingSessionPage.test.tsx
```

预期：通过。

- [ ] **步骤 7：提交任务 6**

```powershell
git add -- 'web/src/pages/SourcingSessionPage.tsx' 'web/src/pages/SourcingSessionPage.test.tsx'
git commit -m "feat(web): show bom line availability"
```

---

### 任务 7：完整验证

**文件：**
- 预期不修改源码；如果验证暴露缺陷，再做最小修复。

- [ ] **步骤 1：运行后端聚焦测试**

运行：

```powershell
& 'C:\Program Files\Go\bin\go.exe' test ./internal/biz ./internal/service -count=1
```

预期：通过。

- [ ] **步骤 2：运行前端聚焦测试**

运行：

```powershell
& 'D:\Program Files\nodejs\npm.cmd' test -- src/api/bomSession.test.ts src/pages/SourcingSessionPage.test.tsx
```

预期：通过。

- [ ] **步骤 3：运行更广的 Go 测试**

运行：

```powershell
& 'C:\Program Files\Go\bin\go.exe' test ./internal/biz/... ./internal/data/... ./internal/service/... ./internal/server/... -count=1
```

预期：通过。若出现无关既有失败，记录具体 package 和错误摘要，再决定修复或报告。

- [ ] **步骤 4：运行前端构建**

运行：

```powershell
& 'D:\Program Files\nodejs\npm.cmd' run build
```

预期：通过。若 Windows native dependency 在沙箱中出现 `EPERM`，按本仓库已有经验使用提权重跑。

- [ ] **步骤 5：如有验证修复则提交**

如果验证阶段产生修复：

```powershell
git add -- '<fixed-file-1>' '<fixed-file-2>'
git commit -m "fix(bom): stabilize line availability verification"
```

如果没有修复，不创建空提交。

---

## 自检

- 规格覆盖：任务 1-3 覆盖行级 availability 分类、`lenient/strict` 行为和 readiness；任务 4 覆盖 API 字段；任务 5 覆盖行接口、配单兼容和导出原因；任务 6 覆盖前端展示；任务 7 覆盖验证。
- 范围检查：本计划不实现人工补录报价或替代料选择，只预留 `resolution_status` 和稳定 code，符合设计文档。
- 类型一致性：状态字符串在 Go、proto、TypeScript 中统一使用 `ready`、`no_data`、`collection_unavailable`、`no_match_after_filter`、`collecting`。
- 占位检查：没有占位标记或未说明的实现步骤。
