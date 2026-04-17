package data

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"caichip/internal/biz"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// HsModelMappingRepo 实现 biz.HsModelMappingRepo。
type HsModelMappingRepo struct {
	d *Data
}

func NewHsModelMappingRepo(d *Data) *HsModelMappingRepo {
	return &HsModelMappingRepo{d: d}
}

func (r *HsModelMappingRepo) DBOk() bool {
	return r != nil && r.d != nil && r.d.DB != nil
}

func (r *HsModelMappingRepo) GetConfirmedByModelManufacturer(ctx context.Context, model, manufacturer string) (*biz.HsModelMappingRecord, error) {
	if !r.DBOk() {
		return nil, nil
	}
	model = strings.TrimSpace(model)
	manufacturer = strings.TrimSpace(manufacturer)
	if model == "" || manufacturer == "" {
		return nil, nil
	}
	var row HsModelMapping
	err := r.d.DB.WithContext(ctx).
		Where("model = ? AND manufacturer = ? AND status = ?", model, manufacturer, "confirmed").
		Limit(1).
		First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &biz.HsModelMappingRecord{
		Model:                 row.Model,
		Manufacturer:          row.Manufacturer,
		CodeTS:                row.CodeTS,
		Source:                row.Source,
		Confidence:            row.Confidence,
		Status:                row.Status,
		FeaturesVersion:       row.FeaturesVersion,
		RecommendationVersion: row.RecommendationVersion,
		CreatedAt:             row.CreatedAt,
		UpdatedAt:             row.UpdatedAt,
	}, nil
}

func (r *HsModelMappingRepo) Save(ctx context.Context, record *biz.HsModelMappingRecord) error {
	if !r.DBOk() || record == nil {
		return gorm.ErrInvalidDB
	}
	model := strings.TrimSpace(record.Model)
	manufacturer := strings.TrimSpace(record.Manufacturer)
	codeTS := strings.TrimSpace(record.CodeTS)
	status := strings.TrimSpace(record.Status)
	if model == "" || manufacturer == "" || codeTS == "" || status == "" {
		return fmt.Errorf("hs_model_mapping: model/manufacturer/code_ts/status required")
	}
	row := HsModelMapping{
		Model:                 model,
		Manufacturer:          manufacturer,
		CodeTS:                codeTS,
		Source:                strings.TrimSpace(record.Source),
		Confidence:            record.Confidence,
		Status:                status,
		FeaturesVersion:       strings.TrimSpace(record.FeaturesVersion),
		RecommendationVersion: strings.TrimSpace(record.RecommendationVersion),
	}
	return r.d.DB.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "model"}, {Name: "manufacturer"}},
		DoUpdates: clause.Assignments(map[string]any{
			"code_ts":                row.CodeTS,
			"source":                 row.Source,
			"confidence":             row.Confidence,
			"status":                 row.Status,
			"features_version":       row.FeaturesVersion,
			"recommendation_version": row.RecommendationVersion,
			"updated_at":             gorm.Expr("CURRENT_TIMESTAMP(3)"),
		}),
	}).Create(&row).Error
}

var _ biz.HsModelMappingRepo = (*HsModelMappingRepo)(nil)
