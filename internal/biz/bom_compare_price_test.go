package biz

import (
	"testing"
)

func TestExtractCompareUnitPriceComparePrice_unitPriceWins(t *testing.T) {
	tiers := `1+ ￥14.0729 | 10+ ￥12.7522 | 100+ ￥10.2928`
	in := QuotePriceInput{
		UnitPrice:     99.99,
		PriceTiers:    tiers,
		MainlandPrice: `1+ ￥1.0`,
	}
	r := ExtractCompareUnitPrice(in, "ickey", 100, true, "")
	if !r.Ok {
		t.Fatal("expected Ok")
	}
	if r.Source != ComparePriceSourceUnitPrice {
		t.Fatalf("source: got %q want unit_price", r.Source)
	}
	if r.Price != 99.99 || r.Ccy != "CNY" {
		t.Fatalf("price/ccy: got %v %q", r.Price, r.Ccy)
	}
}

func TestExtractCompareUnitPriceComparePrice_defaultCcyOverride(t *testing.T) {
	in := QuotePriceInput{UnitPrice: 1.5}
	r := ExtractCompareUnitPrice(in, "unknown_platform", 10, true, "USD")
	if !r.Ok || r.Ccy != "USD" || r.Source != ComparePriceSourceUnitPrice {
		t.Fatalf("got %+v", r)
	}
}

func TestExtractCompareUnitPriceComparePrice_tierOnly(t *testing.T) {
	tiers := `1+ ￥14.0729 | 10+ ￥12.7522 | 100+ ￥10.2928 | 300+ ￥9.7281`
	in := QuotePriceInput{PriceTiers: tiers}
	r := ExtractCompareUnitPrice(in, "ickey", 100, true, "")
	if !r.Ok {
		t.Fatal("expected Ok")
	}
	if r.Source != ComparePriceSourcePriceTiersParsed {
		t.Fatalf("source: got %q", r.Source)
	}
	if r.Price != 10.2928 || r.Ccy != "CNY" {
		t.Fatalf("got price %v ccy %q", r.Price, r.Ccy)
	}
}

func TestExtractCompareUnitPriceComparePrice_parseTierStringsSkipsTiers(t *testing.T) {
	tiers := `1+ ￥14.0729 | 100+ ￥10.2928`
	in := QuotePriceInput{PriceTiers: tiers}
	r := ExtractCompareUnitPrice(in, "ickey", 100, false, "")
	if r.Ok {
		t.Fatalf("expected not Ok, got %+v", r)
	}
}

func TestExtractCompareUnitPriceComparePrice_moqReject(t *testing.T) {
	in := QuotePriceInput{
		UnitPrice: 1.0,
		Moq:       500,
	}
	r := ExtractCompareUnitPrice(in, "ickey", 100, true, "")
	if r.Ok {
		t.Fatalf("expected MOQ reject, got %+v", r)
	}
}

func TestExtractCompareUnitPriceComparePrice_stockReject(t *testing.T) {
	in := QuotePriceInput{
		UnitPrice: 1.0,
		Stock:     "库存 50 PCS",
	}
	r := ExtractCompareUnitPrice(in, "ickey", 100, true, "")
	if r.Ok {
		t.Fatalf("expected stock reject, got %+v", r)
	}
}

func TestExtractCompareUnitPriceComparePrice_stockUnparseableSkipsCheck(t *testing.T) {
	tiers := `100+ ￥10.2928`
	in := QuotePriceInput{
		PriceTiers: tiers,
		Stock:      "充足",
	}
	r := ExtractCompareUnitPrice(in, "ickey", 100, true, "")
	if !r.Ok || r.Price != 10.2928 {
		t.Fatalf("expected tier ok, got %+v", r)
	}
}

func TestExtractCompareUnitPriceComparePrice_mainlandBeforeHk(t *testing.T) {
	in := QuotePriceInput{
		MainlandPrice: `100+ ￥2.0`,
		HkPrice:       `100+ ￥9.0`,
	}
	r := ExtractCompareUnitPrice(in, "ickey", 100, true, "")
	if !r.Ok || r.Source != ComparePriceSourceMainlandPrice || r.Price != 2.0 {
		t.Fatalf("got %+v", r)
	}
}

func TestExtractCompareUnitPriceComparePrice_defaultQuoteCCYHelper(t *testing.T) {
	if DefaultQuoteCCY("find_chips") != "USD" {
		t.Fatal()
	}
	if DefaultQuoteCCY("ickey") != "CNY" {
		t.Fatal()
	}
	if DefaultQuoteCCY("weird") != "" {
		t.Fatal()
	}
}
