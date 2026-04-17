package data

import (
	"context"
	"os"
	"strings"
	"testing"

	"caichip/internal/biz"
	"caichip/internal/conf"
)

func TestHsModelRecommendationRepo_SaveTop3ByRunID(t *testing.T) {
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
	if err := AutoMigrateSchema(db); err != nil {
		t.Fatalf("auto migrate: %v", err)
	}

	ctx := context.Background()
	repo := NewHsModelRecommendationRepo(&Data{DB: db})
	runID := "11111111-1111-1111-1111-111111111111"
	model := "TDD-RECO-001"
	mfr := "TDD-MFR"

	if err := db.WithContext(ctx).Where("run_id = ?", runID).Delete(&HsModelRecommendation{}).Error; err != nil {
		t.Fatalf("cleanup: %v", err)
	}
	defer db.WithContext(ctx).Where("run_id = ?", runID).Delete(&HsModelRecommendation{})

	top3 := []biz.HsModelRecommendationRecord{
		{Model: model, Manufacturer: mfr, RunID: runID, CandidateRank: 1, CodeTS: "1234567890", GName: "A", Score: 0.91, Reason: "r1"},
		{Model: model, Manufacturer: mfr, RunID: runID, CandidateRank: 2, CodeTS: "1234567891", GName: "B", Score: 0.89, Reason: "r2"},
		{Model: model, Manufacturer: mfr, RunID: runID, CandidateRank: 3, CodeTS: "1234567892", GName: "C", Score: 0.87, Reason: "r3"},
	}
	if err := repo.SaveTopN(ctx, top3); err != nil {
		t.Fatalf("save top3: %v", err)
	}

	// 完全一致重放：幂等成功，不报错，不新增记录。
	if err := repo.SaveTopN(ctx, top3); err != nil {
		t.Fatalf("idempotent replay should not error, got: %v", err)
	}

	got, err := repo.ListByRunID(ctx, runID)
	if err != nil {
		t.Fatalf("list by run_id: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("expected 3 records after replay, got %d", len(got))
	}
	if got[0].CandidateRank != 1 || got[0].CodeTS != "1234567890" {
		t.Fatalf("rank1 should keep original row, got %+v", got[0])
	}
}

func TestHsModelRecommendationRepo_SaveTopNConflictByRunRank(t *testing.T) {
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
	if err := AutoMigrateSchema(db); err != nil {
		t.Fatalf("auto migrate: %v", err)
	}

	ctx := context.Background()
	repo := NewHsModelRecommendationRepo(&Data{DB: db})
	runID := "22222222-2222-2222-2222-222222222222"
	model := "TDD-RECO-002"
	mfr := "TDD-MFR"

	if err := db.WithContext(ctx).Where("run_id = ?", runID).Delete(&HsModelRecommendation{}).Error; err != nil {
		t.Fatalf("cleanup: %v", err)
	}
	defer db.WithContext(ctx).Where("run_id = ?", runID).Delete(&HsModelRecommendation{})

	origin := []biz.HsModelRecommendationRecord{
		{Model: model, Manufacturer: mfr, RunID: runID, CandidateRank: 1, CodeTS: "1234567890", GName: "A", Score: 0.91, Reason: "r1"},
	}
	if err := repo.SaveTopN(ctx, origin); err != nil {
		t.Fatalf("save origin: %v", err)
	}

	conflict := []biz.HsModelRecommendationRecord{
		{Model: model, Manufacturer: mfr, RunID: runID, CandidateRank: 1, CodeTS: "2234567890", GName: "DIFF", Score: 0.5, Reason: "changed"},
	}
	err = repo.SaveTopN(ctx, conflict)
	if err == nil {
		t.Fatal("expected conflict error for same run_id+rank with different content, got nil")
	}
	if !strings.Contains(err.Error(), "conflict on run_id") {
		t.Fatalf("expected conflict error, got: %v", err)
	}
}

func TestHsModelRecommendationRepo_SaveTopNIdempotentWithTinyScoreDrift(t *testing.T) {
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
	if err := AutoMigrateSchema(db); err != nil {
		t.Fatalf("auto migrate: %v", err)
	}

	ctx := context.Background()
	repo := NewHsModelRecommendationRepo(&Data{DB: db})
	runID := "33333333-3333-3333-3333-333333333333"
	model := "TDD-RECO-003"
	mfr := "TDD-MFR"

	if err := db.WithContext(ctx).Where("run_id = ?", runID).Delete(&HsModelRecommendation{}).Error; err != nil {
		t.Fatalf("cleanup: %v", err)
	}
	defer db.WithContext(ctx).Where("run_id = ?", runID).Delete(&HsModelRecommendation{})

	origin := []biz.HsModelRecommendationRecord{
		{Model: model, Manufacturer: mfr, RunID: runID, CandidateRank: 1, CodeTS: "1234567890", GName: "A", Score: 0.91, Reason: "r1"},
	}
	if err := repo.SaveTopN(ctx, origin); err != nil {
		t.Fatalf("save origin: %v", err)
	}

	// 仅 score 存在极小误差（< epsilon），应视为同内容幂等重放成功。
	tinyDrift := []biz.HsModelRecommendationRecord{
		{Model: model, Manufacturer: mfr, RunID: runID, CandidateRank: 1, CodeTS: "1234567890", GName: "A", Score: 0.9100005, Reason: "r1"},
	}
	if err := repo.SaveTopN(ctx, tinyDrift); err != nil {
		t.Fatalf("tiny score drift should be idempotent success, got: %v", err)
	}
}
