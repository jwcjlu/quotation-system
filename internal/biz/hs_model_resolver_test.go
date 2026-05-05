package biz

import (
	"context"
	"errors"
	"testing"
	"time"
)

type inMemoryHsModelTaskRepo struct {
	byReq   map[string]*HsModelTaskRecord
	byRun   map[string]*HsModelTaskRecord
	latest  map[string]string
	saveErr error
}

func newInMemoryHsModelTaskRepo() *inMemoryHsModelTaskRepo {
	return &inMemoryHsModelTaskRepo{
		byReq:  make(map[string]*HsModelTaskRecord),
		byRun:  make(map[string]*HsModelTaskRecord),
		latest: make(map[string]string),
	}
}

func (r *inMemoryHsModelTaskRepo) GetByRequestTraceID(_ context.Context, model, manufacturer, requestTraceID string) (*HsModelTaskRecord, error) {
	row := r.byReq[model+"|"+manufacturer+"|"+requestTraceID]
	if row == nil {
		return nil, nil
	}
	cp := *row
	return &cp, nil
}

func (r *inMemoryHsModelTaskRepo) GetByRunID(_ context.Context, runID string) (*HsModelTaskRecord, error) {
	row := r.byRun[runID]
	if row == nil {
		return nil, nil
	}
	cp := *row
	return &cp, nil
}

func (r *inMemoryHsModelTaskRepo) GetLatestByModelManufacturer(_ context.Context, model, manufacturer string) (*HsModelTaskRecord, error) {
	runID := r.latest[model+"|"+manufacturer]
	if runID == "" {
		return nil, nil
	}
	return r.GetByRunID(context.Background(), runID)
}

func (r *inMemoryHsModelTaskRepo) Save(_ context.Context, row *HsModelTaskRecord) error {
	if r.saveErr != nil {
		return r.saveErr
	}
	cp := *row
	cp.UpdatedAt = time.Now()
	r.byRun[row.RunID] = &cp
	r.byReq[row.Model+"|"+row.Manufacturer+"|"+row.RequestTraceID] = &cp
	r.latest[row.Model+"|"+row.Manufacturer] = row.RunID
	return nil
}

type stubFeatureExtractor struct {
	out HsPrefilterInput
	err error
}

type allowAllChecker struct{}

func (allowAllChecker) CanDownload(_ context.Context, _ string) bool { return true }

func (s stubFeatureExtractor) Extract(_ context.Context, _, _ string, _ *HsDatasheetAssetRecord, _ string) (HsPrefilterInput, error) {
	return s.out, s.err
}

type stubCandidateRecommender struct {
	out []HsItemCandidate
	err error
}

type stubCandidatePrefilter struct {
	out []HsItemCandidate
	err error
}

func (s stubCandidatePrefilter) Prefilter(_ context.Context, _ HsPrefilterInput) ([]HsItemCandidate, error) {
	return s.out, s.err
}

func (s stubCandidateRecommender) Recommend(_ context.Context, _ HsPrefilterInput, _ []HsItemCandidate, _ int) ([]HsItemCandidate, error) {
	return s.out, s.err
}

type spyRecommendationRepo struct {
	saved []HsModelRecommendationRecord
}

func (s *spyRecommendationRepo) DBOk() bool { return true }

func (s *spyRecommendationRepo) SaveTopN(_ context.Context, rows []HsModelRecommendationRecord) error {
	s.saved = append(s.saved, rows...)
	return nil
}

func (s *spyRecommendationRepo) ListByRunID(_ context.Context, _ string) ([]HsModelRecommendationRecord, error) {
	out := make([]HsModelRecommendationRecord, len(s.saved))
	copy(out, s.saved)
	return out, nil
}

func (s *spyRecommendationRepo) ListPendingReviews(_ context.Context, _ int, _ int, _ string, _ string) ([]HsPendingReviewRecord, int, error) {
	return nil, 0, nil
}

type spyMappingRepo struct {
	saved     []*HsModelMappingRecord
	confirmed *HsModelMappingRecord
}

func (s *spyMappingRepo) DBOk() bool { return true }

func (s *spyMappingRepo) GetConfirmedByModelManufacturer(_ context.Context, _, _ string) (*HsModelMappingRecord, error) {
	if s.confirmed == nil {
		return nil, nil
	}
	cp := *s.confirmed
	return &cp, nil
}

func (s *spyMappingRepo) Save(_ context.Context, row *HsModelMappingRecord) error {
	cp := *row
	s.saved = append(s.saved, &cp)
	return nil
}

func TestHsModelResolver_IdempotentRunIDReuse(t *testing.T) {
	t.Parallel()
	taskRepo := newInMemoryHsModelTaskRepo()
	recoRepo := &spyRecommendationRepo{}
	mapRepo := &spyMappingRepo{}

	resolver := NewHsModelResolver(allowAllChecker{}).WithStateMachine(taskRepo, recoRepo, mapRepo).
		WithFeatureExtractor(stubFeatureExtractor{out: HsPrefilterInput{TechCategory: "ic"}}).
		WithCandidatePrefilter(stubCandidatePrefilter{out: []HsItemCandidate{{CodeTS: "1234567890", Score: 0.95}}}).
		WithCandidateRecommender(stubCandidateRecommender{
			out: []HsItemCandidate{{CodeTS: "1234567890", Score: 0.95}},
		}).
		WithRunIDGenerator(func() string { return "run-fixed-1" })

	req := HsModelResolveRequest{
		Model:          "STM32F103",
		Manufacturer:   "ST",
		RequestTraceID: "trace-001",
		DatasheetCands: []HsDatasheetCandidate{{ID: 1, DatasheetURL: "https://x/a.pdf", UpdatedAt: time.Now()}},
	}
	first, err := resolver.ResolveByModel(context.Background(), req)
	if err != nil {
		t.Fatalf("first resolve failed: %v", err)
	}
	second, err := resolver.ResolveByModel(context.Background(), req)
	if err != nil {
		t.Fatalf("second resolve failed: %v", err)
	}
	if first.RunID != second.RunID {
		t.Fatalf("expected idempotent run reuse, got %s vs %s", first.RunID, second.RunID)
	}
}

func TestHsModelResolver_AutoAcceptThreshold(t *testing.T) {
	t.Parallel()
	taskRepo := newInMemoryHsModelTaskRepo()
	recoRepo := &spyRecommendationRepo{}
	mapRepo := &spyMappingRepo{}
	resolver := NewHsModelResolver(allowAllChecker{}).WithStateMachine(taskRepo, recoRepo, mapRepo).
		WithFeatureExtractor(stubFeatureExtractor{out: HsPrefilterInput{TechCategory: "ic"}}).
		WithCandidatePrefilter(stubCandidatePrefilter{out: []HsItemCandidate{{CodeTS: "1111222233", Score: 0.99}}}).
		WithRunIDGenerator(func() string { return "run-threshold" })

	resolver.WithCandidateRecommender(stubCandidateRecommender{
		out: []HsItemCandidate{{CodeTS: "1111222233", Score: 0.99}},
	})
	high, err := resolver.ResolveByModel(context.Background(), HsModelResolveRequest{
		Model: "M1", Manufacturer: "NXP", RequestTraceID: "t-high",
		DatasheetCands: []HsDatasheetCandidate{{ID: 1, DatasheetURL: "https://x/high.pdf", UpdatedAt: time.Now()}},
	})
	if err != nil {
		t.Fatalf("high score resolve failed: %v", err)
	}
	if high.ResultStatus != HsResultStatusConfirmed {
		t.Fatalf("expected confirmed for high score, got %s", high.ResultStatus)
	}
	if high.TaskStatus != HsTaskStatusSuccess {
		t.Fatalf("expected success task status for high score, got %s", high.TaskStatus)
	}

	resolver.WithRunIDGenerator(func() string { return "run-threshold-low" })
	resolver.WithCandidateRecommender(stubCandidateRecommender{
		out: []HsItemCandidate{{CodeTS: "1111222233", Score: 0.30}},
	})
	low, err := resolver.ResolveByModel(context.Background(), HsModelResolveRequest{
		Model: "M2", Manufacturer: "NXP", RequestTraceID: "t-low",
		DatasheetCands: []HsDatasheetCandidate{{ID: 2, DatasheetURL: "https://x/low.pdf", UpdatedAt: time.Now()}},
	})
	if err != nil {
		t.Fatalf("low score resolve failed: %v", err)
	}
	if low.ResultStatus != HsResultStatusPendingReview {
		t.Fatalf("expected pending_review for low score, got %s", low.ResultStatus)
	}
	if low.TaskStatus != HsTaskStatusSuccess {
		t.Fatalf("expected success task status for low score, got %s", low.TaskStatus)
	}
	if len(mapRepo.saved) != 2 {
		t.Fatalf("expected two mapping writes, got %d", len(mapRepo.saved))
	}
	if mapRepo.saved[0].Status != HsResultStatusConfirmed || mapRepo.saved[0].Source != "llm_auto" {
		t.Fatalf("unexpected high score mapping: %+v", mapRepo.saved[0])
	}
	if mapRepo.saved[1].Status != HsResultStatusPendingReview || mapRepo.saved[1].Source != "llm_auto" {
		t.Fatalf("unexpected low score mapping: %+v", mapRepo.saved[1])
	}
}

func TestHsModelResolver_FailureStagesAndRetryCounters(t *testing.T) {
	t.Parallel()
	taskRepo := newInMemoryHsModelTaskRepo()
	recoRepo := &spyRecommendationRepo{}
	mapRepo := &spyMappingRepo{}
	resolver := NewHsModelResolver(allowAllChecker{}).WithStateMachine(taskRepo, recoRepo, mapRepo).
		WithFeatureExtractor(stubFeatureExtractor{out: HsPrefilterInput{TechCategory: "ic"}}).
		WithCandidatePrefilter(stubCandidatePrefilter{out: []HsItemCandidate{{CodeTS: "1234567890", Score: 0.95}}}).
		WithCandidateRecommender(stubCandidateRecommender{
			out: []HsItemCandidate{{CodeTS: "1234567890", Score: 0.95}},
		}).
		WithRunIDGenerator(func() string { return "run-fail" })

	_, err := resolver.ResolveByModel(context.Background(), HsModelResolveRequest{
		Model: "M-fail", Manufacturer: "TI", RequestTraceID: "t-ds-fail",
		DatasheetCands: []HsDatasheetCandidate{},
	})
	if err == nil {
		t.Fatal("expected datasheet failure, got nil")
	}
	dsFailed, _ := taskRepo.GetByRunID(context.Background(), "run-fail")
	if dsFailed == nil || dsFailed.Stage != HsTaskStageDatasheetFailed || dsFailed.AttemptCount != 3 || dsFailed.LastError == "" {
		t.Fatalf("unexpected datasheet failed task: %+v", dsFailed)
	}

	resolver.WithRunIDGenerator(func() string { return "run-extract-fail" }).
		WithFeatureExtractor(stubFeatureExtractor{err: errors.New("extract failed")})
	_, err = resolver.ResolveByModel(context.Background(), HsModelResolveRequest{
		Model: "M-extract-fail", Manufacturer: "TI", RequestTraceID: "t-extract-fail",
		DatasheetCands: []HsDatasheetCandidate{{ID: 1, DatasheetURL: "https://x/e.pdf", UpdatedAt: time.Now()}},
	})
	if err == nil {
		t.Fatal("expected extract failure, got nil")
	}
	exFailed, _ := taskRepo.GetByRunID(context.Background(), "run-extract-fail")
	if exFailed == nil || exFailed.Stage != HsTaskStageExtractFailed || exFailed.AttemptCount != 3 || exFailed.LastError == "" {
		t.Fatalf("unexpected extract failed task: %+v", exFailed)
	}

	resolver.WithRunIDGenerator(func() string { return "run-recommend-fail" }).
		WithFeatureExtractor(stubFeatureExtractor{out: HsPrefilterInput{TechCategory: "ic"}}).
		WithCandidateRecommender(stubCandidateRecommender{err: errors.New("recommend failed")})
	_, err = resolver.ResolveByModel(context.Background(), HsModelResolveRequest{
		Model: "M-recommend-fail", Manufacturer: "TI", RequestTraceID: "t-recommend-fail",
		DatasheetCands: []HsDatasheetCandidate{{ID: 2, DatasheetURL: "https://x/r.pdf", UpdatedAt: time.Now()}},
	})
	if err == nil {
		t.Fatal("expected recommend failure, got nil")
	}
	rFailed, _ := taskRepo.GetByRunID(context.Background(), "run-recommend-fail")
	if rFailed == nil || rFailed.Stage != HsTaskStageRecommendFailed || rFailed.AttemptCount != 3 || rFailed.LastError == "" {
		t.Fatalf("unexpected recommend failed task: %+v", rFailed)
	}
}

func TestHsModelResolver_RetryCountConfigurable(t *testing.T) {
	t.Parallel()
	taskRepo := newInMemoryHsModelTaskRepo()
	recoRepo := &spyRecommendationRepo{}
	mapRepo := &spyMappingRepo{}
	resolver := NewHsModelResolver(allowAllChecker{}).WithStateMachine(taskRepo, recoRepo, mapRepo).
		WithMaxStageRetries(1).
		WithFeatureExtractor(stubFeatureExtractor{err: errors.New("extract failed")}).
		WithCandidatePrefilter(stubCandidatePrefilter{out: []HsItemCandidate{{CodeTS: "1234567890", Score: 0.95}}}).
		WithCandidateRecommender(stubCandidateRecommender{
			out: []HsItemCandidate{{CodeTS: "1234567890", Score: 0.95}},
		}).
		WithRunIDGenerator(func() string { return "run-retry-1" })

	_, err := resolver.ResolveByModel(context.Background(), HsModelResolveRequest{
		Model: "M-retry", Manufacturer: "TI", RequestTraceID: "t-retry",
		DatasheetCands: []HsDatasheetCandidate{{ID: 1, DatasheetURL: "https://x/e.pdf", UpdatedAt: time.Now()}},
	})
	if err == nil {
		t.Fatal("expected extract failure, got nil")
	}
	failed, _ := taskRepo.GetByRunID(context.Background(), "run-retry-1")
	if failed == nil || failed.Stage != HsTaskStageExtractFailed || failed.AttemptCount != 2 {
		t.Fatalf("unexpected retry count with max=1: %+v", failed)
	}
}

func TestHsModelResolver_ConfirmedMappingFastPath(t *testing.T) {
	t.Parallel()
	taskRepo := newInMemoryHsModelTaskRepo()
	recoRepo := &spyRecommendationRepo{}
	mapRepo := &spyMappingRepo{
		confirmed: &HsModelMappingRecord{
			Model:        "M-fast",
			Manufacturer: "TI",
			CodeTS:       "9876543210",
			Confidence:   0.97,
			Status:       HsResultStatusConfirmed,
		},
	}
	resolver := NewHsModelResolver(allowAllChecker{}).WithStateMachine(taskRepo, recoRepo, mapRepo).
		WithFeatureExtractor(stubFeatureExtractor{err: errors.New("should not reach extractor")}).
		WithCandidatePrefilter(stubCandidatePrefilter{err: errors.New("should not reach prefilter")}).
		WithCandidateRecommender(stubCandidateRecommender{err: errors.New("should not reach recommender")}).
		WithRunIDGenerator(func() string { return "run-fast-path" })

	got, err := resolver.ResolveByModel(context.Background(), HsModelResolveRequest{
		Model:          "M-fast",
		Manufacturer:   "TI",
		RequestTraceID: "trace-fast",
	})
	if err != nil {
		t.Fatalf("resolve failed: %v", err)
	}
	if got.RunID != "run-fast-path" || got.TaskStatus != HsTaskStatusSuccess || got.ResultStatus != HsResultStatusConfirmed {
		t.Fatalf("unexpected fast path task: %+v", got)
	}
	if got.BestCodeTS != "9876543210" || got.BestScore != 0.97 {
		t.Fatalf("unexpected fast path best candidate: %+v", got)
	}
	if len(recoRepo.saved) != 0 || len(mapRepo.saved) != 0 {
		t.Fatalf("fast path should skip recommend/mapping writes, got reco=%d mapping=%d", len(recoRepo.saved), len(mapRepo.saved))
	}
}

func TestHsModelResolver_ExtractEmptyTechCategoryRejected(t *testing.T) {
	t.Parallel()
	taskRepo := newInMemoryHsModelTaskRepo()
	recoRepo := &spyRecommendationRepo{}
	mapRepo := &spyMappingRepo{}
	resolver := NewHsModelResolver(allowAllChecker{}).WithStateMachine(taskRepo, recoRepo, mapRepo).
		WithFeatureExtractor(stubFeatureExtractor{out: HsPrefilterInput{}}).
		WithCandidatePrefilter(stubCandidatePrefilter{out: []HsItemCandidate{{CodeTS: "1234567890", Score: 0.9}}}).
		WithCandidateRecommender(stubCandidateRecommender{out: []HsItemCandidate{{CodeTS: "1234567890", Score: 0.9}}}).
		WithRunIDGenerator(func() string { return "run-empty-tech" })

	_, err := resolver.ResolveByModel(context.Background(), HsModelResolveRequest{
		Model:          "M-empty",
		Manufacturer:   "NXP",
		RequestTraceID: "trace-empty-tech",
		DatasheetCands: []HsDatasheetCandidate{{ID: 1, DatasheetURL: "https://x/a.pdf", UpdatedAt: time.Now()}},
	})
	if !errors.Is(err, ErrHsResolverNoTechCategory) {
		t.Fatalf("expected ErrHsResolverNoTechCategory, got %v", err)
	}
}

func TestHsModelResolver_ForceRefreshStillUsesMappingFastPath(t *testing.T) {
	t.Parallel()
	taskRepo := newInMemoryHsModelTaskRepo()
	recoRepo := &spyRecommendationRepo{}
	mapRepo := &spyMappingRepo{
		confirmed: &HsModelMappingRecord{
			Model:        "M-fr",
			Manufacturer: "TI",
			CodeTS:       "1111222233",
			Confidence:   0.98,
			Status:       HsResultStatusConfirmed,
		},
	}
	resolver := NewHsModelResolver(allowAllChecker{}).WithStateMachine(taskRepo, recoRepo, mapRepo).
		WithFeatureExtractor(stubFeatureExtractor{err: errors.New("extract must not run")}).
		WithCandidatePrefilter(stubCandidatePrefilter{err: errors.New("prefilter must not run")}).
		WithCandidateRecommender(stubCandidateRecommender{err: errors.New("recommend must not run")}).
		WithRunIDGenerator(func() string { return "run-fr-map" })

	got, err := resolver.ResolveByModel(context.Background(), HsModelResolveRequest{
		Model:          "M-fr",
		Manufacturer:   "TI",
		RequestTraceID: "trace-fr",
		RunID:          "run-fr-map",
		ForceRefresh:   true,
	})
	if err != nil {
		t.Fatalf("resolve failed: %v", err)
	}
	if got.ResultStatus != HsResultStatusConfirmed || got.BestCodeTS != "1111222233" {
		t.Fatalf("expected confirmed mapping under force_refresh, got %+v", got)
	}
	if len(recoRepo.saved) != 0 {
		t.Fatalf("expected no recommendation audit on mapping path, got %d", len(recoRepo.saved))
	}
}

func TestHsModelResolver_MappingUpdateMissAliasDoesNotOverwriteCanonical(t *testing.T) {
	t.Parallel()
	taskRepo := newInMemoryHsModelTaskRepo()
	recoRepo := &spyRecommendationRepo{}
	// 无 confirmed 快路径，走完整 resolve；别名未命中时 biz 不传 ManufacturerCanonicalID（策略 D 由 data 层保留旧值）。
	mapRepo := &spyMappingRepo{confirmed: nil}
	resolver := NewHsModelResolver(allowAllChecker{}).WithStateMachine(taskRepo, recoRepo, mapRepo).
		WithManufacturerCanonicalizer(canonicalizerAliasLookup{
			rows: map[string]string{
				"OTHER-MFR": "mfr-other",
			},
		}).
		WithFeatureExtractor(stubFeatureExtractor{out: HsPrefilterInput{TechCategory: "ic"}}).
		WithCandidatePrefilter(stubCandidatePrefilter{out: []HsItemCandidate{{CodeTS: "2222333344", Score: 0.91}}}).
		WithCandidateRecommender(stubCandidateRecommender{out: []HsItemCandidate{{CodeTS: "2222333344", Score: 0.91}}}).
		WithRunIDGenerator(func() string { return "run-update-miss" })

	_, err := resolver.ResolveByModel(context.Background(), HsModelResolveRequest{
		Model:          "M-upd",
		Manufacturer:   "Legacy Mfr",
		RequestTraceID: "trace-update-miss",
		DatasheetCands: []HsDatasheetCandidate{{ID: 1, DatasheetURL: "https://x/u.pdf", UpdatedAt: time.Now()}},
		ForceRefresh:   true,
	})
	if err != nil {
		t.Fatalf("resolve failed: %v", err)
	}
	if len(mapRepo.saved) != 1 {
		t.Fatalf("expected one mapping write, got %d", len(mapRepo.saved))
	}
	if mapRepo.saved[0].ManufacturerCanonicalID != nil {
		t.Fatalf("expected nil canonical for alias miss(update preserve), got %v", *mapRepo.saved[0].ManufacturerCanonicalID)
	}
}

func TestHsModelResolver_PassesCanonicalWhenAliasHit(t *testing.T) {
	t.Parallel()
	taskRepo := newInMemoryHsModelTaskRepo()
	recoRepo := &spyRecommendationRepo{}
	mapRepo := &spyMappingRepo{}
	resolver := NewHsModelResolver(allowAllChecker{}).WithStateMachine(taskRepo, recoRepo, mapRepo).
		WithManufacturerCanonicalizer(canonicalizerAliasLookup{
			rows: map[string]string{
				"ST": "mfr-st",
			},
		}).
		WithFeatureExtractor(stubFeatureExtractor{out: HsPrefilterInput{TechCategory: "ic"}}).
		WithCandidatePrefilter(stubCandidatePrefilter{out: []HsItemCandidate{{CodeTS: "2222333344", Score: 0.91}}}).
		WithCandidateRecommender(stubCandidateRecommender{out: []HsItemCandidate{{CodeTS: "2222333344", Score: 0.91}}}).
		WithRunIDGenerator(func() string { return "run-hit" })

	_, err := resolver.ResolveByModel(context.Background(), HsModelResolveRequest{
		Model:          "M-hit",
		Manufacturer:   "st",
		RequestTraceID: "trace-hit",
		DatasheetCands: []HsDatasheetCandidate{{ID: 1, DatasheetURL: "https://x/h.pdf", UpdatedAt: time.Now()}},
	})
	if err != nil {
		t.Fatalf("resolve failed: %v", err)
	}
	if len(mapRepo.saved) != 1 {
		t.Fatalf("expected one mapping write, got %d", len(mapRepo.saved))
	}
	if mapRepo.saved[0].ManufacturerCanonicalID == nil || *mapRepo.saved[0].ManufacturerCanonicalID != "mfr-st" {
		t.Fatalf("expected canonical mfr-st, got %v", mapRepo.saved[0].ManufacturerCanonicalID)
	}
	if len(recoRepo.saved) == 0 || recoRepo.saved[0].ManufacturerCanonicalID == nil || *recoRepo.saved[0].ManufacturerCanonicalID != "mfr-st" {
		t.Fatalf("expected reco audit with canonical, got %+v", recoRepo.saved)
	}
}
