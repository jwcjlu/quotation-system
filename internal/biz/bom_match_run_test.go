package biz

import "testing"

func TestSummarizeMatchRunItems(t *testing.T) {
	total, matched, unresolved := SummarizeMatchRunItems([]BOMMatchResultItemDraft{
		{SourceType: MatchResultAutoMatch, Subtotal: 10},
		{SourceType: MatchResultManualQuote, Subtotal: 20},
		{SourceType: MatchResultUnresolved},
	})
	if total != 30 || matched != 2 || unresolved != 1 {
		t.Fatalf("summary total=%v matched=%d unresolved=%d", total, matched, unresolved)
	}
}

func TestMatchResultSourceFromItem(t *testing.T) {
	if got := MatchResultSourceFromMatchStatus("exact", false, false); got != MatchResultAutoMatch {
		t.Fatalf("source=%q", got)
	}
	if got := MatchResultSourceFromMatchStatus("exact", true, false); got != MatchResultManualQuote {
		t.Fatalf("source=%q", got)
	}
	if got := MatchResultSourceFromMatchStatus("exact", false, true); got != MatchResultSubstituteMatch {
		t.Fatalf("source=%q", got)
	}
	if got := MatchResultSourceFromMatchStatus("no_match", false, false); got != MatchResultUnresolved {
		t.Fatalf("source=%q", got)
	}
}
