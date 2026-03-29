package biz

import (
	"math"
	"testing"
)

func TestQuantizeUnitPriceBase_MinorUnit(t *testing.T) {
	q := QuantizeUnitPriceBase("minor_unit", 12.3456789)
	want := int64(math.Round(12.3456789 * 1e6))
	if q != want {
		t.Fatalf("got %d want %d", q, want)
	}
}

func TestQuantizeUnitPriceBase_Decimal6(t *testing.T) {
	q := QuantizeUnitPriceBase("decimal6", 1.2345674)
	// 1.234567 after %.6f
	want := int64(1234567)
	if q != want {
		t.Fatalf("got %d want %d", q, want)
	}
}

func TestMatchSort_LessCandidate_SamePriceShorterLeadWins(t *testing.T) {
	a := MatchSortKey{UnitPriceBaseQuantized: 100, LeadDays: 3, StockParsed: 0, PlatformID: "hqchip"}
	b := MatchSortKey{UnitPriceBaseQuantized: 100, LeadDays: 10, StockParsed: 0, PlatformID: "hqchip"}
	if !LessMatchCandidate(a, b) {
		t.Fatal("a (3d) should rank before b (10d)")
	}
	if LessMatchCandidate(b, a) {
		t.Fatal("b should not rank before a")
	}
}

func TestMatchSort_LessCandidate_UnknownLeadLosesToKnown(t *testing.T) {
	known := MatchSortKey{UnitPriceBaseQuantized: 50, LeadDays: 100, StockParsed: 0, PlatformID: "a"}
	unknown := MatchSortKey{UnitPriceBaseQuantized: 50, LeadDays: MatchLeadDaysUnknown, StockParsed: 0, PlatformID: "a"}
	if !LessMatchCandidate(known, unknown) {
		t.Fatal("known lead should rank before +inf unknown")
	}
	if LessMatchCandidate(unknown, known) {
		t.Fatal("unknown should not rank before known")
	}
}

func TestMatchSort_LessCandidate_StockTieBreak(t *testing.T) {
	a := MatchSortKey{UnitPriceBaseQuantized: 1, LeadDays: 1, StockParsed: 500, PlatformID: "x"}
	b := MatchSortKey{UnitPriceBaseQuantized: 1, LeadDays: 1, StockParsed: 100, PlatformID: "x"}
	if !LessMatchCandidate(a, b) {
		t.Fatal("higher stock should win")
	}
}

func TestMatchSort_LessCandidate_PlatformIDTieBreak(t *testing.T) {
	a := MatchSortKey{UnitPriceBaseQuantized: 1, LeadDays: 1, StockParsed: 10, PlatformID: "aaa"}
	b := MatchSortKey{UnitPriceBaseQuantized: 1, LeadDays: 1, StockParsed: 10, PlatformID: "zzz"}
	if !LessMatchCandidate(a, b) {
		t.Fatal("lexicographically smaller platform_id should win")
	}
	if LessMatchCandidate(b, a) {
		t.Fatal("zzz should not beat aaa")
	}
}

func TestMatchSort_LessCandidate_LowerPriceWins(t *testing.T) {
	cheap := MatchSortKey{UnitPriceBaseQuantized: 10, LeadDays: 0, StockParsed: 0, PlatformID: "z"}
	expensive := MatchSortKey{UnitPriceBaseQuantized: 20, LeadDays: 0, StockParsed: 9999, PlatformID: "a"}
	if !LessMatchCandidate(cheap, expensive) {
		t.Fatal("lower price must win regardless of stock/platform")
	}
}
