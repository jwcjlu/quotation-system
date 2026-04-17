package data

import "time"

// HsMeta 对应 t_hs_meta（HS 核心码元数据配置）。
type HsMeta struct {
	ID            uint64    `gorm:"column:id;primaryKey;autoIncrement"`
	Category      string    `gorm:"column:category;size:64;not null;default:'';index:idx_hs_meta_category"`
	ComponentName string    `gorm:"column:component_name;size:128;not null;default:'';uniqueIndex:uk_hs_meta_core_component,priority:2"`
	CoreHS6       string    `gorm:"column:core_hs6;size:6;not null;uniqueIndex:uk_hs_meta_core_component,priority:1;index:idx_hs_meta_core_hs6"`
	Description   string    `gorm:"column:description;size:512;not null;default:''"`
	Enabled       bool      `gorm:"column:enabled;not null;default:true;index:idx_hs_meta_enabled"`
	SortOrder     int       `gorm:"column:sort_order;not null;default:0"`
	CreatedAt     time.Time `gorm:"column:created_at;precision:3;autoCreateTime"`
	UpdatedAt     time.Time `gorm:"column:updated_at;precision:3;autoUpdateTime"`
}

func (HsMeta) TableName() string { return TableHsMeta }
