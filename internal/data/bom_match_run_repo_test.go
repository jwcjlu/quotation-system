package data

import (
	"context"
	"testing"

	"caichip/internal/biz"
)

func TestBOMMatchRunRepo_CreateMatchRunIncrementsRunNoAndStoresCustomsFields(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()
	repo := NewBOMMatchRunRepo(&Data{DB: db})
	item := biz.BOMMatchResultItemDraft{
		LineID: 1, LineNo: 1, SourceType: biz.MatchResultAutoMatch,
		MatchStatus: "exact", DemandMpn: "LM358", MatchedMpn: "LM358",
		Subtotal: 12.5, Currency: "CNY", CodeTS: "8542399000",
		ControlMark: "A", ImportTaxImpOrdinaryRate: "30%",
		ImportTaxImpDiscountRate: "0%", ImportTaxImpTempRate: "",
	}
	id1, no1, err := repo.CreateMatchRun(context.Background(), "sid", 1, "CNY", "tester", []biz.BOMMatchResultItemDraft{item})
	if err != nil {
		t.Fatalf("create run1: %v", err)
	}
	id2, no2, err := repo.CreateMatchRun(context.Background(), "sid", 1, "CNY", "tester", []biz.BOMMatchResultItemDraft{item})
	if err != nil {
		t.Fatalf("create run2: %v", err)
	}
	if id1 == id2 || no1 != 1 || no2 != 2 {
		t.Fatalf("ids/no = %d/%d %d/%d", id1, no1, id2, no2)
	}
	_, items, err := repo.GetMatchRun(context.Background(), id1)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if len(items) != 1 || items[0].CodeTS != "8542399000" || items[0].ControlMark != "A" {
		t.Fatalf("unexpected item: %+v", items)
	}
}
