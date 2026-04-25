package service

import (
	"context"
	"testing"
	"time"

	v1 "caichip/api/bom/v1"
	"caichip/internal/biz"
	"caichip/internal/data"
)

func TestListLineGapsSyncsAvailabilityGaps(t *testing.T) {
	view := &biz.BOMSessionView{SessionID: "sid", BizDate: time.Now(), PlatformIDs: []string{"icgoo"}}
	session := &bomSessionRepoStub{view: view, fullLines: []data.BomSessionLine{{ID: 1, LineNo: 1, Mpn: "NO-DATA"}}}
	search := &bomSearchTaskRepoStub{
		tasks: []biz.TaskReadinessSnapshot{{MpnNorm: "NO-DATA", PlatformID: "icgoo", State: "no_result"}},
		cacheMap: map[string]*biz.QuoteCacheSnapshot{
			testQuoteCacheKey("NO-DATA", "icgoo"): {Outcome: "no_mpn_match"},
		},
	}
	gaps := &bomLineGapRepoStub{}
	svc := NewBomService(session, search, gaps, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)

	_, err := svc.ListLineGaps(context.Background(), &v1.ListLineGapsRequest{SessionId: "sid", Statuses: []string{biz.LineGapOpen}})
	if err != nil {
		t.Fatalf("ListLineGaps: %v", err)
	}
	if len(gaps.gaps) != 1 || gaps.gaps[0].GapType != biz.LineGapNoData {
		t.Fatalf("synced gaps=%+v", gaps.gaps)
	}
}

func TestResolveLineGapManualQuoteClosesWithManualQuote(t *testing.T) {
	search := &bomSearchTaskRepoStub{}
	gaps := &bomLineGapRepoStub{}
	svc := NewBomService(&bomSessionRepoStub{}, search, gaps, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	_, err := svc.ResolveLineGapManualQuote(context.Background(), &v1.ResolveLineGapManualQuoteRequest{
		GapId: "99", Model: "LM358", Note: "ok",
	})
	if err != nil {
		t.Fatalf("ResolveLineGapManualQuote: %v", err)
	}
	if search.manualQuoteGap != 99 || len(gaps.updated) != 1 || gaps.updated[0] != 99 {
		t.Fatalf("manual gap=%d updated=%v", search.manualQuoteGap, gaps.updated)
	}
}

func TestSelectLineGapSubstituteQueuesSearch(t *testing.T) {
	view := &biz.BOMSessionView{SessionID: "sid", BizDate: time.Now(), PlatformIDs: []string{"icgoo"}, SelectionRevision: 1}
	search := &bomSearchTaskRepoStub{}
	gaps := &bomLineGapRepoStub{gaps: []biz.BOMLineGap{{ID: 99, SessionID: "sid", Status: biz.LineGapOpen}}}
	svc := NewBomService(&bomSessionRepoStub{view: view}, search, gaps, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	_, err := svc.SelectLineGapSubstitute(context.Background(), &v1.SelectLineGapSubstituteRequest{
		GapId: "99", SubstituteMpn: "LM358", Reason: "alt",
	})
	if err != nil {
		t.Fatalf("SelectLineGapSubstitute: %v", err)
	}
	if len(search.pendingPairs) != 1 || search.pendingPairs[0].MpnNorm != "LM358" {
		t.Fatalf("pending=%+v", search.pendingPairs)
	}
	if len(gaps.substitutes) != 1 || gaps.substitutes[0] != "LM358" {
		t.Fatalf("substitutes=%+v", gaps.substitutes)
	}
}
