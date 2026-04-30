package service

import (
	"context"
	"sync"
	"testing"
	"time"

	v1 "caichip/api/bom/v1"
	"caichip/internal/biz"

	kerrors "github.com/go-kratos/kratos/v2/errors"
)

func TestListSessionSearchTasksIncludesMissingAndRetryable(t *testing.T) {
	svc, session, search := newBomSearchTaskStatusServiceTest()
	bizDate := time.Date(2026, 4, 25, 0, 0, 0, 0, time.UTC)
	session.view = &biz.BOMSessionView{
		SessionID: "session-1", Status: "searching", BizDate: bizDate,
		PlatformIDs: []string{"hqchip", "icgoo"}, SelectionRevision: 2,
	}
	session.lines = []biz.BOMSessionLineView{{ID: 12, LineNo: 12, Mpn: "TPS5430DDA"}}
	search.statusRows = []biz.SearchTaskStatusRow{{
		MpnNorm: "TPS5430DDA", PlatformID: "hqchip", SearchTaskState: "failed_terminal",
		DispatchTaskID: "task-1", DispatchTaskState: "failed_terminal", Attempt: 3,
		LastError: "timeout",
	}}

	resp, err := svc.ListSessionSearchTasks(context.Background(), &v1.ListSessionSearchTasksRequest{SessionId: "session-1"})
	if err != nil {
		t.Fatal(err)
	}
	if resp.GetSummary().GetTotal() != 2 || resp.GetSummary().GetFailed() != 1 ||
		resp.GetSummary().GetMissing() != 1 || resp.GetSummary().GetRetryable() != 2 {
		t.Fatalf("unexpected summary: %+v", resp.GetSummary())
	}
	if len(resp.GetTasks()) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(resp.GetTasks()))
	}
	if !resp.GetTasks()[0].GetRetryable() || resp.GetTasks()[0].GetSearchUiState() != "failed" {
		t.Fatalf("failed row should be retryable: %+v", resp.GetTasks()[0])
	}
	if resp.GetTasks()[1].GetSearchTaskState() != "missing" || !resp.GetTasks()[1].GetRetryable() {
		t.Fatalf("missing row should be retryable: %+v", resp.GetTasks()[1])
	}
}

func TestListSessionSearchTasksReconcilesFinishedDispatchWithoutStdout(t *testing.T) {
	svc, session, search := newBomSearchTaskStatusServiceTest()
	bizDate := time.Date(2026, 4, 25, 0, 0, 0, 0, time.UTC)
	session.view = &biz.BOMSessionView{
		SessionID: "session-1", Status: "searching", BizDate: bizDate,
		PlatformIDs: []string{"hqchip"}, SelectionRevision: 2,
	}
	session.lines = []biz.BOMSessionLineView{{ID: 12, LineNo: 12, Mpn: "TPS5430DDA"}}
	search.statusRows = []biz.SearchTaskStatusRow{{
		MpnNorm: "TPS5430DDA", PlatformID: "hqchip", SearchTaskState: "running",
		DispatchTaskID: "task-1", DispatchTaskState: "finished", DispatchResult: "success",
	}}

	resp, err := svc.ListSessionSearchTasks(context.Background(), &v1.ListSessionSearchTasksRequest{SessionId: "session-1"})
	if err != nil {
		t.Fatal(err)
	}
	if search.finalState != "failed_terminal" || search.lastErr == "" {
		t.Fatalf("expected stuck finished dispatch to finalize failed, state=%q err=%q", search.finalState, search.lastErr)
	}
	if resp.GetSummary().GetSearching() != 0 || resp.GetSummary().GetFailed() != 1 || resp.GetSummary().GetRetryable() != 1 {
		t.Fatalf("unexpected summary: %+v", resp.GetSummary())
	}
	if got := resp.GetTasks()[0]; got.GetSearchUiState() != "failed" || !got.GetRetryable() {
		t.Fatalf("expected failed retryable row, got %+v", got)
	}
	if session.setStatus != "data_ready" {
		t.Fatalf("expected session data_ready after reconciliation, got %q", session.setStatus)
	}
}

func TestRetrySearchTasksCreatesMissingPendingTask(t *testing.T) {
	svc, session, search := newBomSearchTaskStatusServiceTest()
	bizDate := time.Date(2026, 4, 25, 0, 0, 0, 0, time.UTC)
	session.view = &biz.BOMSessionView{
		SessionID: "session-1", BizDate: bizDate, PlatformIDs: []string{"hqchip"}, SelectionRevision: 7,
	}
	search.stateByKey = map[string]string{}

	resp, err := svc.RetrySearchTasks(context.Background(), &v1.RetrySearchTasksRequest{
		SessionId: "session-1",
		Items:     []*v1.RetrySearchItem{{Mpn: "TPS5430DDA", PlatformId: "hqchip"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.GetAccepted() != 1 {
		t.Fatalf("expected accepted=1, got %d", resp.GetAccepted())
	}
	if len(search.upsertPairs) != 1 || search.upsertPairs[0].MpnNorm != "TPS5430DDA" ||
		search.upsertPairs[0].PlatformID != "hqchip" {
		t.Fatalf("missing task should be upserted, got %+v", search.upsertPairs)
	}
}

func newBomSearchTaskStatusServiceTest() (*BomService, *searchTaskStatusSessionRepoStub, *searchTaskStatusRepoStub) {
	session := &searchTaskStatusSessionRepoStub{sessionExists: true}
	search := &searchTaskStatusRepoStub{stateByKey: map[string]string{}}
	return NewBomService(session, search, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil), session, search
}

type searchTaskStatusSessionRepoStub struct {
	sessionExists bool
	view          *biz.BOMSessionView
	lines         []biz.BOMSessionLineView
	setStatus     string
}

func (r *searchTaskStatusSessionRepoStub) DBOk() bool { return true }

func (r *searchTaskStatusSessionRepoStub) CreateSession(context.Context, string, []string, *string, *string, *string, *string, *string) (string, time.Time, int, error) {
	return "", time.Time{}, 0, nil
}

func (r *searchTaskStatusSessionRepoStub) GetSession(context.Context, string) (*biz.BOMSessionView, error) {
	if !r.sessionExists {
		return nil, kerrors.NotFound("SESSION_NOT_FOUND", "session not found")
	}
	return r.view, nil
}

func (r *searchTaskStatusSessionRepoStub) PatchSession(context.Context, string, *string, *string, *string, *string, *string, *string) error {
	return nil
}

func (r *searchTaskStatusSessionRepoStub) PutPlatforms(context.Context, string, []string, int32) (int, error) {
	return 0, nil
}

func (r *searchTaskStatusSessionRepoStub) ListSessions(context.Context, int32, int32, string, string, string) ([]biz.BOMSessionListItem, int32, error) {
	return nil, 0, nil
}

func (r *searchTaskStatusSessionRepoStub) ReplaceSessionLines(context.Context, string, []biz.BomImportLine, *string) (int, error) {
	return 0, nil
}

func (r *searchTaskStatusSessionRepoStub) ListSessionLines(context.Context, string) ([]biz.BOMSessionLineView, error) {
	return append([]biz.BOMSessionLineView(nil), r.lines...), nil
}

func (r *searchTaskStatusSessionRepoStub) SetSessionStatus(_ context.Context, _ string, status string) error {
	r.setStatus = status
	return nil
}

func (r *searchTaskStatusSessionRepoStub) CreateSessionLine(context.Context, string, string, string, string, string, string, string, string, string, *string, *float64, *string, *string) (int64, int32, int, error) {
	return 0, 0, 0, nil
}

func (r *searchTaskStatusSessionRepoStub) DeleteSessionLine(context.Context, string, int64) error {
	return nil
}

func (r *searchTaskStatusSessionRepoStub) UpdateSessionLine(context.Context, string, int64, *string, *string, *string, *string, *string, *string, *string, *string, biz.OptionalStringPtr, *float64, *string, *string) (int, error) {
	return 0, nil
}

func (r *searchTaskStatusSessionRepoStub) TryStartImport(context.Context, string, string) (bool, error) {
	return false, nil
}

func (r *searchTaskStatusSessionRepoStub) UpdateImportState(context.Context, string, biz.BOMImportStatePatch) error {
	return nil
}

type searchTaskStatusRepoStub struct {
	mu          sync.Mutex
	statusRows  []biz.SearchTaskStatusRow
	stateByKey  map[string]string
	upsertPairs []biz.MpnPlatformPair
	finalState  string
	lastErr     string
}

func (r *searchTaskStatusRepoStub) DBOk() bool { return true }

func (r *searchTaskStatusRepoStub) LoadSearchTaskByCaichipTaskID(context.Context, string) (*biz.BOMSearchTaskLookup, error) {
	return nil, nil
}

func (r *searchTaskStatusRepoStub) FinalizeSearchTask(_ context.Context, _, mpnNorm, platformID string, _ time.Time, _ string, state string, lastErr *string, _ string, _, _ []byte) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.finalState = state
	if lastErr != nil {
		r.lastErr = *lastErr
	}
	for i := range r.statusRows {
		if r.statusRows[i].MpnNorm == mpnNorm && r.statusRows[i].PlatformID == platformID {
			r.statusRows[i].SearchTaskState = state
			r.statusRows[i].LastError = r.lastErr
		}
	}
	return nil
}

func (r *searchTaskStatusRepoStub) ListSearchTaskStatusRows(context.Context, string) ([]biz.SearchTaskStatusRow, error) {
	return append([]biz.SearchTaskStatusRow(nil), r.statusRows...), nil
}

func (r *searchTaskStatusRepoStub) ListTasksForSession(context.Context, string) ([]biz.TaskReadinessSnapshot, error) {
	out := make([]biz.TaskReadinessSnapshot, 0, len(r.statusRows))
	for _, row := range r.statusRows {
		out = append(out, biz.TaskReadinessSnapshot{
			MpnNorm:    row.MpnNorm,
			PlatformID: row.PlatformID,
			State:      row.SearchTaskState,
		})
	}
	return out, nil
}

func (r *searchTaskStatusRepoStub) ListActiveBySession(context.Context, string) ([]biz.TaskReadinessSnapshot, error) {
	return nil, nil
}

func (r *searchTaskStatusRepoStub) CancelBySessionPlatform(context.Context, string, string) (int64, error) {
	return 0, nil
}

func (r *searchTaskStatusRepoStub) MarkSkippedBySessionPlatform(context.Context, string, string) (int64, error) {
	return 0, nil
}

func (r *searchTaskStatusRepoStub) CancelAllTasksBySession(context.Context, string) error { return nil }

func (r *searchTaskStatusRepoStub) CancelTasksBySessionMpnNorm(context.Context, string, string) error {
	return nil
}

func (r *searchTaskStatusRepoStub) UpsertPendingTasks(_ context.Context, _ string, _ time.Time, _ int, pairs []biz.MpnPlatformPair) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.upsertPairs = append(r.upsertPairs, pairs...)
	return nil
}

func (r *searchTaskStatusRepoStub) GetTaskStateBySessionKey(_ context.Context, _ string, mpnNorm, platformID string, _ time.Time) (string, error) {
	if r.stateByKey == nil {
		return "", nil
	}
	return r.stateByKey[mpnNorm+"\x00"+platformID], nil
}

func (r *searchTaskStatusRepoStub) UpdateTaskStateBySessionKey(context.Context, string, string, string, time.Time, string) error {
	return nil
}

func (r *searchTaskStatusRepoStub) ListSearchTaskLookupsByCaichipTaskID(context.Context, string) ([]biz.BOMSearchTaskLookup, error) {
	return nil, nil
}

func (r *searchTaskStatusRepoStub) ListPendingLookupsByMergeKey(context.Context, string, string, time.Time) ([]biz.BOMSearchTaskLookup, error) {
	return nil, nil
}

func (r *searchTaskStatusRepoStub) LoadQuoteCacheByMergeKey(context.Context, string, string, time.Time) (*biz.QuoteCacheSnapshot, bool, error) {
	return nil, false, nil
}

func (r *searchTaskStatusRepoStub) LoadQuoteCachesForKeys(context.Context, time.Time, []biz.MpnPlatformPair) (map[string]*biz.QuoteCacheSnapshot, error) {
	return nil, nil
}

func (r *searchTaskStatusRepoStub) DistinctPendingMergeKeysForSession(context.Context, string) ([]biz.MergeKey, error) {
	return nil, nil
}

func (r *searchTaskStatusRepoStub) UpsertManualQuote(context.Context, uint64, biz.AgentQuoteRow) error {
	return nil
}
