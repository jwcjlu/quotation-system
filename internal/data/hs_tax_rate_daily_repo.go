package data

import (
	"context"
	"errors"
	"strings"
	"time"

	"caichip/internal/biz"

	"gorm.io/gorm/clause"
)

// HsTaxRateDailyRepo 实现 biz.HsTaxRateDailyRepo。
type HsTaxRateDailyRepo struct {
	d *Data
}

func NewHsTaxRateDailyRepo(d *Data) *HsTaxRateDailyRepo {
	return &HsTaxRateDailyRepo{d: d}
}

func (r *HsTaxRateDailyRepo) DBOk() bool {
	return r != nil && r.d != nil && r.d.DB != nil
}

func truncateToDate(t time.Time) time.Time {
	t = t.In(time.Local)
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.Local)
}

func (r *HsTaxRateDailyRepo) GetManyByBizDate(ctx context.Context, bizDate time.Time, codeTSList []string) (map[string]*biz.HsTaxRateDailyRecord, error) {
	if !r.DBOk() {
		return nil, errors.New("hs_tax_rate_daily: database not configured")
	}
	bd := truncateToDate(bizDate)
	var codes []string
	seen := make(map[string]struct{})
	for _, c := range codeTSList {
		c = strings.TrimSpace(c)
		if c == "" {
			continue
		}
		if _, ok := seen[c]; ok {
			continue
		}
		seen[c] = struct{}{}
		codes = append(codes, c)
	}
	if len(codes) == 0 {
		return map[string]*biz.HsTaxRateDailyRecord{}, nil
	}
	var rows []HsTaxRateDaily
	err := r.d.DB.WithContext(ctx).
		Where("biz_date = ? AND code_ts IN ?", bd, codes).
		Find(&rows).Error
	if err != nil {
		return nil, err
	}
	out := make(map[string]*biz.HsTaxRateDailyRecord, len(rows))
	for i := range rows {
		k := strings.TrimSpace(rows[i].CodeTS)
		out[k] = &biz.HsTaxRateDailyRecord{
			CodeTS:          rows[i].CodeTS,
			BizDate:         truncateToDate(rows[i].BizDate),
			GName:           rows[i].GName,
			ImpDiscountRate: rows[i].ImpDiscountRate,
			ImpTempRate:     rows[i].ImpTempRate,
			ImpOrdinaryRate: rows[i].ImpOrdinaryRate,
		}
	}
	return out, nil
}

func (r *HsTaxRateDailyRepo) Upsert(ctx context.Context, row *biz.HsTaxRateDailyRecord) error {
	if !r.DBOk() {
		return errors.New("hs_tax_rate_daily: database not configured")
	}
	if row == nil || strings.TrimSpace(row.CodeTS) == "" {
		return errors.New("hs_tax_rate_daily: row/code_ts required")
	}
	bd := truncateToDate(row.BizDate)
	ent := HsTaxRateDaily{
		CodeTS:          strings.TrimSpace(row.CodeTS),
		BizDate:         bd,
		GName:           row.GName,
		ImpDiscountRate: row.ImpDiscountRate,
		ImpTempRate:     row.ImpTempRate,
		ImpOrdinaryRate: row.ImpOrdinaryRate,
	}
	return r.d.DB.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "code_ts"}, {Name: "biz_date"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"g_name", "imp_discount_rate", "imp_temp_rate", "imp_ordinary_rate",
		}),
	}).Create(&ent).Error
}

var _ biz.HsTaxRateDailyRepo = (*HsTaxRateDailyRepo)(nil)
