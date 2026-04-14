package data

import (
	"context"
	"encoding/json"

	"caichip/internal/biz"
)

type HSReviewRepo struct {
	data *Data
}

func NewHSReviewRepo(data *Data) *HSReviewRepo { return &HSReviewRepo{data: data} }
func (r *HSReviewRepo) DBOk() bool             { return r != nil && r.data != nil && r.data.DB != nil }
func (r *HSReviewRepo) SaveDecision(ctx context.Context, row biz.HSReviewWrite) error {
	if !r.DBOk() {
		return nil
	}
	b, err := json.Marshal(row.ReviewReasons)
	if err != nil {
		return err
	}
	entity := &HsReviewDecision{
		RequestKey:      row.RequestKey,
		FinalHSCode:     row.FinalHSCode,
		ReviewRequired:  row.ReviewRequired,
		ReviewReasons:   string(b),
		PolicyVersionID: row.PolicyVersionID,
	}
	return r.data.DB.WithContext(ctx).Create(entity).Error
}
