package data

import (
	"context"
	"errors"
	"strings"

	"caichip/internal/biz"

	"gorm.io/gorm"
)

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
func (r *BomManufacturerAliasRepo) CanonicalID(ctx context.Context, aliasNorm string) (string, bool) {
	if r == nil || r.db == nil {
		return "", false
	}
	aliasNorm = strings.TrimSpace(aliasNorm)
	if aliasNorm == "" {
		return "", false
	}
	var row BomManufacturerAlias
	err := r.db.WithContext(ctx).Where("alias_norm = ?", aliasNorm).Select("canonical_id").First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return "", false
	}
	if err != nil {
		return "", false
	}
	if row.CanonicalID == "" {
		return "", false
	}
	return row.CanonicalID, true
}

var _ biz.AliasLookup = (*BomManufacturerAliasRepo)(nil)
