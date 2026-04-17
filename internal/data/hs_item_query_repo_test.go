package data

import (
	"context"
	"os"
	"strings"
	"testing"

	"caichip/internal/biz"
	"caichip/internal/conf"
)

func TestPrefilter_QueryCandidatesByRulesAndTopN(t *testing.T) {
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

	ctx := context.Background()
	if err := db.WithContext(ctx).Exec(`
CREATE TABLE IF NOT EXISTS t_hs_item (
    id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
    code_ts VARCHAR(16) NOT NULL,
    g_name VARCHAR(512) NOT NULL,
    unit_1 VARCHAR(16) NOT NULL DEFAULT '',
    unit_2 VARCHAR(16) NOT NULL DEFAULT '',
    control_mark VARCHAR(64) NOT NULL DEFAULT '',
    source_core_hs6 CHAR(6) NOT NULL DEFAULT '',
    raw_json JSON NULL,
    updated_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    UNIQUE KEY uk_hs_item_code_ts (code_ts)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
`).Error; err != nil {
		t.Fatalf("create t_hs_item: %v", err)
	}

	seedCodes := []string{"8542310001", "8542310002", "8542390001", "8534000000"}
	if err := db.WithContext(ctx).Exec("DELETE FROM t_hs_item WHERE code_ts IN ?", seedCodes).Error; err != nil {
		t.Fatalf("cleanup seed: %v", err)
	}
	defer db.WithContext(ctx).Exec("DELETE FROM t_hs_item WHERE code_ts IN ?", seedCodes)

	if err := db.WithContext(ctx).Exec(`
INSERT INTO t_hs_item (code_ts, g_name, source_core_hs6, raw_json) VALUES
('8542310001', 'MCU Controller QFN 3.3V', '854231', JSON_OBJECT('voltage','3.3V')),
('8542310002', 'MCU Controller QFP 5V', '854231', JSON_OBJECT('voltage','5V')),
('8542390001', 'IC Logic Device BGA', '854239', JSON_OBJECT('voltage','1.8V')),
('8534000000', 'Printed Circuit Board', '853400', JSON_OBJECT('layers','4'));
`).Error; err != nil {
		t.Fatalf("seed t_hs_item: %v", err)
	}

	repo := NewHsItemQueryRepo(&Data{DB: db})
	got, err := repo.QueryCandidatesByRules(ctx, biz.HsPrefilterInput{
		TechCategory:  "集成电路",
		ComponentName: "MCU",
		PackageForm:   "QFN",
		KeySpecs: map[string]string{
			"voltage": "3.3V",
		},
	}, 2)
	if err != nil {
		t.Fatalf("query candidates: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected top2 candidates, got %d", len(got))
	}
	if got[0].CodeTS != "8542310001" {
		t.Fatalf("expected best candidate 8542310001, got %+v", got[0])
	}
	if !got[0].ScoreDetail.TechCategoryMatched || !got[0].ScoreDetail.ComponentNameMatched {
		t.Fatalf("expected rule details to be recorded, got %+v", got[0].ScoreDetail)
	}
}

func TestPrefilter_QueryCandidatesByRules_ComponentSynonymRecall(t *testing.T) {
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

	ctx := context.Background()
	if err := db.WithContext(ctx).Exec(`
CREATE TABLE IF NOT EXISTS t_hs_item (
    id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
    code_ts VARCHAR(16) NOT NULL,
    g_name VARCHAR(512) NOT NULL,
    unit_1 VARCHAR(16) NOT NULL DEFAULT '',
    unit_2 VARCHAR(16) NOT NULL DEFAULT '',
    control_mark VARCHAR(64) NOT NULL DEFAULT '',
    source_core_hs6 CHAR(6) NOT NULL DEFAULT '',
    raw_json JSON NULL,
    updated_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    UNIQUE KEY uk_hs_item_code_ts (code_ts)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
`).Error; err != nil {
		t.Fatalf("create t_hs_item: %v", err)
	}

	seedCodes := []string{"8542310010", "8534000001"}
	if err := db.WithContext(ctx).Exec("DELETE FROM t_hs_item WHERE code_ts IN ?", seedCodes).Error; err != nil {
		t.Fatalf("cleanup seed: %v", err)
	}
	defer db.WithContext(ctx).Exec("DELETE FROM t_hs_item WHERE code_ts IN ?", seedCodes)

	if err := db.WithContext(ctx).Exec(`
INSERT INTO t_hs_item (code_ts, g_name, source_core_hs6, raw_json) VALUES
('8542310010', 'Microcontroller Unit QFN', '854231', JSON_OBJECT('voltage','3.3V')),
('8534000001', 'Printed Circuit Board', '853400', JSON_OBJECT('layers','4'));
`).Error; err != nil {
		t.Fatalf("seed t_hs_item: %v", err)
	}

	repo := NewHsItemQueryRepo(&Data{DB: db})
	got, err := repo.QueryCandidatesByRules(ctx, biz.HsPrefilterInput{
		TechCategory:  "集成电路",
		ComponentName: "单片机",
	}, 5)
	if err != nil {
		t.Fatalf("query candidates by synonym: %v", err)
	}
	if len(got) == 0 {
		t.Fatal("expected synonym recall candidate, got empty")
	}
	if got[0].CodeTS != "8542310010" {
		t.Fatalf("expected synonym-recalled candidate 8542310010, got %+v", got[0])
	}
	if !got[0].ScoreDetail.ComponentNameMatched {
		t.Fatalf("expected ComponentNameMatched=true for synonym recall, got %+v", got[0].ScoreDetail)
	}
}
