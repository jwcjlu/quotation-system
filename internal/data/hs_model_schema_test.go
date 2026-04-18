package data

import (
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"caichip/internal/conf"
)

func TestHsModelResolveTables(t *testing.T) {
	t.Run("table names", func(t *testing.T) {
		if (HsModelMapping{}).TableName() != TableHsModelMapping {
			t.Fatalf("HsModelMapping table mismatch: got %q", (HsModelMapping{}).TableName())
		}
		if (HsModelFeatures{}).TableName() != TableHsModelFeatures {
			t.Fatalf("HsModelFeatures table mismatch: got %q", (HsModelFeatures{}).TableName())
		}
		if (HsModelRecommendation{}).TableName() != TableHsModelRecommendation {
			t.Fatalf("HsModelRecommendation table mismatch: got %q", (HsModelRecommendation{}).TableName())
		}
		if (HsModelTask{}).TableName() != TableHsModelTask {
			t.Fatalf("HsModelTask table mismatch: got %q", (HsModelTask{}).TableName())
		}
		if (HsDatasheetAsset{}).TableName() != TableHsDatasheetAsset {
			t.Fatalf("HsDatasheetAsset table mismatch: got %q", (HsDatasheetAsset{}).TableName())
		}
	})

	t.Run("gorm tags", func(t *testing.T) {
		mappingCodeTag := gormTag(t, HsModelMapping{}, "CodeTS")
		if !strings.Contains(mappingCodeTag, "type:char(10)") || !strings.Contains(mappingCodeTag, "REGEXP '^[0-9]{10}$'") {
			t.Fatalf("HsModelMapping.CodeTS gorm tag missing code_ts constraints: %q", mappingCodeTag)
		}

		recoCodeTag := gormTag(t, HsModelRecommendation{}, "CodeTS")
		if !strings.Contains(recoCodeTag, "type:char(10)") || !strings.Contains(recoCodeTag, "REGEXP '^[0-9]{10}$'") {
			t.Fatalf("HsModelRecommendation.CodeTS gorm tag missing code_ts constraints: %q", recoCodeTag)
		}

		runIDTag := gormTag(t, HsModelRecommendation{}, "RunID")
		rankTag := gormTag(t, HsModelRecommendation{}, "CandidateRank")
		if !strings.Contains(runIDTag, "size:384") || !strings.Contains(runIDTag, "not null") || !strings.Contains(runIDTag, "uniqueIndex:uk_hs_model_reco_run_rank") {
			t.Fatalf("HsModelRecommendation.RunID gorm tag missing widened run_id / unique constraint: %q", runIDTag)
		}
		if !strings.Contains(rankTag, "uniqueIndex:uk_hs_model_reco_run_rank") {
			t.Fatalf("HsModelRecommendation.CandidateRank gorm tag missing run_id+rank unique key: %q", rankTag)
		}

		assetIDTag := gormTag(t, HsModelFeatures{}, "AssetID")
		if !strings.Contains(assetIDTag, "column:asset_id") {
			t.Fatalf("HsModelFeatures.AssetID gorm tag missing asset_id mapping: %q", assetIDTag)
		}
	})

	t.Run("automigrate include models", func(t *testing.T) {
		_, thisFile, _, ok := runtime.Caller(0)
		if !ok {
			t.Fatal("runtime.Caller failed")
		}
		migrateFile := filepath.Join(filepath.Dir(thisFile), "migrate.go")
		b, err := os.ReadFile(migrateFile)
		if err != nil {
			t.Fatalf("read migrate.go: %v", err)
		}
		content := string(b)
		required := []string{
			"&HsModelMapping{}",
			"&HsDatasheetAsset{}",
			"&HsModelFeatures{}",
			"&HsModelRecommendation{}",
			"&HsModelTask{}",
		}
		for _, token := range required {
			if !strings.Contains(content, token) {
				t.Fatalf("migrate.go missing AutoMigrate model: %s", token)
			}
		}
	})

	t.Run("automigrate creates hs tables", func(t *testing.T) {
		dsn := strings.TrimSpace(os.Getenv("TEST_DATABASE_URL"))
		if dsn == "" {
			t.Skip("TEST_DATABASE_URL not set")
		}
		driver := strings.TrimSpace(os.Getenv("TEST_DATABASE_DRIVER"))
		if driver == "" {
			driver = "mysql"
		}
		db, cleanup, err := NewDB(&conf.Data{
			Database: &conf.DataDatabase{
				Driver: driver,
				Dsn:    dsn,
			},
		})
		if err != nil {
			t.Fatalf("open test db: %v", err)
		}
		defer cleanup()
		if db == nil {
			t.Fatal("expected non-nil db")
		}
		if err := AutoMigrateSchema(db); err != nil {
			t.Fatalf("AutoMigrateSchema: %v", err)
		}

		m := db.Migrator()
		for _, table := range []string{
			TableHsModelMapping,
			TableHsDatasheetAsset,
			TableHsModelFeatures,
			TableHsModelRecommendation,
			TableHsModelTask,
		} {
			if !m.HasTable(table) {
				t.Fatalf("table not found after AutoMigrateSchema: %s", table)
			}
		}
	})
}

func gormTag(t *testing.T, model any, fieldName string) string {
	t.Helper()
	typ := reflect.TypeOf(model)
	f, ok := typ.FieldByName(fieldName)
	if !ok {
		t.Fatalf("field %q not found in %s", fieldName, typ.Name())
	}
	return f.Tag.Get("gorm")
}
