package data

import (
	"bytes"
	"context"
	"fmt"
	"math"
	"strings"

	"caichip/internal/biz"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// HsModelRecommendationRepo 实现 biz.HsModelRecommendationRepo。
type HsModelRecommendationRepo struct {
	d *Data
}

func NewHsModelRecommendationRepo(d *Data) *HsModelRecommendationRepo {
	return &HsModelRecommendationRepo{d: d}
}

func (r *HsModelRecommendationRepo) DBOk() bool {
	return r != nil && r.d != nil && r.d.DB != nil
}

func (r *HsModelRecommendationRepo) SaveTopN(ctx context.Context, rows []biz.HsModelRecommendationRecord) error {
	if !r.DBOk() {
		return gorm.ErrInvalidDB
	}
	if len(rows) == 0 {
		return nil
	}
	models := make([]HsModelRecommendation, 0, len(rows))
	for _, item := range rows {
		model := strings.TrimSpace(item.Model)
		manufacturer := strings.TrimSpace(item.Manufacturer)
		runID := strings.TrimSpace(item.RunID)
		codeTS := strings.TrimSpace(item.CodeTS)
		if model == "" || manufacturer == "" || runID == "" || item.CandidateRank == 0 || codeTS == "" {
			return fmt.Errorf("hs_model_recommendation: model/manufacturer/run_id/rank/code_ts required")
		}
		models = append(models, HsModelRecommendation{
			Model:             model,
			Manufacturer:      manufacturer,
			RunID:             runID,
			CandidateRank:     item.CandidateRank,
			CodeTS:            codeTS,
			GName:             strings.TrimSpace(item.GName),
			Score:             item.Score,
			Reason:            strings.TrimSpace(item.Reason),
			InputSnapshotJSON: append([]byte(nil), item.InputSnapshotJSON...),
			RecommendModel:    strings.TrimSpace(item.RecommendModel),
			RecommendVersion:  strings.TrimSpace(item.RecommendVersion),
		})
	}
	return r.d.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for i := range models {
			res := tx.Clauses(clause.OnConflict{
				Columns:   []clause.Column{{Name: "run_id"}, {Name: "candidate_rank"}},
				DoNothing: true,
			}).Create(&models[i])
			if res.Error != nil {
				return res.Error
			}
			if res.RowsAffected > 0 {
				continue
			}

			var existing HsModelRecommendation
			if err := tx.Where("run_id = ? AND candidate_rank = ?", models[i].RunID, models[i].CandidateRank).
				Limit(1).
				First(&existing).Error; err != nil {
				return err
			}
			if hsModelRecommendationEquivalent(existing, models[i]) {
				continue
			}
			return fmt.Errorf("hs_model_recommendation: conflict on run_id=%s candidate_rank=%d", models[i].RunID, models[i].CandidateRank)
		}
		return nil
	})
}

func hsModelRecommendationEquivalent(a, b HsModelRecommendation) bool {
	return a.Model == b.Model &&
		a.Manufacturer == b.Manufacturer &&
		a.RunID == b.RunID &&
		a.CandidateRank == b.CandidateRank &&
		a.CodeTS == b.CodeTS &&
		a.GName == b.GName &&
		floatAlmostEqual(a.Score, b.Score, 1e-6) &&
		a.Reason == b.Reason &&
		bytes.Equal(a.InputSnapshotJSON, b.InputSnapshotJSON) &&
		a.RecommendModel == b.RecommendModel &&
		a.RecommendVersion == b.RecommendVersion
}

func floatAlmostEqual(a, b, epsilon float64) bool {
	return math.Abs(a-b) <= epsilon
}

func (r *HsModelRecommendationRepo) ListByRunID(ctx context.Context, runID string) ([]biz.HsModelRecommendationRecord, error) {
	if !r.DBOk() {
		return nil, nil
	}
	runID = strings.TrimSpace(runID)
	if runID == "" {
		return nil, nil
	}
	var rows []HsModelRecommendation
	if err := r.d.DB.WithContext(ctx).
		Where("run_id = ?", runID).
		Order("candidate_rank ASC").
		Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]biz.HsModelRecommendationRecord, 0, len(rows))
	for i := range rows {
		out = append(out, biz.HsModelRecommendationRecord{
			Model:             rows[i].Model,
			Manufacturer:      rows[i].Manufacturer,
			RunID:             rows[i].RunID,
			CandidateRank:     rows[i].CandidateRank,
			CodeTS:            rows[i].CodeTS,
			GName:             rows[i].GName,
			Score:             rows[i].Score,
			Reason:            rows[i].Reason,
			InputSnapshotJSON: append([]byte(nil), rows[i].InputSnapshotJSON...),
			RecommendModel:    rows[i].RecommendModel,
			RecommendVersion:  rows[i].RecommendVersion,
			CreatedAt:         rows[i].CreatedAt,
		})
	}
	return out, nil
}

var _ biz.HsModelRecommendationRepo = (*HsModelRecommendationRepo)(nil)
