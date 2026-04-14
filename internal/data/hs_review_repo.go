package data

import (
	"context"

	"caichip/internal/biz"
)

type HSReviewRepo struct {
	data *Data
}

func NewHSReviewRepo(data *Data) *HSReviewRepo { return &HSReviewRepo{data: data} }
func (r *HSReviewRepo) DBOk() bool             { return r != nil && r.data != nil }
func (r *HSReviewRepo) SaveDecision(context.Context, biz.HSReviewWrite) error {
	return nil
}
