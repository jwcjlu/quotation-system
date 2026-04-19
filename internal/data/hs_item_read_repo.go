package data

import (
	"context"
	"errors"
	"strings"

	"caichip/internal/biz"

	"gorm.io/gorm"
)

// HsItemReadRepo 实现 biz.HsItemReadRepo。
type HsItemReadRepo struct {
	d *Data
}

func NewHsItemReadRepo(d *Data) *HsItemReadRepo {
	return &HsItemReadRepo{d: d}
}

func (r *HsItemReadRepo) DBOk() bool {
	return r != nil && r.d != nil && r.d.DB != nil
}

type hsItemReadRow struct {
	CodeTS        string `gorm:"column:code_ts"`
	GName         string `gorm:"column:g_name"`
	Unit1         string `gorm:"column:unit_1"`
	Unit2         string `gorm:"column:unit_2"`
	ControlMark   string `gorm:"column:control_mark"`
	SourceCoreHS6 string `gorm:"column:source_core_hs6"`
	RawJSON       []byte `gorm:"column:raw_json"`
}

func (r *HsItemReadRepo) List(ctx context.Context, filter biz.HsItemListFilter) ([]biz.HsItemRecord, int64, error) {
	if !r.DBOk() {
		return nil, 0, errors.New("hs_item: database not configured")
	}
	page := filter.Page
	if page < 1 {
		page = 1
	}
	pageSize := filter.PageSize
	if pageSize < 1 {
		pageSize = 20
	}
	if pageSize > 200 {
		pageSize = 200
	}
	q := r.d.DB.Debug().WithContext(ctx).Table(TableHsItem)
	if v := strings.TrimSpace(filter.CodeTS); v != "" {
		q = q.Where("code_ts = ?", v)
	}
	if v := strings.TrimSpace(filter.GName); v != "" {
		q = q.Where("g_name LIKE ?", "%"+v+"%")
	}
	if v := strings.TrimSpace(filter.SourceCoreHS6); v != "" {
		q = q.Where("source_core_hs6 = ?", v)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []hsItemReadRow
	err := q.Select("code_ts, g_name, unit_1, unit_2, control_mark, source_core_hs6, raw_json").
		Order("updated_at DESC").
		Offset(int((page - 1) * pageSize)).
		Limit(int(pageSize)).
		Find(&rows).Error
	if err != nil {
		return nil, 0, err
	}
	out := make([]biz.HsItemRecord, 0, len(rows))
	for i := range rows {
		out = append(out, biz.HsItemRecord{
			CodeTS:        rows[i].CodeTS,
			GName:         rows[i].GName,
			Unit1:         rows[i].Unit1,
			Unit2:         rows[i].Unit2,
			ControlMark:   rows[i].ControlMark,
			SourceCoreHS6: rows[i].SourceCoreHS6,
			RawJSON:       append([]byte(nil), rows[i].RawJSON...),
		})
	}
	return out, total, nil
}

func (r *HsItemReadRepo) GetByCodeTS(ctx context.Context, codeTS string) (*biz.HsItemRecord, error) {
	if !r.DBOk() {
		return nil, errors.New("hs_item: database not configured")
	}
	code := strings.TrimSpace(codeTS)
	if code == "" {
		return nil, nil
	}
	var row hsItemReadRow
	err := r.d.DB.WithContext(ctx).
		Table(TableHsItem).
		Select("code_ts, g_name, unit_1, unit_2, control_mark, source_core_hs6, raw_json").
		Where("code_ts = ?", code).
		Take(&row).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &biz.HsItemRecord{
		CodeTS:        row.CodeTS,
		GName:         row.GName,
		Unit1:         row.Unit1,
		Unit2:         row.Unit2,
		ControlMark:   row.ControlMark,
		SourceCoreHS6: row.SourceCoreHS6,
		RawJSON:       append([]byte(nil), row.RawJSON...),
	}, nil
}

func (r *HsItemReadRepo) MapByCodeTS(ctx context.Context, codeTSList []string) (map[string]*biz.HsItemRecord, error) {
	if !r.DBOk() {
		return nil, errors.New("hs_item: database not configured")
	}
	seen := make(map[string]struct{})
	var codes []string
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
		return map[string]*biz.HsItemRecord{}, nil
	}
	var rows []hsItemReadRow
	err := r.d.DB.WithContext(ctx).
		Table(TableHsItem).
		Select("code_ts, g_name, unit_1, unit_2, control_mark, source_core_hs6, raw_json").
		Where("code_ts IN ?", codes).
		Find(&rows).Error
	if err != nil {
		return nil, err
	}
	out := make(map[string]*biz.HsItemRecord, len(rows))
	for i := range rows {
		c := strings.TrimSpace(rows[i].CodeTS)
		cp := biz.HsItemRecord{
			CodeTS:        rows[i].CodeTS,
			GName:         rows[i].GName,
			Unit1:         rows[i].Unit1,
			Unit2:         rows[i].Unit2,
			ControlMark:   rows[i].ControlMark,
			SourceCoreHS6: rows[i].SourceCoreHS6,
			RawJSON:       append([]byte(nil), rows[i].RawJSON...),
		}
		out[c] = &cp
	}
	return out, nil
}

var _ biz.HsItemReadRepo = (*HsItemReadRepo)(nil)
