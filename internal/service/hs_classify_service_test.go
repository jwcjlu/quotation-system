package service

import (
	"context"
	"io"
	"testing"
	"time"

	v1 "caichip/api/bom/v1"
	"caichip/internal/biz"

	"github.com/go-kratos/kratos/v2/log"
)

func TestClassifyByModel(t *testing.T) {
	svc := &BomService{hsUC: biz.NewHSClassifyUsecase(&fakePolicyRepo2{}, &fakeCaseRepo2{}, &fakeReviewRepo2{}, biz.NewHSFinalDecisionEngine(), log.NewStdLogger(io.Discard))}
	_, err := svc.ClassifyByModel(context.Background(), &v1.ClassifyByModelRequest{})
	if err == nil {
		t.Fatalf("expect invalid argument")
	}
	res, err := svc.ClassifyByModel(context.Background(), &v1.ClassifyByModelRequest{
		TradeDirection:  "import",
		DeclarationDate: "2026-04-14",
		Model:           "STM32F103C8T6",
		ProductNameCn:   "微控制器",
		Manufacturer:    "ST",
		Package:         "LQFP48",
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.FinalSuggestion == nil {
		t.Fatalf("expect final suggestion")
	}
	if res.FinalSuggestion.ReviewRequired && len(res.FinalSuggestion.ReviewReasonCodes) == 0 {
		t.Fatalf("review required must have reason codes")
	}
}

type fakePolicyRepo2 struct{}

func (f *fakePolicyRepo2) DBOk() bool { return true }
func (f *fakePolicyRepo2) LoadByDeclarationDate(context.Context, time.Time) (*biz.HSClassifyPolicy, error) {
	return &biz.HSClassifyPolicy{VersionID: "mvp-v1", AutoPassConfidenceMin: 85, AutoPassCompletenessMin: 0.9, AutoPassTopGapMin: 8, QuickReviewTopGapMin: 5, ForceReviewConfidenceMax: 70, ForceReviewCompleteness: 0.6}, nil
}

type fakeCaseRepo2 struct{}

func (f *fakeCaseRepo2) DBOk() bool { return true }
func (f *fakeCaseRepo2) SearchTopCases(context.Context, *biz.HSClassifyRequest, int) ([]biz.HSReferenceCase, error) {
	return []biz.HSReferenceCase{{HSCode: "8542319000", Score: 88, Title: "case-1"}}, nil
}

type fakeReviewRepo2 struct{}

func (f *fakeReviewRepo2) DBOk() bool { return true }
func (f *fakeReviewRepo2) SaveDecision(context.Context, biz.HSReviewWrite) error { return nil }
