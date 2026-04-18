package data

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"caichip/internal/biz"

	"gorm.io/gorm"
)

// HsDatasheetAssetRepo 实现 biz.HsDatasheetAssetRepo。
type HsDatasheetAssetRepo struct {
	d *Data
}

func NewHsDatasheetAssetRepo(d *Data) *HsDatasheetAssetRepo {
	return &HsDatasheetAssetRepo{d: d}
}

func (r *HsDatasheetAssetRepo) DBOk() bool {
	return r != nil && r.d != nil && r.d.DB != nil
}

func (r *HsDatasheetAssetRepo) GetLatestByModelManufacturer(ctx context.Context, model, manufacturer string) (*biz.HsDatasheetAssetRecord, error) {
	if !r.DBOk() {
		return nil, nil
	}
	model = strings.TrimSpace(model)
	manufacturer = strings.TrimSpace(manufacturer)
	if model == "" || manufacturer == "" {
		return nil, nil
	}
	var row HsDatasheetAsset
	err := r.d.DB.WithContext(ctx).
		Where("model = ? AND manufacturer = ?", model, manufacturer).
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
		ID:             row.ID,
		Model:          row.Model,
		Manufacturer:   row.Manufacturer,
		DatasheetURL:   row.DatasheetURL,
		LocalPath:      row.LocalPath,
		SHA256:         row.SHA256,
		DownloadStatus: row.DownloadStatus,
		ErrorMsg:       row.ErrorMsg,
		UpdatedAt:      row.UpdatedAt,
	}, nil
}

func (r *HsDatasheetAssetRepo) Save(ctx context.Context, record *biz.HsDatasheetAssetRecord) error {
	if !r.DBOk() || record == nil {
		return gorm.ErrInvalidDB
	}
	model := strings.TrimSpace(record.Model)
	manufacturer := strings.TrimSpace(record.Manufacturer)
	if model == "" || manufacturer == "" {
		return fmt.Errorf("hs_datasheet_asset: model/manufacturer required")
	}
	row := HsDatasheetAsset{
		Model:          model,
		Manufacturer:   manufacturer,
		DatasheetURL:   strings.TrimSpace(record.DatasheetURL),
		LocalPath:      strings.TrimSpace(record.LocalPath),
		SHA256:         strings.TrimSpace(record.SHA256),
		DownloadStatus: strings.TrimSpace(record.DownloadStatus),
		ErrorMsg:       strings.TrimSpace(record.ErrorMsg),
	}
	if row.DownloadStatus == "" {
		row.DownloadStatus = "failed"
	}
	if err := r.d.DB.WithContext(ctx).Create(&row).Error; err != nil {
		return err
	}
	record.ID = row.ID
	return nil
}

var _ biz.HsDatasheetAssetRepo = (*HsDatasheetAssetRepo)(nil)
