package data

import (
	"context"
	"os"
	"strings"
	"testing"

	"caichip/internal/biz"
	"caichip/internal/conf"
)

func TestBomSessionRepo_UpdateImportState(t *testing.T) {
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
	repo := NewBomSessionRepo(&Data{DB: db})
	sessionID, _, _, err := repo.CreateSession(ctx, "import-state-test", []string{"digikey"}, nil, nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	defer func() {
		_ = db.WithContext(ctx).Where("session_id = ?", sessionID).Delete(&BomSessionLine{}).Error
		_ = db.WithContext(ctx).Where("id = ?", sessionID).Delete(&BomSession{}).Error
	}()

	msg := "chunk 2/5"
	errCode := "HEADER_NOT_FOUND"
	errDetail := "missing mpn column"
	patch := biz.BOMImportStatePatch{
		Status:    biz.BOMImportStatusFailed,
		Progress:  120,
		Stage:     biz.BOMImportStageFailed,
		Message:   &msg,
		ErrorCode: &errCode,
		Error:     &errDetail,
	}
	if err := repo.UpdateImportState(ctx, sessionID, patch); err != nil {
		t.Fatalf("update import state: %v", err)
	}

	var got BomSession
	if err := db.WithContext(ctx).Where("id = ?", sessionID).First(&got).Error; err != nil {
		t.Fatalf("query session: %v", err)
	}
	if got.ImportStatus != biz.BOMImportStatusFailed {
		t.Fatalf("unexpected import_status: %q", got.ImportStatus)
	}
	if got.ImportProgress != 100 {
		t.Fatalf("unexpected import_progress: %d", got.ImportProgress)
	}
	if got.ImportStage != biz.BOMImportStageFailed {
		t.Fatalf("unexpected import_stage: %q", got.ImportStage)
	}
	if got.ImportMessage == nil || *got.ImportMessage != msg {
		t.Fatalf("unexpected import_message: %#v", got.ImportMessage)
	}
	if got.ImportErrorCode == nil || *got.ImportErrorCode != errCode {
		t.Fatalf("unexpected import_error_code: %#v", got.ImportErrorCode)
	}
	if got.ImportError == nil || *got.ImportError != errDetail {
		t.Fatalf("unexpected import_error: %#v", got.ImportError)
	}
	if got.ImportUpdatedAt == nil {
		t.Fatalf("import_updated_at should not be nil")
	}

	if err := repo.UpdateImportState(ctx, sessionID, biz.BOMImportStatePatch{
		Status:   biz.BOMImportStatusParsing,
		Progress: 10,
		Stage:    biz.BOMImportStageChunkParsing,
	}); err != nil {
		t.Fatalf("failed -> parsing should be allowed: %v", err)
	}
	if err := db.WithContext(ctx).Where("id = ?", sessionID).First(&got).Error; err != nil {
		t.Fatalf("query session after failed -> parsing: %v", err)
	}
	if got.ImportProgress != 100 {
		t.Fatalf("unexpected import_progress for parsing chunk with GREATEST: %d", got.ImportProgress)
	}

	if err := repo.UpdateImportState(ctx, sessionID, biz.BOMImportStatePatch{
		Status:   biz.BOMImportStatusReady,
		Progress: 100,
		Stage:    biz.BOMImportStageDone,
	}); err != nil {
		t.Fatalf("parsing -> ready should be allowed: %v", err)
	}
	if err := repo.UpdateImportState(ctx, sessionID, biz.BOMImportStatePatch{
		Status:   biz.BOMImportStatusParsing,
		Progress: 5,
		Stage:    biz.BOMImportStageValidating,
	}); err != nil {
		t.Fatalf("ready -> parsing should be allowed: %v", err)
	}
	if err := db.WithContext(ctx).Where("id = ?", sessionID).First(&got).Error; err != nil {
		t.Fatalf("query session after ready -> parsing validating: %v", err)
	}
	if got.ImportProgress != 5 {
		t.Fatalf("unexpected import_progress for parsing validating direct-write: %d", got.ImportProgress)
	}

	if err := repo.UpdateImportState(ctx, sessionID, biz.BOMImportStatePatch{
		Status:   biz.BOMImportStatusFailed,
		Progress: 40,
		Stage:    biz.BOMImportStageFailed,
	}); err != nil {
		t.Fatalf("update import state failed override: %v", err)
	}
	if err := db.WithContext(ctx).Where("id = ?", sessionID).First(&got).Error; err != nil {
		t.Fatalf("query session failed override: %v", err)
	}
	if got.ImportProgress != 40 {
		t.Fatalf("unexpected failed override import_progress: %d", got.ImportProgress)
	}
}
