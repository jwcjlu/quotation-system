package service

import (
	"context"
	"sync"
	"testing"
	"time"

	v1 "caichip/api/bom/v1"
	"caichip/internal/biz"
	"caichip/internal/data"

	kerrors "github.com/go-kratos/kratos/v2/errors"
)

func TestUploadBOM_LLM_ReturnsAcceptedImmediately(t *testing.T) {
	session := &bomSessionRepoStub{
		sessionExists: true,
		view: &biz.BOMSessionView{
			SessionID:         "sid-1",
			BizDate:           time.Now(),
			PlatformIDs:       []string{"digikey"},
			SelectionRevision: 1,
		},
	}
	search := &bomSearchTaskRepoStub{}
	svc := NewBomService(session, search, nil, nil, nil, &data.OpenAIChat{}, nil, nil, nil, nil, nil, nil, nil, nil)

	resp, err := svc.UploadBOM(context.Background(), &v1.UploadBOMRequest{
		SessionId: "sid-1",
		ParseMode: "llm",
		File:      []byte("not-an-excel"),
	})
	if err != nil {
		t.Fatalf("UploadBOM llm should be accepted immediately: %v", err)
	}
	if !resp.GetAccepted() {
		t.Fatalf("accepted should be true")
	}
	if resp.GetImportStatus() != biz.BOMImportStatusParsing {
		t.Fatalf("unexpected import_status: %q", resp.GetImportStatus())
	}
	if resp.GetBomId() != "sid-1" {
		t.Fatalf("unexpected bom_id: %q", resp.GetBomId())
	}
	if resp.GetTotal() != 0 || len(resp.GetItems()) != 0 {
		t.Fatalf("async accepted response should not include parsed items")
	}
}

func TestUploadBOM_NonLLM_RemainsSync(t *testing.T) {
	session := &bomSessionRepoStub{
		sessionExists: true,
		view: &biz.BOMSessionView{
			SessionID:         "sid-2",
			BizDate:           time.Now(),
			PlatformIDs:       []string{"digikey"},
			SelectionRevision: 1,
		},
	}
	search := &bomSearchTaskRepoStub{}
	svc := NewBomService(session, search, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)

	resp, err := svc.UploadBOM(context.Background(), &v1.UploadBOMRequest{
		SessionId: "sid-2",
		File:      makeSimpleBOMExcel(t),
	})
	if err != nil {
		t.Fatalf("UploadBOM non-llm should succeed synchronously: %v", err)
	}
	if !resp.GetAccepted() {
		t.Fatalf("accepted should be true")
	}
	if resp.GetImportStatus() != biz.BOMImportStatusReady {
		t.Fatalf("unexpected import_status: %q", resp.GetImportStatus())
	}
	if resp.GetTotal() == 0 || len(resp.GetItems()) == 0 {
		t.Fatalf("sync non-llm should return parsed items")
	}
	session.mu.Lock()
	replaced := session.replaced
	session.mu.Unlock()
	if !replaced {
		t.Fatalf("ReplaceSessionLines should be called in non-llm sync flow")
	}
	search.mu.Lock()
	upsert := search.upsertTasks
	search.mu.Unlock()
	if !upsert {
		t.Fatalf("UpsertPendingTasks should be called in non-llm sync flow")
	}
}

func TestUploadBOM_LLM_RejectWhenImportParsing(t *testing.T) {
	session := &bomSessionRepoStub{
		sessionExists: true,
		view: &biz.BOMSessionView{
			SessionID:         "sid-3",
			BizDate:           time.Now(),
			PlatformIDs:       []string{"digikey"},
			SelectionRevision: 1,
			ImportStatus:      biz.BOMImportStatusParsing,
		},
	}
	search := &bomSearchTaskRepoStub{}
	svc := NewBomService(session, search, nil, nil, nil, &data.OpenAIChat{}, nil, nil, nil, nil, nil, nil, nil, nil)

	_, err := svc.UploadBOM(context.Background(), &v1.UploadBOMRequest{
		SessionId: "sid-3",
		ParseMode: "llm",
		File:      []byte("x"),
	})
	if err == nil {
		t.Fatalf("expected conflict when llm import already parsing")
	}
	se := kerrors.FromError(err)
	if se == nil || se.Code != 409 || se.Reason != "BOM_IMPORT_IN_PROGRESS" {
		t.Fatalf("unexpected error: %+v", se)
	}
	session.mu.Lock()
	patchCount := len(session.patches)
	replaced := session.replaced
	tryStartCalls := session.tryStartCalls
	tryStartSuccesses := session.tryStartSuccesses
	session.mu.Unlock()
	if patchCount != 0 {
		t.Fatalf("should not update import state when rejected, got patches=%d", patchCount)
	}
	if replaced {
		t.Fatalf("should not start import flow when rejected")
	}
	if tryStartCalls != 1 || tryStartSuccesses != 0 {
		t.Fatalf("unexpected try start stats: calls=%d successes=%d", tryStartCalls, tryStartSuccesses)
	}
}

func TestUploadBOM_LLM_ConcurrentOnlyOneAccepted(t *testing.T) {
	session := &bomSessionRepoStub{
		sessionExists: true,
		view: &biz.BOMSessionView{
			SessionID:         "sid-4",
			BizDate:           time.Now(),
			PlatformIDs:       []string{"digikey"},
			SelectionRevision: 1,
		},
	}
	search := &bomSearchTaskRepoStub{}
	svc := NewBomService(session, search, nil, nil, nil, &data.OpenAIChat{}, nil, nil, nil, nil, nil, nil, nil, nil)

	start := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(2)
	type callRes struct {
		resp *v1.UploadBOMReply
		err  error
	}
	out := make(chan callRes, 2)
	call := func() {
		defer wg.Done()
		<-start
		resp, err := svc.UploadBOM(context.Background(), &v1.UploadBOMRequest{
			SessionId: "sid-4",
			ParseMode: "llm",
			File:      []byte("not-an-excel"),
		})
		out <- callRes{resp: resp, err: err}
	}
	go call()
	go call()
	close(start)
	wg.Wait()
	close(out)

	accepted := 0
	conflicts := 0
	for r := range out {
		if r.err == nil && r.resp != nil && r.resp.GetAccepted() {
			accepted++
			continue
		}
		if se := kerrors.FromError(r.err); se != nil && se.Code == 409 && se.Reason == "BOM_IMPORT_IN_PROGRESS" {
			conflicts++
			continue
		}
		t.Fatalf("unexpected concurrent result: resp=%+v err=%v", r.resp, r.err)
	}
	if accepted != 1 || conflicts != 1 {
		t.Fatalf("expected one accepted and one conflict, got accepted=%d conflicts=%d", accepted, conflicts)
	}
}

func TestUploadBOM_NotFoundWhenSessionMissing(t *testing.T) {
	session := &bomSessionRepoStub{sessionExists: false}
	search := &bomSearchTaskRepoStub{}
	svc := NewBomService(session, search, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)

	_, err := svc.UploadBOM(context.Background(), &v1.UploadBOMRequest{
		SessionId: "missing-session",
		File:      makeSimpleBOMExcel(t),
	})
	if err == nil {
		t.Fatalf("expected not found when session missing")
	}
	se := kerrors.FromError(err)
	if se == nil || se.Code != 404 || se.Reason != "SESSION_NOT_FOUND" {
		t.Fatalf("unexpected error: %+v", se)
	}
}

func TestUploadBOM_NonLLM_ConcurrentOnlyOneAccepted(t *testing.T) {
	session := &bomSessionRepoStub{
		sessionExists: true,
		view: &biz.BOMSessionView{
			SessionID:         "sid-5",
			BizDate:           time.Now(),
			PlatformIDs:       []string{"digikey"},
			SelectionRevision: 1,
		},
	}
	search := &bomSearchTaskRepoStub{}
	svc := NewBomService(session, search, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)

	start := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(2)
	type callRes struct {
		resp *v1.UploadBOMReply
		err  error
	}
	out := make(chan callRes, 2)
	call := func() {
		defer wg.Done()
		<-start
		resp, err := svc.UploadBOM(context.Background(), &v1.UploadBOMRequest{
			SessionId: "sid-5",
			File:      makeSimpleBOMExcel(t),
		})
		out <- callRes{resp: resp, err: err}
	}
	go call()
	go call()
	close(start)
	wg.Wait()
	close(out)

	accepted := 0
	conflicts := 0
	for r := range out {
		if r.err == nil && r.resp != nil && r.resp.GetAccepted() {
			accepted++
			continue
		}
		if se := kerrors.FromError(r.err); se != nil && se.Code == 409 && se.Reason == "BOM_IMPORT_IN_PROGRESS" {
			conflicts++
			continue
		}
		t.Fatalf("unexpected concurrent non-llm result: resp=%+v err=%v", r.resp, r.err)
	}
	if accepted != 1 || conflicts != 1 {
		t.Fatalf("expected one accepted and one conflict, got accepted=%d conflicts=%d", accepted, conflicts)
	}
}
