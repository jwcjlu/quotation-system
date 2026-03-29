package data

import (
	"context"
	"errors"
	"strings"
	"time"

	"caichip/internal/biz"

	"gorm.io/gorm"
)

// BomFxRateRepo 配单汇率表 t_bom_fx_rate；实现 biz.FXRateLookup。
type BomFxRateRepo struct {
	db *gorm.DB
}

// NewBomFxRateRepo 无 DB 时仍返回非 nil，Rate 恒为 !ok。
func NewBomFxRateRepo(db *gorm.DB) *BomFxRateRepo {
	return &BomFxRateRepo{db: db}
}

// NewBomFxRateRepoFromData 供 Wire 注入；与 session/search 一致，无 DB 时 db 为 nil。
func NewBomFxRateRepoFromData(d *Data) *BomFxRateRepo {
	if d == nil || d.DB == nil {
		return &BomFxRateRepo{}
	}
	return &BomFxRateRepo{db: d.DB}
}

// DBOk 是否已连接数据库（配单比价汇率查询依赖）。
func (r *BomFxRateRepo) DBOk() bool {
	return r != nil && r.db != nil
}

// Rate 按 **精确 biz_date**（日历日，UTC 日期部分）匹配一行。
// 若同一 (from,to,date) 下因历史数据或迁移存在多行，取 **id 最大** 的一行（确定性，与单测约定一致）。
func (r *BomFxRateRepo) Rate(ctx context.Context, from, to string, date time.Time) (rate float64, tableVersion, source string, ok bool) {
	if r == nil || r.db == nil {
		return 0, "", "", false
	}
	from = strings.ToUpper(strings.TrimSpace(from))
	to = strings.ToUpper(strings.TrimSpace(to))
	if from == "" || to == "" || date.IsZero() {
		return 0, "", "", false
	}
	bizDay := truncateBomFxDateUTC(date)

	var row BomFxRate
	err := r.db.WithContext(ctx).
		Where("from_ccy = ? AND to_ccy = ? AND biz_date = ?", from, to, bizDay).
		Order("id DESC").
		Limit(1).
		Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return 0, "", "", false
	}
	if err != nil {
		return 0, "", "", false
	}
	return row.Rate, row.TableVersion, row.Source, true
}

func truncateBomFxDateUTC(t time.Time) time.Time {
	y, m, d := t.In(time.UTC).Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

var _ biz.FXRateLookup = (*BomFxRateRepo)(nil)
