package service

import (
	"context"
	"testing"
	"time"

	v1 "caichip/api/bom/v1"
	"caichip/internal/biz"
	"caichip/internal/data"
)

func TestSaveMatchRunPersistsExactAndUnresolvedRows(t *testing.T) {
	view := &biz.BOMSessionView{SessionID: "sid", Status: "data_ready", BizDate: time.Now(), PlatformIDs: []string{"icgoo"}, SelectionRevision: 1}
	session := &bomSessionRepoStub{view: view, fullLines: []data.BomSessionLine{
		{ID: 1, LineNo: 1, Mpn: "OK"},
		{ID: 2, LineNo: 2, Mpn: "NO-DATA"},
	}}
	runs := &bomMatchRunRepoStub{}
	gaps := &bomLineGapRepoStub{gaps: []biz.BOMLineGap{{ID: 99, SessionID: "sid", LineID: 2, LineNo: 2, Mpn: "NO-DATA", GapType: biz.LineGapNoData, Status: biz.LineGapOpen}}}
	search := &bomSearchTaskRepoStub{cacheMap: map[string]*biz.QuoteCacheSnapshot{}}
	svc := NewBomService(session, search, gaps, runs, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)

	resp, err := svc.SaveMatchRun(context.Background(), &v1.SaveMatchRunRequest{SessionId: "sid"})
	if err != nil {
		t.Fatalf("SaveMatchRun: %v", err)
	}
	if resp.GetRunNo() != 1 || len(runs.items) != 2 {
		t.Fatalf("run resp=%+v items=%+v", resp, runs.items)
	}
	if runs.items[1].SourceType != biz.MatchResultUnresolved || runs.items[1].GapID != 99 {
		t.Fatalf("unresolved item=%+v", runs.items[1])
	}
}

func TestListAndGetMatchRuns(t *testing.T) {
	runs := &bomMatchRunRepoStub{
		runs:  []biz.BOMMatchRunView{{ID: 7, RunNo: 1, SessionID: "sid", Status: biz.MatchRunSaved}},
		items: []biz.BOMMatchResultItemDraft{{LineID: 1, LineNo: 1, SourceType: biz.MatchResultAutoMatch}},
	}
	svc := NewBomService(&bomSessionRepoStub{}, &bomSearchTaskRepoStub{}, &bomLineGapRepoStub{}, runs, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	list, err := svc.ListMatchRuns(context.Background(), &v1.ListMatchRunsRequest{SessionId: "sid"})
	if err != nil || len(list.GetRuns()) != 1 {
		t.Fatalf("ListMatchRuns resp=%+v err=%v", list, err)
	}
	got, err := svc.GetMatchRun(context.Background(), &v1.GetMatchRunRequest{RunId: "7"})
	if err != nil || len(got.GetItems()) != 1 {
		t.Fatalf("GetMatchRun resp=%+v err=%v", got, err)
	}
}

func TestExportSessionUsesRunSnapshot(t *testing.T) {
	runs := &bomMatchRunRepoStub{
		runs:  []biz.BOMMatchRunView{{ID: 7, RunNo: 1, SessionID: "sid", Status: biz.MatchRunSaved}},
		items: []biz.BOMMatchResultItemDraft{{LineID: 1, LineNo: 1, DemandMpn: "LM358", SourceType: biz.MatchResultAutoMatch}},
	}
	svc := NewBomService(&bomSessionRepoStub{}, &bomSearchTaskRepoStub{}, &bomLineGapRepoStub{}, runs, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	resp, err := svc.ExportSession(context.Background(), &v1.ExportSessionRequest{RunId: "7"})
	if err != nil {
		t.Fatalf("ExportSession: %v", err)
	}
	if len(resp.GetFile()) == 0 || resp.GetFilename() == "" {
		t.Fatalf("empty export: %+v", resp)
	}
}
