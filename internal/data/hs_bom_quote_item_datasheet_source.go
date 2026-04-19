package data

import (
	"context"
	"errors"
	"strings"

	"caichip/internal/biz"

	"gorm.io/gorm"
)

// HsBomQuoteItemDatasheetSource 从 t_bom_quote_item 读取 datasheet 候选。
type HsBomQuoteItemDatasheetSource struct {
	d     *Data
	alias biz.AliasLookup // 将需求厂牌解析为 canonical_id，与 manufacturer_canonical_id 对齐（避免报价原文别名查不到行）
}

func NewHsBomQuoteItemDatasheetSource(d *Data, alias biz.AliasLookup) *HsBomQuoteItemDatasheetSource {
	return &HsBomQuoteItemDatasheetSource{d: d, alias: alias}
}

func NewHsBomQuoteItemDatasheetSourceFromAssetRepo(assetRepo *HsDatasheetAssetRepo, alias biz.AliasLookup) *HsBomQuoteItemDatasheetSource {
	if assetRepo == nil {
		return nil
	}
	return NewHsBomQuoteItemDatasheetSource(assetRepo.d, alias)
}

// scopeModelManufacturer 按型号 + 厂牌定位明细：有 canonical 时优先 manufacturer_canonical_id，旧数据无 canonical 时回退 manufacturer 原文等值。
func (s *HsBomQuoteItemDatasheetSource) scopeModelManufacturer(db *gorm.DB, ctx context.Context, model, manufacturer string) *gorm.DB {
	return db.Where("model = ?", model)
	/*if s == nil || s.alias == nil {
		return db.Where("manufacturer = ?", manufacturer)
	}
	cid, hit, err := biz.ResolveManufacturerCanonical(ctx, manufacturer, s.alias)
	if err != nil || !hit {
		return db.Where("manufacturer = ?", manufacturer)
	}
	cid = strings.TrimSpace(cid)
	if cid == "" {
		return db.Where("manufacturer = ?", manufacturer)
	}
	return db.Debug().Where(
		"(manufacturer_canonical_id = ? OR ((manufacturer_canonical_id IS NULL OR TRIM(manufacturer_canonical_id) = '') AND manufacturer = ?))",
		cid,
		manufacturer,
	)*/
}

// ListQuoteDatasheetCandidates 返回同型号+厂牌下所有非空 datasheet 行（设计 §4.2 顺序：updated_at DESC, id DESC）。
func (s *HsBomQuoteItemDatasheetSource) ListQuoteDatasheetCandidates(ctx context.Context, model, manufacturer string) ([]biz.HsDatasheetCandidate, error) {
	if s == nil || s.d == nil || s.d.DB == nil {
		return nil, nil
	}
	model = strings.TrimSpace(model)
	manufacturer = strings.TrimSpace(manufacturer)
	if model == "" || manufacturer == "" {
		return nil, nil
	}
	var rows []BomQuoteItem
	q := s.scopeModelManufacturer(s.d.DB.WithContext(ctx), ctx, model, manufacturer).
		Where("datasheet_url IS NOT NULL AND TRIM(datasheet_url) <> ''").
		Order("updated_at DESC, id DESC")
	err := q.Find(&rows).Error
	if err != nil {
		return nil, err
	}
	out := make([]biz.HsDatasheetCandidate, 0, len(rows))
	for i := range rows {
		out = append(out, biz.HsDatasheetCandidate{
			ID:           rows[i].ID,
			DatasheetURL: strings.TrimSpace(rows[i].DatasheetURL),
			UpdatedAt:    rows[i].UpdatedAt,
		})
	}
	return out, nil
}

func (s *HsBomQuoteItemDatasheetSource) GetLatestByModelManufacturer(ctx context.Context, model, manufacturer string) (*biz.HsDatasheetAssetRecord, error) {
	if s == nil || s.d == nil || s.d.DB == nil {
		return nil, nil
	}
	model = strings.TrimSpace(model)
	manufacturer = strings.TrimSpace(manufacturer)
	if model == "" || manufacturer == "" {
		return nil, nil
	}

	var row BomQuoteItem
	q := s.scopeModelManufacturer(s.d.DB.WithContext(ctx), ctx, model, manufacturer).
		Where("datasheet_url IS NOT NULL AND TRIM(datasheet_url) <> ''").
		Order("updated_at DESC, id DESC").
		Limit(1)
	err := q.First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	return &biz.HsDatasheetAssetRecord{
		ID:           row.ID,
		Model:        row.Model,
		Manufacturer: row.Manufacturer,
		DatasheetURL: strings.TrimSpace(row.DatasheetURL),
		UpdatedAt:    row.UpdatedAt,
	}, nil
}
