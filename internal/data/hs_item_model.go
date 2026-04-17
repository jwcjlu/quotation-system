package data

import "time"

// HsItem 对应 t_hs_item（HS 条目查询数据）。
type HsItem struct {
	ID            uint64    `gorm:"column:id;primaryKey;autoIncrement"`
	CodeTS        string    `gorm:"column:code_ts;size:16;not null;uniqueIndex:uk_hs_item_code_ts"`
	GName         string    `gorm:"column:g_name;size:512;not null"`
	Unit1         string    `gorm:"column:unit_1;size:16;not null;default:''"`
	Unit2         string    `gorm:"column:unit_2;size:16;not null;default:''"`
	ControlMark   string    `gorm:"column:control_mark;size:64;not null;default:''"`
	SourceCoreHS6 string    `gorm:"column:source_core_hs6;size:6;not null;default:'';index:idx_hs_item_source_core_hs6"`
	RawJSON       []byte    `gorm:"column:raw_json;type:json"`
	UpdatedAt     time.Time `gorm:"column:updated_at;precision:3;autoUpdateTime;index:idx_hs_item_updated"`
}

func (HsItem) TableName() string { return TableHsItem }
