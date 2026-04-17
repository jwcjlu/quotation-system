package data

import (
	"context"
	"errors"

	"caichip/internal/biz"
	"gorm.io/gorm/clause"
)

// HsItemWriteRepo 实现 biz.HsItemWriteRepo。
type HsItemWriteRepo struct {
	d *Data
}

func NewHsItemWriteRepo(d *Data) *HsItemWriteRepo {
	return &HsItemWriteRepo{d: d}
}

func (r *HsItemWriteRepo) DBOk() bool {
	return r != nil && r.d != nil && r.d.DB != nil
}

func (r *HsItemWriteRepo) UpsertByCodeTS(ctx context.Context, rows []biz.HsItemRecord) error {
	if !r.DBOk() {
		return errors.New("hs_item: database not configured")
	}
	if len(rows) == 0 {
		return nil
	}
	models := make([]HsItem, 0, len(rows))
	for i := range rows {
		models = append(models, HsItem{
			CodeTS:        rows[i].CodeTS,
			GName:         rows[i].GName,
			Unit1:         rows[i].Unit1,
			Unit2:         rows[i].Unit2,
			ControlMark:   rows[i].ControlMark,
			SourceCoreHS6: rows[i].SourceCoreHS6,
			RawJSON:       append([]byte(nil), rows[i].RawJSON...),
		})
	}
	return r.d.DB.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "code_ts"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"g_name", "unit_1", "unit_2", "control_mark", "source_core_hs6", "raw_json", "updated_at",
		}),
	}).Create(&models).Error
}

var _ biz.HsItemWriteRepo = (*HsItemWriteRepo)(nil)
