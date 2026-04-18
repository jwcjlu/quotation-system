package biz

import (
	"context"
	"testing"
	"time"
)

type spyFeaturesRepo struct {
	creates []HsModelFeaturesRecord
}

func (s *spyFeaturesRepo) DBOk() bool { return true }

func (s *spyFeaturesRepo) Create(_ context.Context, row *HsModelFeaturesRecord) (uint64, error) {
	if row == nil {
		return 0, nil
	}
	cp := *row
	s.creates = append(s.creates, cp)
	return uint64(len(s.creates)), nil
}

func TestHsModelResolver_FeaturesPersist_CanonicalOnAliasHit(t *testing.T) {
	t.Parallel()
	taskRepo := newInMemoryHsModelTaskRepo()
	recoRepo := &spyRecommendationRepo{}
	mapRepo := &spyMappingRepo{}
	featRepo := &spyFeaturesRepo{}
	assetRepo := &stubDatasheetAssetRepo{}
	resolver := NewHsModelResolver(allowAllChecker{}).WithStateMachine(taskRepo, recoRepo, mapRepo).
		WithAssetPersistence(stubDatasheetDownloader{
			downloadFn: func(_ context.Context, model, manufacturer, datasheetURL string) (*HsDatasheetAssetRecord, error) {
				return &HsDatasheetAssetRecord{
					Model:          model,
					Manufacturer:   manufacturer,
					DatasheetURL:   datasheetURL,
					DownloadStatus: "ok",
				}, nil
			},
		}, assetRepo).
		WithFeaturesRepo(featRepo).
		WithManufacturerCanonicalizer(canonicalizerAliasLookup{
			rows: map[string]string{"ST": "mfr-st"},
		}).
		WithFeatureExtractor(stubFeatureExtractor{out: HsPrefilterInput{TechCategory: "ic"}}).
		WithCandidatePrefilter(stubCandidatePrefilter{out: []HsItemCandidate{{CodeTS: "3333444455", Score: 0.92}}}).
		WithCandidateRecommender(stubCandidateRecommender{out: []HsItemCandidate{{CodeTS: "3333444455", Score: 0.92}}}).
		WithRunIDGenerator(func() string { return "run-feat-hit" })

	_, err := resolver.ResolveByModel(context.Background(), HsModelResolveRequest{
		Model:          "M-feat",
		Manufacturer:   "st",
		RequestTraceID: "trace-feat-hit",
		DatasheetCands: []HsDatasheetCandidate{{ID: 1, DatasheetURL: "https://x/f.pdf", UpdatedAt: time.Now()}},
	})
	if err != nil {
		t.Fatalf("resolve failed: %v", err)
	}
	if len(featRepo.creates) != 1 {
		t.Fatalf("expected one features row, got %d", len(featRepo.creates))
	}
	if featRepo.creates[0].ManufacturerCanonicalID == nil || *featRepo.creates[0].ManufacturerCanonicalID != "mfr-st" {
		t.Fatalf("expected canonical mfr-st on features, got %v", featRepo.creates[0].ManufacturerCanonicalID)
	}
	if featRepo.creates[0].AssetID == 0 {
		t.Fatalf("expected non-zero asset id, got %d", featRepo.creates[0].AssetID)
	}
}

func TestHsModelResolver_FeaturesPersist_MissAliasLeavesCanonicalNil(t *testing.T) {
	t.Parallel()
	taskRepo := newInMemoryHsModelTaskRepo()
	recoRepo := &spyRecommendationRepo{}
	mapRepo := &spyMappingRepo{}
	featRepo := &spyFeaturesRepo{}
	assetRepo := &stubDatasheetAssetRepo{}
	resolver := NewHsModelResolver(allowAllChecker{}).WithStateMachine(taskRepo, recoRepo, mapRepo).
		WithAssetPersistence(stubDatasheetDownloader{
			downloadFn: func(_ context.Context, model, manufacturer, datasheetURL string) (*HsDatasheetAssetRecord, error) {
				return &HsDatasheetAssetRecord{
					Model:          model,
					Manufacturer:   manufacturer,
					DatasheetURL:   datasheetURL,
					DownloadStatus: "ok",
				}, nil
			},
		}, assetRepo).
		WithFeaturesRepo(featRepo).
		WithManufacturerCanonicalizer(canonicalizerAliasLookup{
			rows: map[string]string{"OTHER": "mfr-other"},
		}).
		WithFeatureExtractor(stubFeatureExtractor{out: HsPrefilterInput{TechCategory: "ic"}}).
		WithCandidatePrefilter(stubCandidatePrefilter{out: []HsItemCandidate{{CodeTS: "4444555566", Score: 0.91}}}).
		WithCandidateRecommender(stubCandidateRecommender{out: []HsItemCandidate{{CodeTS: "4444555566", Score: 0.91}}}).
		WithRunIDGenerator(func() string { return "run-feat-miss" })

	_, err := resolver.ResolveByModel(context.Background(), HsModelResolveRequest{
		Model:          "M-feat-miss",
		Manufacturer:   "No Alias Mfr",
		RequestTraceID: "trace-feat-miss",
		DatasheetCands: []HsDatasheetCandidate{{ID: 1, DatasheetURL: "https://x/m.pdf", UpdatedAt: time.Now()}},
	})
	if err != nil {
		t.Fatalf("resolve failed: %v", err)
	}
	if len(featRepo.creates) != 1 {
		t.Fatalf("expected one features row, got %d", len(featRepo.creates))
	}
	if featRepo.creates[0].ManufacturerCanonicalID != nil {
		t.Fatalf("expected nil canonical on alias miss, got %v", *featRepo.creates[0].ManufacturerCanonicalID)
	}
}
