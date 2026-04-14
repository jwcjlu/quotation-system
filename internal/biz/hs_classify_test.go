package biz

import (
	"context"
	"io"
	"testing"
	"time"

	"github.com/go-kratos/kratos/v2/log"
)

type fakePolicyRepo struct{}

func (f *fakePolicyRepo) DBOk() bool { return true }
func (f *fakePolicyRepo) LoadByDeclarationDate(context.Context, time.Time) (*HSClassifyPolicy, error) {
	return &HSClassifyPolicy{VersionID: "mvp-v1", AutoPassConfidenceMin: 85, AutoPassCompletenessMin: 0.9, AutoPassTopGapMin: 8, QuickReviewTopGapMin: 5, ForceReviewConfidenceMax: 70, ForceReviewCompleteness: 0.6}, nil
}

type fakeCaseRepo struct{}

func (f *fakeCaseRepo) DBOk() bool { return true }
func (f *fakeCaseRepo) SearchTopCases(_ context.Context, req *HSClassifyRequest, _ int) ([]HSReferenceCase, error) {
	if req.Model == "LOWCONF" {
		return []HSReferenceCase{{HSCode: "8542399000", Score: 61}, {HSCode: "8542398000", Score: 60.5}}, nil
	}
	return []HSReferenceCase{{HSCode: "8532241000", Score: 90}, {HSCode: "8532249000", Score: 80}}, nil
}

type fakeReviewRepo struct{}

func (f *fakeReviewRepo) DBOk() bool                                { return true }
func (f *fakeReviewRepo) SaveDecision(context.Context, HSReviewWrite) error { return nil }

func TestHsClassify(t *testing.T) {
	uc := NewHSClassifyUsecase(&fakePolicyRepo{}, &fakeCaseRepo{}, &fakeReviewRepo{}, NewHSFinalDecisionEngine(), log.NewStdLogger(io.Discard))
	res, err := uc.ClassifyByModel(context.Background(), &HSClassifyRequest{
		TradeDirection:  "import",
		DeclarationDate: "2026-04-14",
		Model:           "CL10B104KB8NNNC",
		ProductNameCN:   "MLCC",
		Manufacturer:    "Samsung",
		Package:         "0402",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Candidates) == 0 || res.Trace.PolicyVersionID == "" {
		t.Fatalf("expect candidates and policy trace")
	}
}
