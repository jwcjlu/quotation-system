package data

import (
	"context"
	"testing"

	"caichip/internal/biz"
)

func TestBOMLineGapRepo_UpsertOpenGapsDedupesActiveGap(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()
	repo := NewBOMLineGapRepo(&Data{DB: db})
	gap := biz.BOMLineGap{
		SessionID: "sid",
		LineID:    1,
		LineNo:    1,
		Mpn:       "NO-DATA",
		GapType:   biz.LineGapNoData,
		Status:    biz.LineGapOpen,
	}
	if err := repo.UpsertOpenGaps(context.Background(), []biz.BOMLineGap{gap, gap}); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	got, err := repo.ListLineGaps(context.Background(), "sid", []string{biz.LineGapOpen})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("gap count=%d, want 1", len(got))
	}
}
