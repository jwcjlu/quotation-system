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
	if err := db.WithContext(ctx).AutoMigrate(&HsItem{}); err != nil {
		t.Fatalf("create t_hs_item: %v", err)
	}

	seedCodes := []string{"8542310001", "8542310002", "8542390001", "8534000000"}
	if err := db.WithContext(ctx).Where("code_ts IN ?", seedCodes).Delete(&HsItem{}).Error; err != nil {
		t.Fatalf("cleanup seed: %v", err)
	}
	defer db.WithContext(ctx).Where("code_ts IN ?", seedCodes).Delete(&HsItem{})

	seedRows := []HsItem{
		{CodeTS: "8542310001", GName: "MCU Controller QFN 3.3V", SourceCoreHS6: "854231", RawJSON: []byte(`{"voltage":"3.3V"}`)},
		{CodeTS: "8542310002", GName: "MCU Controller QFP 5V", SourceCoreHS6: "854231", RawJSON: []byte(`{"voltage":"5V"}`)},
		{CodeTS: "8542390001", GName: "IC Logic Device BGA", SourceCoreHS6: "854239", RawJSON: []byte(`{"voltage":"1.8V"}`)},
		{CodeTS: "8534000000", GName: "Printed Circuit Board", SourceCoreHS6: "853400", RawJSON: []byte(`{"layers":"4"}`)},
	}
	if err := db.WithContext(ctx).Create(&seedRows).Error; err != nil {
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
	if err := db.WithContext(ctx).AutoMigrate(&HsItem{}); err != nil {
		t.Fatalf("create t_hs_item: %v", err)
	}

	seedCodes := []string{"8542310010", "8534000001"}
	if err := db.WithContext(ctx).Where("code_ts IN ?", seedCodes).Delete(&HsItem{}).Error; err != nil {
		t.Fatalf("cleanup seed: %v", err)
	}
	defer db.WithContext(ctx).Where("code_ts IN ?", seedCodes).Delete(&HsItem{})

	seedRows := []HsItem{
		{CodeTS: "8542310010", GName: "Microcontroller Unit QFN", SourceCoreHS6: "854231", RawJSON: []byte(`{"voltage":"3.3V"}`)},
		{CodeTS: "8534000001", GName: "Printed Circuit Board", SourceCoreHS6: "853400", RawJSON: []byte(`{"layers":"4"}`)},
	}
	if err := db.WithContext(ctx).Create(&seedRows).Error; err != nil {
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
