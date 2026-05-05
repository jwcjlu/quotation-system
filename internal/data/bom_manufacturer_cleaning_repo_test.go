package data

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"caichip/internal/biz"
	"caichip/internal/conf"

	"gorm.io/gorm"
)

func openCleaningITDB(t *testing.T) (*gorm.DB, func()) {
	t.Helper()
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
	if err := AutoMigrateSchema(db); err != nil {
		cleanup()
		t.Fatalf("auto migrate: %v", err)
	}
	return db, cleanup
}

// Phase 1：仅行回填不得更新同 session 下报价明细的 manufacturer_canonical_id。
func TestBomManufacturerCleaningRepo_BackfillSessionLineManufacturerCanonical_OnlySessionLines(t *testing.T) {
	db, cleanup := openCleaningITDB(t)
	defer cleanup()
	ctx := context.Background()

	brand := "MFRP1" + strings.ReplaceAll(t.Name(), "/", "_")
	if len(brand) > 200 {
		brand = brand[:200]
	}
	aliasNorm := biz.NormalizeMfrString(brand)
	sid, lineID, qid, del := seedSessionLineAndQuoteItem(t, db, ctx, brand)
	defer del()

	aliasRepo := NewBomManufacturerAliasRepo(&Data{DB: db})
	cleaning := NewBomManufacturerCleaningRepo(&Data{DB: db}, aliasRepo)

	res, err := cleaning.BackfillSessionLineManufacturerCanonical(ctx, sid, aliasNorm, "CANON-APPROVAL", false)
	if err != nil {
		t.Fatalf("BackfillSessionLineManufacturerCanonical: %v", err)
	}
	if res.SessionLineUpdated < 1 {
		t.Fatalf("SessionLineUpdated: got %d want >=1", res.SessionLineUpdated)
	}
	if res.QuoteItemUpdated != 0 {
		t.Fatalf("QuoteItemUpdated: got %d want 0 (阶段一不写 quote_item)", res.QuoteItemUpdated)
	}

	var line BomSessionLine
	if err := db.WithContext(ctx).First(&line, lineID).Error; err != nil {
		t.Fatalf("load line: %v", err)
	}
	if line.ManufacturerCanonicalID == nil || *line.ManufacturerCanonicalID != "CANON-APPROVAL" {
		t.Fatalf("line canonical: %#v want CANON-APPROVAL", line.ManufacturerCanonicalID)
	}

	var qi BomQuoteItem
	if err := db.WithContext(ctx).First(&qi, qid).Error; err != nil {
		t.Fatalf("load quote item: %v", err)
	}
	if qi.ManufacturerCanonicalID != nil {
		t.Fatalf("quote_item manufacturer_canonical_id must stay nil, got %#v", qi.ManufacturerCanonicalID)
	}
}

// Phase 1：ApplyKnownAliasesToSession 只写需求行，不写报价明细。
func TestBomManufacturerCleaningRepo_ApplyKnownAliasesToSession_OnlySessionLines(t *testing.T) {
	db, cleanup := openCleaningITDB(t)
	defer cleanup()
	ctx := context.Background()

	brand := "MFRP1A" + strings.ReplaceAll(t.Name(), "/", "_")
	if len(brand) > 200 {
		brand = brand[:200]
	}
	aliasNorm := biz.NormalizeMfrString(brand)

	aliasRow := BomManufacturerAlias{
		CanonicalID: "CANON-KNOWN",
		DisplayName: "Known Display",
		Alias:       brand,
		AliasNorm:   aliasNorm,
	}
	if err := db.WithContext(ctx).Create(&aliasRow).Error; err != nil {
		t.Fatalf("create alias: %v", err)
	}
	t.Cleanup(func() {
		_ = db.WithContext(ctx).Where("id = ?", aliasRow.ID).Delete(&BomManufacturerAlias{}).Error
	})

	sid, lineID, qid, del := seedSessionLineAndQuoteItem(t, db, ctx, brand)
	defer del()

	aliasRepo := NewBomManufacturerAliasRepo(&Data{DB: db})
	cleaning := NewBomManufacturerCleaningRepo(&Data{DB: db}, aliasRepo)

	res, err := cleaning.ApplyKnownAliasesToSession(ctx, sid)
	if err != nil {
		t.Fatalf("ApplyKnownAliasesToSession: %v", err)
	}
	if res.SessionLineUpdated < 1 {
		t.Fatalf("SessionLineUpdated: got %d want >=1", res.SessionLineUpdated)
	}
	if res.QuoteItemUpdated != 0 {
		t.Fatalf("QuoteItemUpdated: got %d want 0", res.QuoteItemUpdated)
	}

	var line BomSessionLine
	if err := db.WithContext(ctx).First(&line, lineID).Error; err != nil {
		t.Fatalf("load line: %v", err)
	}
	if line.ManufacturerCanonicalID == nil || *line.ManufacturerCanonicalID != "CANON-KNOWN" {
		t.Fatalf("line canonical: %#v want CANON-KNOWN", line.ManufacturerCanonicalID)
	}

	var qi BomQuoteItem
	if err := db.WithContext(ctx).First(&qi, qid).Error; err != nil {
		t.Fatalf("load quote item: %v", err)
	}
	if qi.ManufacturerCanonicalID != nil {
		t.Fatalf("quote_item canonical must stay nil, got %#v", qi.ManufacturerCanonicalID)
	}
}

func TestBomManufacturerCleaningRepo_UpdateQuoteItemManufacturerReview(t *testing.T) {
	db, cleanup := openCleaningITDB(t)
	defer cleanup()
	ctx := context.Background()

	brand := "MFRP1R" + strings.ReplaceAll(t.Name(), "/", "_")
	if len(brand) > 200 {
		brand = brand[:200]
	}
	_, _, qid, del := seedSessionLineAndQuoteItem(t, db, ctx, brand)
	defer del()

	aliasRepo := NewBomManufacturerAliasRepo(&Data{DB: db})
	cleaning := NewBomManufacturerCleaningRepo(&Data{DB: db}, aliasRepo)

	canon := "CANON-ACC"
	reason := "mismatch"
	tests := []struct {
		name      string
		status    string
		canon     *string
		reason    *string
		wantCanon *string
		wantReas  *string
	}{
		{
			name:      "accept",
			status:    biz.MfrReviewAccepted,
			canon:     &canon,
			reason:    nil,
			wantCanon: strPtr("CANON-ACC"),
			wantReas:  nil,
		},
		{
			name:      "reject",
			status:    biz.MfrReviewRejected,
			canon:     nil,
			reason:    &reason,
			wantCanon: nil,
			wantReas:  strPtr("mismatch"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := cleaning.UpdateQuoteItemManufacturerReview(ctx, qid, tt.status, tt.canon, tt.reason); err != nil {
				t.Fatalf("UpdateQuoteItemManufacturerReview: %v", err)
			}
			var qi BomQuoteItem
			if err := db.WithContext(ctx).First(&qi, qid).Error; err != nil {
				t.Fatalf("load quote item: %v", err)
			}
			if qi.ManufacturerReviewStatus != tt.status {
				t.Fatalf("status: got %q want %q", qi.ManufacturerReviewStatus, tt.status)
			}
			if !ptrStrEqual(qi.ManufacturerCanonicalID, tt.wantCanon) {
				t.Fatalf("canonical: got %#v want %#v", derefStrPtr(qi.ManufacturerCanonicalID), derefStrPtr(tt.wantCanon))
			}
			if !ptrStrEqual(qi.ManufacturerReviewReason, tt.wantReas) {
				t.Fatalf("reason: got %#v want %#v", derefStrPtr(qi.ManufacturerReviewReason), derefStrPtr(tt.wantReas))
			}
			if qi.ManufacturerReviewedAt == nil {
				t.Fatal("expected manufacturer_reviewed_at")
			}
		})
	}
}

func derefStrPtr(p *string) string {
	if p == nil {
		return "<nil>"
	}
	return *p
}

func ptrStrEqual(a *string, b *string) bool {
	switch {
	case a == nil && b == nil:
		return true
	case a == nil || b == nil:
		return false
	default:
		return *a == *b
	}
}

func seedSessionLineAndQuoteItem(t *testing.T, db *gorm.DB, ctx context.Context, brand string) (sessionID string, lineID int64, quoteItemID uint64, cleanup func()) {
	t.Helper()
	srepo := NewBomSessionRepo(&Data{DB: db})
	sid, _, _, err := srepo.CreateSession(ctx, "phase1-"+t.Name(), []string{"test"}, nil, nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	lineID, _, _, err = srepo.CreateSessionLine(ctx, sid, "MPN-PH1", "", "", "", "", "", brand, "", nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("CreateSessionLine: %v", err)
	}

	suffix := strings.ReplaceAll(t.Name(), "/", "_")
	cache := &BomQuoteCache{
		MpnNorm:    "p1c-" + suffix + "-mpn",
		PlatformID: "p1-plat",
		BizDate:    time.Date(2026, 5, 6, 0, 0, 0, 0, time.UTC),
		Outcome:    "ok",
	}
	if err := db.WithContext(ctx).Create(cache).Error; err != nil {
		t.Fatalf("create quote cache: %v", err)
	}

	sidPtr := sid
	lid := lineID
	item := BomQuoteItem{
		QuoteID:       cache.ID,
		SessionID:     &sidPtr,
		LineID:        &lid,
		Manufacturer:  brand,
	}
	if err := db.WithContext(ctx).
		Omit("ManufacturerReviewStatus", "ManufacturerReviewReason", "ManufacturerReviewedAt").
		Create(&item).Error; err != nil {
		t.Fatalf("create quote item: %v", err)
	}

	cleanup = func() {
		_ = db.WithContext(ctx).Where("id = ?", item.ID).Delete(&BomQuoteItem{}).Error
		_ = db.WithContext(ctx).Where("id = ?", cache.ID).Delete(&BomQuoteCache{}).Error
		_ = db.WithContext(ctx).Where("session_id = ?", sid).Delete(&BomSessionLine{}).Error
		_ = db.WithContext(ctx).Where("id = ?", sid).Delete(&BomSession{}).Error
	}
	return sid, lineID, item.ID, cleanup
}
