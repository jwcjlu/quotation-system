package service

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"caichip/internal/biz"
	"caichip/internal/data"
	"github.com/xuri/excelize/v2"
	"gorm.io/gorm"
)

type bomSessionRepoStub struct {
	mu                sync.Mutex
	sessionExists     bool
	importInProgress  bool
	view              *biz.BOMSessionView
	fullLines         []data.BomSessionLine
	patches           []biz.BOMImportStatePatch
	replaced          bool
	replacedLineNos   []int
	tryStartCalls     int
	tryStartSuccesses int
}

func (s *bomSessionRepoStub) DBOk() bool { return true }

func (s *bomSessionRepoStub) CreateSession(ctx context.Context, title string, platformIDs []string, customerName, contactPhone, contactEmail, contactExtra, readinessMode *string) (string, time.Time, int, error) {
	return "", time.Time{}, 0, nil
}

func (s *bomSessionRepoStub) GetSession(ctx context.Context, sessionID string) (*biz.BOMSessionView, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	exists := s.sessionExists || s.view != nil || len(s.fullLines) > 0
	if !exists {
		return nil, gorm.ErrRecordNotFound
	}
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
	s.mu.Lock()
	defer s.mu.Unlock()

	s.replaced = true
	s.replacedLineNos = s.replacedLineNos[:0]
	s.fullLines = s.fullLines[:0]
	for idx, line := range lines {
		lineNo := line.LineNo
		if lineNo <= 0 {
			lineNo = idx + 1
		}
		s.replacedLineNos = append(s.replacedLineNos, lineNo)
		s.fullLines = append(s.fullLines, data.BomSessionLine{
			ID:                      int64(idx + 1),
			LineNo:                  lineNo,
			Mpn:                     line.Mpn,
			UnifiedMpn:              stringPtrIfNotEmpty(line.UnifiedMpn),
			ReferenceDesignator:     stringPtrIfNotEmpty(line.ReferenceDesignator),
			SubstituteMpn:           stringPtrIfNotEmpty(line.SubstituteMpn),
			Remark:                  stringPtrIfNotEmpty(line.Remark),
			Description:             stringPtrIfNotEmpty(line.Description),
			ManufacturerCanonicalID: line.ManufacturerCanonicalID,
		})
	}
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

func (s *bomSessionRepoStub) CreateSessionLine(ctx context.Context, sessionID, mpn, unifiedMpn, referenceDesignator, substituteMpn, remark, description, mfr, pkg string, manufacturerCanonicalID *string, qty *float64, rawText, extraJSON *string) (int64, int32, int, error) {
	return 0, 0, 0, nil
}

func (s *bomSessionRepoStub) DeleteSessionLine(ctx context.Context, sessionID string, lineID int64) error {
	return nil
}

func (s *bomSessionRepoStub) UpdateSessionLine(ctx context.Context, sessionID string, lineID int64, mpn, unifiedMpn, referenceDesignator, substituteMpn, remark, description, mfr, pkg *string, manufacturerCanonicalID biz.OptionalStringPtr, qty *float64, rawText, extraJSON *string) (int, error) {
	return 0, nil
}

func (s *bomSessionRepoStub) TryStartImport(ctx context.Context, sessionID, startedMessage string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.tryStartCalls++
	exists := s.sessionExists || s.view != nil || len(s.fullLines) > 0
	if !exists {
		return false, gorm.ErrRecordNotFound
	}
	if s.view == nil {
		s.view = &biz.BOMSessionView{SessionID: sessionID}
	}
	if s.importInProgress || s.view.ImportStatus == biz.BOMImportStatusParsing {
		return false, nil
	}
	s.importInProgress = true
	s.view.ImportStatus = biz.BOMImportStatusParsing
	s.view.ImportProgress = 0
	s.view.ImportStage = biz.BOMImportStageValidating
	s.view.ImportMessage = startedMessage
	now := time.Now()
	s.view.ImportUpdatedAt = &now
	s.tryStartSuccesses++
	return true, nil
}

func (s *bomSessionRepoStub) UpdateImportState(ctx context.Context, sessionID string, patch biz.BOMImportStatePatch) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.patches = append(s.patches, patch)
	if s.view == nil {
		s.view = &biz.BOMSessionView{SessionID: sessionID}
	}
	s.view.ImportStatus = patch.Status
	s.view.ImportProgress = patch.Progress
	s.view.ImportStage = patch.Stage
	if patch.Message != nil {
		s.view.ImportMessage = *patch.Message
	}
	if patch.ErrorCode != nil {
		s.view.ImportErrorCode = *patch.ErrorCode
	}
	if patch.Error != nil {
		s.view.ImportError = *patch.Error
	}
	now := time.Now()
	s.view.ImportUpdatedAt = &now
	if patch.Status == biz.BOMImportStatusIdle {
		s.importInProgress = false
	}
	return nil
}

type bomSearchTaskRepoStub struct {
	mu              sync.Mutex
	tasks           []biz.TaskReadinessSnapshot
	cacheMap        map[string]*biz.QuoteCacheSnapshot
	candidateRows   map[string][]biz.AgentQuoteRow
	candidateCalls  int
	cacheBatchCalls int
	manualQuoteGap  uint64
	pendingPairs    []biz.MpnPlatformPair
	upsertTasks     bool
}

func (s *bomSearchTaskRepoStub) DBOk() bool { return true }

func (s *bomSearchTaskRepoStub) LoadSearchTaskByCaichipTaskID(ctx context.Context, caichipTaskID string) (*biz.BOMSearchTaskLookup, error) {
	return nil, nil
}

func (s *bomSearchTaskRepoStub) FinalizeSearchTask(ctx context.Context, sessionID, mpnNorm, platformID string, bizDate time.Time, caichipTaskID, state string, lastErr *string, quoteOutcome string, quotesJSON, noMpnDetail []byte) error {
	return nil
}

func (s *bomSearchTaskRepoStub) ListSearchTaskStatusRows(ctx context.Context, sessionID string) ([]biz.SearchTaskStatusRow, error) {
	return nil, nil
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
	s.mu.Lock()
	defer s.mu.Unlock()

	s.upsertTasks = true
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
	s.mu.Lock()
	defer s.mu.Unlock()

	snap := cloneQuoteCacheSnapshot(s.cacheMap[testQuoteCacheKey(mpnNorm, platformID)])
	return snap, snap != nil, nil
}

func (s *bomSearchTaskRepoStub) LoadQuoteCachesForKeys(ctx context.Context, bizDate time.Time, pairs []biz.MpnPlatformPair) (map[string]*biz.QuoteCacheSnapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.cacheBatchCalls++
	out := make(map[string]*biz.QuoteCacheSnapshot, len(s.cacheMap))
	for k, v := range s.cacheMap {
		out[k] = cloneQuoteCacheSnapshot(v)
	}
	return out, nil
}

func (s *bomSearchTaskRepoStub) ListManufacturerAliasQuoteRows(ctx context.Context, bizDate time.Time, pairs []biz.MpnPlatformPair) (map[string][]biz.AgentQuoteRow, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.candidateCalls++
	out := make(map[string][]biz.AgentQuoteRow, len(s.candidateRows))
	for k, rows := range s.candidateRows {
		out[k] = append([]biz.AgentQuoteRow(nil), rows...)
	}
	return out, nil
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
	return quoteCachePairKey(mpnNorm, platformID)
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
	return &out
}

func cloneBomSessionLine(in data.BomSessionLine) data.BomSessionLine {
	out := in
	out.RawText = cloneStringPtr(in.RawText)
	out.UnifiedMpn = cloneStringPtr(in.UnifiedMpn)
	out.ReferenceDesignator = cloneStringPtr(in.ReferenceDesignator)
	out.SubstituteMpn = cloneStringPtr(in.SubstituteMpn)
	out.Remark = cloneStringPtr(in.Remark)
	out.Description = cloneStringPtr(in.Description)
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

func stringPtrIfNotEmpty(s string) *string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	return &s
}

func cloneTimePtr(in *time.Time) *time.Time {
	if in == nil {
		return nil
	}
	out := *in
	return &out
}

func makeSimpleBOMExcel(t *testing.T) []byte {
	t.Helper()
	return makeBOMExcelWithRows(t, 1)
}

func makeBOMExcelWithRows(t *testing.T, rowCount int) []byte {
	t.Helper()

	f := excelize.NewFile()
	sheet := f.GetSheetName(0)
	if err := f.SetSheetRow(sheet, "A1", &[]any{"Model", "Manufacturer", "Package", "Quantity"}); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < rowCount; i++ {
		cell, err := excelize.CoordinatesToCellName(1, i+2)
		if err != nil {
			t.Fatal(err)
		}
		row := []any{
			"PART-" + time.Now().Format("150405") + "-" + string(rune('A'+(i%26))),
			"",
			"",
			1,
		}
		if err := f.SetSheetRow(sheet, cell, &row); err != nil {
			t.Fatal(err)
		}
	}
	var buf bytes.Buffer
	if err := f.Write(&buf); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}
