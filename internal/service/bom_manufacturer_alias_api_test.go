package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	v1 "caichip/api/bom/v1"
	"caichip/internal/biz"

	khttp "github.com/go-kratos/kratos/v2/transport/http"
)

func TestListManufacturerCanonicalsRoutePrefersStaticPath(t *testing.T) {
	svc := &BomService{alias: manufacturerAliasRepoStub{}}
	srv := khttp.NewServer()
	v1.RegisterBomServiceHTTPServer(srv, svc)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/bom/manufacturer-canonicals?limit=500", nil)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s, want 200", rec.Code, rec.Body.String())
	}
}

func TestApproveManufacturerAliasCleaningBackfillsSession(t *testing.T) {
	cleaning := &manufacturerCleaningRepoStub{}
	svc := &BomService{
		alias:       manufacturerAliasRepoStub{},
		mfrCleaning: cleaning,
	}

	reply, err := svc.ApproveManufacturerAliasCleaning(context.Background(), &v1.ApproveManufacturerAliasCleaningRequest{
		SessionId:   "session-1",
		Alias:       "TI",
		CanonicalId: "MFR_TI",
		DisplayName: "Texas Instruments",
	})
	if err != nil {
		t.Fatalf("ApproveManufacturerAliasCleaning() error = %v", err)
	}
	if cleaning.backfillSessionID != "session-1" || cleaning.backfillAliasNorm != "TI" || cleaning.backfillCanonicalID != "MFR_TI" {
		t.Fatalf("backfill args = session %q alias %q canonical %q", cleaning.backfillSessionID, cleaning.backfillAliasNorm, cleaning.backfillCanonicalID)
	}
	if reply.GetSessionLineUpdated() != 2 || reply.GetQuoteItemUpdated() != 3 {
		t.Fatalf("reply = %+v, want line=2 quote=3", reply)
	}
}

func TestApplyKnownManufacturerAliasesToSession(t *testing.T) {
	cleaning := &manufacturerCleaningRepoStub{}
	svc := &BomService{mfrCleaning: cleaning}

	reply, err := svc.ApplyKnownManufacturerAliasesToSession(context.Background(), &v1.ApplyKnownManufacturerAliasesToSessionRequest{
		SessionId: "session-1",
	})
	if err != nil {
		t.Fatalf("ApplyKnownManufacturerAliasesToSession() error = %v", err)
	}
	if cleaning.applySessionID != "session-1" {
		t.Fatalf("apply session = %q, want session-1", cleaning.applySessionID)
	}
	if reply.GetSessionLineUpdated() != 5 || reply.GetQuoteItemUpdated() != 7 {
		t.Fatalf("reply = %+v, want line=5 quote=7", reply)
	}
}

type manufacturerCleaningRepoStub struct {
	backfillSessionID   string
	backfillAliasNorm   string
	backfillCanonicalID string
	applySessionID      string
}

func (s *manufacturerCleaningRepoStub) DBOk() bool { return true }

func (s *manufacturerCleaningRepoStub) BackfillSessionManufacturerCanonical(ctx context.Context, sessionID, aliasNorm, canonicalID string, overwrite bool) (biz.ManufacturerCleaningResult, error) {
	s.backfillSessionID = sessionID
	s.backfillAliasNorm = aliasNorm
	s.backfillCanonicalID = canonicalID
	return biz.ManufacturerCleaningResult{SessionLineUpdated: 2, QuoteItemUpdated: 3}, nil
}

func (s *manufacturerCleaningRepoStub) ApplyKnownAliasesToSession(ctx context.Context, sessionID string) (biz.ManufacturerCleaningResult, error) {
	s.applySessionID = sessionID
	return biz.ManufacturerCleaningResult{SessionLineUpdated: 5, QuoteItemUpdated: 7}, nil
}
