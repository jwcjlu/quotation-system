package service

import (
	"context"
	"testing"
	"time"

	"caichip/internal/biz"
	"caichip/internal/data"
)

func TestListManufacturerAliasCandidatesUsesLightQuoteRows(t *testing.T) {
	view := &biz.BOMSessionView{
		SessionID:   "session-1",
		BizDate:     time.Date(2026, 4, 25, 0, 0, 0, 0, time.UTC),
		PlatformIDs: []string{"find_chips", "hqchip"},
	}
	canonTI := "MFR_TI"
	lines := []data.BomSessionLine{
		{LineNo: 1, Mpn: "PART-A", Mfr: strPtr("TI"), ManufacturerCanonicalID: &canonTI, Package: strPtr("QFN"), Qty: floatPtr(1)},
		{LineNo: 2, Mpn: "PART-B", Mfr: strPtr("TI"), ManufacturerCanonicalID: &canonTI, Package: strPtr("QFN"), Qty: floatPtr(1)},
	}
	cacheMap := map[string]*biz.QuoteCacheSnapshot{}
	candidateRows := map[string][]biz.AgentQuoteRow{}
	for _, line := range lines {
		mpnNorm := biz.NormalizeMPNForBOMSearch(line.Mpn)
		for _, platformID := range view.PlatformIDs {
			key := quoteCachePairKey(mpnNorm, platformID)
			cacheMap[key] = &biz.QuoteCacheSnapshot{
				Outcome: "ok",
				QuotesJSON: quoteRowsJSONFromRows(t, []biz.AgentQuoteRow{
					{Model: line.Mpn, Package: "QFN", Manufacturer: "ADI"},
					{Model: line.Mpn, Package: "SOP", Manufacturer: "PACKAGE-MISMATCH"},
					{Model: "OTHER-" + line.Mpn, Package: "QFN", Manufacturer: "MODEL-MISMATCH"},
					{Model: line.Mpn, Package: "QFN", Manufacturer: "TI"},
				}),
			}
			candidateRows[key] = []biz.AgentQuoteRow{
				{Model: line.Mpn, Package: "QFN", Manufacturer: "ADI"},
				{Model: line.Mpn, Package: "SOP", Manufacturer: "PACKAGE-MISMATCH"},
				{Model: "OTHER-" + line.Mpn, Package: "QFN", Manufacturer: "MODEL-MISMATCH"},
				{Model: line.Mpn, Package: "QFN", Manufacturer: "TI"},
			}
		}
	}
	search := &bomSearchTaskRepoStub{cacheMap: cacheMap, candidateRows: candidateRows}
	svc := &BomService{
		session: &bomSessionRepoStub{view: view, fullLines: lines},
		search:  search,
		alias:   manufacturerAliasRepoStub{"TI": "MFR_TI", "ADI": "MFR_ADI"},
	}

	reply, err := svc.ListManufacturerAliasCandidates(context.Background(), "session-1")
	if err != nil {
		t.Fatalf("ListManufacturerAliasCandidates() error = %v", err)
	}
	if search.candidateCalls != 1 {
		t.Fatalf("candidate row calls = %d, want 1", search.candidateCalls)
	}
	if search.cacheBatchCalls != 0 {
		t.Fatalf("full cache batch calls = %d, want 0", search.cacheBatchCalls)
	}
	if len(reply.Items) != 1 {
		t.Fatalf("items len = %d, want 1: %+v", len(reply.Items), reply.Items)
	}
	got := map[string]ManufacturerAliasCandidate{}
	for _, item := range reply.Items {
		got[item.Kind+":"+item.Alias] = item
	}
	item, ok := got["quote:ADI"]
	if !ok {
		t.Fatalf("missing alias ADI in %+v", reply.Items)
	}
	if item.DemandHint != "TI" {
		t.Fatalf("ADI demand hint = %q, want TI", item.DemandHint)
	}
	if len(item.LineNos) != 2 || item.LineNos[0] != 1 || item.LineNos[1] != 2 {
		t.Fatalf("ADI line nos = %v, want [1 2]", item.LineNos)
	}
	if _, ok := got["quote:TI"]; ok {
		t.Fatalf("same canonical quote manufacturer should be skipped: %+v", reply.Items)
	}
	if _, ok := got["quote:PACKAGE-MISMATCH"]; ok {
		t.Fatalf("package mismatch quote manufacturer should be skipped: %+v", reply.Items)
	}
	if _, ok := got["quote:MODEL-MISMATCH"]; ok {
		t.Fatalf("model mismatch quote manufacturer should be skipped: %+v", reply.Items)
	}
}

func TestListManufacturerAliasCandidatesSkipsBomManufacturerAliasMiss(t *testing.T) {
	view := &biz.BOMSessionView{
		SessionID:   "session-1",
		BizDate:     time.Date(2026, 4, 25, 0, 0, 0, 0, time.UTC),
		PlatformIDs: []string{"find_chips"},
	}
	lines := []data.BomSessionLine{
		{LineNo: 4, Mpn: "VDRS05C275BSE", Mfr: strPtr("Vishay BCcomponents"), Package: strPtr("THT 7x11x4.9")},
	}
	mpnNorm := biz.NormalizeMPNForBOMSearch(lines[0].Mpn)
	svc := &BomService{
		session: &bomSessionRepoStub{view: view, fullLines: lines},
		search: &bomSearchTaskRepoStub{candidateRows: map[string][]biz.AgentQuoteRow{
			quoteCachePairKey(mpnNorm, "find_chips"): {
				{Model: "VDRS05C275BSE", Package: "THT 7x11x4.9", Manufacturer: "Vishay Intertechnologies"},
				{Model: "VDRS05C275BSE", Package: "THT 7x11x4.9", Manufacturer: "ADI"},
			},
		}},
		alias: manufacturerAliasRepoStub{"VISHAY": "MFR_VISHAY"},
	}

	reply, err := svc.ListManufacturerAliasCandidates(context.Background(), "session-1")
	if err != nil {
		t.Fatalf("ListManufacturerAliasCandidates() error = %v", err)
	}
	if len(reply.Items) != 1 {
		t.Fatalf("items len = %d, want 1 demand candidate when BOM manufacturer alias misses: %+v", len(reply.Items), reply.Items)
	}
	if reply.Items[0].Kind != "demand" || reply.Items[0].Alias != "Vishay BCcomponents" {
		t.Fatalf("candidate = %+v, want demand Vishay BCcomponents", reply.Items[0])
	}
}

func TestListManufacturerAliasCandidatesSkipsCleanedQuoteManufacturers(t *testing.T) {
	view := &biz.BOMSessionView{
		SessionID:   "session-1",
		BizDate:     time.Date(2026, 4, 25, 0, 0, 0, 0, time.UTC),
		PlatformIDs: []string{"find_chips"},
	}
	canonTI := "MFR_TI"
	lines := []data.BomSessionLine{
		{LineNo: 1, Mpn: "PART-A", Mfr: strPtr("TI"), ManufacturerCanonicalID: &canonTI, Package: strPtr("QFN")},
		{LineNo: 2, Mpn: "PART-B", Mfr: strPtr("UnknownDemand"), Package: strPtr("QFN")},
	}
	mpnNorm := biz.NormalizeMPNForBOMSearch("PART-A")
	svc := &BomService{
		session: &bomSessionRepoStub{view: view, fullLines: lines},
		search: &bomSearchTaskRepoStub{candidateRows: map[string][]biz.AgentQuoteRow{
			quoteCachePairKey(mpnNorm, "find_chips"): {
				{Model: "PART-A", Package: "QFN", Manufacturer: "Texas Instruments", ManufacturerCanonicalID: &canonTI},
				{Model: "PART-A", Package: "QFN", Manufacturer: "ADI"},
			},
		}},
		alias: manufacturerAliasRepoStub{"TI": "MFR_TI", "TEXAS INSTRUMENTS": "MFR_TI", "ADI": "MFR_ADI"},
	}

	reply, err := svc.ListManufacturerAliasCandidates(context.Background(), "session-1")
	if err != nil {
		t.Fatalf("ListManufacturerAliasCandidates() error = %v", err)
	}
	got := map[string]ManufacturerAliasCandidate{}
	for _, item := range reply.Items {
		got[item.Kind+":"+item.Alias] = item
	}
	if _, ok := got["quote:Texas Instruments"]; ok {
		t.Fatalf("cleaned quote manufacturer should be skipped: %+v", reply.Items)
	}
	quoteADI, ok := got["quote:ADI"]
	if !ok {
		t.Fatalf("missing uncleaned quote manufacturer ADI in %+v", reply.Items)
	}
	if quoteADI.RecommendedCanonicalID != "MFR_TI" {
		t.Fatalf("ADI recommended canonical = %q, want MFR_TI", quoteADI.RecommendedCanonicalID)
	}
	demandUnknown, ok := got["demand:UnknownDemand"]
	if !ok {
		t.Fatalf("missing uncleaned demand manufacturer in %+v", reply.Items)
	}
	if demandUnknown.RecommendedCanonicalID != "" {
		t.Fatalf("unknown demand recommended canonical = %q, want empty", demandUnknown.RecommendedCanonicalID)
	}
}
