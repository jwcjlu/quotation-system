package data

import (
	"context"

	"caichip/internal/biz"
)

type HSCaseRepo struct {
	data *Data
}

func NewHSCaseRepo(data *Data) *HSCaseRepo { return &HSCaseRepo{data: data} }
func (r *HSCaseRepo) DBOk() bool           { return r != nil && r.data != nil }

func (r *HSCaseRepo) SearchTopCases(_ context.Context, req *biz.HSClassifyRequest, topN int) ([]biz.HSReferenceCase, error) {
	out := []biz.HSReferenceCase{
		{HSCode: "8542399000", Title: "fallback-1", Reason: "rule default", Score: 72, Evidence: []string{req.Model}},
		{HSCode: "8532241000", Title: "fallback-2", Reason: "similar package", Score: 66, Evidence: []string{req.Package}},
	}
	if topN < len(out) {
		return out[:topN], nil
	}
	return out, nil
}
