# BOM Search Task Status Retry Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 为 BOM 会话页新增搜索任务状态总览和明细，并支持整单异常重试与单任务重试。

**Architecture:** 后端新增专用 `ListSessionSearchTasks` API，返回 BOM 搜索任务状态、Agent 调度状态、汇总计数和 `retryable` 判定。`internal/biz` 承载状态映射与重试规则，`internal/data` 只用 GORM 读取 `t_bom_search_task` 与 `t_caichip_dispatch_task`，`internal/service` 组装 `line × platform` 明细并修复缺失任务重试。前端新增独立状态面板组件，`SourcingSessionPage` 只负责加载、刷新和调用现有重试 API。

**Tech Stack:** Go、Kratos、GORM、protobuf、React、TypeScript、Vitest。

---

## File Structure

- Create `internal/biz/bom_search_task_status.go`: 搜索任务 UI 状态映射、重试规则、汇总计数类型与函数。
- Create `internal/biz/bom_search_task_status_test.go`: 覆盖状态映射、批量/单条重试、汇总计数。
- Modify `internal/biz/repo.go`: 在 `BOMSearchTaskRepo` 增加只读任务状态查询方法。
- Create `internal/data/bom_search_task_status_repo.go`: 用 GORM 查询 `t_bom_search_task` 并按 `caichip_task_id` 批量关联 `t_caichip_dispatch_task`。
- Create `internal/data/bom_search_task_status_repo_test.go`: 覆盖任务状态读取与 dispatch 关联。
- Modify `api/bom/v1/bom.proto`: 新增 `ListSessionSearchTasks` RPC 和 DTO。
- Generate `api/bom/v1/bom.pb.go`, `api/bom/v1/bom_http.pb.go`, `api/bom/v1/bom_grpc.pb.go`: 由 `make api` 生成。
- Modify `internal/service/bom_service.go`: 实现 `ListSessionSearchTasks`，并让 `RetrySearchTasks` 能恢复缺失任务。
- Create or modify `internal/service/bom_service_test_helpers_test.go`: 给 service 测试提供 `newBomServiceForTest`、`bomSessionRepoStub`、`bomSearchTaskRepoStub`，并增加新 repo 方法。
- Create or modify `internal/service/bom_search_task_status_test.go`: 覆盖 service 汇总、缺失任务、重试语义。
- Modify `web/src/api/types.ts`: 新增搜索任务状态类型。
- Modify `web/src/api/bomSession.ts`: 新增 `listSessionSearchTasks` API 解析。
- Modify `web/src/api/bomSession.test.ts`: 覆盖 API JSON 转换。
- Create `web/src/pages/sourcing-session/SearchTaskStatusPanel.tsx`: 顶部总览和明细表组件。
- Create `web/src/pages/sourcing-session/SearchTaskStatusPanel.test.tsx`: 覆盖渲染、批量重试过滤、`no_result` 单条重试。
- Modify `web/src/pages/SourcingSessionPage.tsx`: 加载任务状态，接入面板，重试后刷新。
- Modify `web/src/pages/SourcingSessionPage.test.tsx`: 覆盖页面集成刷新和重试调用。

## Task 1: biz 状态映射与重试规则

**Files:**
- Create: `internal/biz/bom_search_task_status.go`
- Create: `internal/biz/bom_search_task_status_test.go`

- [ ] **Step 1: 写失败测试**

Create `internal/biz/bom_search_task_status_test.go`:

```go
package biz

import "testing"

func TestMapBOMSearchTaskUIState(t *testing.T) {
	cases := map[string]string{
		"":                 SearchTaskUIStateMissing,
		"pending":          SearchTaskUIStatePending,
		"running":          SearchTaskUIStateSearching,
		"failed_retryable": SearchTaskUIStateFailed,
		"failed_terminal":  SearchTaskUIStateFailed,
		"succeeded":        SearchTaskUIStateSucceeded,
		"no_result":        SearchTaskUIStateNoData,
		"skipped":          SearchTaskUIStateSkipped,
		"cancelled":        SearchTaskUIStateCancelled,
		"unknown":          SearchTaskUIStateMissing,
	}
	for in, want := range cases {
		if got := MapBOMSearchTaskUIState(in); got != want {
			t.Fatalf("state %q: got %q, want %q", in, got, want)
		}
	}
}

func TestCanRetryBOMSearchTask(t *testing.T) {
	batchYes := []string{"failed_retryable", "failed_terminal", "skipped", "missing"}
	for _, st := range batchYes {
		if !CanRetryBOMSearchTask(st, SearchTaskRetryBatchAnomaly) {
			t.Fatalf("batch should retry %q", st)
		}
	}
	batchNo := []string{"pending", "running", "succeeded", "no_result", "cancelled"}
	for _, st := range batchNo {
		if CanRetryBOMSearchTask(st, SearchTaskRetryBatchAnomaly) {
			t.Fatalf("batch should not retry %q", st)
		}
	}
	if !CanRetryBOMSearchTask("no_result", SearchTaskRetrySingleManual) {
		t.Fatal("single manual should retry no_result")
	}
	if CanRetryBOMSearchTask("running", SearchTaskRetrySingleManual) {
		t.Fatal("single manual should not retry running")
	}
}

func TestBuildSearchTaskStatusSummary(t *testing.T) {
	rows := []SearchTaskStatusRow{
		{SearchState: "pending", DispatchState: "pending"},
		{SearchState: "running", DispatchState: "leased"},
		{SearchState: "succeeded", DispatchState: "finished"},
		{SearchState: "no_result", DispatchState: "finished"},
		{SearchState: "failed_terminal", DispatchState: "failed_terminal"},
		{SearchState: "missing"},
	}
	got := BuildSearchTaskStatusSummary(rows)
	if got.Total != 6 || got.Pending != 1 || got.Running != 1 || got.Succeeded != 1 ||
		got.NoResult != 1 || got.Failed != 1 || got.Missing != 1 {
		t.Fatalf("unexpected search summary: %+v", got)
	}
	if got.DispatchPending != 1 || got.DispatchLeased != 1 || got.DispatchFinished != 2 || got.DispatchFailed != 1 {
		t.Fatalf("unexpected dispatch summary: %+v", got)
	}
}
```

- [ ] **Step 2: 运行测试确认失败**

Run:

```powershell
& 'C:\Program Files\Go\bin\go.exe' test ./internal/biz -run 'TestMapBOMSearchTaskUIState|TestCanRetryBOMSearchTask|TestBuildSearchTaskStatusSummary' -count=1
```

Expected: FAIL，提示 `SearchTaskUIStateMissing` 或 `MapBOMSearchTaskUIState` 未定义。

- [ ] **Step 3: 实现最小 biz 代码**

Create `internal/biz/bom_search_task_status.go`:

```go
package biz

import (
	"strings"
	"time"
)

const (
	SearchTaskUIStatePending   = "pending"
	SearchTaskUIStateSearching = "searching"
	SearchTaskUIStateSucceeded = "succeeded"
	SearchTaskUIStateNoData    = "no_data"
	SearchTaskUIStateFailed    = "failed"
	SearchTaskUIStateSkipped   = "skipped"
	SearchTaskUIStateCancelled = "cancelled"
	SearchTaskUIStateMissing   = "missing"
)

type SearchTaskRetryMode string

const (
	SearchTaskRetryBatchAnomaly SearchTaskRetryMode = "batch_anomaly"
	SearchTaskRetrySingleManual SearchTaskRetryMode = "single_manual"
)

type SearchTaskStatusSummary struct {
	Total            int
	Pending          int
	Running          int
	Succeeded        int
	NoResult         int
	Failed           int
	Skipped          int
	Cancelled        int
	Missing          int
	DispatchPending  int
	DispatchLeased   int
	DispatchFinished int
	DispatchFailed   int
}

type SearchTaskStatusRow struct {
	LineID               string
	LineNo               int
	MPN                  string
	MpnNorm              string
	PlatformID           string
	SearchState          string
	SearchUIState        string
	CaichipTaskID        string
	DispatchState        string
	DispatchResultStatus string
	Attempt              int
	RetryMax             int
	LeasedToAgentID      string
	LeaseDeadlineAt      *time.Time
	LastError            string
	UpdatedAt            *time.Time
	Retryable            bool
}

func NormalizeBOMSearchTaskState(state string) string {
	st := strings.ToLower(strings.TrimSpace(state))
	if st == "" {
		return "missing"
	}
	return st
}

func MapBOMSearchTaskUIState(state string) string {
	switch NormalizeBOMSearchTaskState(state) {
	case "pending":
		return SearchTaskUIStatePending
	case "running":
		return SearchTaskUIStateSearching
	case "succeeded":
		return SearchTaskUIStateSucceeded
	case "no_result":
		return SearchTaskUIStateNoData
	case "failed_retryable", "failed_terminal":
		return SearchTaskUIStateFailed
	case "skipped":
		return SearchTaskUIStateSkipped
	case "cancelled":
		return SearchTaskUIStateCancelled
	case "missing":
		return SearchTaskUIStateMissing
	default:
		return SearchTaskUIStateMissing
	}
}

func CanRetryBOMSearchTask(state string, mode SearchTaskRetryMode) bool {
	switch NormalizeBOMSearchTaskState(state) {
	case "failed_retryable", "failed_terminal", "skipped", "missing":
		return true
	case "no_result":
		return mode == SearchTaskRetrySingleManual
	default:
		return false
	}
}

func BuildSearchTaskStatusSummary(rows []SearchTaskStatusRow) SearchTaskStatusSummary {
	var out SearchTaskStatusSummary
	for _, row := range rows {
		out.Total++
		switch NormalizeBOMSearchTaskState(row.SearchState) {
		case "pending":
			out.Pending++
		case "running":
			out.Running++
		case "succeeded":
			out.Succeeded++
		case "no_result":
			out.NoResult++
		case "failed_retryable", "failed_terminal":
			out.Failed++
		case "skipped":
			out.Skipped++
		case "cancelled":
			out.Cancelled++
		case "missing":
			out.Missing++
		}
		switch strings.ToLower(strings.TrimSpace(row.DispatchState)) {
		case "pending":
			out.DispatchPending++
		case "leased":
			out.DispatchLeased++
		case "finished":
			out.DispatchFinished++
		case "failed_terminal":
			out.DispatchFailed++
		}
	}
	return out
}
```

- [ ] **Step 4: 运行测试确认通过**

Run:

```powershell
& 'C:\Program Files\Go\bin\go.exe' test ./internal/biz -run 'TestMapBOMSearchTaskUIState|TestCanRetryBOMSearchTask|TestBuildSearchTaskStatusSummary' -count=1
```

Expected: PASS.

- [ ] **Step 5: 提交**

```powershell
git add -- internal/biz/bom_search_task_status.go internal/biz/bom_search_task_status_test.go
git commit -m "feat(biz): define bom search task status rules"
```

## Task 2: data 层只读状态查询

**Files:**
- Modify: `internal/biz/repo.go`
- Create: `internal/data/bom_search_task_status_repo.go`
- Create: `internal/data/bom_search_task_status_repo_test.go`

- [ ] **Step 1: 写失败测试**

Create `internal/data/bom_search_task_status_repo_test.go`:

```go
package data

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestBOMSearchTaskRepo_ListSearchTaskStatusRows(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&BomSearchTask{}, &CaichipDispatchTask{}); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 4, 25, 12, 0, 0, 0, time.UTC)
	leasedAgent := sql.NullString{String: "agent-1", Valid: true}
	taskID := sql.NullString{String: "task-1", Valid: true}
	lastErr := sql.NullString{String: "timeout", Valid: true}
	if err := db.Create(&BomSearchTask{
		SessionID: "session-1", MpnNorm: "TPS5430DDA", PlatformID: "hqchip",
		BizDate: time.Date(2026, 4, 25, 0, 0, 0, 0, time.UTC), State: "failed_terminal",
		AutoAttempt: 2, ManualAttempt: 1, SelectionRevision: 3, CaichipTaskID: taskID,
		LastError: lastErr, CreatedAt: now, UpdatedAt: now,
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&CaichipDispatchTask{
		TaskID: "task-1", Queue: "default", ScriptID: "hqchip", Version: "1.0.0",
		State: "failed_terminal", Attempt: 3, RetryMax: 4, LeasedToAgentID: leasedAgent,
		ResultStatus: sql.NullString{String: "failed", Valid: true},
		LastError: sql.NullString{String: "agent failed", Valid: true},
		CreatedAt: now, UpdatedAt: now,
	}).Error; err != nil {
		t.Fatal(err)
	}

	repo := &BOMSearchTaskRepo{db: db}
	rows, err := repo.ListSearchTaskStatusRows(context.Background(), "session-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	got := rows[0]
	if got.MpnNorm != "TPS5430DDA" || got.PlatformID != "hqchip" || got.SearchState != "failed_terminal" {
		t.Fatalf("unexpected search row: %+v", got)
	}
	if got.CaichipTaskID != "task-1" || got.DispatchState != "failed_terminal" ||
		got.DispatchResultStatus != "failed" || got.Attempt != 3 || got.RetryMax != 4 ||
		got.LeasedToAgentID != "agent-1" {
		t.Fatalf("unexpected dispatch fields: %+v", got)
	}
	if got.LastError != "timeout" {
		t.Fatalf("BOM last error should win, got %q", got.LastError)
	}
}
```

- [ ] **Step 2: 运行测试确认失败**

Run:

```powershell
& 'C:\Program Files\Go\bin\go.exe' test ./internal/data -run TestBOMSearchTaskRepo_ListSearchTaskStatusRows -count=1
```

Expected: FAIL，提示 `ListSearchTaskStatusRows` 未定义。

- [ ] **Step 3: 扩展 repo 接口**

Modify `internal/biz/repo.go` in `BOMSearchTaskRepo`:

```go
	ListSearchTaskStatusRows(ctx context.Context, sessionID string) ([]SearchTaskStatusRow, error)
```

- [ ] **Step 4: 实现 GORM 查询**

Create `internal/data/bom_search_task_status_repo.go`:

```go
package data

import (
	"context"
	"strings"

	"caichip/internal/biz"
)

func (r *BOMSearchTaskRepo) ListSearchTaskStatusRows(ctx context.Context, sessionID string) ([]biz.SearchTaskStatusRow, error) {
	if !r.DBOk() {
		return nil, nil
	}
	sessionID = strings.TrimSpace(sessionID)
	var tasks []BomSearchTask
	if err := r.db.WithContext(ctx).
		Where("session_id = ?", sessionID).
		Order("mpn_norm ASC, platform_id ASC").
		Find(&tasks).Error; err != nil {
		return nil, err
	}
	taskIDs := make([]string, 0, len(tasks))
	for _, t := range tasks {
		if t.CaichipTaskID.Valid && strings.TrimSpace(t.CaichipTaskID.String) != "" {
			taskIDs = append(taskIDs, strings.TrimSpace(t.CaichipTaskID.String))
		}
	}
	dispatchByID := make(map[string]CaichipDispatchTask, len(taskIDs))
	if len(taskIDs) > 0 {
		var dispatch []CaichipDispatchTask
		if err := r.db.WithContext(ctx).Where("task_id IN ?", taskIDs).Find(&dispatch).Error; err != nil {
			return nil, err
		}
		for _, d := range dispatch {
			dispatchByID[strings.TrimSpace(d.TaskID)] = d
		}
	}
	out := make([]biz.SearchTaskStatusRow, 0, len(tasks))
	for _, t := range tasks {
		row := biz.SearchTaskStatusRow{
			MpnNorm:       strings.TrimSpace(t.MpnNorm),
			PlatformID:    strings.TrimSpace(t.PlatformID),
			SearchState:   strings.ToLower(strings.TrimSpace(t.State)),
			Attempt:       t.AutoAttempt + t.ManualAttempt,
			LastError:     nullStringValue(t.LastError),
			UpdatedAt:     &t.UpdatedAt,
			CaichipTaskID: nullStringValue(t.CaichipTaskID),
		}
		if d, ok := dispatchByID[row.CaichipTaskID]; ok {
			row.DispatchState = strings.ToLower(strings.TrimSpace(d.State))
			row.DispatchResultStatus = nullStringValue(d.ResultStatus)
			row.Attempt = d.Attempt
			row.RetryMax = d.RetryMax
			row.LeasedToAgentID = nullStringValue(d.LeasedToAgentID)
			row.LeaseDeadlineAt = d.LeaseDeadlineAt
			if row.LastError == "" {
				row.LastError = nullStringValue(d.LastError)
			}
			row.UpdatedAt = &d.UpdatedAt
		}
		out = append(out, row)
	}
	return out, nil
}
```

Add helper to the same file:

```go
func nullStringValue(v sql.NullString) string {
	if !v.Valid {
		return ""
	}
	return strings.TrimSpace(v.String)
}
```

Add import:

```go
import "database/sql"
```

- [ ] **Step 5: 运行测试确认通过**

Run:

```powershell
& 'C:\Program Files\Go\bin\go.exe' test ./internal/data -run TestBOMSearchTaskRepo_ListSearchTaskStatusRows -count=1
```

Expected: PASS.

- [ ] **Step 6: 提交**

```powershell
git add -- internal/biz/repo.go internal/data/bom_search_task_status_repo.go internal/data/bom_search_task_status_repo_test.go
git commit -m "feat(data): read bom search task status rows"
```

## Task 3: proto 与 service 接口

**Files:**
- Modify: `api/bom/v1/bom.proto`
- Generate: `api/bom/v1/bom.pb.go`
- Generate: `api/bom/v1/bom_http.pb.go`
- Generate: `api/bom/v1/bom_grpc.pb.go`
- Modify: `internal/service/bom_service.go`
- Create or modify: `internal/service/bom_service_test_helpers_test.go`
- Create: `internal/service/bom_search_task_status_test.go`

- [ ] **Step 1: 写 service 失败测试**

Create `internal/service/bom_service_test_helpers_test.go` if the helper file is absent. The helper must define `newBomServiceForTest` and expose the fields used below:

```go
func newBomServiceForTest() (*BomService, *bomSessionRepoStub, *bomSearchTaskRepoStub) {
	session := &bomSessionRepoStub{sessionExists: true}
	search := &bomSearchTaskRepoStub{stateByKey: map[string]string{}}
	return NewBomService(session, search, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil), session, search
}
```

Extend `bomSessionRepoStub` with:

```go
type bomSessionRepoStub struct {
	mu            sync.Mutex
	sessionExists bool
	view          *biz.BOMSessionView
	lines         []biz.BOMSessionLineView
	replaced      bool
}
```

Ensure these methods exist on `bomSessionRepoStub`:

```go
func (r *bomSessionRepoStub) DBOk() bool { return true }

func (r *bomSessionRepoStub) GetSession(context.Context, string) (*biz.BOMSessionView, error) {
	if !r.sessionExists {
		return nil, data.ErrSessionNotFound
	}
	return r.view, nil
}

func (r *bomSessionRepoStub) ListSessionLines(context.Context, string) ([]biz.BOMSessionLineView, error) {
	return append([]biz.BOMSessionLineView(nil), r.lines...), nil
}
```

Extend `bomSearchTaskRepoStub` with:

```go
type bomSearchTaskRepoStub struct {
	mu          sync.Mutex
	upsertTasks bool
	statusRows  []biz.SearchTaskStatusRow
	upsertPairs []biz.MpnPlatformPair
	stateByKey  map[string]string
}
```

Ensure these methods exist on `bomSearchTaskRepoStub`:

```go
func (r *bomSearchTaskRepoStub) DBOk() bool { return true }

func (r *bomSearchTaskRepoStub) ListSearchTaskStatusRows(context.Context, string) ([]biz.SearchTaskStatusRow, error) {
	return append([]biz.SearchTaskStatusRow(nil), r.statusRows...), nil
}

func (r *bomSearchTaskRepoStub) GetTaskStateBySessionKey(_ context.Context, _ string, mpnNorm, platformID string, _ time.Time) (string, error) {
	if r.stateByKey == nil {
		return "", nil
	}
	return r.stateByKey[mpnNorm+"\x00"+platformID], nil
}

func (r *bomSearchTaskRepoStub) UpsertPendingTasks(_ context.Context, _ string, _ time.Time, _ int, pairs []biz.MpnPlatformPair) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.upsertTasks = true
	r.upsertPairs = append(r.upsertPairs, pairs...)
	return nil
}
```

If compiling the package shows additional interface methods missing, add deterministic no-op implementations to the same helper file. Each no-op should return a zero value and `nil` error, except methods used by existing tests, which must preserve their current behavior.

Create `internal/service/bom_search_task_status_test.go` with a stubbed service test matching existing service test patterns:

```go
package service

import (
	"context"
	"testing"
	"time"

	v1 "caichip/api/bom/v1"
	"caichip/internal/biz"
)

func TestListSessionSearchTasksIncludesMissingAndRetryable(t *testing.T) {
	svc, session, search := newBomServiceForTest()
	bizDate := time.Date(2026, 4, 25, 0, 0, 0, 0, time.UTC)
	session.view = &biz.BOMSessionView{
		SessionID: "session-1", Status: "searching", BizDate: bizDate,
		PlatformIDs: []string{"hqchip", "icgoo"}, SelectionRevision: 2,
	}
	session.lines = []biz.BOMSessionLineView{{ID: 12, LineNo: 12, Mpn: "TPS5430DDA"}}
	search.statusRows = []biz.SearchTaskStatusRow{{
		MpnNorm: "TPS5430DDA", PlatformID: "hqchip", SearchState: "failed_terminal",
		CaichipTaskID: "task-1", DispatchState: "failed_terminal", Attempt: 3,
		LastError: "timeout",
	}}

	resp, err := svc.ListSessionSearchTasks(context.Background(), &v1.ListSessionSearchTasksRequest{SessionId: "session-1"})
	if err != nil {
		t.Fatal(err)
	}
	if resp.GetSummary().GetTotal() != 2 || resp.GetSummary().GetFailed() != 1 || resp.GetSummary().GetMissing() != 1 {
		t.Fatalf("unexpected summary: %+v", resp.GetSummary())
	}
	if len(resp.GetTasks()) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(resp.GetTasks()))
	}
	if !resp.GetTasks()[0].GetRetryable() || resp.GetTasks()[0].GetSearchUiState() != "failed" {
		t.Fatalf("failed row should be retryable: %+v", resp.GetTasks()[0])
	}
	if resp.GetTasks()[1].GetSearchState() != "missing" || !resp.GetTasks()[1].GetRetryable() {
		t.Fatalf("missing row should be retryable: %+v", resp.GetTasks()[1])
	}
}

func TestRetrySearchTasksCreatesMissingPendingTask(t *testing.T) {
	svc, session, search := newBomServiceForTest()
	bizDate := time.Date(2026, 4, 25, 0, 0, 0, 0, time.UTC)
	session.view = &biz.BOMSessionView{
		SessionID: "session-1", BizDate: bizDate, PlatformIDs: []string{"hqchip"}, SelectionRevision: 7,
	}
	search.stateByKey = map[string]string{}

	resp, err := svc.RetrySearchTasks(context.Background(), &v1.RetrySearchTasksRequest{
		SessionId: "session-1",
		Items: []*v1.RetrySearchItem{{Mpn: "TPS5430DDA", PlatformId: "hqchip"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.GetAccepted() != 1 {
		t.Fatalf("expected accepted=1, got %d", resp.GetAccepted())
	}
	if len(search.upsertPairs) != 1 || search.upsertPairs[0].MpnNorm != "TPS5430DDA" || search.upsertPairs[0].PlatformID != "hqchip" {
		t.Fatalf("missing task should be upserted, got %+v", search.upsertPairs)
	}
}
```

- [ ] **Step 2: 运行测试确认失败**

Run:

```powershell
& 'C:\Program Files\Go\bin\go.exe' test ./internal/service -run 'TestListSessionSearchTasksIncludesMissingAndRetryable|TestRetrySearchTasksCreatesMissingPendingTask' -count=1
```

Expected: FAIL，提示 `ListSessionSearchTasks` proto 方法或 stub 字段未定义。

- [ ] **Step 3: 扩展 proto**

Modify `api/bom/v1/bom.proto` in `service BomService` after `GetSessionSearchTaskCoverage`:

```proto
  rpc ListSessionSearchTasks(ListSessionSearchTasksRequest) returns (ListSessionSearchTasksReply) {
    option (google.api.http) = {
      get: "/api/v1/bom-sessions/{session_id}/search-tasks"
    };
  }
```

Add messages after `GetSessionSearchTaskCoverageReply`:

```proto
message ListSessionSearchTasksRequest {
  string session_id = 1;
}

message SearchTaskStatusSummary {
  int32 total = 1;
  int32 pending = 2;
  int32 running = 3;
  int32 succeeded = 4;
  int32 no_result = 5;
  int32 failed = 6;
  int32 skipped = 7;
  int32 cancelled = 8;
  int32 missing = 9;
  int32 dispatch_pending = 10;
  int32 dispatch_leased = 11;
  int32 dispatch_finished = 12;
  int32 dispatch_failed = 13;
}

message SessionSearchTaskRow {
  string line_id = 1;
  int32 line_no = 2;
  string mpn = 3;
  string mpn_norm = 4;
  string platform_id = 5;
  string search_state = 6;
  string search_ui_state = 7;
  string caichip_task_id = 8;
  string dispatch_state = 9;
  string dispatch_result_status = 10;
  int32 attempt = 11;
  int32 retry_max = 12;
  string leased_to_agent_id = 13;
  string lease_deadline_at = 14;
  string last_error = 15;
  string updated_at = 16;
  bool retryable = 17;
}

message ListSessionSearchTasksReply {
  string session_id = 1;
  SearchTaskStatusSummary summary = 2;
  repeated SessionSearchTaskRow tasks = 3;
}
```

- [ ] **Step 4: 生成 proto 代码**

Run:

```powershell
make api
```

Expected: PASS，并更新 `api/bom/v1/bom.pb.go`, `api/bom/v1/bom_http.pb.go`, `api/bom/v1/bom_grpc.pb.go`。

- [ ] **Step 5: 实现 service 方法**

Add helper functions to `internal/service/bom_service.go` near existing session helpers:

```go
func searchTaskKey(mpnNorm, platformID string) string {
	return biz.NormalizeMPNForBOMSearch(mpnNorm) + "\x00" + biz.NormalizePlatformID(platformID)
}

func formatTimePtr(t *time.Time) string {
	if t == nil || t.IsZero() {
		return ""
	}
	return t.Format(time.RFC3339)
}
```

Add method:

```go
func (s *BomService) ListSessionSearchTasks(ctx context.Context, req *v1.ListSessionSearchTasksRequest) (*v1.ListSessionSearchTasksReply, error) {
	if !s.dbOK() {
		return nil, kerrors.ServiceUnavailable("DB_DISABLED", "database not configured")
	}
	sid := strings.TrimSpace(req.GetSessionId())
	view, err := s.session.GetSession(ctx, sid)
	if err != nil {
		return nil, err
	}
	lines, err := s.session.ListSessionLines(ctx, sid)
	if err != nil {
		return nil, err
	}
	existing, err := s.search.ListSearchTaskStatusRows(ctx, sid)
	if err != nil {
		return nil, err
	}
	byKey := make(map[string]biz.SearchTaskStatusRow, len(existing))
	for _, row := range existing {
		byKey[searchTaskKey(row.MpnNorm, row.PlatformID)] = row
	}
	rows := make([]biz.SearchTaskStatusRow, 0, len(lines)*len(view.PlatformIDs))
	for _, ln := range lines {
		mn := biz.NormalizeMPNForBOMSearch(ln.Mpn)
		for _, pidRaw := range view.PlatformIDs {
			pid := biz.NormalizePlatformID(pidRaw)
			row, ok := byKey[searchTaskKey(mn, pid)]
			if !ok {
				row = biz.SearchTaskStatusRow{SearchState: "missing"}
			}
			row.LineID = strconv.FormatInt(ln.ID, 10)
			row.LineNo = ln.LineNo
			row.MPN = ln.Mpn
			row.MpnNorm = mn
			row.PlatformID = pid
			row.SearchState = biz.NormalizeBOMSearchTaskState(row.SearchState)
			row.SearchUIState = biz.MapBOMSearchTaskUIState(row.SearchState)
			row.Retryable = biz.CanRetryBOMSearchTask(row.SearchState, biz.SearchTaskRetrySingleManual)
			rows = append(rows, row)
		}
	}
	summary := biz.BuildSearchTaskStatusSummary(rows)
	out := &v1.ListSessionSearchTasksReply{
		SessionId: sid,
		Summary: &v1.SearchTaskStatusSummary{
			Total: int32(summary.Total), Pending: int32(summary.Pending), Running: int32(summary.Running),
			Succeeded: int32(summary.Succeeded), NoResult: int32(summary.NoResult), Failed: int32(summary.Failed),
			Skipped: int32(summary.Skipped), Cancelled: int32(summary.Cancelled), Missing: int32(summary.Missing),
			DispatchPending: int32(summary.DispatchPending), DispatchLeased: int32(summary.DispatchLeased),
			DispatchFinished: int32(summary.DispatchFinished), DispatchFailed: int32(summary.DispatchFailed),
		},
		Tasks: make([]*v1.SessionSearchTaskRow, 0, len(rows)),
	}
	for _, row := range rows {
		out.Tasks = append(out.Tasks, &v1.SessionSearchTaskRow{
			LineId: row.LineID, LineNo: int32(row.LineNo), Mpn: row.MPN, MpnNorm: row.MpnNorm,
			PlatformId: row.PlatformID, SearchState: row.SearchState, SearchUiState: row.SearchUIState,
			CaichipTaskId: row.CaichipTaskID, DispatchState: row.DispatchState,
			DispatchResultStatus: row.DispatchResultStatus, Attempt: int32(row.Attempt),
			RetryMax: int32(row.RetryMax), LeasedToAgentId: row.LeasedToAgentID,
			LeaseDeadlineAt: formatTimePtr(row.LeaseDeadlineAt), LastError: row.LastError,
			UpdatedAt: formatTimePtr(row.UpdatedAt), Retryable: row.Retryable,
		})
	}
	return out, nil
}
```

Modify `RetrySearchTasks` missing branch:

```go
		if err != nil {
			continue
		}
		if cur == "" {
			if err := s.search.UpsertPendingTasks(ctx, sid, view.BizDate, view.SelectionRevision, []biz.MpnPlatformPair{{MpnNorm: mn, PlatformID: pid}}); err != nil {
				continue
			}
			n++
			continue
		}
```

- [ ] **Step 6: 更新测试 stub**

In `internal/service/bom_service_test_helpers_test.go`, add fields and methods to the existing search repo stub:

```go
	statusRows []biz.SearchTaskStatusRow
	upsertPairs []biz.MpnPlatformPair
	stateByKey map[string]string
```

Add method:

```go
func (r *bomSearchTaskRepoStub) ListSearchTaskStatusRows(context.Context, string) ([]biz.SearchTaskStatusRow, error) {
	return append([]biz.SearchTaskStatusRow(nil), r.statusRows...), nil
}
```

Ensure existing `GetTaskStateBySessionKey` reads `stateByKey[mpnNorm+"\x00"+platformID]` when present, and existing `UpsertPendingTasks` appends to `upsertPairs`.

- [ ] **Step 7: 运行 service 测试确认通过**

Run:

```powershell
& 'C:\Program Files\Go\bin\go.exe' test ./internal/service -run 'TestListSessionSearchTasksIncludesMissingAndRetryable|TestRetrySearchTasksCreatesMissingPendingTask' -count=1
```

Expected: PASS.

- [ ] **Step 8: 提交**

```powershell
git add -- api/bom/v1/bom.proto api/bom/v1/bom.pb.go api/bom/v1/bom_http.pb.go api/bom/v1/bom_grpc.pb.go internal/service/bom_service.go internal/service/bom_service_test_helpers_test.go internal/service/bom_search_task_status_test.go
git commit -m "feat(api): expose bom search task statuses"
```

## Task 4: 前端 API 与状态面板组件

**Files:**
- Modify: `web/src/api/types.ts`
- Modify: `web/src/api/bomSession.ts`
- Modify: `web/src/api/bomSession.test.ts`
- Create: `web/src/pages/sourcing-session/SearchTaskStatusPanel.tsx`
- Create: `web/src/pages/sourcing-session/SearchTaskStatusPanel.test.tsx`

- [ ] **Step 1: 写前端 API 失败测试**

Append to `web/src/api/bomSession.test.ts`:

```ts
it('parses session search task status rows', async () => {
  mockFetchJson.mockResolvedValueOnce({
    session_id: 'session-1',
    summary: { total: 2, failed: 1, no_result: 1, dispatch_failed: 1 },
    tasks: [
      {
        line_id: '12',
        line_no: 12,
        mpn: 'TPS5430DDA',
        mpn_norm: 'TPS5430DDA',
        platform_id: 'hqchip',
        search_state: 'failed_terminal',
        search_ui_state: 'failed',
        caichip_task_id: 'task-1',
        dispatch_state: 'failed_terminal',
        dispatch_result_status: 'failed',
        attempt: 3,
        retry_max: 4,
        leased_to_agent_id: 'agent-1',
        lease_deadline_at: '2026-04-25T12:00:00Z',
        last_error: 'timeout',
        updated_at: '2026-04-25T12:01:00Z',
        retryable: true,
      },
    ],
  })

  const out = await listSessionSearchTasks('session-1')

  expect(mockFetchJson).toHaveBeenCalledWith('/api/v1/bom-sessions/session-1/search-tasks')
  expect(out.summary.total).toBe(2)
  expect(out.tasks[0].retryable).toBe(true)
  expect(out.tasks[0].dispatch_state).toBe('failed_terminal')
})
```

- [ ] **Step 2: 写组件失败测试**

Create `web/src/pages/sourcing-session/SearchTaskStatusPanel.test.tsx`:

```tsx
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'
import { SearchTaskStatusPanel } from './SearchTaskStatusPanel'

const reply = {
  session_id: 'session-1',
  summary: {
    total: 3, pending: 0, running: 0, succeeded: 1, no_result: 1, failed: 1,
    skipped: 0, cancelled: 0, missing: 0, dispatch_pending: 0, dispatch_leased: 0,
    dispatch_finished: 2, dispatch_failed: 1,
  },
  tasks: [
    { line_id: '1', line_no: 1, mpn: 'OK', mpn_norm: 'OK', platform_id: 'icgoo', search_state: 'succeeded', search_ui_state: 'succeeded', retryable: false },
    { line_id: '2', line_no: 2, mpn: 'EMPTY', mpn_norm: 'EMPTY', platform_id: 'szlcsc', search_state: 'no_result', search_ui_state: 'no_data', retryable: true },
    { line_id: '3', line_no: 3, mpn: 'FAIL', mpn_norm: 'FAIL', platform_id: 'hqchip', search_state: 'failed_terminal', search_ui_state: 'failed', dispatch_state: 'failed_terminal', attempt: 3, retryable: true },
  ],
}

describe('SearchTaskStatusPanel', () => {
  it('renders summary and allows batch retry for anomaly tasks only', async () => {
    const onRetry = vi.fn()
    render(<SearchTaskStatusPanel data={reply} loading={false} onRefresh={vi.fn()} onRetry={onRetry} />)
    expect(screen.getByText('搜索任务状态')).toBeInTheDocument()
    expect(screen.getByText('无报价')).toBeInTheDocument()

    await userEvent.click(screen.getByRole('button', { name: '重试异常任务' }))

    expect(onRetry).toHaveBeenCalledWith([{ mpn: 'FAIL', platform_id: 'hqchip' }])
  })

  it('allows single retry for no_result', async () => {
    const onRetry = vi.fn()
    render(<SearchTaskStatusPanel data={reply} loading={false} onRefresh={vi.fn()} onRetry={onRetry} />)

    await userEvent.click(screen.getAllByRole('button', { name: '重试' })[0])

    expect(onRetry).toHaveBeenCalledWith([{ mpn: 'EMPTY', platform_id: 'szlcsc' }])
  })
})
```

- [ ] **Step 3: 运行前端测试确认失败**

Run:

```powershell
Set-Location web
& 'D:\Program Files\nodejs\npm.cmd' test -- src/api/bomSession.test.ts src/pages/sourcing-session/SearchTaskStatusPanel.test.tsx
```

Expected: FAIL，提示 `listSessionSearchTasks` 或 `SearchTaskStatusPanel` 未定义。

- [ ] **Step 4: 新增前端类型**

Modify `web/src/api/types.ts`:

```ts
export interface SearchTaskStatusSummary {
  total: number
  pending: number
  running: number
  succeeded: number
  no_result: number
  failed: number
  skipped: number
  cancelled: number
  missing: number
  dispatch_pending: number
  dispatch_leased: number
  dispatch_finished: number
  dispatch_failed: number
}

export interface SessionSearchTaskRow {
  line_id: string
  line_no: number
  mpn: string
  mpn_norm: string
  platform_id: string
  search_state: string
  search_ui_state: string
  caichip_task_id?: string
  dispatch_state?: string
  dispatch_result_status?: string
  attempt: number
  retry_max: number
  leased_to_agent_id?: string
  lease_deadline_at?: string
  last_error?: string
  updated_at?: string
  retryable: boolean
}

export interface ListSessionSearchTasksReply {
  session_id: string
  summary: SearchTaskStatusSummary
  tasks: SessionSearchTaskRow[]
}
```

- [ ] **Step 5: 新增 API 解析**

Modify imports in `web/src/api/bomSession.ts` to include `ListSessionSearchTasksReply`.

Add function:

```ts
export async function listSessionSearchTasks(
  sessionId: string
): Promise<ListSessionSearchTasksReply> {
  const json = await fetchJson<Record<string, unknown>>(
    `${BASE}/${encodeURIComponent(sessionId)}/search-tasks`
  )
  const s = (json.summary ?? {}) as Record<string, unknown>
  const tasksRaw = (json.tasks ?? []) as Record<string, unknown>[]
  return {
    session_id: str(json.session_id ?? json.sessionId),
    summary: {
      total: num(s.total, 0),
      pending: num(s.pending, 0),
      running: num(s.running, 0),
      succeeded: num(s.succeeded, 0),
      no_result: num(s.no_result ?? s.noResult, 0),
      failed: num(s.failed, 0),
      skipped: num(s.skipped, 0),
      cancelled: num(s.cancelled, 0),
      missing: num(s.missing, 0),
      dispatch_pending: num(s.dispatch_pending ?? s.dispatchPending, 0),
      dispatch_leased: num(s.dispatch_leased ?? s.dispatchLeased, 0),
      dispatch_finished: num(s.dispatch_finished ?? s.dispatchFinished, 0),
      dispatch_failed: num(s.dispatch_failed ?? s.dispatchFailed, 0),
    },
    tasks: tasksRaw.map((row) => ({
      line_id: str(row.line_id ?? row.lineId),
      line_no: num(row.line_no ?? row.lineNo, 0),
      mpn: str(row.mpn),
      mpn_norm: str(row.mpn_norm ?? row.mpnNorm),
      platform_id: str(row.platform_id ?? row.platformId),
      search_state: str(row.search_state ?? row.searchState),
      search_ui_state: str(row.search_ui_state ?? row.searchUiState),
      caichip_task_id: str(row.caichip_task_id ?? row.caichipTaskId) || undefined,
      dispatch_state: str(row.dispatch_state ?? row.dispatchState) || undefined,
      dispatch_result_status: str(row.dispatch_result_status ?? row.dispatchResultStatus) || undefined,
      attempt: num(row.attempt, 0),
      retry_max: num(row.retry_max ?? row.retryMax, 0),
      leased_to_agent_id: str(row.leased_to_agent_id ?? row.leasedToAgentId) || undefined,
      lease_deadline_at: str(row.lease_deadline_at ?? row.leaseDeadlineAt) || undefined,
      last_error: str(row.last_error ?? row.lastError) || undefined,
      updated_at: str(row.updated_at ?? row.updatedAt) || undefined,
      retryable: Boolean(row.retryable),
    })),
  }
}
```

- [ ] **Step 6: 新增状态面板组件**

Create `web/src/pages/sourcing-session/SearchTaskStatusPanel.tsx`:

```tsx
import type { ListSessionSearchTasksReply, SessionSearchTaskRow } from '../../api'

interface SearchTaskStatusPanelProps {
  data: ListSessionSearchTasksReply | null
  loading: boolean
  onRefresh: () => void
  onRetry: (items: { mpn: string; platform_id: string }[]) => void
}

const labels: Record<string, string> = {
  pending: '待执行',
  searching: '搜索中',
  succeeded: '已完成',
  no_data: '无报价',
  failed: '失败',
  skipped: '已跳过',
  cancelled: '已取消',
  missing: '任务缺失',
}

function statusLabel(row: SessionSearchTaskRow) {
  return labels[row.search_ui_state] ?? row.search_ui_state || row.search_state || '-'
}

function isBatchRetryRow(row: SessionSearchTaskRow) {
  return row.retryable && row.search_ui_state !== 'no_data'
}

export function SearchTaskStatusPanel({ data, loading, onRefresh, onRetry }: SearchTaskStatusPanelProps) {
  const tasks = data?.tasks ?? []
  const retryableAnomalies = tasks.filter(isBatchRetryRow)

  return (
    <section className="rounded-xl border border-slate-200 bg-white p-6 shadow-sm">
      <div className="mb-4 flex flex-wrap items-center justify-between gap-3">
        <div>
          <h3 className="font-semibold text-slate-800">搜索任务状态</h3>
          <p className="mt-1 text-sm text-slate-500">查看每个平台任务是否仍在排队、执行或已结束。</p>
        </div>
        <div className="flex flex-wrap gap-2">
          <button type="button" onClick={onRefresh} className="rounded-lg border border-slate-300 bg-white px-3 py-1.5 text-sm hover:bg-slate-50">
            刷新
          </button>
          <button
            type="button"
            disabled={retryableAnomalies.length === 0}
            onClick={() => onRetry(retryableAnomalies.map((row) => ({ mpn: row.mpn, platform_id: row.platform_id })))}
            className="rounded-lg bg-blue-600 px-3 py-1.5 text-sm font-medium text-white hover:bg-blue-700 disabled:bg-slate-200 disabled:text-slate-400"
          >
            重试异常任务
          </button>
        </div>
      </div>
      {loading ? (
        <div className="py-8 text-center text-sm text-slate-500">正在加载搜索任务...</div>
      ) : !data || tasks.length === 0 ? (
        <div className="py-8 text-center text-sm text-slate-500">当前没有搜索任务</div>
      ) : (
        <>
          <div className="mb-4 grid gap-3 sm:grid-cols-2 lg:grid-cols-7">
            {[
              ['全部', data.summary.total],
              ['待执行', data.summary.pending],
              ['执行中', data.summary.running],
              ['已完成', data.summary.succeeded],
              ['无报价', data.summary.no_result],
              ['失败', data.summary.failed],
              ['缺失', data.summary.missing],
            ].map(([label, value]) => (
              <div key={label} className="rounded-lg border border-slate-200 bg-slate-50 px-3 py-2">
                <div className="text-lg font-semibold text-slate-800">{value}</div>
                <div className="text-xs text-slate-500">{label}</div>
              </div>
            ))}
          </div>
          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead>
                <tr className="border-b border-slate-200 bg-slate-50 text-left">
                  <th className="px-2 py-2">行</th>
                  <th className="px-2 py-2">MPN</th>
                  <th className="px-2 py-2">平台</th>
                  <th className="px-2 py-2">搜索状态</th>
                  <th className="px-2 py-2">Agent 状态</th>
                  <th className="px-2 py-2">尝试</th>
                  <th className="px-2 py-2">最后错误</th>
                  <th className="px-2 py-2">操作</th>
                </tr>
              </thead>
              <tbody>
                {tasks.map((row) => (
                  <tr key={`${row.line_id}-${row.platform_id}`} className="border-b border-slate-100 align-top">
                    <td className="px-2 py-2">{row.line_no}</td>
                    <td className="px-2 py-2 font-mono">{row.mpn}</td>
                    <td className="px-2 py-2 font-mono">{row.platform_id}</td>
                    <td className="px-2 py-2">{statusLabel(row)}</td>
                    <td className="px-2 py-2">{row.dispatch_state || '-'}</td>
                    <td className="px-2 py-2">{row.retry_max ? `${row.attempt}/${row.retry_max}` : row.attempt}</td>
                    <td className="max-w-xs px-2 py-2 text-xs text-slate-500">{row.last_error || '-'}</td>
                    <td className="px-2 py-2">
                      {row.retryable ? (
                        <button type="button" onClick={() => onRetry([{ mpn: row.mpn, platform_id: row.platform_id }])} className="text-xs text-blue-600 hover:underline">
                          重试
                        </button>
                      ) : null}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </>
      )}
    </section>
  )
}
```

- [ ] **Step 7: 运行前端测试确认通过**

Run:

```powershell
Set-Location web
& 'D:\Program Files\nodejs\npm.cmd' test -- src/api/bomSession.test.ts src/pages/sourcing-session/SearchTaskStatusPanel.test.tsx
```

Expected: PASS.

- [ ] **Step 8: 提交**

```powershell
git add -- web/src/api/types.ts web/src/api/bomSession.ts web/src/api/bomSession.test.ts web/src/pages/sourcing-session/SearchTaskStatusPanel.tsx web/src/pages/sourcing-session/SearchTaskStatusPanel.test.tsx
git commit -m "feat(web): add bom search task status panel"
```

## Task 5: 页面集成与全量验证

**Files:**
- Modify: `web/src/pages/SourcingSessionPage.tsx`
- Modify: `web/src/pages/SourcingSessionPage.test.tsx`

- [ ] **Step 1: 写页面集成失败测试**

Modify `web/src/pages/SourcingSessionPage.test.tsx` mock setup:

```ts
const {
  getSession,
  getBOMLines,
  getSessionSearchTaskCoverage,
  listSessionSearchTasks,
  createSessionLine,
  deleteSessionLine,
  exportSessionFile,
  patchSession,
  patchSessionLine,
  putPlatforms,
  retrySearchTasks,
} = vi.hoisted(() => ({
  getSession: vi.fn(),
  getBOMLines: vi.fn(),
  getSessionSearchTaskCoverage: vi.fn(),
  listSessionSearchTasks: vi.fn(),
  createSessionLine: vi.fn(),
  deleteSessionLine: vi.fn(),
  exportSessionFile: vi.fn(),
  patchSession: vi.fn(),
  patchSessionLine: vi.fn(),
  putPlatforms: vi.fn(),
  retrySearchTasks: vi.fn(),
}))
```

Add to mocked API exports:

```ts
listSessionSearchTasks,
```

Add default mock:

```ts
listSessionSearchTasks.mockResolvedValue({
  session_id: 'session-1',
  summary: {
    total: 1, pending: 0, running: 0, succeeded: 0, no_result: 0, failed: 1,
    skipped: 0, cancelled: 0, missing: 0, dispatch_pending: 0, dispatch_leased: 0,
    dispatch_finished: 0, dispatch_failed: 1,
  },
  tasks: [
    {
      line_id: '1', line_no: 1, mpn: 'FAIL', mpn_norm: 'FAIL', platform_id: 'hqchip',
      search_state: 'failed_terminal', search_ui_state: 'failed', retryable: true,
      attempt: 3, retry_max: 4, dispatch_state: 'failed_terminal',
    },
  ],
})
```

Add test:

```tsx
it('shows search task status and retries anomaly tasks', async () => {
  getSession.mockResolvedValueOnce({ ...baseSession, import_status: 'ready', status: 'searching' })
  render(<SourcingSessionPage sessionId="session-1" onEnterMatch={vi.fn()} />)

  await act(async () => {
    await flushAsyncWork()
  })

  expect(screen.getByText('搜索任务状态')).toBeInTheDocument()

  await act(async () => {
    screen.getByRole('button', { name: '重试异常任务' }).click()
    await flushAsyncWork()
  })

  expect(retrySearchTasks).toHaveBeenCalledWith('session-1', [{ mpn: 'FAIL', platform_id: 'hqchip' }])
  expect(listSessionSearchTasks).toHaveBeenCalledTimes(2)
})
```

- [ ] **Step 2: 运行页面测试确认失败**

Run:

```powershell
Set-Location web
& 'D:\Program Files\nodejs\npm.cmd' test -- src/pages/SourcingSessionPage.test.tsx
```

Expected: FAIL，页面未渲染 `搜索任务状态`。

- [ ] **Step 3: 集成页面加载与刷新**

Modify imports in `web/src/pages/SourcingSessionPage.tsx`:

```ts
  listSessionSearchTasks,
  type ListSessionSearchTasksReply,
```

Import component:

```ts
import { SearchTaskStatusPanel } from './sourcing-session/SearchTaskStatusPanel'
```

Add state:

```ts
const [searchTasks, setSearchTasks] = useState<ListSessionSearchTasksReply | null>(null)
const [searchTasksLoading, setSearchTasksLoading] = useState(false)
```

Add loader:

```ts
const loadSearchTasks = useCallback(async () => {
  setSearchTasksLoading(true)
  try {
    const rows = await listSessionSearchTasks(sessionId)
    setSearchTasks(rows)
  } catch {
    setSearchTasks(null)
  } finally {
    setSearchTasksLoading(false)
  }
}, [sessionId])
```

Call it in initial effect after `loadLines()`:

```ts
await loadSearchTasks()
```

Update refresh button:

```tsx
onClick={() => void Promise.all([loadSession(), loadLines(), loadSearchTasks()])}
```

Add retry handler:

```ts
const handleRetrySearchTasks = async (items: { mpn: string; platform_id: string }[]) => {
  if (items.length === 0) {
    setErr('暂无可重试的异常任务')
    return
  }
  try {
    await retrySearchTasks(sessionId, items)
    await Promise.all([loadLines(), loadSearchTasks()])
  } catch (e) {
    setErr(e instanceof Error ? e.message : '重试搜索任务失败')
  }
}
```

Render before BOM lines section:

```tsx
<SearchTaskStatusPanel
  data={searchTasks}
  loading={searchTasksLoading}
  onRefresh={() => void loadSearchTasks()}
  onRetry={(items) => void handleRetrySearchTasks(items)}
/>
```

Change `handleRetryFirstGap` to call `handleRetrySearchTasks([{ mpn: row.mpn, platform_id: g.platform_id }])` or remove the old “重试第一个缺口” button if the new panel covers the workflow.

- [ ] **Step 4: 运行页面测试确认通过**

Run:

```powershell
Set-Location web
& 'D:\Program Files\nodejs\npm.cmd' test -- src/pages/SourcingSessionPage.test.tsx
```

Expected: PASS.

- [ ] **Step 5: 运行后端目标测试**

Run:

```powershell
& 'C:\Program Files\Go\bin\go.exe' test ./internal/biz ./internal/data ./internal/service -run 'BOMSearchTask|SessionSearchTasks|RetrySearchTasks' -count=1
```

Expected: PASS.

- [ ] **Step 6: 运行前端目标测试**

Run:

```powershell
Set-Location web
& 'D:\Program Files\nodejs\npm.cmd' test -- src/api/bomSession.test.ts src/pages/sourcing-session/SearchTaskStatusPanel.test.tsx src/pages/SourcingSessionPage.test.tsx
```

Expected: PASS.

- [ ] **Step 7: 构建前端**

Run:

```powershell
Set-Location web
& 'D:\Program Files\nodejs\npm.cmd' run build
```

Expected: PASS.

- [ ] **Step 8: 提交**

```powershell
git add -- web/src/pages/SourcingSessionPage.tsx web/src/pages/SourcingSessionPage.test.tsx
git commit -m "feat(web): show bom search task status in sessions"
```

## Final Verification

- [ ] Run backend targeted tests:

```powershell
& 'C:\Program Files\Go\bin\go.exe' test ./internal/biz ./internal/data ./internal/service -run 'BOMSearchTask|SessionSearchTasks|RetrySearchTasks' -count=1
```

- [ ] Run frontend targeted tests:

```powershell
Set-Location web
& 'D:\Program Files\nodejs\npm.cmd' test -- src/api/bomSession.test.ts src/pages/sourcing-session/SearchTaskStatusPanel.test.tsx src/pages/SourcingSessionPage.test.tsx
```

- [ ] Run frontend build:

```powershell
Set-Location web
& 'D:\Program Files\nodejs\npm.cmd' run build
```

- [ ] Inspect `git diff --stat HEAD` and confirm only planned files changed.

## Self-Review

- Spec coverage: The plan covers dedicated search task API, business/dispatch status separation, summary and detail UI, batch anomaly retry, single-row retry for `no_result`, missing task recovery, and frontend tests.
- Placeholder scan: The plan contains no unresolved placeholders.
- Type consistency: `SearchTaskStatusRow`, `SearchTaskStatusSummary`, `ListSessionSearchTasksReply`, `SessionSearchTaskRow`, `search_ui_state`, and `retryable` are named consistently across Go, proto, TypeScript, and tests.
