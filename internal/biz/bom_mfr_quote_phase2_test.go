package biz

import (
	"errors"
	"testing"
)

func TestQuoteItemEligibleForPhase2ReviewList(t *testing.T) {
	demand := "MFR_TI"
	other := "MFR_ST"
	ptr := func(s string) *string { return &s }
	tests := []struct {
		name       string
		demand     string
		quoteMfr   string
		quoteItem  *string
		resCanon   string
		resHit     bool
		wantEligib bool
	}{
		{name: "empty_quote_mfr", demand: demand, quoteMfr: "", quoteItem: nil, resCanon: "", resHit: false, wantEligib: false},
		{name: "quote_item_canon_set", demand: demand, quoteMfr: "TI", quoteItem: ptr("X"), resCanon: demand, resHit: true, wantEligib: false},
		{name: "alias_matches_demand_excluded", demand: demand, quoteMfr: "TI", quoteItem: nil, resCanon: demand, resHit: true, wantEligib: false},
		{name: "alias_miss_included", demand: demand, quoteMfr: "TI", quoteItem: nil, resCanon: "", resHit: false, wantEligib: true},
		{name: "alias_hit_mismatch_included", demand: demand, quoteMfr: "TI", quoteItem: nil, resCanon: other, resHit: true, wantEligib: true},
		{name: "demand_empty", demand: "  ", quoteMfr: "TI", quoteItem: nil, resCanon: "", resHit: false, wantEligib: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := QuoteItemEligibleForPhase2ReviewList(tt.demand, tt.quoteMfr, tt.quoteItem, tt.resCanon, tt.resHit)
			if got != tt.wantEligib {
				t.Fatalf("eligible=%v want %v", got, tt.wantEligib)
			}
		})
	}
}

func TestRequireParentManufacturerCanonicalForQuoteMfrReview(t *testing.T) {
	c := "MFR_TI"
	if err := RequireParentManufacturerCanonicalForQuoteMfrReview(&c); err != nil {
		t.Fatal(err)
	}
	if err := RequireParentManufacturerCanonicalForQuoteMfrReview(nil); !errors.Is(err, ErrParentLineMissingManufacturerCanonical) {
		t.Fatalf("nil parent: err=%v", err)
	}
	blank := ""
	if err := RequireParentManufacturerCanonicalForQuoteMfrReview(&blank); !errors.Is(err, ErrParentLineMissingManufacturerCanonical) {
		t.Fatalf("blank parent: err=%v", err)
	}
}
