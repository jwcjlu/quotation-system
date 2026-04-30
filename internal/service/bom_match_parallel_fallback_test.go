package service

import (
	"context"
	"io"
	"testing"
	"time"

	"caichip/internal/biz"
	"caichip/internal/conf"
	"caichip/internal/data"
	"github.com/go-kratos/kratos/v2/log"
)

func TestComputeMatchItems_SubstituteFallbackWhenPrimaryNoMatch(t *testing.T) {
	view := &biz.BOMSessionView{
		SessionID:   "sid-fallback",
		BizDate:     time.Date(2026, 4, 25, 0, 0, 0, 0, time.UTC),
		PlatformIDs: []string{"find_chips"},
	}
	lines := []data.BomSessionLine{
		{ID: 1, LineNo: 1, Mpn: "PRIMARY-NO-HIT", SubstituteMpn: strPtr("SUB-HIT"), Qty: floatPtr(3)},
	}
	primaryKey := biz.NormalizeMPNForBOMSearch("PRIMARY-NO-HIT")
	subKey := biz.NormalizeMPNForBOMSearch("SUB-HIT")
	search := &bomSearchTaskRepoStub{
		cacheMap: map[string]*biz.QuoteCacheSnapshot{
			quoteCachePairKey(primaryKey, "find_chips"): {Outcome: "ok", QuotesJSON: quoteRowsJSONFromRows(t, []biz.AgentQuoteRow{{Model: "OTHER-MODEL", PriceTiers: "1+ ￥1.0000", MOQ: "1", Stock: "100"}})},
			quoteCachePairKey(subKey, "find_chips"):     {Outcome: "ok", QuotesJSON: quoteRowsJSONFromRows(t, []biz.AgentQuoteRow{{Model: "SUB-HIT", PriceTiers: "1+ ￥1.0000", MOQ: "1", Stock: "100"}})},
		},
	}
	svc := &BomService{
		search:   search,
		fx:       data.NewBomFxRateRepo(nil),
		alias:    manufacturerAliasRepoStub{},
		bomMatch: &conf.BomMatch{BaseCcy: "CNY", ParsePriceTierStrings: true},
		log:      log.NewHelper(log.NewStdLogger(io.Discard)),
	}

	items, _, err := svc.computeMatchItems(context.Background(), view, lines, view.PlatformIDs)
	if err != nil {
		t.Fatalf("computeMatchItems() error = %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("items len = %d, want 1", len(items))
	}
	if items[0].GetMatchStatus() != "exact" {
		t.Fatalf("match_status = %q, want exact", items[0].GetMatchStatus())
	}
	if items[0].GetMatchedBy() != "substitute" {
		t.Fatalf("matched_by = %q, want substitute", items[0].GetMatchedBy())
	}
	if items[0].GetMatchedQueryMpn() != subKey {
		t.Fatalf("matched_query_mpn = %q, want %q", items[0].GetMatchedQueryMpn(), subKey)
	}
}

func TestComputeMatchItems_KeepOriginalWhenPrimaryMatches(t *testing.T) {
	view := &biz.BOMSessionView{
		SessionID:   "sid-original",
		BizDate:     time.Date(2026, 4, 25, 0, 0, 0, 0, time.UTC),
		PlatformIDs: []string{"find_chips"},
	}
	lines := []data.BomSessionLine{
		{ID: 1, LineNo: 1, Mpn: "PRIMARY-HIT", SubstituteMpn: strPtr("SUB-ALSO-HIT"), Qty: floatPtr(2)},
	}
	primaryKey := biz.NormalizeMPNForBOMSearch("PRIMARY-HIT")
	subKey := biz.NormalizeMPNForBOMSearch("SUB-ALSO-HIT")
	search := &bomSearchTaskRepoStub{
		cacheMap: map[string]*biz.QuoteCacheSnapshot{
			quoteCachePairKey(primaryKey, "find_chips"): {Outcome: "ok", QuotesJSON: quoteRowsJSONFromRows(t, []biz.AgentQuoteRow{{Model: "PRIMARY-HIT", PriceTiers: "1+ ￥1.0000", MOQ: "1", Stock: "100"}})},
			quoteCachePairKey(subKey, "find_chips"):     {Outcome: "ok", QuotesJSON: quoteRowsJSONFromRows(t, []biz.AgentQuoteRow{{Model: "SUB-ALSO-HIT", PriceTiers: "1+ ￥0.5000", MOQ: "1", Stock: "100"}})},
		},
	}
	svc := &BomService{
		search:   search,
		fx:       data.NewBomFxRateRepo(nil),
		alias:    manufacturerAliasRepoStub{},
		bomMatch: &conf.BomMatch{BaseCcy: "CNY", ParsePriceTierStrings: true},
		log:      log.NewHelper(log.NewStdLogger(io.Discard)),
	}

	items, _, err := svc.computeMatchItems(context.Background(), view, lines, view.PlatformIDs)
	if err != nil {
		t.Fatalf("computeMatchItems() error = %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("items len = %d, want 1", len(items))
	}
	if items[0].GetMatchedBy() != "original" {
		t.Fatalf("matched_by = %q, want original", items[0].GetMatchedBy())
	}
	if items[0].GetMatchedQueryMpn() != primaryKey {
		t.Fatalf("matched_query_mpn = %q, want %q", items[0].GetMatchedQueryMpn(), primaryKey)
	}
}
