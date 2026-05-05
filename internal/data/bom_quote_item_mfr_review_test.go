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

// Phase 0（SRS REQ-MIG-001 / REQ-DATA-002）：迁移与模型对齐后，
// 插入报价明细时省略评审列应得到 DB 默认 manufacturer_review_status = pending。
func TestBomQuoteItem_ManufacturerReviewStatus_DBDefaultPending(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("TEST_DATABASE_URL"))
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	driver := strings.TrimSpace(os.Getenv("TEST_DATABASE_DRIVER"))
	if driver == "" {
		driver = "mysql"
	}
	db, cleanup, err := NewDB(&conf.Data{
		Database: &conf.DataDatabase{Driver: driver, Dsn: dsn},
	})
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	defer cleanup()
	if err := AutoMigrateSchema(db); err != nil {
		t.Fatalf("auto migrate: %v", err)
	}
	for _, col := range []string{
		"ManufacturerReviewStatus",
		"ManufacturerReviewReason",
		"ManufacturerReviewedAt",
	} {
		if !db.Migrator().HasColumn(&BomQuoteItem{}, col) {
			t.Fatalf("expected BomQuoteItem column %s after automigrate", col)
		}
	}

	ctx := context.Background()
	suffix := strings.ReplaceAll(t.Name(), "/", "_")
	cache := &BomQuoteCache{
		MpnNorm:    "mfr0-" + suffix + "-mpn",
		PlatformID: "mfr0-plat",
		BizDate:    time.Date(2026, 5, 4, 0, 0, 0, 0, time.UTC),
		Outcome:    "ok",
	}
	if err := db.WithContext(ctx).Create(cache).Error; err != nil {
		t.Fatalf("create quote cache: %v", err)
	}
	t.Cleanup(func() {
		_ = db.WithContext(ctx).Where("id = ?", cache.ID).Delete(&BomQuoteCache{}).Error
	})

	item := BomQuoteItem{QuoteID: cache.ID}
	if err := db.WithContext(ctx).
		Omit("ManufacturerReviewStatus", "ManufacturerReviewReason", "ManufacturerReviewedAt").
		Create(&item).Error; err != nil {
		t.Fatalf("create quote item: %v", err)
	}
	t.Cleanup(func() {
		_ = db.WithContext(ctx).Where("id = ?", item.ID).Delete(&BomQuoteItem{}).Error
	})

	var loaded BomQuoteItem
	if err := db.WithContext(ctx).First(&loaded, item.ID).Error; err != nil {
		t.Fatalf("load quote item: %v", err)
	}
	if loaded.ManufacturerReviewStatus != biz.MfrReviewPending {
		t.Fatalf("manufacturer_review_status: got %q want %q (DB default / 与 biz 常量一致)",
			loaded.ManufacturerReviewStatus, biz.MfrReviewPending)
	}
	if loaded.ManufacturerReviewReason != nil {
		t.Fatalf("manufacturer_review_reason: want nil, got %#v", loaded.ManufacturerReviewReason)
	}
	if loaded.ManufacturerReviewedAt != nil {
		t.Fatalf("manufacturer_reviewed_at: want nil, got %#v", loaded.ManufacturerReviewedAt)
	}
}

// 与 docs/schema/migrations/20260504_bom_quote_item_mfr_review.sql 语义一致：显式写入的终态可读出。
func TestBomQuoteItem_ManufacturerReviewStatus_PersistAccepted(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("TEST_DATABASE_URL"))
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	driver := strings.TrimSpace(os.Getenv("TEST_DATABASE_DRIVER"))
	if driver == "" {
		driver = "mysql"
	}
	db, cleanup, err := NewDB(&conf.Data{
		Database: &conf.DataDatabase{Driver: driver, Dsn: dsn},
	})
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	defer cleanup()
	if err := AutoMigrateSchema(db); err != nil {
		t.Fatalf("auto migrate: %v", err)
	}

	ctx := context.Background()
	suffix := strings.ReplaceAll(t.Name(), "/", "_")
	cache := &BomQuoteCache{
		MpnNorm:    "mfr0acc-" + suffix + "-mpn",
		PlatformID: "mfr0-plat",
		BizDate:    time.Date(2026, 5, 5, 0, 0, 0, 0, time.UTC),
		Outcome:    "ok",
	}
	if err := db.WithContext(ctx).Create(cache).Error; err != nil {
		t.Fatalf("create quote cache: %v", err)
	}
	t.Cleanup(func() {
		_ = db.WithContext(ctx).Where("id = ?", cache.ID).Delete(&BomQuoteCache{}).Error
	})

	now := time.Now().UTC().Truncate(time.Millisecond)
	canon := "canon-x"
	item := BomQuoteItem{
		QuoteID:                  cache.ID,
		ManufacturerReviewStatus: biz.MfrReviewAccepted,
		ManufacturerCanonicalID:  &canon,
		ManufacturerReviewedAt:   &now,
	}
	if err := db.WithContext(ctx).Create(&item).Error; err != nil {
		t.Fatalf("create quote item: %v", err)
	}
	t.Cleanup(func() {
		_ = db.WithContext(ctx).Where("id = ?", item.ID).Delete(&BomQuoteItem{}).Error
	})

	var loaded BomQuoteItem
	if err := db.WithContext(ctx).First(&loaded, item.ID).Error; err != nil {
		t.Fatalf("load quote item: %v", err)
	}
	if loaded.ManufacturerReviewStatus != biz.MfrReviewAccepted {
		t.Fatalf("status: got %q want %q", loaded.ManufacturerReviewStatus, biz.MfrReviewAccepted)
	}
	if loaded.ManufacturerCanonicalID == nil || *loaded.ManufacturerCanonicalID != canon {
		t.Fatalf("canonical: got %#v want %q", loaded.ManufacturerCanonicalID, canon)
	}
	if loaded.ManufacturerReviewedAt == nil {
		t.Fatal("expected manufacturer_reviewed_at set")
	}
}
