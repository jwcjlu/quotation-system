package service

import (
	"context"
	"reflect"
	"testing"
	"time"

	"caichip/internal/biz"
	"caichip/internal/data"
)

func TestMatchReadinessError_BlocksStrictAvailabilityGap(t *testing.T) {
	bizDate := time.Date(2026, 4, 25, 0, 0, 0, 0, time.UTC)
	key := biz.NormalizeMPNForBOMSearch("NO-DATA")
	view := &biz.BOMSessionView{
		SessionID:   "session-1",
		BizDate:     bizDate,
		Status:      "data_ready",
		PlatformIDs: []string{"find_chips"},
	}
	lines := []data.BomSessionLine{{LineNo: 1, Mpn: "NO-DATA"}}
	search := &bomSearchTaskRepoStub{
		tasks: []biz.TaskReadinessSnapshot{
			{MpnNorm: key, PlatformID: "find_chips", State: "no_result"},
		},
		cacheMap: map[string]*biz.QuoteCacheSnapshot{
			quoteCachePairKey(key, "find_chips"): {Outcome: "no_mpn_match"},
		},
	}
	svc := &BomService{search: search}

	err := svc.matchReadinessError(context.Background(), "session-1", view, lines, false)
	if err == nil {
		t.Fatalf("matchReadinessError() error = nil, want strict availability gap")
	}
	if got := err.Error(); got == "" {
		t.Fatalf("matchReadinessError() returned empty error")
	}
}

func TestMatchReadinessError_AllowsCollectingWithoutStrictGap(t *testing.T) {
	view := &biz.BOMSessionView{
		SessionID:     "session-1",
		BizDate:       time.Date(2026, 4, 25, 0, 0, 0, 0, time.UTC),
		Status:        "data_ready",
		ReadinessMode: biz.ReadinessStrict,
		PlatformIDs:   []string{"find_chips"},
	}
	lines := []data.BomSessionLine{{LineNo: 1, Mpn: "PENDING"}}
	search := &bomSearchTaskRepoStub{
		tasks: []biz.TaskReadinessSnapshot{
			{MpnNorm: biz.NormalizeMPNForBOMSearch("PENDING"), PlatformID: "find_chips", State: "running"},
		},
		cacheMap: map[string]*biz.QuoteCacheSnapshot{},
	}
	svc := &BomService{search: search}

	if err := svc.matchReadinessError(context.Background(), "session-1", view, lines, false); err != nil {
		t.Fatalf("matchReadinessError() error = %v, want nil", err)
	}
}

func TestMatchReadinessError_AutoMatchAllowsNoMatchAfterFilterGap(t *testing.T) {
	bizDate := time.Date(2026, 4, 25, 0, 0, 0, 0, time.UTC)
	key := biz.NormalizeMPNForBOMSearch("FILTERED")
	view := &biz.BOMSessionView{
		SessionID:   "session-1",
		BizDate:     bizDate,
		Status:      "data_ready",
		PlatformIDs: []string{"find_chips"},
	}
	lines := []data.BomSessionLine{
		{LineNo: 1, Mpn: "FILTERED", Mfr: strPtr("TI")},
	}
	search := &bomSearchTaskRepoStub{
		tasks: []biz.TaskReadinessSnapshot{
			{MpnNorm: key, PlatformID: "find_chips", State: "succeeded"},
		},
		cacheMap: map[string]*biz.QuoteCacheSnapshot{
			quoteCachePairKey(key, "find_chips"): {Outcome: "ok", QuotesJSON: quoteRowsJSON(t, "FILTERED", "ADI")},
		},
	}
	svc := &BomService{search: search, alias: manufacturerAliasRepoStub{"TI": "MFR_TI", "ADI": "MFR_ADI"}}

	if err := svc.matchReadinessError(context.Background(), "session-1", view, lines, true); err != nil {
		t.Fatalf("matchReadinessError(..., allowNoMatchAfterFilter=true) error = %v, want nil", err)
	}
	err := svc.matchReadinessError(context.Background(), "session-1", view, lines, false)
	if err == nil {
		t.Fatalf("matchReadinessError(..., allowNoMatchAfterFilter=false) error = nil, want strict availability gap")
	}
}

func TestMatchReadinessError_AutoMatchSkipsTaskGateWhenNoHardGap(t *testing.T) {
	bizDate := time.Date(2026, 4, 25, 0, 0, 0, 0, time.UTC)
	key := biz.NormalizeMPNForBOMSearch("PENDING")
	view := &biz.BOMSessionView{
		SessionID:     "session-1",
		BizDate:       bizDate,
		Status:        "draft",
		ReadinessMode: biz.ReadinessStrict,
		PlatformIDs:   []string{"find_chips"},
	}
	lines := []data.BomSessionLine{{LineNo: 1, Mpn: "PENDING"}}
	search := &bomSearchTaskRepoStub{
		tasks: []biz.TaskReadinessSnapshot{
			{MpnNorm: key, PlatformID: "find_chips", State: "running"},
		},
		cacheMap: map[string]*biz.QuoteCacheSnapshot{},
	}
	svc := &BomService{search: search}

	if err := svc.matchReadinessError(context.Background(), "session-1", view, lines, true); err != nil {
		t.Fatalf("AutoMatch readiness: error = %v, want nil (skip task gate when no hard gap)", err)
	}
	if err := svc.matchReadinessError(context.Background(), "session-1", view, lines, false); err == nil {
		t.Fatalf("non-AutoMatch: error = nil, want BOM_NOT_READY when session not data_ready and tasks incomplete")
	}
}

func TestDemandManufacturerCleaningRequired(t *testing.T) {
	canon := "MFR_TI"
	lines := []data.BomSessionLine{
		{LineNo: 1, Mpn: "PART-A", Mfr: strPtr("TI"), ManufacturerCanonicalID: &canon},
		{LineNo: 2, Mpn: "PART-B", Mfr: strPtr("UnknownMfr")},
		{LineNo: 3, Mpn: "PART-C"},
	}
	got := demandManufacturerCleaningRequired(lines)
	if want := []int{2}; !reflect.DeepEqual(got, want) {
		t.Fatalf("demandManufacturerCleaningRequired() = %v, want %v", got, want)
	}
}
