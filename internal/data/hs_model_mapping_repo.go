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
	d     *Data
	alias biz.AliasLookup // 将 BOM/解析侧厂牌解析为 canonical_id，与 t_hs_model_mapping.manufacturer_canonical_id 对齐
}

func NewHsModelMappingRepo(d *Data, alias biz.AliasLookup) *HsModelMappingRepo {
	if d == nil || d.DB == nil {
		return &HsModelMappingRepo{alias: alias}
	}
	return &HsModelMappingRepo{d: d, alias: alias}
}

// scopeConfirmedManufacturer 非空厂牌：优先 manufacturer_canonical_id，旧行无 canonical 时回退 manufacturer 原文等值。
func (r *HsModelMappingRepo) scopeConfirmedManufacturer(db *gorm.DB, ctx context.Context, manufacturer string) *gorm.DB {
	if r == nil || r.alias == nil {
		return db.Where("manufacturer = ?", manufacturer)
	}
	cid, hit, err := biz.ResolveManufacturerCanonical(ctx, manufacturer, r.alias)
	if err != nil || !hit {
		return db.Where("manufacturer = ?", manufacturer)
	}
	cid = strings.TrimSpace(cid)
	if cid == "" {
		return db.Where("manufacturer = ?", manufacturer)
	}
	return db.Where(
		"(manufacturer_canonical_id = ? OR ((manufacturer_canonical_id IS NULL OR TRIM(manufacturer_canonical_id) = '') AND manufacturer = ?))",
		cid,
		manufacturer,
	)
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
	if model == "" {
		return nil, nil
	}
	// manufacturer 允许为空字符串：对应库内 (model, '') 的 confirmed 行（空厂牌映射）。
	q := r.d.DB.WithContext(ctx).Where("model = ? AND status = ?", model, "confirmed")
	if manufacturer == "" {
		q = q.Where("manufacturer = ?", "")
	} else {
		q = r.scopeConfirmedManufacturer(q, ctx, manufacturer)
	}
	var row HsModelMapping
	err := q.Limit(1).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &biz.HsModelMappingRecord{
		Model:                   row.Model,
		Manufacturer:            row.Manufacturer,
		ManufacturerCanonicalID: row.ManufacturerCanonicalID,
		CodeTS:                  row.CodeTS,
		Source:                  row.Source,
		Confidence:              row.Confidence,
		Status:                  row.Status,
		FeaturesVersion:         row.FeaturesVersion,
		RecommendationVersion:   row.RecommendationVersion,
		CreatedAt:               row.CreatedAt,
		UpdatedAt:               row.UpdatedAt,
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
	if model == "" || codeTS == "" || status == "" {
		return fmt.Errorf("hs_model_mapping: model/code_ts/status required")
	}
	// manufacturer 允许 ''，与 uk_hs_model_mapping_model_mfr (model, manufacturer) 中空厂牌行一致。
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
	if record.ManufacturerCanonicalID != nil {
		row.ManufacturerCanonicalID = record.ManufacturerCanonicalID
	}
	updates := map[string]any{
		"code_ts":                row.CodeTS,
		"source":                 row.Source,
		"confidence":             row.Confidence,
		"status":                 row.Status,
		"features_version":       row.FeaturesVersion,
		"recommendation_version": row.RecommendationVersion,
		"updated_at":             gorm.Expr("CURRENT_TIMESTAMP(3)"),
	}
	if record.ManufacturerCanonicalID != nil {
		updates["manufacturer_canonical_id"] = record.ManufacturerCanonicalID
	}
	return r.d.DB.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "model"}, {Name: "manufacturer"}},
		DoUpdates: clause.Assignments(updates),
	}).Create(&row).Error
}

var _ biz.HsModelMappingRepo = (*HsModelMappingRepo)(nil)
