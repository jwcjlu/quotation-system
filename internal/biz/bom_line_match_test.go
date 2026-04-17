package biz

import (
	"context"
	"strings"
	"testing"
	"time"
)

func mustRows(t *testing.T, s string) []AgentQuoteRow {
	t.Helper()
	rows, ok := ParseTaskStdoutQuoteRows(s)
	if !ok {
		t.Fatalf("ParseTaskStdoutQuoteRows failed: %s", s)
	}
	return rows
}

type stringAliasMap map[string]string

func (m stringAliasMap) CanonicalID(ctx context.Context, aliasNorm string) (canonicalID string, ok bool, err error) {
	if m == nil {
		return "", false, nil
	}
	c, ok := m[aliasNorm]
	return c, ok, nil
}

type fakeFX struct {
	USDToCNY float64
}

func (f fakeFX) Rate(ctx context.Context, from, to string, date time.Time) (rate float64, tableVersion, source string, ok bool, err error) {
	fr := strings.ToUpper(strings.TrimSpace(from))
	tr := strings.ToUpper(strings.TrimSpace(to))
	if fr == "USD" && tr == "CNY" && f.USDToCNY > 0 {
		return f.USDToCNY, "test", "fake", true, nil
	}
	if fr == "CNY" && tr == "USD" && f.USDToCNY > 0 {
		return 1 / f.USDToCNY, "test", "fake", true, nil
	}
	return 0, "", "", false, nil
}

func TestLineMatch_TwoPricesFXCheaper(t *testing.T) {
	// Two USD tier quotes; base CNY @ 7 — pick lower USD (8 vs 10).
	quotes := `[
  {"seq":1,"model":"LM358","manufacturer":"TI","package":"SOP-8","desc":"","stock":"500","moq":"1","price_tiers":"1+ $10.0000","hk_price":"","mainland_price":"","lead_time":"5天"},
  {"seq":2,"model":"LM358","manufacturer":"TI","package":"SOP-8","desc":"","stock":"500","moq":"1","price_tiers":"1+ $8.0000","hk_price":"","mainland_price":"","lead_time":"5天"}
]`
	alias := stringAliasMap{
		"TI": "MFR_TI",
	}
	ctx := context.Background()
	in := LineMatchInput{
		BomMpn:           "lm358",
		BomPackage:       "SOP-8",
		BomMfr:           "TI",
		BomQty:           50,
		PlatformID:       "find_chips",
		QuoteRows:        mustRows(t, quotes),
		BizDate:          time.Date(2026, 3, 28, 0, 0, 0, 0, time.UTC),
		RequestDay:       time.Date(2026, 3, 28, 0, 0, 0, 0, time.UTC),
		BaseCCY:          "CNY",
		RoundingMode:     "decimal6",
		ParseTierStrings: true,
	}
	pick, err := PickBestQuoteForLine(ctx, in, fakeFX{USDToCNY: 7}, alias)
	if err != nil {
		t.Fatal(err)
	}
	if !pick.Ok {
		t.Fatalf("expected Ok, reason=%q", pick.Reason)
	}
	if pick.RowIndex != 1 {
		t.Fatalf("want row 1 (cheaper after FX), got index %d price_base=%v orig=%v %s", pick.RowIndex, pick.UnitPriceBase, pick.OriginalPrice, pick.OriginalCCY)
	}
	if pick.OriginalPrice != 8 || pick.OriginalCCY != "USD" {
		t.Fatalf("original: got %v %q", pick.OriginalPrice, pick.OriginalCCY)
	}
	wantBase := 56.0
	if pick.UnitPriceBase < wantBase-1e-6 || pick.UnitPriceBase > wantBase+1e-6 {
		t.Fatalf("unit_price_base want ~%v got %v", wantBase, pick.UnitPriceBase)
	}
	if pick.ComparePriceSource != ComparePriceSourcePriceTiersParsed {
		t.Fatalf("source: %q", pick.ComparePriceSource)
	}
}

func TestLineMatch_BomMfrHintMatchesResolve(t *testing.T) {
	quotes := `[{"seq":1,"model":"LM358","manufacturer":"TI","package":"SOP-8","desc":"","stock":"500","moq":"1","price_tiers":"1+ $10.0000","hk_price":"","mainland_price":"","lead_time":"5天"}]`
	alias := stringAliasMap{"TI": "MFR_TI"}
	ctx := context.Background()
	base := LineMatchInput{
		BomMpn:           "lm358",
		BomPackage:       "SOP-8",
		BomMfr:           "TI",
		BomQty:           50,
		PlatformID:       "find_chips",
		QuoteRows:        mustRows(t, quotes),
		BizDate:          time.Date(2026, 3, 28, 0, 0, 0, 0, time.UTC),
		RequestDay:       time.Date(2026, 3, 28, 0, 0, 0, 0, time.UTC),
		BaseCCY:          "CNY",
		RoundingMode:     "decimal6",
		ParseTierStrings: true,
	}
	p1, err := PickBestQuoteForLine(ctx, base, fakeFX{USDToCNY: 7}, alias)
	if err != nil {
		t.Fatal(err)
	}
	in2 := base
	in2.BomMfrHint = &BomManufacturerResolveHint{Hit: true, CanonID: "MFR_TI"}
	p2, err := PickBestQuoteForLine(ctx, in2, fakeFX{USDToCNY: 7}, alias)
	if err != nil {
		t.Fatal(err)
	}
	if !p1.Ok || !p2.Ok {
		t.Fatalf("ok p1=%v p2=%v r1=%q r2=%q", p1.Ok, p2.Ok, p1.Reason, p2.Reason)
	}
	if p1.RowIndex != p2.RowIndex || p1.UnitPriceBase != p2.UnitPriceBase {
		t.Fatalf("pick mismatch p1=%+v p2=%+v", p1, p2)
	}
}

func TestLineMatch_MfrMismatchCollected(t *testing.T) {
	// BOM 要 TI；同型号封装有一条 ON Semi，应记入 MfrMismatchQuoteManufacturers，仍能从 TI 行配单成功。
	quotes := `[
  {"seq":1,"model":"LM358","manufacturer":"ON Semi","package":"SOP-8","desc":"","stock":"500","moq":"1","price_tiers":"1+ ￥1.0000","hk_price":"","mainland_price":"","lead_time":"5天"},
  {"seq":2,"model":"LM358","manufacturer":"TI","package":"SOP-8","desc":"","stock":"500","moq":"1","price_tiers":"1+ ￥2.0000","hk_price":"","mainland_price":"","lead_time":"5天"}
]`
	alias := stringAliasMap{
		"TI":      "MFR_TI",
		"ON SEMI": "MFR_ON",
	}
	ctx := context.Background()
	in := LineMatchInput{
		BomMpn:           "lm358",
		BomPackage:       "SOP-8",
		BomMfr:           "TI",
		BomQty:           1,
		PlatformID:       "ickey",
		QuoteRows:        mustRows(t, quotes),
		BizDate:          time.Date(2026, 3, 28, 0, 0, 0, 0, time.UTC),
		RequestDay:       time.Date(2026, 3, 28, 0, 0, 0, 0, time.UTC),
		BaseCCY:          "CNY",
		RoundingMode:     "decimal6",
		ParseTierStrings: true,
	}
	pick, err := PickBestQuoteForLine(ctx, in, fakeFX{}, alias)
	if err != nil {
		t.Fatal(err)
	}
	if !pick.Ok {
		t.Fatalf("expected Ok, reason=%q", pick.Reason)
	}
	if len(pick.MfrMismatchQuoteManufacturers) != 1 || pick.MfrMismatchQuoteManufacturers[0] != "ON Semi" {
		t.Fatalf("mfr mismatch: %+v", pick.MfrMismatchQuoteManufacturers)
	}
}

func TestLineMatch_TiePriceShorterLead(t *testing.T) {
	// Same CNY unit price after quantize; shorter lead wins (§1.10).
	quotes := `[
  {"seq":1,"model":"LM358","manufacturer":"TI","package":"SOP-8","desc":"","stock":"500","moq":"1","price_tiers":"1+ ￥10.000000","hk_price":"","mainland_price":"","lead_time":"7天"},
  {"seq":2,"model":"LM358","manufacturer":"TI","package":"SOP-8","desc":"","stock":"500","moq":"1","price_tiers":"1+ ￥10.000000","hk_price":"","mainland_price":"","lead_time":"3天"}
]`
	alias := stringAliasMap{"TI": "MFR_TI"}
	ctx := context.Background()
	in := LineMatchInput{
		BomMpn:           "LM358",
		BomPackage:       "SOP-8",
		BomMfr:           "TI",
		BomQty:           1,
		PlatformID:       "ickey",
		QuoteRows:        mustRows(t, quotes),
		BizDate:          time.Date(2026, 3, 28, 0, 0, 0, 0, time.UTC),
		RequestDay:       time.Date(2026, 3, 28, 0, 0, 0, 0, time.UTC),
		BaseCCY:          "CNY",
		RoundingMode:     "decimal6",
		ParseTierStrings: true,
	}
	pick, err := PickBestQuoteForLine(ctx, in, fakeFX{}, alias)
	if err != nil {
		t.Fatal(err)
	}
	if !pick.Ok {
		t.Fatalf("expected Ok, reason=%q", pick.Reason)
	}
	if pick.RowIndex != 1 {
		t.Fatalf("want row 1 (3天 lead), got %d lead=%q", pick.RowIndex, pick.Row.LeadTime)
	}
}

func TestLineMatch_Table(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name    string
		in      LineMatchInput
		fx      FXRateLookup
		alias   AliasLookup
		wantOk  bool
		wantIdx int
		reason  string
		err     bool
	}{
		{
			name: "no quote rows",
			in: LineMatchInput{
				BomMpn: "X", BomQty: 1, BaseCCY: "CNY",
				QuoteRows: nil,
			},
			wantOk: false,
			reason: lineMatchReasonNoQuotes,
		},
		{
			name: "bom mfr required alias miss",
			in: LineMatchInput{
				BomMpn: "X", BomMfr: "UNKNOWNBRAND", BomQty: 1, BaseCCY: "CNY",
				QuoteRows: []AgentQuoteRow{},
			},
			alias:  stringAliasMap{"TI": "MFR_TI"},
			wantOk: false,
			reason: lineMatchReasonBomManufacturerMiss,
		},
		{
			name: "programmer bom_qty",
			in: LineMatchInput{
				BomMpn: "X", BomQty: 0, BaseCCY: "CNY",
				QuoteRows: []AgentQuoteRow{},
			},
			err: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			pick, err := PickBestQuoteForLine(ctx, tc.in, tc.fx, tc.alias)
			if tc.err {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if pick.Ok != tc.wantOk {
				t.Fatalf("Ok=%v reason=%q", pick.Ok, pick.Reason)
			}
			if pick.Reason != tc.reason && tc.reason != "" {
				t.Fatalf("Reason=%q want %q", pick.Reason, tc.reason)
			}
			if tc.wantOk && pick.RowIndex != tc.wantIdx {
				t.Fatalf("RowIndex=%d want %d", pick.RowIndex, tc.wantIdx)
			}
		})
	}
}
