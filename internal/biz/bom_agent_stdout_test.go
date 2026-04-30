package biz

import (
	"context"
	"testing"
	"time"
)

type stdoutSearchRepoStub struct {
	lookups    []BOMSearchTaskLookup
	tasks      []TaskReadinessSnapshot
	finalState string
	lastErr    string
}

func (r *stdoutSearchRepoStub) DBOk() bool { return true }

func (r *stdoutSearchRepoStub) LoadSearchTaskByCaichipTaskID(context.Context, string) (*BOMSearchTaskLookup, error) {
	return nil, nil
}

func (r *stdoutSearchRepoStub) FinalizeSearchTask(_ context.Context, sessionID, mpnNorm, platformID string, _ time.Time, _ string, state string, lastErr *string, _ string, _, _ []byte) error {
	r.finalState = state
	if lastErr != nil {
		r.lastErr = *lastErr
	}
	for i := range r.tasks {
		if r.tasks[i].MpnNorm == mpnNorm && r.tasks[i].PlatformID == platformID {
			r.tasks[i].State = state
		}
	}
	return nil
}

func (r *stdoutSearchRepoStub) ListSearchTaskStatusRows(context.Context, string) ([]SearchTaskStatusRow, error) {
	return nil, nil
}

func (r *stdoutSearchRepoStub) ListTasksForSession(context.Context, string) ([]TaskReadinessSnapshot, error) {
	return append([]TaskReadinessSnapshot(nil), r.tasks...), nil
}

func (r *stdoutSearchRepoStub) ListActiveBySession(context.Context, string) ([]TaskReadinessSnapshot, error) {
	return nil, nil
}

func (r *stdoutSearchRepoStub) CancelBySessionPlatform(context.Context, string, string) (int64, error) {
	return 0, nil
}

func (r *stdoutSearchRepoStub) MarkSkippedBySessionPlatform(context.Context, string, string) (int64, error) {
	return 0, nil
}

func (r *stdoutSearchRepoStub) CancelAllTasksBySession(context.Context, string) error { return nil }

func (r *stdoutSearchRepoStub) CancelTasksBySessionMpnNorm(context.Context, string, string) error {
	return nil
}

func (r *stdoutSearchRepoStub) UpsertPendingTasks(context.Context, string, time.Time, int, []MpnPlatformPair) error {
	return nil
}

func (r *stdoutSearchRepoStub) GetTaskStateBySessionKey(context.Context, string, string, string, time.Time) (string, error) {
	return "", nil
}

func (r *stdoutSearchRepoStub) UpdateTaskStateBySessionKey(context.Context, string, string, string, time.Time, string) error {
	return nil
}

func (r *stdoutSearchRepoStub) ListSearchTaskLookupsByCaichipTaskID(context.Context, string) ([]BOMSearchTaskLookup, error) {
	return append([]BOMSearchTaskLookup(nil), r.lookups...), nil
}

func (r *stdoutSearchRepoStub) ListPendingLookupsByMergeKey(context.Context, string, string, time.Time) ([]BOMSearchTaskLookup, error) {
	return nil, nil
}

func (r *stdoutSearchRepoStub) LoadQuoteCacheByMergeKey(context.Context, string, string, time.Time) (*QuoteCacheSnapshot, bool, error) {
	return nil, false, nil
}

func (r *stdoutSearchRepoStub) LoadQuoteCachesForKeys(context.Context, time.Time, []MpnPlatformPair) (map[string]*QuoteCacheSnapshot, error) {
	return nil, nil
}

func (r *stdoutSearchRepoStub) DistinctPendingMergeKeysForSession(context.Context, string) ([]MergeKey, error) {
	return nil, nil
}

func (r *stdoutSearchRepoStub) UpsertManualQuote(context.Context, uint64, AgentQuoteRow) error {
	return nil
}

type stdoutSessionRepoStub struct {
	view      *BOMSessionView
	lines     []BOMSessionLineView
	setStatus string
}

func (r *stdoutSessionRepoStub) DBOk() bool { return true }

func (r *stdoutSessionRepoStub) CreateSession(context.Context, string, []string, *string, *string, *string, *string, *string) (string, time.Time, int, error) {
	return "", time.Time{}, 0, nil
}

func (r *stdoutSessionRepoStub) GetSession(context.Context, string) (*BOMSessionView, error) {
	return r.view, nil
}

func (r *stdoutSessionRepoStub) PatchSession(context.Context, string, *string, *string, *string, *string, *string, *string) error {
	return nil
}

func (r *stdoutSessionRepoStub) PutPlatforms(context.Context, string, []string, int32) (int, error) {
	return 0, nil
}

func (r *stdoutSessionRepoStub) ListSessions(context.Context, int32, int32, string, string, string) ([]BOMSessionListItem, int32, error) {
	return nil, 0, nil
}

func (r *stdoutSessionRepoStub) ReplaceSessionLines(context.Context, string, []BomImportLine, *string) (int, error) {
	return 0, nil
}

func (r *stdoutSessionRepoStub) ListSessionLines(context.Context, string) ([]BOMSessionLineView, error) {
	return append([]BOMSessionLineView(nil), r.lines...), nil
}

func (r *stdoutSessionRepoStub) SetSessionStatus(_ context.Context, _ string, status string) error {
	r.setStatus = status
	return nil
}

func (r *stdoutSessionRepoStub) CreateSessionLine(context.Context, string, string, string, string, string, string, string, string, string, *string, *float64, *string, *string) (int64, int32, int, error) {
	return 0, 0, 0, nil
}

func (r *stdoutSessionRepoStub) DeleteSessionLine(context.Context, string, int64) error { return nil }

func (r *stdoutSessionRepoStub) UpdateSessionLine(context.Context, string, int64, *string, *string, *string, *string, *string, *string, *string, *string, OptionalStringPtr, *float64, *string, *string) (int, error) {
	return 0, nil
}

func (r *stdoutSessionRepoStub) TryStartImport(context.Context, string, string) (bool, error) {
	return false, nil
}

func (r *stdoutSessionRepoStub) UpdateImportState(context.Context, string, BOMImportStatePatch) error {
	return nil
}

func TestApplyBOMQuotesFromAgentStdout_ParseRejectedFinalizesTask(t *testing.T) {
	bizDate := time.Date(2026, 4, 25, 0, 0, 0, 0, time.UTC)
	search := &stdoutSearchRepoStub{
		lookups: []BOMSearchTaskLookup{{
			SessionID:  "sid",
			MpnNorm:    "ABC",
			PlatformID: "icgoo",
			BizDate:    bizDate,
		}},
		tasks: []TaskReadinessSnapshot{{
			MpnNorm:    "ABC",
			PlatformID: "icgoo",
			State:      "running",
		}},
	}
	session := &stdoutSessionRepoStub{
		view: &BOMSessionView{
			SessionID:     "sid",
			Status:        "searching",
			ReadinessMode: ReadinessLenient,
			BizDate:       bizDate,
			PlatformIDs:   []string{"icgoo"},
		},
		lines: []BOMSessionLineView{{Mpn: "ABC"}},
	}

	applied, err := ApplyBOMQuotesFromAgentStdout(context.Background(), search, session, "task-1", "success", "completed without quote json")
	if err != nil {
		t.Fatalf("ApplyBOMQuotesFromAgentStdout returned error: %v", err)
	}
	if !applied {
		t.Fatalf("expected parse-rejected BOM task to be applied as terminal")
	}
	if search.finalState != "failed_terminal" {
		t.Fatalf("expected failed_terminal, got %q", search.finalState)
	}
	if session.setStatus != "data_ready" {
		t.Fatalf("expected session data_ready after terminal task, got %q", session.setStatus)
	}
	if search.lastErr == "" {
		t.Fatalf("expected parse rejection reason to be recorded")
	}
}

func TestApplyBOMQuotesFromAgentStdout_EmptyStdoutFinalizesTask(t *testing.T) {
	bizDate := time.Date(2026, 4, 25, 0, 0, 0, 0, time.UTC)
	search := &stdoutSearchRepoStub{
		lookups: []BOMSearchTaskLookup{{
			SessionID:  "sid",
			MpnNorm:    "ABC",
			PlatformID: "icgoo",
			BizDate:    bizDate,
		}},
		tasks: []TaskReadinessSnapshot{{
			MpnNorm:    "ABC",
			PlatformID: "icgoo",
			State:      "running",
		}},
	}
	session := &stdoutSessionRepoStub{
		view: &BOMSessionView{
			SessionID:     "sid",
			Status:        "searching",
			ReadinessMode: ReadinessLenient,
			BizDate:       bizDate,
			PlatformIDs:   []string{"icgoo"},
		},
		lines: []BOMSessionLineView{{Mpn: "ABC"}},
	}

	applied, err := ApplyBOMQuotesFromAgentStdout(context.Background(), search, session, "task-1", "success", "")
	if err != nil {
		t.Fatalf("ApplyBOMQuotesFromAgentStdout returned error: %v", err)
	}
	if !applied {
		t.Fatalf("expected empty-stdout BOM task to be applied as terminal")
	}
	if search.finalState != "failed_terminal" {
		t.Fatalf("expected failed_terminal, got %q", search.finalState)
	}
	if session.setStatus != "data_ready" {
		t.Fatalf("expected session data_ready after terminal task, got %q", session.setStatus)
	}
	if search.lastErr == "" {
		t.Fatalf("expected missing stdout reason to be recorded")
	}
}
