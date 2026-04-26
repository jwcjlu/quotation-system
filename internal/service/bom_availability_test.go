package service

import (
	"context"
	"encoding/json"
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

func TestComputeLineAvailability_CachesManufacturerAliasWithinRequest(t *testing.T) {
	view := &biz.BOMSessionView{
		SessionID:   "session-1",
		BizDate:     time.Date(2026, 4, 25, 0, 0, 0, 0, time.UTC),
		PlatformIDs: []string{"find_chips", "hqchip"},
	}
	lines := []data.BomSessionLine{
		{LineNo: 1, Mpn: "PART-A", Mfr: strPtr("TI"), Qty: floatPtr(1)},
		{LineNo: 2, Mpn: "PART-B", Mfr: strPtr("TI"), Qty: floatPtr(1)},
	}
	cacheMap := map[string]*biz.QuoteCacheSnapshot{}
	var tasks []biz.TaskReadinessSnapshot
	for _, line := range lines {
		mpnNorm := biz.NormalizeMPNForBOMSearch(line.Mpn)
		for _, platformID := range view.PlatformIDs {
			tasks = append(tasks, biz.TaskReadinessSnapshot{MpnNorm: mpnNorm, PlatformID: platformID, State: "succeeded"})
			cacheMap[quoteCachePairKey(mpnNorm, platformID)] = &biz.QuoteCacheSnapshot{
				Outcome:    "ok",
				QuotesJSON: quoteRowsJSON(t, line.Mpn, "TI"),
			}
		}
	}
	alias := newCountingManufacturerAliasRepo(map[string]string{"TI": "MFR_TI"})
	svc := &BomService{
		search:   &bomSearchTaskRepoStub{tasks: tasks, cacheMap: cacheMap},
		fx:       data.NewBomFxRateRepo(nil),
		alias:    alias,
		bomMatch: &conf.BomMatch{BaseCcy: "USD", ParsePriceTierStrings: true},
	}

	availability, _, err := svc.computeLineAvailability(context.Background(), view, lines, view.PlatformIDs)
	if err != nil {
		t.Fatalf("computeLineAvailability() error = %v", err)
	}
	for _, item := range availability {
		if item.Status != biz.LineAvailabilityReady {
			t.Fatalf("availability status = %q, want %q", item.Status, biz.LineAvailabilityReady)
		}
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
	return quoteRowsJSONFromRows(t, rows)
}

func quoteRowsJSONFromRows(t *testing.T, rows []biz.AgentQuoteRow) []byte {
	t.Helper()
	raw, err := json.Marshal(rows)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func strPtr(v string) *string { return &v }

func floatPtr(v float64) *float64 { return &v }

type countingManufacturerAliasRepo struct {
	values map[string]string
	calls  map[string]int
}

func newCountingManufacturerAliasRepo(values map[string]string) *countingManufacturerAliasRepo {
	return &countingManufacturerAliasRepo{
		values: values,
		calls:  make(map[string]int),
	}
}

func (m *countingManufacturerAliasRepo) CanonicalID(ctx context.Context, aliasNorm string) (string, bool, error) {
	m.calls[aliasNorm]++
	v, ok := m.values[aliasNorm]
	return v, ok, nil
}

func (m *countingManufacturerAliasRepo) DBOk() bool { return true }

func (m *countingManufacturerAliasRepo) ListDistinctCanonicals(ctx context.Context, limit int) ([]biz.ManufacturerCanonicalDisplay, error) {
	return nil, nil
}

func (m *countingManufacturerAliasRepo) CreateRow(ctx context.Context, canonicalID, displayName, alias, aliasNorm string) error {
	return nil
}

func (m *countingManufacturerAliasRepo) CallCount(alias string) int {
	return m.calls[biz.NormalizeMfrString(alias)]
}
