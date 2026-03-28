package data

import (
	"fmt"

	"gorm.io/gorm"
)

// AutoMigrateSchema 根据 GORM 模型创建或更新表：新增表、缺列补列、按 tag 补索引。
// 限制：不删除列/索引；重命名、改类型、复杂约束仍需手工 SQL（见 docs/schema）。
func AutoMigrateSchema(db *gorm.DB) error {
	if db == nil {
		return nil
	}
	// 父表在前，便于外键（若模型上声明了 constraint）
	if err := db.AutoMigrate(
		&BomSession{},
		&BomSessionLine{},
		&BomQuoteCache{},
		&BomSearchTask{},
		&BomMergeInflight{},
		&BomPlatformScript{},
		&CaichipAgent{},
		&CaichipAgentTag{},
		&CaichipAgentInstalledScript{},
		&CaichipAgentScriptAuth{},
		&CaichipDispatchTask{},
		&AgentScriptPackage{},
	); err != nil {
		return fmt.Errorf("gorm automigrate: %w", err)
	}
	return seedBOMPlatformScriptsIfEmpty(db)
}

func seedBOMPlatformScriptsIfEmpty(db *gorm.DB) error {
	var n int64
	if err := db.Model(&BomPlatformScript{}).Limit(1).Count(&n).Error; err != nil {
		return fmt.Errorf("count bom_platform_script: %w", err)
	}
	if n > 0 {
		return nil
	}
	rows := []BomPlatformScript{
		{PlatformID: "find_chips", ScriptID: "find_chips", DisplayName: strPtr("FindChips"), Enabled: true},
		{PlatformID: "hqchip", ScriptID: "hqchip", DisplayName: strPtr("HQChip"), Enabled: true},
		{PlatformID: "icgoo", ScriptID: "icgoo", DisplayName: strPtr("ICGOO"), Enabled: true},
		{PlatformID: "ickey", ScriptID: "ickey", DisplayName: strPtr("云汉芯城"), Enabled: true},
		{PlatformID: "szlcsc", ScriptID: "szlcsc", DisplayName: strPtr("立创商城"), Enabled: true},
	}
	return db.Create(&rows).Error
}

func strPtr(s string) *string { return &s }
