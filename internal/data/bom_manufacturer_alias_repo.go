package data

import (
	"context"
	"errors"
	"strings"

	"caichip/internal/biz"

	"gorm.io/gorm"
)

// ManufacturerCanonicalRow distinct canonical_id + 任一展示名（MIN 聚合）。
type ManufacturerCanonicalRow struct {
	CanonicalID string `gorm:"column:canonical_id"`
	DisplayName string `gorm:"column:display_name"`
}

// BomManufacturerAliasRepo 厂牌别名表（t_bom_manufacturer_alias）；无 DB 时 CanonicalID 恒为未命中。
type BomManufacturerAliasRepo struct {
	db *gorm.DB
}

// NewBomManufacturerAliasRepo ...
func NewBomManufacturerAliasRepo(d *Data) *BomManufacturerAliasRepo {
	if d == nil || d.DB == nil {
		return &BomManufacturerAliasRepo{}
	}
	return &BomManufacturerAliasRepo{db: d.DB}
}

// CanonicalID 按 alias_norm 精确匹配一行，返回 canonical_id。
// ok=true 且 err=nil 表示命中；ok=false 且 err=nil 表示无行；err!=nil 表示数据库等基础设施错误。
func (r *BomManufacturerAliasRepo) CanonicalID(ctx context.Context, aliasNorm string) (string, bool, error) {
	if r == nil || r.db == nil {
		return "", false, nil
	}
	aliasNorm = strings.TrimSpace(aliasNorm)
	if aliasNorm == "" {
		return "", false, nil
	}
	var row BomManufacturerAlias
	err := r.db.WithContext(ctx).Where("alias_norm = ?", aliasNorm).Select("canonical_id").First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	if row.CanonicalID == "" {
		return "", false, nil
	}
	return row.CanonicalID, true, nil
}

func dedupeNonEmptyAliasNormKeys(keys []string) []string {
	seen := make(map[string]struct{})
	out := make([]string, 0, len(keys))
	for _, raw := range keys {
		k := strings.TrimSpace(raw)
		if k == "" {
			continue
		}
		if _, ok := seen[k]; ok {
			continue
		}
		seen[k] = struct{}{}
		out = append(out, k)
	}
	return out
}

// CanonicalIDsByNormKeys 一次 IN 查询解析多个 alias_norm（与逐条 CanonicalID 语义一致：首行优先）。
func (r *BomManufacturerAliasRepo) CanonicalIDsByNormKeys(ctx context.Context, aliasNormKeys []string) (map[string]string, error) {
	if r == nil || r.db == nil {
		return map[string]string{}, nil
	}
	uniq := dedupeNonEmptyAliasNormKeys(aliasNormKeys)
	if len(uniq) == 0 {
		return map[string]string{}, nil
	}
	var rows []BomManufacturerAlias
	err := r.db.WithContext(ctx).
		Where("alias_norm IN ?", uniq).
		Order("alias_norm ASC, id ASC").
		Find(&rows).Error
	if err != nil {
		return nil, err
	}
	out := make(map[string]string, len(uniq))
	for i := range rows {
		n := strings.TrimSpace(rows[i].AliasNorm)
		c := strings.TrimSpace(rows[i].CanonicalID)
		if n == "" || c == "" {
			continue
		}
		if _, ok := out[n]; ok {
			continue
		}
		out[n] = c
	}
	return out, nil
}

// CreateRow 插入一条别名；alias_norm 须由调用方按 biz.NormalizeMfrString 与配单一致地计算。
func (r *BomManufacturerAliasRepo) CreateRow(ctx context.Context, canonicalID, displayName, alias, aliasNorm string) error {
	if r == nil || r.db == nil {
		return errors.New("bom manufacturer alias: database not configured")
	}
	row := BomManufacturerAlias{
		CanonicalID: canonicalID,
		DisplayName: displayName,
		Alias:       alias,
		AliasNorm:   aliasNorm,
	}
	return r.db.WithContext(ctx).Create(&row).Error
}

// ListDistinctCanonicals 按 canonical_id 去重，display_name 取字典序最小的一条（便于下拉展示）。
func (r *BomManufacturerAliasRepo) ListDistinctCanonicals(ctx context.Context, limit int) ([]biz.ManufacturerCanonicalDisplay, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("bom manufacturer alias: database not configured")
	}
	if limit <= 0 {
		limit = 300
	}
	if limit > 1000 {
		limit = 1000
	}
	var raw []ManufacturerCanonicalRow
	err := r.db.WithContext(ctx).
		Model(&BomManufacturerAlias{}).
		Select("canonical_id, MIN(display_name) AS display_name").
		Group("canonical_id").
		Order("canonical_id ASC").
		Limit(limit).
		Scan(&raw).Error
	if err != nil {
		return nil, err
	}
	out := make([]biz.ManufacturerCanonicalDisplay, 0, len(raw))
	for _, row := range raw {
		out = append(out, biz.ManufacturerCanonicalDisplay{
			CanonicalID: row.CanonicalID,
			DisplayName: row.DisplayName,
		})
	}
	return out, nil
}

// DBOk 是否已连接数据库（厂牌别名解析依赖）。
func (r *BomManufacturerAliasRepo) DBOk() bool {
	return r != nil && r.db != nil
}

var _ biz.AliasLookup = (*BomManufacturerAliasRepo)(nil)
var _ biz.BomManufacturerAliasRepo = (*BomManufacturerAliasRepo)(nil)
