package biz

import (
	"context"
	"errors"
	"testing"
)

type inMemoryHsModelConfirmRepo struct {
	byReq map[string]*HsModelConfirmResult
}

func newInMemoryHsModelConfirmRepo() *inMemoryHsModelConfirmRepo {
	return &inMemoryHsModelConfirmRepo{byReq: make(map[string]*HsModelConfirmResult)}
}

func (r *inMemoryHsModelConfirmRepo) GetByConfirmRequestID(_ context.Context, confirmRequestID string) (*HsModelConfirmResult, error) {
	row := r.byReq[confirmRequestID]
	if row == nil {
		return nil, nil
	}
	cp := *row
	return &cp, nil
}

func (r *inMemoryHsModelConfirmRepo) Save(_ context.Context, row *HsModelConfirmResult) error {
	cp := *row
	r.byReq[row.ConfirmRequestID] = &cp
	return nil
}

func TestHsModelResolverConfirm_OnlyLatestRunAllowed(t *testing.T) {
	t.Parallel()
	taskRepo := newInMemoryHsModelTaskRepo()
	recoRepo := &spyRecommendationRepo{
		saved: []HsModelRecommendationRecord{
			{RunID: "run-old", CandidateRank: 1, CodeTS: "1234567890", Score: 0.88},
		},
	}
	mapRepo := &spyMappingRepo{}
	confirmRepo := newInMemoryHsModelConfirmRepo()
	taskRepo.Save(context.Background(), &HsModelTaskRecord{Model: "M", Manufacturer: "NXP", RunID: "run-old", ResultStatus: HsResultStatusPendingReview})
	taskRepo.Save(context.Background(), &HsModelTaskRecord{Model: "M", Manufacturer: "NXP", RunID: "run-new", ResultStatus: HsResultStatusPendingReview})

	svc := NewHsModelConfirmService(taskRepo, recoRepo, mapRepo, confirmRepo)
	_, err := svc.Confirm(context.Background(), HsModelConfirmRequest{
		RunID: "run-old", CandidateRank: 1, ExpectedCodeTS: "1234567890", ConfirmRequestID: "c-1",
	})
	if err == nil || !errors.Is(err, ErrHsResolverConfirmRunNotLatest) {
		t.Fatalf("expected run-not-latest error, got %v", err)
	}
}

func TestHsModelResolverConfirm_IdempotentByConfirmRequestID(t *testing.T) {
	t.Parallel()
	taskRepo := newInMemoryHsModelTaskRepo()
	recoRepo := &spyRecommendationRepo{
		saved: []HsModelRecommendationRecord{
			{Model: "M", Manufacturer: "NXP", RunID: "run-1", CandidateRank: 1, CodeTS: "1234567890", Score: 0.88},
		},
	}
	mapRepo := &spyMappingRepo{}
	confirmRepo := newInMemoryHsModelConfirmRepo()
	taskRepo.Save(context.Background(), &HsModelTaskRecord{Model: "M", Manufacturer: "NXP", RunID: "run-1", ResultStatus: HsResultStatusPendingReview})

	svc := NewHsModelConfirmService(taskRepo, recoRepo, mapRepo, confirmRepo)
	first, err := svc.Confirm(context.Background(), HsModelConfirmRequest{
		RunID: "run-1", CandidateRank: 1, ExpectedCodeTS: "1234567890", ConfirmRequestID: "confirm-1",
	})
	if err != nil {
		t.Fatalf("first confirm failed: %v", err)
	}
	second, err := svc.Confirm(context.Background(), HsModelConfirmRequest{
		RunID: "run-1", CandidateRank: 1, ExpectedCodeTS: "1234567890", ConfirmRequestID: "confirm-1",
	})
	if err != nil {
		t.Fatalf("second confirm failed: %v", err)
	}
	if first.RunID != second.RunID || len(mapRepo.saved) != 1 {
		t.Fatalf("expected idempotent confirm, got first=%+v second=%+v writes=%d", first, second, len(mapRepo.saved))
	}
}

func TestHsModelConfirm_MissAliasLeavesMappingCanonicalNil(t *testing.T) {
	t.Parallel()
	taskRepo := newInMemoryHsModelTaskRepo()
	recoRepo := &spyRecommendationRepo{
		saved: []HsModelRecommendationRecord{
			{Model: "M", Manufacturer: "UnknownMfr", RunID: "run-cfm", CandidateRank: 1, CodeTS: "1234567890", Score: 0.88},
		},
	}
	mapRepo := &spyMappingRepo{}
	confirmRepo := newInMemoryHsModelConfirmRepo()
	taskRepo.Save(context.Background(), &HsModelTaskRecord{Model: "M", Manufacturer: "UnknownMfr", RunID: "run-cfm", ResultStatus: HsResultStatusPendingReview})

	svc := NewHsModelConfirmService(taskRepo, recoRepo, mapRepo, confirmRepo).
		WithManufacturerCanonicalizer(canonicalizerAliasLookup{rows: map[string]string{"TI": "mfr-ti"}})

	_, err := svc.Confirm(context.Background(), HsModelConfirmRequest{
		RunID: "run-cfm", CandidateRank: 1, ExpectedCodeTS: "1234567890", ConfirmRequestID: "confirm-miss",
	})
	if err != nil {
		t.Fatalf("confirm failed: %v", err)
	}
	if len(mapRepo.saved) != 1 {
		t.Fatalf("expected one mapping save, got %d", len(mapRepo.saved))
	}
	if mapRepo.saved[0].ManufacturerCanonicalID != nil {
		t.Fatalf("expected nil canonical on alias miss (strategy D preserve in data), got %v", mapRepo.saved[0].ManufacturerCanonicalID)
	}
}

func TestHsModelResolverConfirm_RejectWhenCandidateTupleMismatch(t *testing.T) {
	t.Parallel()
	taskRepo := newInMemoryHsModelTaskRepo()
	recoRepo := &spyRecommendationRepo{
		saved: []HsModelRecommendationRecord{
			{Model: "M", Manufacturer: "NXP", RunID: "run-1", CandidateRank: 1, CodeTS: "9999999999", Score: 0.88},
		},
	}
	mapRepo := &spyMappingRepo{}
	confirmRepo := newInMemoryHsModelConfirmRepo()
	taskRepo.Save(context.Background(), &HsModelTaskRecord{Model: "M", Manufacturer: "NXP", RunID: "run-1", ResultStatus: HsResultStatusPendingReview})

	svc := NewHsModelConfirmService(taskRepo, recoRepo, mapRepo, confirmRepo)
	_, err := svc.Confirm(context.Background(), HsModelConfirmRequest{
		RunID: "run-1", CandidateRank: 1, ExpectedCodeTS: "1234567890", ConfirmRequestID: "c-mismatch",
	})
	if err == nil || !errors.Is(err, ErrHsResolverConfirmTupleMismatch) {
		t.Fatalf("expected tuple mismatch error, got %v", err)
	}
}
