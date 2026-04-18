package data

import (
	"context"
	"fmt"
	"strings"

	"caichip/internal/biz"

	"gorm.io/gorm"
)

// HsModelFeaturesRepo 实现 biz.HsModelFeaturesRepo。
type HsModelFeaturesRepo struct {
	d *Data
}

func NewHsModelFeaturesRepo(d *Data) *HsModelFeaturesRepo {
	return &HsModelFeaturesRepo{d: d}
}

func (r *HsModelFeaturesRepo) DBOk() bool {
	return r != nil && r.d != nil && r.d.DB != nil
}

func (r *HsModelFeaturesRepo) Create(ctx context.Context, record *biz.HsModelFeaturesRecord) (uint64, error) {
	if !r.DBOk() || record == nil {
		return 0, gorm.ErrInvalidDB
	}
	model := strings.TrimSpace(record.Model)
	manufacturer := strings.TrimSpace(record.Manufacturer)
	if model == "" || manufacturer == "" || record.AssetID == 0 {
		return 0, fmt.Errorf("hs_model_features: model/manufacturer/asset_id required")
	}
	row := HsModelFeatures{
		Model:                  model,
		Manufacturer:           manufacturer,
		AssetID:                record.AssetID,
		TechCategory:           strings.TrimSpace(record.TechCategory),
		TechCategoryRankedJSON: append([]byte(nil), record.TechCategoryRankedJSON...),
		ComponentName:          strings.TrimSpace(record.ComponentName),
		PackageForm:            strings.TrimSpace(record.PackageForm),
		KeySpecsJSON:           append([]byte(nil), record.KeySpecsJSON...),
		RawExtractJSON:         append([]byte(nil), record.RawExtractJSON...),
		ExtractModel:           strings.TrimSpace(record.ExtractModel),
		ExtractVersion:         strings.TrimSpace(record.ExtractVersion),
	}
	if err := r.d.DB.WithContext(ctx).Create(&row).Error; err != nil {
		return 0, err
	}
	return row.ID, nil
}

var _ biz.HsModelFeaturesRepo = (*HsModelFeaturesRepo)(nil)
