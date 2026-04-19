package data

import "time"

// HsTaxRateDaily 对应 t_hs_tax_rate_daily；列与税率接口 data.data[] 单条字段一致（见 docs/tax_rate_api）。
type HsTaxRateDaily struct {
	ID              uint64    `gorm:"column:id;primaryKey;autoIncrement"`
	CodeTS          string    `gorm:"column:code_ts;type:char(10);not null;uniqueIndex:uk_hs_tax_rate_daily,priority:1"`
	BizDate         time.Time `gorm:"column:biz_date;type:date;not null;uniqueIndex:uk_hs_tax_rate_daily,priority:2"`
	GName           string    `gorm:"column:g_name;size:512;not null;default:''"`
	ImpDiscountRate string    `gorm:"column:imp_discount_rate;size:32;not null;default:''"`
	ImpTempRate     string    `gorm:"column:imp_temp_rate;size:32;not null;default:''"`
	ImpOrdinaryRate string    `gorm:"column:imp_ordinary_rate;size:32;not null;default:''"`
	CreatedAt       time.Time `gorm:"column:created_at;precision:3;autoCreateTime"`
	UpdatedAt       time.Time `gorm:"column:updated_at;precision:3;autoUpdateTime"`
}

func (HsTaxRateDaily) TableName() string { return TableHsTaxRateDaily }
