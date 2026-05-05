package service

import (
	"testing"

	"caichip/internal/biz"
)

func TestMergeQuoteRowsWithSessionLineReads_FillsStatusAndCanon(t *testing.T) {
	row := biz.AgentQuoteRow{Model: "LM358", Manufacturer: "TI", Package: "SOP-8"}
	db := []biz.BomQuoteItemReadRow{{
		PlatformID: "find_chips", ItemID: 9,
		Model: "lm358", Manufacturer: "ti", Package: "SOP-8",
		ManufacturerReviewStatus: biz.MfrReviewAccepted,
		ManufacturerCanonicalID:  "MFR_TI",
	}}
	out := mergeQuoteRowsWithSessionLineReads([]biz.AgentQuoteRow{row}, db, "find_chips")
	if len(out) != 1 {
		t.Fatalf("len=%d", len(out))
	}
	if out[0].ManufacturerReviewStatus != biz.MfrReviewAccepted {
		t.Fatalf("status=%q", out[0].ManufacturerReviewStatus)
	}
	if out[0].ManufacturerCanonicalID == nil || *out[0].ManufacturerCanonicalID != "MFR_TI" {
		t.Fatalf("canon=%v", out[0].ManufacturerCanonicalID)
	}
}
