package data

import (
	"encoding/json"
	"fmt"

	"gorm.io/gorm"
)

// bomDefaultRunParamsJSON 与历史硬编码 --parse-workers 8 对齐。
var bomDefaultRunParamsJSON = func() []byte {
	b, err := json.Marshal(map[string]any{"parse_workers": 8})
	if err != nil {
		panic(err)
	}
	return b
}()

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
		&BomSearchTask{},
		&BomMergeInflight{},
		&BomMergeProxyWait{},
		&BomPlatformScript{},
		&BomManufacturerAlias{},
		&BomFxRate{},
		&HsPolicyVersion{},
		&HsCase{},
		&HsReviewDecision{},
		&HsModelMapping{},
		&HsDatasheetAsset{},
		&HsManualDatasheetUpload{},
		&HsModelFeatures{},
		&HsModelRecommendation{},
		&HsModelTask{},
		&HsMeta{},
		&HsItem{},
		&HsTaxRateDaily{},
		&CaichipAgent{},
		&CaichipAgentTag{},
		&CaichipAgentInstalledScript{},
		&CaichipAgentScriptAuth{},
		&CaichipDispatchTask{},
		&AgentScriptPackage{},
	); err != nil {
		return fmt.Errorf("gorm automigrate: %w", err)
	}
	if err := migrateBOMQuoteTables(db); err != nil {
		return fmt.Errorf("gorm automigrate bom quote tables: %w", err)
	}
	if err := backfillBOMPlatformRunParams(db); err != nil {
		return fmt.Errorf("backfill bom_platform_script.run_params: %w", err)
	}
	return seedBOMPlatformScriptsIfEmpty(db)
}

type bomQuoteCacheLegacyID struct {
	ID uint64 `gorm:"column:id;type:bigint unsigned"`
}

func (bomQuoteCacheLegacyID) TableName() string { return TableBomQuoteCache }

func migrateBOMQuoteTables(db *gorm.DB) error {
	if db == nil {
		return nil
	}
	// 新库：直接按最新模型建表。
	if !db.Migrator().HasTable(&BomQuoteCache{}) {
		return db.AutoMigrate(&BomQuoteCache{}, &BomQuoteItem{})
	}
	// 旧库兼容：若 cache 表还没有 id，先仅补列，不触碰既有主键，避免 "Multiple primary key defined"。
	if !db.Migrator().HasColumn(&BomQuoteCache{}, "id") {
		if err := db.Migrator().AddColumn(&bomQuoteCacheLegacyID{}, "ID"); err != nil {
			return err
		}
	}
	// 现网可能仍是旧主键形态，避免 AutoMigrate 尝试重建主键；仅确保明细表存在。
	return db.AutoMigrate(&BomQuoteItem{})
}

func backfillBOMPlatformRunParams(db *gorm.DB) error {
	if db == nil || len(bomDefaultRunParamsJSON) == 0 {
		return nil
	}
	// 已有行在新增列后 run_params 可能为 NULL，补默认与旧行为一致
	return db.Model(&BomPlatformScript{}).Where("run_params IS NULL").Update("run_params", bomDefaultRunParamsJSON).Error
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
		{PlatformID: "find_chips", ScriptID: "find_chips", DisplayName: strPtr("FindChips"), Enabled: true, RunParamsJSON: append([]byte(nil), bomDefaultRunParamsJSON...)},
		{PlatformID: "hqchip", ScriptID: "hqchip", DisplayName: strPtr("HQChip"), Enabled: true, RunParamsJSON: append([]byte(nil), bomDefaultRunParamsJSON...)},
		{PlatformID: "icgoo", ScriptID: "icgoo", DisplayName: strPtr("ICGOO"), Enabled: true, RunParamsJSON: append([]byte(nil), bomDefaultRunParamsJSON...)},
		{PlatformID: "ickey", ScriptID: "ickey", DisplayName: strPtr("云汉芯城"), Enabled: true, RunParamsJSON: append([]byte(nil), bomDefaultRunParamsJSON...)},
		{PlatformID: "szlcsc", ScriptID: "szlcsc", DisplayName: strPtr("立创商城"), Enabled: true, RunParamsJSON: append([]byte(nil), bomDefaultRunParamsJSON...)},
	}
	return db.Create(&rows).Error
}

func strPtr(s string) *string { return &s }
