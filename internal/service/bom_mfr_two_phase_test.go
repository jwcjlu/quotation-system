package service

import (
	"context"
	"strings"
	"testing"

	v1 "caichip/api/bom/v1"
	"caichip/internal/data"

	kerrors "github.com/go-kratos/kratos/v2/errors"
)

func TestBomService_ListSessionLineMfrCandidates_DBDisabled(t *testing.T) {
	s := &BomService{}
	_, err := s.ListSessionLineMfrCandidates(context.Background(), &v1.ListSessionLineMfrCandidatesRequest{SessionId: "s1"})
	if err == nil {
		t.Fatal("expected error")
	}
	if !kerrors.IsServiceUnavailable(err) {
		t.Fatalf("expected service unavailable, got %v", err)
	}
}

func TestMergeMpnKeysForBOMLine_mainAndSubstitute(t *testing.T) {
	sub := "  alt-mpn  "
	ln := data.BomSessionLine{Mpn: "MAIN", SubstituteMpn: &sub}
	keys := mergeMpnKeysForBOMLine(ln)
	if len(keys) != 2 {
		t.Fatalf("expected 2 merge keys, got %v", keys)
	}
}

func TestMergeMpnKeysForBOMLine_sameSubstituteDeduped(t *testing.T) {
	sub := "main"
	ln := data.BomSessionLine{Mpn: "MAIN", SubstituteMpn: &sub}
	keys := mergeMpnKeysForBOMLine(ln)
	if len(keys) != 1 {
		t.Fatalf("expected 1 merge key, got %v", keys)
	}
}

func TestBomService_ListQuoteItemMfrReviews_DBDisabled(t *testing.T) {
	s := &BomService{}
	_, err := s.ListQuoteItemMfrReviews(context.Background(), &v1.ListQuoteItemMfrReviewsRequest{SessionId: "s1"})
	if err == nil {
		t.Fatal("expected error")
	}
	if !kerrors.IsServiceUnavailable(err) {
		t.Fatalf("expected service unavailable, got %v", err)
	}
}

func TestBomService_SubmitQuoteItemMfrReview_DBDisabled(t *testing.T) {
	s := &BomService{}
	_, err := s.SubmitQuoteItemMfrReview(context.Background(), &v1.SubmitQuoteItemMfrReviewRequest{
		SessionId:    "s1",
		QuoteItemId:  1,
		Decision:     "accept",
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if !kerrors.IsServiceUnavailable(err) {
		t.Fatalf("expected service unavailable, got %v", err)
	}
}

func TestApproveSessionLineMfrCleaningUsesBackfillStub(t *testing.T) {
	cleaning := &manufacturerCleaningRepoStub{}
	svc := &BomService{alias: manufacturerAliasRepoStub{}, mfrCleaning: cleaning}
	_, err := svc.ApproveSessionLineMfrCleaning(context.Background(), &v1.ApproveSessionLineMfrCleaningRequest{
		SessionId:   "session-1",
		Alias:       "TI",
		CanonicalId: "MFR_TI",
		DisplayName: "Texas Instruments",
	})
	if err != nil {
		t.Fatal(err)
	}
	if cleaning.backfillSessionID != "session-1" || !strings.Contains(cleaning.backfillAliasNorm, "TI") {
		t.Fatalf("backfill args session=%q alias=%q", cleaning.backfillSessionID, cleaning.backfillAliasNorm)
	}
}
