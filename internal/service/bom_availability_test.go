package service

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"caichip/internal/biz"
	"caichip/internal/conf"
	"caichip/internal/data"
)

func TestComputeLineAvailability(t *testing.T) {
	view := &biz.BOMSessionView{
		SessionID:   "session-1",
		BizDate:     time.Date(2026, 4, 25, 0, 0, 0, 0, time.UTC),
		PlatformIDs: []string{"find_chips"},
	}
	lines := []data.BomSessionLine{
		{LineNo: 1, Mpn: "READY", Qty: floatPtr(5)},
		{LineNo: 2, Mpn: "NO-DATA"},
		{LineNo: 3, Mpn: "BROKEN"},
		{LineNo: 4, Mpn: "FILTERED", Mfr: strPtr("TI"), Qty: floatPtr(5)},
		{LineNo: 5, Mpn: "PENDING"},
	}
	plats := []string{"find_chips"}
	readyKey := biz.NormalizeMPNForBOMSearch("READY")
	noDataKey := biz.NormalizeMPNForBOMSearch("NO-DATA")
	brokenKey := biz.NormalizeMPNForBOMSearch("BROKEN")
	filteredKey := biz.NormalizeMPNForBOMSearch("FILTERED")
	pendingKey := biz.NormalizeMPNForBOMSearch("PENDING")
	search := &bomSearchTaskRepoStub{
		tasks: []biz.TaskReadinessSnapshot{
			{MpnNorm: readyKey, PlatformID: "find_chips", State: "succeeded"},
			{MpnNorm: noDataKey, PlatformID: "find_chips", State: "no_result"},
			{MpnNorm: brokenKey, PlatformID: "find_chips", State: "failed_terminal"},
			{MpnNorm: filteredKey, PlatformID: "find_chips", State: "succeeded"},
			{MpnNorm: pendingKey, PlatformID: "find_chips", State: "running"},
		},
		cacheMap: map[string]*biz.QuoteCacheSnapshot{
			quoteCachePairKey(readyKey, "find_chips"):    {Outcome: "ok", QuotesJSON: quoteRowsJSON(t, "READY", "")},
			quoteCachePairKey(noDataKey, "find_chips"):   {Outcome: "no_mpn_match"},
			quoteCachePairKey(brokenKey, "find_chips"):   {Outcome: "failed"},
			quoteCachePairKey(filteredKey, "find_chips"): {Outcome: "ok", QuotesJSON: quoteRowsJSON(t, "FILTERED", "ADI")},
		},
	}
	svc := &BomService{
		search:   search,
		fx:       data.NewBomFxRateRepo(nil),
		alias:    manufacturerAliasRepoStub{"TI": "MFR_TI", "ADI": "MFR_ADI"},
		bomMatch: &conf.BomMatch{BaseCcy: "USD", ParsePriceTierStrings: true},
	}

	availability, summary, err := svc.computeLineAvailability(context.Background(), view, lines, plats)
	if err != nil {
		t.Fatalf("computeLineAvailability() error = %v", err)
	}
	if len(availability) != 5 {
		t.Fatalf("availability len = %d, want 5", len(availability))
	}
	want := []string{
		biz.LineAvailabilityReady,
		biz.LineAvailabilityNoData,
		biz.LineAvailabilityCollectionUnavailable,
		biz.LineAvailabilityNoMatchAfterFilter,
		biz.LineAvailabilityCollecting,
	}
	for i, status := range want {
		if availability[i].Status != status {
			t.Fatalf("line %d status = %q, want %q", i+1, availability[i].Status, status)
		}
	}
	if summary.ReadyLineCount != 1 || summary.NoDataLineCount != 1 ||
		summary.CollectionUnavailableLineCount != 1 ||
		summary.NoMatchAfterFilterLineCount != 1 || summary.CollectingLineCount != 1 {
		t.Fatalf("summary = %+v", summary)
	}
	if !summary.HasStrictBlockingGap() {
		t.Fatalf("summary.HasStrictBlockingGap() = false, want true")
	}
}

func TestComputeLineAvailability_NilDepsAndEmptyInput(t *testing.T) {
	svc := &BomService{}
	availability, summary, err := svc.computeLineAvailability(context.Background(), nil, nil, []string{"find_chips"})
	if err != nil {
		t.Fatalf("computeLineAvailability() error = %v", err)
	}
	if len(availability) != 0 || summary.LineTotal != 0 {
		t.Fatalf("availability=%v summary=%+v, want empty", availability, summary)
	}

	lines := []data.BomSessionLine{{LineNo: 1, Mpn: "UNKNOWN"}}
	availability, summary, err = svc.computeLineAvailability(context.Background(), nil, lines, []string{"find_chips"})
	if err != nil {
		t.Fatalf("computeLineAvailability() with nil deps error = %v", err)
	}
	if len(availability) != 1 || availability[0].Status != biz.LineAvailabilityCollecting {
		t.Fatalf("availability=%+v, want collecting", availability)
	}
	if summary.CollectingLineCount != 1 || summary.HasStrictBlockingGap() {
		t.Fatalf("summary=%+v, want one non-blocking collecting line", summary)
	}
}

func TestComputeLineAvailability_ReusesSessionAliasLookups(t *testing.T) {
	view := &biz.BOMSessionView{
		SessionID:   "session-1",
		BizDate:     time.Date(2026, 4, 25, 0, 0, 0, 0, time.UTC),
		PlatformIDs: []string{"find_chips"},
	}
	lines := []data.BomSessionLine{
		{LineNo: 1, Mpn: "READY-1", Mfr: strPtr("TI"), Qty: floatPtr(5)},
		{LineNo: 2, Mpn: "READY-2", Mfr: strPtr("TI"), Qty: floatPtr(5)},
		{LineNo: 3, Mpn: "READY-3", Mfr: strPtr("TI"), Qty: floatPtr(5)},
	}
	plats := []string{"find_chips"}
	search := &bomSearchTaskRepoStub{
		tasks: []biz.TaskReadinessSnapshot{
			{MpnNorm: biz.NormalizeMPNForBOMSearch("READY-1"), PlatformID: "find_chips", State: "succeeded"},
			{MpnNorm: biz.NormalizeMPNForBOMSearch("READY-2"), PlatformID: "find_chips", State: "succeeded"},
			{MpnNorm: biz.NormalizeMPNForBOMSearch("READY-3"), PlatformID: "find_chips", State: "succeeded"},
		},
		cacheMap: map[string]*biz.QuoteCacheSnapshot{
			quoteCachePairKey(biz.NormalizeMPNForBOMSearch("READY-1"), "find_chips"): {Outcome: "ok", QuotesJSON: quoteRowsJSON(t, "READY-1", "TI")},
			quoteCachePairKey(biz.NormalizeMPNForBOMSearch("READY-2"), "find_chips"): {Outcome: "ok", QuotesJSON: quoteRowsJSON(t, "READY-2", "TI")},
			quoteCachePairKey(biz.NormalizeMPNForBOMSearch("READY-3"), "find_chips"): {Outcome: "ok", QuotesJSON: quoteRowsJSON(t, "READY-3", "TI")},
		},
	}
	alias := newCountingAliasLookup(map[string]string{"TI": "MFR_TI"})
	svc := &BomService{
		search:   search,
		fx:       data.NewBomFxRateRepo(nil),
		alias:    alias,
		bomMatch: &conf.BomMatch{BaseCcy: "USD", ParsePriceTierStrings: true},
	}

	_, _, err := svc.computeLineAvailability(context.Background(), view, lines, plats)
	if err != nil {
		t.Fatalf("computeLineAvailability() error = %v", err)
	}
	if got := alias.CallCount("TI"); got != 1 {
		t.Fatalf("alias lookup count for TI = %d, want 1", got)
	}
}

func quoteRowsJSON(t *testing.T, model, mfr string) []byte {
	t.Helper()
	rows := []biz.AgentQuoteRow{{
		Model:        model,
		Manufacturer: mfr,
		Stock:        "100",
		MOQ:          "1",
		PriceTiers:   "1+ $1.0000",
	}}
	raw, err := json.Marshal(rows)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func strPtr(v string) *string { return &v }

func floatPtr(v float64) *float64 { return &v }

type countingAliasLookup struct {
	mu    sync.Mutex
	hits  map[string]string
	calls map[string]int
}

func newCountingAliasLookup(hits map[string]string) *countingAliasLookup {
	return &countingAliasLookup{
		hits:  hits,
		calls: make(map[string]int),
	}
}

func (c *countingAliasLookup) CanonicalID(ctx context.Context, aliasNorm string) (string, bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.calls[aliasNorm]++
	id, ok := c.hits[aliasNorm]
	return id, ok, nil
}

func (c *countingAliasLookup) CallCount(aliasNorm string) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.calls[aliasNorm]
}

func (c *countingAliasLookup) DBOk() bool { return true }

func (c *countingAliasLookup) ListDistinctCanonicals(ctx context.Context, limit int) ([]biz.ManufacturerCanonicalDisplay, error) {
	return nil, nil
}

func (c *countingAliasLookup) CreateRow(ctx context.Context, canonicalID, displayName, alias, aliasNorm string) error {
	return nil
}
