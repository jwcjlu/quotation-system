package biz_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	v1 "caichip/api/bom/v1"
	"caichip/internal/biz"
	"caichip/internal/service"
)

type acceptanceTaskRepo struct {
	mu     sync.RWMutex
	byReq  map[string]*biz.HsModelTaskRecord
	byRun  map[string]*biz.HsModelTaskRecord
	latest map[string]string
}

func newAcceptanceTaskRepo() *acceptanceTaskRepo {
	return &acceptanceTaskRepo{byReq: map[string]*biz.HsModelTaskRecord{}, byRun: map[string]*biz.HsModelTaskRecord{}, latest: map[string]string{}}
}
func (r *acceptanceTaskRepo) keyReq(model, mfr, trace string) string {
	return model + "|" + mfr + "|" + trace
}
func (r *acceptanceTaskRepo) keyMM(model, mfr string) string { return model + "|" + mfr }
func (r *acceptanceTaskRepo) GetByRequestTraceID(_ context.Context, model, manufacturer, requestTraceID string) (*biz.HsModelTaskRecord, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	row := r.byReq[r.keyReq(model, manufacturer, requestTraceID)]
	if row == nil {
		return nil, nil
	}
	cp := *row
	return &cp, nil
}
func (r *acceptanceTaskRepo) GetByRunID(_ context.Context, runID string) (*biz.HsModelTaskRecord, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	row := r.byRun[runID]
	if row == nil {
		return nil, nil
	}
	cp := *row
	return &cp, nil
}
func (r *acceptanceTaskRepo) GetLatestByModelManufacturer(_ context.Context, model, manufacturer string) (*biz.HsModelTaskRecord, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	runID := r.latest[r.keyMM(model, manufacturer)]
	row := r.byRun[runID]
	if row == nil {
		return nil, nil
	}
	cp := *row
	return &cp, nil
}
func (r *acceptanceTaskRepo) Save(_ context.Context, row *biz.HsModelTaskRecord) error {
	if row == nil {
		return nil
	}
	cp := *row
	cp.UpdatedAt = time.Now()
	r.mu.Lock()
	r.byRun[cp.RunID] = &cp
	r.byReq[r.keyReq(cp.Model, cp.Manufacturer, cp.RequestTraceID)] = &cp
	r.latest[r.keyMM(cp.Model, cp.Manufacturer)] = cp.RunID
	r.mu.Unlock()
	return nil
}

type acceptanceRecoRepo struct {
	mu    sync.RWMutex
	saved []biz.HsModelRecommendationRecord
}

func (r *acceptanceRecoRepo) DBOk() bool { return true }
func (r *acceptanceRecoRepo) SaveTopN(_ context.Context, rows []biz.HsModelRecommendationRecord) error {
	r.mu.Lock()
	r.saved = append(r.saved, rows...)
	r.mu.Unlock()
	return nil
}
func (r *acceptanceRecoRepo) ListByRunID(_ context.Context, runID string) ([]biz.HsModelRecommendationRecord, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]biz.HsModelRecommendationRecord, 0)
	for _, row := range r.saved {
		if row.RunID == runID {
			out = append(out, row)
		}
	}
	return out, nil
}

type acceptanceMappingRepo struct {
	mu        sync.RWMutex
	confirmed *biz.HsModelMappingRecord
	saved     []*biz.HsModelMappingRecord
}

func (r *acceptanceMappingRepo) DBOk() bool { return true }
func (r *acceptanceMappingRepo) GetConfirmedByModelManufacturer(_ context.Context, _, _ string) (*biz.HsModelMappingRecord, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.confirmed == nil {
		return nil, nil
	}
	cp := *r.confirmed
	return &cp, nil
}
func (r *acceptanceMappingRepo) Save(_ context.Context, row *biz.HsModelMappingRecord) error {
	if row == nil {
		return nil
	}
	cp := *row
	r.mu.Lock()
	r.saved = append(r.saved, &cp)
	if cp.Status == biz.HsResultStatusConfirmed && cp.Source == "manual" {
		r.confirmed = &cp
	}
	r.mu.Unlock()
	return nil
}

type acceptanceConfirmRepo struct {
	mu   sync.RWMutex
	byID map[string]*biz.HsModelConfirmResult
}

func newAcceptanceConfirmRepo() *acceptanceConfirmRepo {
	return &acceptanceConfirmRepo{byID: map[string]*biz.HsModelConfirmResult{}}
}
func (r *acceptanceConfirmRepo) GetByConfirmRequestID(_ context.Context, confirmRequestID string) (*biz.HsModelConfirmResult, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	row := r.byID[confirmRequestID]
	if row == nil {
		return nil, nil
	}
	cp := *row
	return &cp, nil
}
func (r *acceptanceConfirmRepo) Save(_ context.Context, row *biz.HsModelConfirmResult) error {
	cp := *row
	r.mu.Lock()
	r.byID[row.ConfirmRequestID] = &cp
	r.mu.Unlock()
	return nil
}

type acceptanceChecker struct{}

func (acceptanceChecker) CanDownload(_ context.Context, _ string) bool { return true }

type countingDownloader struct{ calls int }

func (d *countingDownloader) CanDownload(_ context.Context, _ string) bool { return true }
func (d *countingDownloader) Download(_ context.Context, model, mfr, url string) (*biz.HsDatasheetAssetRecord, error) {
	d.calls++
	return &biz.HsDatasheetAssetRecord{Model: model, Manufacturer: mfr, DatasheetURL: url, DownloadStatus: "ok"}, nil
}

type noopAssetRepo struct{}

func (noopAssetRepo) DBOk() bool { return true }
func (noopAssetRepo) GetLatestByModelManufacturer(_ context.Context, _, _ string) (*biz.HsDatasheetAssetRecord, error) {
	return nil, nil
}
func (noopAssetRepo) Save(_ context.Context, _ *biz.HsDatasheetAssetRecord) error { return nil }

type delayedExtractor struct {
	wait  time.Duration
	calls int
}

func (e *delayedExtractor) Extract(_ context.Context, model, _ string, _ *biz.HsDatasheetAssetRecord) (biz.HsPrefilterInput, error) {
	e.calls++
	if e.wait > 0 {
		time.Sleep(e.wait)
	}
	return biz.HsPrefilterInput{ComponentName: model, TechCategory: "ic", KeySpecs: map[string]string{"v": "3.3"}}, nil
}

type countingPrefilter struct{ calls int }

func (p *countingPrefilter) Prefilter(_ context.Context, _ biz.HsPrefilterInput) ([]biz.HsItemCandidate, error) {
	p.calls++
	return []biz.HsItemCandidate{
		{CodeTS: "1234567890", Score: 0.93},
		{CodeTS: "2234567890", Score: 0.81},
	}, nil
}

type countingRecommender struct{ calls int }

func (r *countingRecommender) Recommend(_ context.Context, _ biz.HsPrefilterInput, candidates []biz.HsItemCandidate, _ int) ([]biz.HsItemCandidate, error) {
	r.calls++
	out := make([]biz.HsItemCandidate, len(candidates))
	copy(out, candidates)
	return out, nil
}

type staticDatasheetSource struct{}

func (staticDatasheetSource) GetLatestByModelManufacturer(_ context.Context, _, _ string) (*biz.HsDatasheetAssetRecord, error) {
	return &biz.HsDatasheetAssetRecord{ID: 1, DatasheetURL: "https://example.com/ds.pdf", UpdatedAt: time.Now()}, nil
}

func newAcceptanceResolver(runID string, taskRepo biz.HsModelTaskRepo, recoRepo biz.HsModelRecommendationRepo, mapRepo biz.HsModelMappingRepo, extractor biz.HsModelFeatureExtractor, prefilter biz.HsModelCandidatePrefilter, recommender biz.HsModelCandidateRecommender) *biz.HsModelResolver {
	return biz.NewHsModelResolver(acceptanceChecker{}).
		WithStateMachine(taskRepo, recoRepo, mapRepo).
		WithFeatureExtractor(extractor).
		WithCandidatePrefilter(prefilter).
		WithCandidateRecommender(recommender).
		WithRunIDGenerator(func() string { return runID })
}

func TestAcceptance_RequestTraceIdIdempotentReplay(t *testing.T) {
	taskRepo, recoRepo, mapRepo := newAcceptanceTaskRepo(), &acceptanceRecoRepo{}, &acceptanceMappingRepo{}
	resolver := newAcceptanceResolver("run-fixed", taskRepo, recoRepo, mapRepo, &delayedExtractor{}, &countingPrefilter{}, &countingRecommender{})
	req := biz.HsModelResolveRequest{Model: "STM32F103", Manufacturer: "ST", RequestTraceID: "trace-1", DatasheetCands: []biz.HsDatasheetCandidate{{ID: 1, DatasheetURL: "https://example.com/a.pdf", UpdatedAt: time.Now()}}}
	first, err := resolver.ResolveByModel(context.Background(), req)
	if err != nil {
		t.Fatalf("first resolve failed: %v", err)
	}
	firstRows, err := recoRepo.ListByRunID(context.Background(), first.RunID)
	if err != nil {
		t.Fatalf("first list by run id failed: %v", err)
	}
	firstRanks := make(map[uint8]struct{}, len(firstRows))
	for _, row := range firstRows {
		firstRanks[row.CandidateRank] = struct{}{}
	}
	second, err := resolver.ResolveByModel(context.Background(), req)
	if err != nil {
		t.Fatalf("second resolve failed: %v", err)
	}
	if first.RunID != second.RunID {
		t.Fatalf("expected same run id, got %s vs %s", first.RunID, second.RunID)
	}
	secondRows, err := recoRepo.ListByRunID(context.Background(), second.RunID)
	if err != nil {
		t.Fatalf("second list by run id failed: %v", err)
	}
	if len(secondRows) != len(firstRows) {
		t.Fatalf("expected recommendation rows unchanged after idempotent replay, first=%d second=%d", len(firstRows), len(secondRows))
	}
	secondRanks := make(map[uint8]struct{}, len(secondRows))
	for _, row := range secondRows {
		secondRanks[row.CandidateRank] = struct{}{}
	}
	if len(secondRanks) != len(firstRanks) {
		t.Fatalf("expected unique rank set unchanged, first=%d second=%d", len(firstRanks), len(secondRanks))
	}
	for rank := range firstRanks {
		if _, ok := secondRanks[rank]; !ok {
			t.Fatalf("expected rank %d still exists after replay, second ranks=%v", rank, secondRanks)
		}
	}
}

func TestAcceptance_ConfirmLatestRunOnlyAndConcurrentConflict(t *testing.T) {
	taskRepo, recoRepo, mapRepo, confirmRepo := newAcceptanceTaskRepo(), &acceptanceRecoRepo{}, &acceptanceMappingRepo{}, newAcceptanceConfirmRepo()
	_ = taskRepo.Save(context.Background(), &biz.HsModelTaskRecord{Model: "M", Manufacturer: "NXP", RunID: "run-old"})
	_ = taskRepo.Save(context.Background(), &biz.HsModelTaskRecord{Model: "M", Manufacturer: "NXP", RunID: "run-new"})
	_ = recoRepo.SaveTopN(context.Background(), []biz.HsModelRecommendationRecord{
		{RunID: "run-old", CandidateRank: 1, CodeTS: "1111111111", Score: 0.7},
		{RunID: "run-new", CandidateRank: 1, CodeTS: "2222222222", Score: 0.9},
	})
	confirmer := biz.NewHsModelConfirmService(taskRepo, recoRepo, mapRepo, confirmRepo)
	errCh := make(chan error, 2)
	go func() {
		_, err := confirmer.Confirm(context.Background(), biz.HsModelConfirmRequest{RunID: "run-old", CandidateRank: 1, ExpectedCodeTS: "1111111111", ConfirmRequestID: "c-old"})
		errCh <- err
	}()
	go func() {
		_, err := confirmer.Confirm(context.Background(), biz.HsModelConfirmRequest{RunID: "run-new", CandidateRank: 1, ExpectedCodeTS: "2222222222", ConfirmRequestID: "c-new"})
		errCh <- err
	}()
	var oldFail, newOK bool
	for i := 0; i < 2; i++ {
		err := <-errCh
		if err == nil {
			newOK = true
			continue
		}
		if errors.Is(err, biz.ErrHsResolverConfirmRunNotLatest) {
			oldFail = true
		}
	}
	if !oldFail || !newOK {
		t.Fatalf("expected old run fail + new run success, oldFail=%v newOK=%v", oldFail, newOK)
	}
}

func TestAcceptance_TimeoutThenAsyncPollingGetsResult(t *testing.T) {
	taskRepo, recoRepo, mapRepo := newAcceptanceTaskRepo(), &acceptanceRecoRepo{}, &acceptanceMappingRepo{}
	extractor, recommender := &delayedExtractor{wait: 50 * time.Millisecond}, &countingRecommender{}
	resolver := newAcceptanceResolver("run-timeout", taskRepo, recoRepo, mapRepo, extractor, &countingPrefilter{}, recommender)
	svc := service.NewHsResolveService(resolver, taskRepo, recoRepo, staticDatasheetSource{}, nil, 2*time.Millisecond)
	reply, err := svc.ResolveByModel(context.Background(), &v1.HsResolveByModelRequest{Model: "STM32F103", Manufacturer: "ST", RequestTraceId: "trace-timeout"})
	if err != nil {
		t.Fatalf("resolve failed: %v", err)
	}
	if !reply.GetAccepted() || reply.GetTaskStatus() != biz.HsTaskStatusRunning {
		t.Fatalf("expected async accepted running, got %#v", reply)
	}
	time.Sleep(80 * time.Millisecond)
	taskReply, err := svc.GetResolveTask(context.Background(), &v1.HsResolveTaskRequest{TaskId: reply.GetTaskId()})
	if err != nil {
		t.Fatalf("poll failed: %v", err)
	}
	if taskReply.GetTaskStatus() != biz.HsTaskStatusSuccess || taskReply.GetBestCodeTs() == "" {
		t.Fatalf("expected completed task on polling, got %#v", taskReply)
	}
}

func TestAcceptance_MappingHitSkipsDownloadAndLLMSideEffects(t *testing.T) {
	taskRepo, recoRepo := newAcceptanceTaskRepo(), &acceptanceRecoRepo{}
	mapRepo := &acceptanceMappingRepo{confirmed: &biz.HsModelMappingRecord{Model: "M-fast", Manufacturer: "TI", CodeTS: "9876543210", Confidence: 0.96, Status: biz.HsResultStatusConfirmed}}
	extractor, recommender, downloader := &delayedExtractor{}, &countingRecommender{}, &countingDownloader{}
	resolver := newAcceptanceResolver("run-fast", taskRepo, recoRepo, mapRepo, extractor, &countingPrefilter{}, recommender).
		WithAssetPersistence(downloader, noopAssetRepo{})
	got, err := resolver.ResolveByModel(context.Background(), biz.HsModelResolveRequest{Model: "M-fast", Manufacturer: "TI", RequestTraceID: "trace-fast"})
	if err != nil {
		t.Fatalf("resolve failed: %v", err)
	}
	if got.BestCodeTS != "9876543210" || got.ResultStatus != biz.HsResultStatusConfirmed {
		t.Fatalf("unexpected fast-path task: %#v", got)
	}
	if downloader.calls != 0 || extractor.calls != 0 || recommender.calls != 0 {
		t.Fatalf("expected no side effects, downloader=%d extractor=%d recommender=%d", downloader.calls, extractor.calls, recommender.calls)
	}
}

func TestAcceptance_ManualConfirmHasLongTermPriorityOverAuto(t *testing.T) {
	taskRepo, recoRepo, mapRepo, confirmRepo := newAcceptanceTaskRepo(), &acceptanceRecoRepo{}, &acceptanceMappingRepo{}, newAcceptanceConfirmRepo()
	resolver := newAcceptanceResolver("run-1", taskRepo, recoRepo, mapRepo, &delayedExtractor{}, &countingPrefilter{}, &countingRecommender{})
	_, err := resolver.ResolveByModel(context.Background(), biz.HsModelResolveRequest{
		Model: "M-manual", Manufacturer: "ST", RequestTraceID: "trace-1",
		DatasheetCands: []biz.HsDatasheetCandidate{{ID: 1, DatasheetURL: "https://example.com/ds.pdf", UpdatedAt: time.Now()}},
	})
	if err != nil {
		t.Fatalf("initial resolve failed: %v", err)
	}
	confirmer := biz.NewHsModelConfirmService(taskRepo, recoRepo, mapRepo, confirmRepo)
	_, err = confirmer.Confirm(context.Background(), biz.HsModelConfirmRequest{RunID: "run-1", CandidateRank: 2, ExpectedCodeTS: "2234567890", ConfirmRequestID: "confirm-1"})
	if err != nil {
		t.Fatalf("manual confirm failed: %v", err)
	}
	resolver.WithRunIDGenerator(func() string { return "run-2" })
	next, err := resolver.ResolveByModel(context.Background(), biz.HsModelResolveRequest{Model: "M-manual", Manufacturer: "ST", RequestTraceID: "trace-2"})
	if err != nil {
		t.Fatalf("second resolve failed: %v", err)
	}
	if next.RunID != "run-2" || next.BestCodeTS != "2234567890" || next.ResultStatus != biz.HsResultStatusConfirmed {
		t.Fatalf("expected manual mapping still preferred, got %#v", next)
	}
}

func TestAcceptance_HistoryByRunIdContainsInputSnapshotAndRecommendations(t *testing.T) {
	taskRepo, recoRepo, mapRepo := newAcceptanceTaskRepo(), &acceptanceRecoRepo{}, &acceptanceMappingRepo{}
	resolver := newAcceptanceResolver("run-history", taskRepo, recoRepo, mapRepo, &delayedExtractor{}, &countingPrefilter{}, &countingRecommender{})
	_, err := resolver.ResolveByModel(context.Background(), biz.HsModelResolveRequest{
		Model: "M-history", Manufacturer: "TI", RequestTraceID: "trace-history",
		DatasheetCands: []biz.HsDatasheetCandidate{{ID: 1, DatasheetURL: "https://example.com/h.pdf", UpdatedAt: time.Now()}},
	})
	if err != nil {
		t.Fatalf("resolve failed: %v", err)
	}
	rows, err := recoRepo.ListByRunID(context.Background(), "run-history")
	if err != nil {
		t.Fatalf("list by run id failed: %v", err)
	}
	if len(rows) == 0 {
		t.Fatal("expected recommendation history rows")
	}
	if len(rows[0].InputSnapshotJSON) == 0 {
		t.Fatalf("expected input snapshot persisted, got %#v", rows[0])
	}
}
