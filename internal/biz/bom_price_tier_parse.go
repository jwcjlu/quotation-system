package biz

import (
	"regexp"
	"strconv"
	"strings"
)

// PriceTier is one parsed segment from a price_tiers string (§1.11).
type PriceTier struct {
	Moq   int
	Price float64
	Ccy   string
}

var priceTierSegmentRe = regexp.MustCompile(`^\s*(\d+)\+\s*([$￥¥])\s*(\d+(?:\.\d+)?)\s*$`)

// ParsePriceTiers splits s by '|', trims each segment, and parses every segment per §1.11.1.
// If any segment is empty (after trim) or does not match the pattern, ok is false — no partial tiers.
func ParsePriceTiers(s string) (tiers []PriceTier, ok bool) {
	parts := strings.Split(s, "|")
	if len(parts) == 0 {
		return nil, false
	}
	tiers = make([]PriceTier, 0, len(parts))
	for _, raw := range parts {
		seg := strings.TrimSpace(raw)
		if seg == "" {
			return nil, false
		}
		sub := priceTierSegmentRe.FindStringSubmatch(seg)
		if sub == nil {
			return nil, false
		}
		moq, err := strconv.Atoi(sub[1])
		if err != nil || moq <= 0 {
			return nil, false
		}
		price, err := strconv.ParseFloat(sub[3], 64)
		if err != nil {
			return nil, false
		}
		var ccy string
		switch sub[2] {
		case "$":
			ccy = "USD"
		case "￥", "¥":
			ccy = "CNY"
		default:
			return nil, false
		}
		tiers = append(tiers, PriceTier{Moq: moq, Price: price, Ccy: ccy})
	}
	return tiers, true
}

// PickCompareUnitPriceFromPriceTiers parses s and selects the compare unit price for demand quantity q per §1.9 / §1.11.2:
// among tiers with moq <= q, the tier with the largest moq; if none, ok is false.
// If multiple tiers tie on that max moq but disagree on price or currency, ok is false (§1.9).
func PickCompareUnitPriceFromPriceTiers(s string, q int) (price float64, ccy string, ok bool) {
	if q <= 0 {
		return 0, "", false
	}
	tiers, ok := ParsePriceTiers(s)
	if !ok || len(tiers) == 0 {
		return 0, "", false
	}
	var bestMoq int
	var found bool
	for _, t := range tiers {
		if t.Moq <= q && (!found || t.Moq > bestMoq) {
			bestMoq = t.Moq
			found = true
		}
	}
	if !found {
		return 0, "", false
	}
	var p float64
	var c string
	first := true
	for _, t := range tiers {
		if t.Moq != bestMoq {
			continue
		}
		if first {
			p, c = t.Price, t.Ccy
			first = false
			continue
		}
		if t.Price != p || t.Ccy != c {
			return 0, "", false
		}
	}
	return p, c, true
}
