package biz

import (
	"context"
	"testing"
	"time"
)

func TestParseQuotesJSON(t *testing.T) {
	t.Parallel()
	raw := []byte(`[{"matched_model":"ABC","manufacturer":"X","unit_price":1.2,"stock":10}]`)
	qs := ParseQuotesJSON(raw, "ickey")
	if len(qs) != 1 {
		t.Fatalf("len=%d", len(qs))
	}
	if qs[0].Platform != "ickey" || qs[0].MatchedModel != "ABC" {
		t.Fatalf("got %+v", qs[0])
	}
}

func TestLoadItemQuotesForSession_groupsByModel(t *testing.T) {
	t.Parallel()
	src := fakeSrc{
		rows: []SessionQuoteRawRow{
			{MpnNorm: "ABC", PlatformID: "p1", QuotesJSON: []byte(`[{"matched_model":"ABC","manufacturer":"M1","unit_price":1}]`)},
			{MpnNorm: "ABC", PlatformID: "p2", QuotesJSON: []byte(`[{"matched_model":"ABC","manufacturer":"M2","unit_price":2}]`)},
		},
	}
	bom := &BOM{
		ID: "sess-1",
		Items: []*BOMItem{
			{Model: "abc", Quantity: 3},
			{Model: "other", Quantity: 1},
		},
	}
	iq, err := LoadItemQuotesForSession(t.Context(), src, bom, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if len(iq) != 1 {
		t.Fatalf("len=%d", len(iq))
	}
	if len(iq[0].Quotes) != 2 {
		t.Fatalf("quotes len=%d", len(iq[0].Quotes))
	}
	if iq[0].Model != "abc" {
		t.Fatalf("model %q", iq[0].Model)
	}
}

type fakeSrc struct {
	rows []SessionQuoteRawRow
}

func (f fakeSrc) LoadSucceededQuoteRowsForSession(ctx context.Context, sessionID string, bizDate time.Time) ([]SessionQuoteRawRow, error) {
	_, _, _ = ctx, sessionID, bizDate
	return f.rows, nil
}
