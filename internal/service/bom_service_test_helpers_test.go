package service

import (
	"context"
	"errors"
	"time"

	"caichip/internal/biz"
	"caichip/internal/data"
)

type bomSessionRepoStub struct {
	view      *biz.BOMSessionView
	fullLines []data.BomSessionLine
}

func (s *bomSessionRepoStub) DBOk() bool { return true }
func (s *bomSessionRepoStub) CreateSession(ctx context.Context, title string, platformIDs []string, customerName, contactPhone, contactEmail, contactExtra, readinessMode *string) (string, time.Time, int, error) {
	return "", time.Time{}, 0, errors.New("not implemented")
}
func (s *bomSessionRepoStub) GetSession(ctx context.Context, sessionID string) (*biz.BOMSessionView, error) {
	return s.view, nil
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
	out := make([]biz.BOMSessionLineView, 0, len(s.fullLines))
	for _, line := range s.fullLines {
		out = append(out, biz.BOMSessionLineView{ID: line.ID, LineNo: line.LineNo, Mpn: line.Mpn})
	}
	return out, nil
}
func (s *bomSessionRepoStub) ListSessionLinesFull(ctx context.Context, sessionID string) ([]data.BomSessionLine, error) {
	return append([]data.BomSessionLine(nil), s.fullLines...), nil
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

type bomSearchTaskRepoStub struct {
	tasks          []biz.TaskReadinessSnapshot
	cacheMap       map[string]*biz.QuoteCacheSnapshot
	manualQuoteGap uint64
	pendingPairs   []biz.MpnPlatformPair
}

func (s *bomSearchTaskRepoStub) DBOk() bool { return true }
func (s *bomSearchTaskRepoStub) LoadSearchTaskByCaichipTaskID(ctx context.Context, caichipTaskID string) (*biz.BOMSearchTaskLookup, error) {
	return nil, nil
}
func (s *bomSearchTaskRepoStub) FinalizeSearchTask(ctx context.Context, sessionID, mpnNorm, platformID string, bizDate time.Time, caichipTaskID, state string, lastErr *string, quoteOutcome string, quotesJSON, noMpnDetail []byte) error {
	return nil
}
func (s *bomSearchTaskRepoStub) ListTasksForSession(ctx context.Context, sessionID string) ([]biz.TaskReadinessSnapshot, error) {
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
	s.pendingPairs = append(s.pendingPairs, pairs...)
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
	v := s.cacheMap[testQuoteCacheKey(mpnNorm, platformID)]
	return v, v != nil, nil
}
func (s *bomSearchTaskRepoStub) LoadQuoteCachesForKeys(ctx context.Context, bizDate time.Time, pairs []biz.MpnPlatformPair) (map[string]*biz.QuoteCacheSnapshot, error) {
	return s.cacheMap, nil
}
func (s *bomSearchTaskRepoStub) DistinctPendingMergeKeysForSession(ctx context.Context, sessionID string) ([]biz.MergeKey, error) {
	return nil, nil
}
func (s *bomSearchTaskRepoStub) UpsertManualQuote(ctx context.Context, gapID uint64, row biz.AgentQuoteRow) error {
	s.manualQuoteGap = gapID
	return nil
}

type bomLineGapRepoStub struct {
	gaps        []biz.BOMLineGap
	updated     []uint64
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

func testQuoteCacheKey(mpnNorm, platformID string) string {
	return biz.NormalizeMPNForBOMSearch(mpnNorm) + "\x00" + biz.NormalizePlatformID(platformID)
}

type bomMatchRunRepoStub struct {
	items []biz.BOMMatchResultItemDraft
	runs  []biz.BOMMatchRunView
}

func (s *bomMatchRunRepoStub) DBOk() bool { return true }
func (s *bomMatchRunRepoStub) CreateMatchRun(ctx context.Context, sessionID string, selectionRevision int, currency string, createdBy string, items []biz.BOMMatchResultItemDraft) (uint64, int, error) {
	s.items = append([]biz.BOMMatchResultItemDraft(nil), items...)
	run := biz.BOMMatchRunView{ID: 7, RunNo: len(s.runs) + 1, SessionID: sessionID, Status: biz.MatchRunSaved, LineTotal: len(items), Currency: currency}
	s.runs = append(s.runs, run)
	return run.ID, run.RunNo, nil
}
func (s *bomMatchRunRepoStub) ListMatchRuns(ctx context.Context, sessionID string) ([]biz.BOMMatchRunView, error) {
	return append([]biz.BOMMatchRunView(nil), s.runs...), nil
}
func (s *bomMatchRunRepoStub) GetMatchRun(ctx context.Context, runID uint64) (*biz.BOMMatchRunView, []biz.BOMMatchResultItemDraft, error) {
	if len(s.runs) == 0 {
		s.runs = append(s.runs, biz.BOMMatchRunView{ID: runID, RunNo: 1, SessionID: "sid", Status: biz.MatchRunSaved})
	}
	return &s.runs[0], append([]biz.BOMMatchResultItemDraft(nil), s.items...), nil
}
func (s *bomMatchRunRepoStub) SupersedePreviousRuns(ctx context.Context, sessionID string, keepRunID uint64) error {
	return nil
}
