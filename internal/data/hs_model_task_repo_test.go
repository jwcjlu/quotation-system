package data

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"caichip/internal/biz"
	"caichip/internal/conf"
)

func TestHsModelTaskRepo_SaveAndGet(t *testing.T) {
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
	repo := NewHsModelTaskRepo(&Data{DB: db})
	runID := "TDD-TASK|NXP|trace-1"
	_ = db.WithContext(ctx).Where("run_id = ?", runID).Delete(&HsModelTask{}).Error
	defer db.WithContext(ctx).Where("run_id = ?", runID).Delete(&HsModelTask{})

	row := &biz.HsModelTaskRecord{
		Model:          "TDD-TASK",
		Manufacturer:   "NXP",
		RequestTraceID: "trace-1",
		RunID:          runID,
		TaskStatus:     biz.HsTaskStatusRunning,
		ResultStatus:   biz.HsResultStatusPendingReview,
		Stage:          biz.HsTaskStageDatasheet,
		AttemptCount:   1,
		BestScore:      0,
		BestCodeTS:     "",
		UpdatedAt:      time.Now(),
	}
	if err := repo.Save(ctx, row); err != nil {
		t.Fatalf("save: %v", err)
	}
	byRun, err := repo.GetByRunID(ctx, runID)
	if err != nil {
		t.Fatalf("get by run: %v", err)
	}
	if byRun == nil || byRun.RunID != runID {
		t.Fatalf("unexpected by run: %+v", byRun)
	}
	byReq, err := repo.GetByRequestTraceID(ctx, "TDD-TASK", "NXP", "trace-1")
	if err != nil {
		t.Fatalf("get by req: %v", err)
	}
	if byReq == nil || byReq.RunID != runID {
		t.Fatalf("unexpected by req: %+v", byReq)
	}
	latest, err := repo.GetLatestByModelManufacturer(ctx, "TDD-TASK", "NXP")
	if err != nil {
		t.Fatalf("latest: %v", err)
	}
	if latest == nil || latest.RunID != runID {
		t.Fatalf("unexpected latest: %+v", latest)
	}
}
