package service

import (
	"context"
	"testing"
	"time"

	v1 "caichip/api/bom/v1"
	"caichip/internal/biz"
	"caichip/internal/conf"
	"caichip/internal/data"
)

func TestGetReadinessReturnsLineAvailabilitySummary(t *testing.T) {
	svc := newAvailabilityAPITestService(t)

	reply, err := svc.GetReadiness(context.Background(), &v1.GetReadinessRequest{SessionId: "session-1"})
	if err != nil {
		t.Fatalf("GetReadiness() error = %v", err)
	}

	if reply.LineTotal != 3 {
		t.Fatalf("LineTotal = %d, want 3", reply.LineTotal)
	}
	if reply.ReadyLineCount != 1 || reply.NoDataLineCount != 1 ||
		reply.NoMatchAfterFilterLineCount != 1 || reply.GapLineCount != 2 {
		t.Fatalf("reply summary = %+v", reply)
	}
	if !reply.HasStrictBlockingGap {
		t.Fatalf("HasStrictBlockingGap = false, want true")
	}
}

func TestGetBOMLinesReturnsLineAvailabilityFields(t *testing.T) {
	svc := newAvailabilityAPITestService(t)

	reply, err := svc.GetBOMLines(context.Background(), &v1.GetBOMLinesRequest{SessionId: "session-1"})
	if err != nil {
		t.Fatalf("GetBOMLines() error = %v", err)
	}
	if len(reply.Lines) != 3 {
		t.Fatalf("lines len = %d, want 3", len(reply.Lines))
	}

	ready := reply.Lines[0]
	if ready.AvailabilityStatus != biz.LineAvailabilityReady ||
		ready.AvailabilityReasonCode != "READY" || !ready.HasUsableQuote ||
		ready.RawQuotePlatformCount != 1 || ready.UsableQuotePlatformCount != 1 ||
		ready.ResolutionStatus != "open" {
		t.Fatalf("ready line availability = %+v", ready)
	}

	filtered := reply.Lines[2]
	if filtered.AvailabilityStatus != biz.LineAvailabilityNoMatchAfterFilter ||
		filtered.AvailabilityReasonCode != "NO_MATCH_AFTER_FILTER" ||
		filtered.AvailabilityReason == "" || filtered.HasUsableQuote ||
		filtered.RawQuotePlatformCount != 1 || filtered.UsableQuotePlatformCount != 0 ||
		filtered.ResolutionStatus != "open" {
		t.Fatalf("filtered line availability = %+v", filtered)
	}
}

func newAvailabilityAPITestService(t *testing.T) *BomService {
	t.Helper()

	bizDate := time.Date(2026, 4, 25, 0, 0, 0, 0, time.UTC)
	readyKey := biz.NormalizeMPNForBOMSearch("READY")
	noDataKey := biz.NormalizeMPNForBOMSearch("NO-DATA")
	filteredKey := biz.NormalizeMPNForBOMSearch("FILTERED")

	session := &bomSessionRepoStub{
		view: &biz.BOMSessionView{
			SessionID:         "session-1",
			BizDate:           bizDate,
			SelectionRevision: 7,
			Status:            "data_ready",
			PlatformIDs:       []string{"find_chips"},
			ReadinessMode:     biz.ReadinessStrict,
		},
		fullLines: []data.BomSessionLine{
			{ID: 101, LineNo: 1, Mpn: "READY", Qty: floatPtr(5)},
			{ID: 102, LineNo: 2, Mpn: "NO-DATA"},
			{ID: 103, LineNo: 3, Mpn: "FILTERED", Mfr: strPtr("TI"), Qty: floatPtr(5)},
		},
	}
	search := &bomSearchTaskRepoStub{
		tasks: []biz.TaskReadinessSnapshot{
			{MpnNorm: readyKey, PlatformID: "find_chips", State: "succeeded"},
			{MpnNorm: noDataKey, PlatformID: "find_chips", State: "no_result"},
			{MpnNorm: filteredKey, PlatformID: "find_chips", State: "succeeded"},
		},
		cacheMap: map[string]*biz.QuoteCacheSnapshot{
			quoteCachePairKey(readyKey, "find_chips"):    {Outcome: "ok", QuotesJSON: quoteRowsJSON(t, "READY", "")},
			quoteCachePairKey(noDataKey, "find_chips"):   {Outcome: "no_mpn_match"},
			quoteCachePairKey(filteredKey, "find_chips"): {Outcome: "ok", QuotesJSON: quoteRowsJSON(t, "FILTERED", "ADI")},
		},
	}
	return &BomService{
		session:  session,
		search:   search,
		fx:       data.NewBomFxRateRepo(nil),
		alias:    manufacturerAliasRepoStub{"TI": "MFR_TI", "ADI": "MFR_ADI"},
		bomMatch: &conf.BomMatch{BaseCcy: "USD", ParsePriceTierStrings: true},
	}
}
