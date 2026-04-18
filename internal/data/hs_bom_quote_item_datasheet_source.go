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
	d *Data
}

func NewHsBomQuoteItemDatasheetSource(d *Data) *HsBomQuoteItemDatasheetSource {
	return &HsBomQuoteItemDatasheetSource{d: d}
}

func NewHsBomQuoteItemDatasheetSourceFromAssetRepo(assetRepo *HsDatasheetAssetRepo) *HsBomQuoteItemDatasheetSource {
	if assetRepo == nil {
		return nil
	}
	return NewHsBomQuoteItemDatasheetSource(assetRepo.d)
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
	err := s.d.DB.WithContext(ctx).
		Where("model = ? AND manufacturer = ?", model, manufacturer).
		Where("datasheet_url IS NOT NULL AND TRIM(datasheet_url) <> ''").
		Order("updated_at DESC, id DESC").
		Find(&rows).Error
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
	err := s.d.DB.WithContext(ctx).
		Where("model = ? AND manufacturer = ?", model, manufacturer).
		Where("datasheet_url IS NOT NULL AND TRIM(datasheet_url) <> ''").
		Order("updated_at DESC, id DESC").
		Limit(1).
		First(&row).Error
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
