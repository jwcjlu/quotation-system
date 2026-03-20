package biz

import "testing"

func TestFilterFullyMatched(t *testing.T) {
	item := &BOMItem{Model: "ABC123", Manufacturer: "TI", Package: "TSSOP-16"}
	quotes := []*Quote{
		{MatchedModel: "ABC123", Manufacturer: "TI", Package: "TSSOP-16", UnitPrice: 1.0},
		{MatchedModel: "ABC123", Manufacturer: "TI", Package: "SOT-23", UnitPrice: 0.8},
		{MatchedModel: "ABC123", Manufacturer: "MPS", Package: "TSSOP-16", UnitPrice: 0.9},
		{MatchedModel: "XYZ", Manufacturer: "TI", Package: "TSSOP-16", UnitPrice: 0.5},
	}
	got := filterFullyMatched(quotes, item)
	if len(got) != 1 {
		t.Fatalf("filterFullyMatched: got %d, want 1", len(got))
	}
	if got[0].UnitPrice != 1.0 {
		t.Errorf("got UnitPrice %v, want 1.0", got[0].UnitPrice)
	}
}

func TestFilterFullyMatched_EmptyItemFields(t *testing.T) {
	item := &BOMItem{Model: "ABC123", Manufacturer: "", Package: ""}
	quotes := []*Quote{
		{MatchedModel: "ABC123", Manufacturer: "TI", Package: "TSSOP-16", UnitPrice: 1.0},
	}
	got := filterFullyMatched(quotes, item)
	if len(got) != 1 {
		t.Fatalf("empty item fields: got %d, want 1", len(got))
	}
}
