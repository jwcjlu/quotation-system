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

func TestHsManualDatasheetUpload_TableName(t *testing.T) {
	if (HsManualDatasheetUpload{}).TableName() != TableHsManualDatasheetUpload {
		t.Fatalf("table: got %q", (HsManualDatasheetUpload{}).TableName())
	}
}

func TestHsManualDatasheetUploadRepo_CreateGetMark(t *testing.T) {
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
		t.Fatalf("open db: %v", err)
	}
	defer cleanup()
	if err := AutoMigrateSchema(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	_ = db.Exec("DELETE FROM "+TableHsManualDatasheetUpload+" WHERE upload_id = ?", "tdd-manual-upload-1").Error

	repo := NewHsManualDatasheetUploadRepo(&Data{DB: db})
	ctx := context.Background()
	row := &biz.HsManualDatasheetUploadRecord{
		UploadID:  "tdd-manual-upload-1",
		LocalPath: "/tmp/staging/x.pdf",
		SHA256:    "a" + strings.Repeat("0", 63),
		ExpiresAt: time.Now().Add(24 * time.Hour).UTC(),
	}
	if err := repo.Create(ctx, row); err != nil {
		t.Fatalf("create: %v", err)
	}
	got, err := repo.GetByUploadID(ctx, "tdd-manual-upload-1")
	if err != nil || got == nil || got.UploadID != row.UploadID {
		t.Fatalf("get: err=%v got=%v", err, got)
	}
	if err := repo.MarkConsumed(ctx, "tdd-manual-upload-1"); err != nil {
		t.Fatalf("mark: %v", err)
	}
	got2, err := repo.GetByUploadID(ctx, "tdd-manual-upload-1")
	if err != nil || got2 == nil || got2.ConsumedAt == nil {
		t.Fatalf("expected consumed_at: %+v err=%v", got2, err)
	}
}
