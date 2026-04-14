package data

import (
	"context"
	"encoding/json"
	"strings"

	"caichip/internal/biz"
)

type HSCaseRepo struct {
	data *Data
}

func NewHSCaseRepo(data *Data) *HSCaseRepo { return &HSCaseRepo{data: data} }
func (r *HSCaseRepo) DBOk() bool           { return r != nil && r.data != nil && r.data.DB != nil }

func (r *HSCaseRepo) SearchTopCases(ctx context.Context, req *biz.HSClassifyRequest, topN int) ([]biz.HSReferenceCase, error) {
	if r.DBOk() {
		var rows []HsCase
		q := r.data.DB.WithContext(ctx).Model(&HsCase{})
		if m := strings.TrimSpace(req.Model); m != "" {
			q = q.Where("model = ? OR model LIKE ?", m, m+"%")
		}
		if n := strings.TrimSpace(req.ProductNameCN); n != "" {
			q = q.Or("product_name_cn LIKE ?", "%"+n+"%")
		}
		if p := strings.TrimSpace(req.Package); p != "" {
			q = q.Or("package = ?", p)
		}
		if err := q.Order("source_trust DESC, updated_at DESC").Limit(topN).Find(&rows).Error; err == nil && len(rows) > 0 {
			out := make([]biz.HSReferenceCase, 0, len(rows))
			for _, row := range rows {
				var evidence []string
				_ = json.Unmarshal(row.EvidenceJSON, &evidence)
				out = append(out, biz.HSReferenceCase{
					HSCode:   row.HSCode,
					Title:    row.Title,
					Reason:   row.Reason,
					Score:    row.SourceTrust * 100,
					Evidence: evidence,
				})
			}
			return out, nil
		}
	}
	out := []biz.HSReferenceCase{
		{HSCode: "8542399000", Title: "fallback-1", Reason: "rule default", Score: 72, Evidence: []string{req.Model}},
		{HSCode: "8532241000", Title: "fallback-2", Reason: "similar package", Score: 66, Evidence: []string{req.Package}},
	}
	if topN < len(out) {
		return out[:topN], nil
	}
	return out, nil
}
