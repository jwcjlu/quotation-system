package biz

import (
	"math"
	"testing"
)

const ickeyPriceTiersSample = "1+ ￥14.0729 | 10+ ￥12.7522 | 30+ ￥10.5661 | 100+ ￥10.2928 | 300+ ￥9.7281 | 1000+ ￥9.1087"

const findChipsPriceTiersSample = "3000+ $4.1930 | 1000+ $4.4418 | 500+ $4.6114 | 250+ $4.7914 | 100+ $5.0464 | 25+ $5.4720 | 10+ $5.7820 | 1+ $6.6700"

const hqchipPriceTiersSample = "1+ ￥19.8235 | 10+ ￥14.6639 | 100+ ￥12.569 | 1000+ ￥10.4742"

func TestPriceTier_Ickey_Q100(t *testing.T) {
	p, ccy, ok := PickCompareUnitPriceFromPriceTiers(ickeyPriceTiersSample, 100)
	if !ok || ccy != "CNY" || math.Abs(p-10.2928) > 1e-6 {
		t.Fatalf("got %v %s %v", p, ccy, ok)
	}
}

func TestPriceTier_FindChips_Q500(t *testing.T) {
	p, ccy, ok := PickCompareUnitPriceFromPriceTiers(findChipsPriceTiersSample, 500)
	if !ok || ccy != "USD" || math.Abs(p-4.6114) > 1e-6 {
		t.Fatalf("got %v %s %v", p, ccy, ok)
	}
}

func TestPriceTier_Hqchip_Q50(t *testing.T) {
	// moq <= 50: 1+, 10+ → largest moq 10 → 14.6639 CNY
	p, ccy, ok := PickCompareUnitPriceFromPriceTiers(hqchipPriceTiersSample, 50)
	if !ok || ccy != "CNY" || math.Abs(p-14.6639) > 1e-6 {
		t.Fatalf("got %v %s %v", p, ccy, ok)
	}
}

func TestPriceTier_Hqchip_Q1000(t *testing.T) {
	p, ccy, ok := PickCompareUnitPriceFromPriceTiers(hqchipPriceTiersSample, 1000)
	if !ok || ccy != "CNY" || math.Abs(p-10.4742) > 1e-6 {
		t.Fatalf("got %v %s %v", p, ccy, ok)
	}
}

func TestPriceTier_InvalidSegmentFailsWholeString(t *testing.T) {
	s := ickeyPriceTiersSample + " | bad-segment"
	_, _, ok := PickCompareUnitPriceFromPriceTiers(s, 100)
	if ok {
		t.Fatal("expected ok=false for invalid trailing segment")
	}
	_, ok2 := ParsePriceTiers(s)
	if ok2 {
		t.Fatal("ParsePriceTiers expected ok=false")
	}
}

func TestPriceTier_QBelowAllMOQs(t *testing.T) {
	s := "10+ ￥1.00 | 20+ ￥0.50"
	_, _, ok := PickCompareUnitPriceFromPriceTiers(s, 5)
	if ok {
		t.Fatal("expected ok=false when Q is below every tier MOQ")
	}
}

func TestPriceTier_ParsePriceTiers_OrderIndependent(t *testing.T) {
	// shuffled order should still pick 100+ tier for Q=100
	shuffled := "100+ ￥10.2928 | 1+ ￥14.0729 | 300+ ￥9.7281"
	p, ccy, ok := PickCompareUnitPriceFromPriceTiers(shuffled, 100)
	if !ok || ccy != "CNY" || math.Abs(p-10.2928) > 1e-6 {
		t.Fatalf("got %v %s %v", p, ccy, ok)
	}
}

func TestPriceTier_YenHalfwidthSymbol(t *testing.T) {
	s := "1+ ¥2.5 | 10+ ¥1.0"
	p, ccy, ok := PickCompareUnitPriceFromPriceTiers(s, 5)
	if !ok || ccy != "CNY" || math.Abs(p-2.5) > 1e-9 {
		t.Fatalf("got %v %s %v", p, ccy, ok)
	}
}

func TestPriceTier_DuplicateMOQConflictingPrice(t *testing.T) {
	s := "100+ ￥10.00 | 100+ ￥11.00"
	_, _, ok := PickCompareUnitPriceFromPriceTiers(s, 100)
	if ok {
		t.Fatal("expected ok=false when same max MOQ ties with different prices")
	}
}
