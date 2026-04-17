package data

import (
	"context"
	"os"
	"strings"
	"testing"

	"caichip/internal/conf"
)

func TestHsModelMappingRepo_GetConfirmedByModelManufacturer(t *testing.T) {
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
	repo := NewHsModelMappingRepo(&Data{DB: db})
	model := "TDD-MAP-001"
	mfr := "TDD-MFR"

	if err := db.WithContext(ctx).
		Where("model = ? AND manufacturer = ?", model, mfr).
		Delete(&HsModelMapping{}).Error; err != nil {
		t.Fatalf("cleanup: %v", err)
	}
	defer db.WithContext(ctx).
		Where("model = ? AND manufacturer = ?", model, mfr).
		Delete(&HsModelMapping{})

	pending := &HsModelMapping{
		Model:        model,
		Manufacturer: mfr,
		CodeTS:       "1234567890",
		Source:       "llm_auto",
		Confidence:   0.6500,
		Status:       "pending_review",
	}
	if err := db.WithContext(ctx).Create(pending).Error; err != nil {
		t.Fatalf("seed pending: %v", err)
	}

	got, err := repo.GetConfirmedByModelManufacturer(ctx, model, mfr)
	if err != nil {
		t.Fatalf("query pending only: %v", err)
	}
	if got != nil {
		t.Fatalf("expected nil for pending row, got %+v", got)
	}

	if err := db.WithContext(ctx).
		Where("model = ? AND manufacturer = ?", model, mfr).
		Delete(&HsModelMapping{}).Error; err != nil {
		t.Fatalf("cleanup before confirmed: %v", err)
	}
	confirmed := &HsModelMapping{
		Model:        model,
		Manufacturer: mfr,
		CodeTS:       "0987654321",
		Source:       "manual",
		Confidence:   1.0000,
		Status:       "confirmed",
	}
	if err := db.WithContext(ctx).Create(confirmed).Error; err != nil {
		t.Fatalf("seed confirmed: %v", err)
	}

	got, err = repo.GetConfirmedByModelManufacturer(ctx, model, mfr)
	if err != nil {
		t.Fatalf("query confirmed: %v", err)
	}
	if got == nil {
		t.Fatal("expected confirmed mapping, got nil")
	}
	if got.CodeTS != "0987654321" || got.Status != "confirmed" {
		t.Fatalf("unexpected mapping: %+v", got)
	}
}
