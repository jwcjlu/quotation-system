package data

import (
	"context"
	"time"

	"caichip/internal/biz"
)

type HSPolicyRepo struct {
	data *Data
}

func NewHSPolicyRepo(data *Data) *HSPolicyRepo { return &HSPolicyRepo{data: data} }
func (r *HSPolicyRepo) DBOk() bool             { return r != nil && r.data != nil }

func (r *HSPolicyRepo) LoadByDeclarationDate(_ context.Context, _ time.Time) (*biz.HSClassifyPolicy, error) {
	return &biz.HSClassifyPolicy{
		VersionID:                "mvp-2026-04-13-v1",
		AutoPassConfidenceMin:    85,
		AutoPassCompletenessMin:  0.9,
		AutoPassTopGapMin:        8,
		QuickReviewTopGapMin:     5,
		QuickReviewConfidenceMin: 70,
		ForceReviewConfidenceMax: 70,
		ForceReviewCompleteness:  0.6,
	}, nil
}
