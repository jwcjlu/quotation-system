package data

import (
	"context"
	"errors"
	"strings"

	"caichip/internal/biz"

	"gorm.io/gorm"
)

// HsMetaRepo 实现 biz.HsMetaRepo。
type HsMetaRepo struct {
	db *gorm.DB
}

// NewHsMetaRepo ...
func NewHsMetaRepo(d *Data) *HsMetaRepo {
	if d == nil || d.DB == nil {
		return &HsMetaRepo{}
	}
	return &HsMetaRepo{db: d.DB}
}

func (r *HsMetaRepo) DBOk() bool { return r.db != nil }

func (r *HsMetaRepo) List(ctx context.Context, filter biz.HsMetaListFilter) ([]biz.HsMetaRecord, int64, error) {
	if r.db == nil {
		return nil, 0, errors.New("hs_meta: database not configured")
	}
	page := filter.Page
	if page < 1 {
		page = 1
	}
	ps := filter.PageSize
	if ps < 1 {
		ps = 20
	}
	if ps > 200 {
		ps = 200
	}
	q := r.db.WithContext(ctx).Model(&HsMeta{})
	if cat := strings.TrimSpace(filter.Category); cat != "" {
		q = q.Where("category = ?", cat)
	}
	if cn := strings.TrimSpace(filter.ComponentName); cn != "" {
		q = q.Where("component_name LIKE ?", "%"+cn+"%")
	}
	if ch := strings.TrimSpace(filter.CoreHS6); ch != "" {
		q = q.Where("core_hs6 = ?", ch)
	}
	if filter.Enabled != nil {
		q = q.Where("enabled = ?", *filter.Enabled)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	offset := int((page - 1) * ps)
	var rows []HsMeta
	if err := q.Order("sort_order ASC, id ASC").Offset(offset).Limit(int(ps)).Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	out := make([]biz.HsMetaRecord, 0, len(rows))
	for _, m := range rows {
		out = append(out, metaToBiz(&m))
	}
	return out, total, nil
}

func (r *HsMetaRepo) Create(ctx context.Context, row *biz.HsMetaRecord) error {
	if r.db == nil {
		return errors.New("hs_meta: database not configured")
	}
	if row == nil {
		return errors.New("hs_meta: nil row")
	}
	m := bizToMeta(row)
	m.ID = 0
	return r.db.WithContext(ctx).Create(&m).Error
}

func (r *HsMetaRepo) Update(ctx context.Context, row *biz.HsMetaRecord) error {
	if r.db == nil {
		return errors.New("hs_meta: database not configured")
	}
	if row == nil || row.ID == 0 {
		return errors.New("hs_meta: invalid id")
	}
	m := bizToMeta(row)
	res := r.db.WithContext(ctx).Model(&HsMeta{}).Where("id = ?", row.ID).Updates(map[string]any{
		"category":       m.Category,
		"component_name": m.ComponentName,
		"core_hs6":       m.CoreHS6,
		"description":    m.Description,
		"enabled":        m.Enabled,
		"sort_order":     m.SortOrder,
	})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (r *HsMetaRepo) Delete(ctx context.Context, id uint64) error {
	if r.db == nil {
		return errors.New("hs_meta: database not configured")
	}
	if id == 0 {
		return errors.New("hs_meta: invalid id")
	}
	res := r.db.WithContext(ctx).Delete(&HsMeta{}, id)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (r *HsMetaRepo) CountByCoreAndComponent(ctx context.Context, coreHS6, componentName string, excludeID uint64) (int64, error) {
	if r.db == nil {
		return 0, errors.New("hs_meta: database not configured")
	}
	q := r.db.WithContext(ctx).Model(&HsMeta{}).
		Where("core_hs6 = ? AND component_name = ?", strings.TrimSpace(coreHS6), strings.TrimSpace(componentName))
	if excludeID > 0 {
		q = q.Where("id <> ?", excludeID)
	}
	var n int64
	if err := q.Count(&n).Error; err != nil {
		return 0, err
	}
	return n, nil
}

func metaToBiz(m *HsMeta) biz.HsMetaRecord {
	return biz.HsMetaRecord{
		ID:            m.ID,
		Category:      m.Category,
		ComponentName: m.ComponentName,
		CoreHS6:       m.CoreHS6,
		Description:   m.Description,
		Enabled:       m.Enabled,
		SortOrder:     int32(m.SortOrder),
		UpdatedAt:     m.UpdatedAt,
	}
}

func bizToMeta(row *biz.HsMetaRecord) HsMeta {
	return HsMeta{
		ID:            row.ID,
		Category:      row.Category,
		ComponentName: row.ComponentName,
		CoreHS6:       row.CoreHS6,
		Description:   row.Description,
		Enabled:       row.Enabled,
		SortOrder:     int(row.SortOrder),
	}
}

var _ biz.HsMetaRepo = (*HsMetaRepo)(nil)
