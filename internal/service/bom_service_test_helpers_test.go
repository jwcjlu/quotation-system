package service

import (
	"context"
	"sync"
	"time"

	"caichip/internal/biz"
	"caichip/internal/data"
)

type bomSessionRepoStub struct {
	mu        sync.Mutex
	view      *biz.BOMSessionView
	lines     []biz.BOMSessionLineView
	fullLines []data.BomSessionLine
}

func (s *bomSessionRepoStub) DBOk() bool { return true }

func (s *bomSessionRepoStub) CreateSession(ctx context.Context, title string, platformIDs []string, customerName, contactPhone, contactEmail, contactExtra, readinessMode *string) (string, time.Time, int, error) {
	return "", time.Time{}, 0, nil
}

func (s *bomSessionRepoStub) GetSession(ctx context.Context, sessionID string) (*biz.BOMSessionView, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	return cloneBOMSessionView(s.view), nil
}

func (s *bomSessionRepoStub) PatchSession(ctx context.Context, sessionID string, title, customerName, contactPhone, contactEmail, contactExtra, readinessMode *string) error {
	return nil
}

func (s *bomSessionRepoStub) PutPlatforms(ctx context.Context, sessionID string, platformIDs []string, expectedRevision int32) (int, error) {
	return 0, nil
}

func (s *bomSessionRepoStub) ListSessions(ctx context.Context, page, pageSize int32, status, bizDate, q string) ([]biz.BOMSessionListItem, int32, error) {
	return nil, 0, nil
}

func (s *bomSessionRepoStub) ReplaceSessionLines(ctx context.Context, sessionID string, lines []biz.BomImportLine, parseMode *string) (int, error) {
	return 0, nil
}

func (s *bomSessionRepoStub) ListSessionLines(ctx context.Context, sessionID string) ([]biz.BOMSessionLineView, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	out := make([]biz.BOMSessionLineView, 0, len(s.fullLines))
	for _, line := range s.fullLines {
		out = append(out, biz.BOMSessionLineView{ID: line.ID, LineNo: line.LineNo, Mpn: line.Mpn})
	}
	return out, nil
}

func (s *bomSessionRepoStub) ListSessionLinesFull(ctx context.Context, sessionID string) ([]data.BomSessionLine, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	out := make([]data.BomSessionLine, 0, len(s.fullLines))
	for _, line := range s.fullLines {
		out = append(out, cloneBomSessionLine(line))
	}
	return out, nil
}

func (s *bomSessionRepoStub) SetSessionStatus(ctx context.Context, sessionID, status string) error {
	return nil
}

func (s *bomSessionRepoStub) CreateSessionLine(ctx context.Context, sessionID, mpn, mfr, pkg string, qty *float64, rawText, extraJSON *string) (int64, int32, int, error) {
	return 0, 0, 0, nil
}

func (s *bomSessionRepoStub) DeleteSessionLine(ctx context.Context, sessionID string, lineID int64) error {
	return nil
}

func (s *bomSessionRepoStub) UpdateSessionLine(ctx context.Context, sessionID string, lineID int64, mpn, mfr, pkg *string, qty *float64, rawText, extraJSON *string) (int, error) {
	return 0, nil
}

func (s *bomSessionRepoStub) TryStartImport(ctx context.Context, sessionID, startedMessage string) (bool, error) {
	return false, nil
}

func (s *bomSessionRepoStub) UpdateImportState(ctx context.Context, sessionID string, patch biz.BOMImportStatePatch) error {
	return nil
}

type bomSearchTaskRepoStub struct {
	mu       sync.Mutex
	tasks    []biz.TaskReadinessSnapshot
	cacheMap map[string]*biz.QuoteCacheSnapshot
}

func (s *bomSearchTaskRepoStub) DBOk() bool { return true }

func (s *bomSearchTaskRepoStub) LoadSearchTaskByCaichipTaskID(ctx context.Context, caichipTaskID string) (*biz.BOMSearchTaskLookup, error) {
	return nil, nil
}

func (s *bomSearchTaskRepoStub) FinalizeSearchTask(ctx context.Context, sessionID, mpnNorm, platformID string, bizDate time.Time, caichipTaskID, state string, lastErr *string, quoteOutcome string, quotesJSON, noMpnDetail []byte) error {
	return nil
}

func (s *bomSearchTaskRepoStub) ListTasksForSession(ctx context.Context, sessionID string) ([]biz.TaskReadinessSnapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	return append([]biz.TaskReadinessSnapshot(nil), s.tasks...), nil
}

func (s *bomSearchTaskRepoStub) ListActiveBySession(ctx context.Context, sessionID string) ([]biz.TaskReadinessSnapshot, error) {
	return nil, nil
}

func (s *bomSearchTaskRepoStub) CancelBySessionPlatform(ctx context.Context, sessionID, platformID string) (int64, error) {
	return 0, nil
}

func (s *bomSearchTaskRepoStub) MarkSkippedBySessionPlatform(ctx context.Context, sessionID, platformID string) (int64, error) {
	return 0, nil
}

func (s *bomSearchTaskRepoStub) CancelAllTasksBySession(ctx context.Context, sessionID string) error {
	return nil
}

func (s *bomSearchTaskRepoStub) CancelTasksBySessionMpnNorm(ctx context.Context, sessionID, mpnNorm string) error {
	return nil
}

func (s *bomSearchTaskRepoStub) UpsertPendingTasks(ctx context.Context, sessionID string, bizDate time.Time, selectionRevision int, pairs []biz.MpnPlatformPair) error {
	return nil
}

func (s *bomSearchTaskRepoStub) GetTaskStateBySessionKey(ctx context.Context, sessionID, mpnNorm, platformID string, bizDate time.Time) (string, error) {
	return "", nil
}

func (s *bomSearchTaskRepoStub) UpdateTaskStateBySessionKey(ctx context.Context, sessionID, mpnNorm, platformID string, bizDate time.Time, state string) error {
	return nil
}

func (s *bomSearchTaskRepoStub) ListSearchTaskLookupsByCaichipTaskID(ctx context.Context, caichipTaskID string) ([]biz.BOMSearchTaskLookup, error) {
	return nil, nil
}

func (s *bomSearchTaskRepoStub) ListPendingLookupsByMergeKey(ctx context.Context, mpnNorm, platformID string, bizDate time.Time) ([]biz.BOMSearchTaskLookup, error) {
	return nil, nil
}

func (s *bomSearchTaskRepoStub) LoadQuoteCacheByMergeKey(ctx context.Context, mpnNorm, platformID string, bizDate time.Time) (*biz.QuoteCacheSnapshot, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	snap := cloneQuoteCacheSnapshot(s.cacheMap[quoteCachePairKey(mpnNorm, platformID)])
	return snap, snap != nil, nil
}

func (s *bomSearchTaskRepoStub) LoadQuoteCachesForKeys(ctx context.Context, bizDate time.Time, pairs []biz.MpnPlatformPair) (map[string]*biz.QuoteCacheSnapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	out := make(map[string]*biz.QuoteCacheSnapshot, len(s.cacheMap))
	for k, v := range s.cacheMap {
		if v == nil {
			out[k] = nil
			continue
		}
		out[k] = cloneQuoteCacheSnapshot(v)
	}
	return out, nil
}

func (s *bomSearchTaskRepoStub) DistinctPendingMergeKeysForSession(ctx context.Context, sessionID string) ([]biz.MergeKey, error) {
	return nil, nil
}

type manufacturerAliasRepoStub map[string]string

func (m manufacturerAliasRepoStub) CanonicalID(ctx context.Context, aliasNorm string) (string, bool, error) {
	v, ok := m[aliasNorm]
	return v, ok, nil
}

func (m manufacturerAliasRepoStub) DBOk() bool { return true }

func (m manufacturerAliasRepoStub) ListDistinctCanonicals(ctx context.Context, limit int) ([]biz.ManufacturerCanonicalDisplay, error) {
	return nil, nil
}

func (m manufacturerAliasRepoStub) CreateRow(ctx context.Context, canonicalID, displayName, alias, aliasNorm string) error {
	return nil
}

func cloneBOMSessionView(in *biz.BOMSessionView) *biz.BOMSessionView {
	if in == nil {
		return nil
	}
	out := *in
	out.PlatformIDs = append([]string(nil), in.PlatformIDs...)
	out.ImportUpdatedAt = cloneTimePtr(in.ImportUpdatedAt)
	return &out
}

func cloneBomSessionLine(in data.BomSessionLine) data.BomSessionLine {
	out := in
	out.RawText = cloneStringPtr(in.RawText)
	out.Mfr = cloneStringPtr(in.Mfr)
	out.Package = cloneStringPtr(in.Package)
	out.Qty = cloneFloat64Ptr(in.Qty)
	out.ExtraJSON = append([]byte(nil), in.ExtraJSON...)
	return out
}

func cloneQuoteCacheSnapshot(in *biz.QuoteCacheSnapshot) *biz.QuoteCacheSnapshot {
	if in == nil {
		return nil
	}
	out := *in
	out.QuotesJSON = append([]byte(nil), in.QuotesJSON...)
	out.NoMpnDetail = append([]byte(nil), in.NoMpnDetail...)
	return &out
}

func cloneStringPtr(in *string) *string {
	if in == nil {
		return nil
	}
	out := *in
	return &out
}

func cloneFloat64Ptr(in *float64) *float64 {
	if in == nil {
		return nil
	}
	out := *in
	return &out
}

func cloneTimePtr(in *time.Time) *time.Time {
	if in == nil {
		return nil
	}
	out := *in
	return &out
}
