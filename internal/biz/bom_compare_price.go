package biz

import (
	"strconv"
	"strings"
)

// QuotePriceInput is a minimal quote snapshot for compare-time price extraction (§1.7 / §1.9),
// kept free of proto imports for easy unit tests.
type QuotePriceInput struct {
	UnitPrice     float64
	MainlandPrice string
	HkPrice       string
	PriceTiers    string
	Moq           int    // 0 = unknown / not set
	Stock         string // optional; digits-only parse for threshold check
}

// ComparePriceSource records which §1.7 step produced compare_unit_price.
type ComparePriceSource string

const (
	ComparePriceSourceUnitPrice        ComparePriceSource = "unit_price"
	ComparePriceSourceMainlandPrice    ComparePriceSource = "mainland_price"
	ComparePriceSourceHkPrice          ComparePriceSource = "hk_price"
	ComparePriceSourcePriceTiersParsed ComparePriceSource = "price_tiers_parsed"
)

// ComparePriceResult is the outcome of ExtractCompareUnitPrice.
type ComparePriceResult struct {
	Price  float64
	Ccy    string
	Source ComparePriceSource
	Ok     bool
}

// DefaultQuoteCCY returns a platform default quote currency when structured unit_price has no embedded ccy (V1 stub).
// Unknown platform returns "" so callers must pass defaultCcyWhenUnitPrice or another path.
func DefaultQuoteCCY(platformID string) string {
	id := NormalizePlatformID(platformID)
	switch id {
	case "find_chips":
		return "USD"
	case "hqchip", "ickey", "szlcsc":
		return "CNY"
	default:
		return ""
	}
}

// ParseCompareStock extracts a non-negative integer from s by keeping ASCII digits only.
// If there are no digits or the value overflows int64 parsing, parsed is false (unknown stock — skip check).
func ParseCompareStock(s string) (qty int64, parsed bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, false
	}
	var b strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	if b.Len() == 0 {
		return 0, false
	}
	v, err := strconv.ParseInt(b.String(), 10, 64)
	if err != nil {
		return 0, false
	}
	return v, true
}

// ExtractCompareUnitPrice applies §1.7 priority and §1.9 MOQ / stock gates (no FX — Task 5).
// bomQty is BOM line quantity Q; must be > 0 for tier selection.
// If parseTierStrings is false, the price_tiers string step is skipped (§1.5 switch-off).
func ExtractCompareUnitPrice(in QuotePriceInput, platformID string, bomQty int, parseTierStrings bool, defaultCcyWhenUnitPrice string) ComparePriceResult {
	if bomQty <= 0 {
		return ComparePriceResult{}
	}
	if in.Moq > 0 && in.Moq > bomQty {
		return ComparePriceResult{}
	}
	if stock, ok := ParseCompareStock(in.Stock); ok && stock < int64(bomQty) {
		return ComparePriceResult{}
	}

	if in.UnitPrice > 0 {
		ccy := strings.TrimSpace(defaultCcyWhenUnitPrice)
		if ccy == "" {
			ccy = DefaultQuoteCCY(platformID)
		}
		if ccy != "" {
			return ComparePriceResult{
				Price:  in.UnitPrice,
				Ccy:    ccy,
				Source: ComparePriceSourceUnitPrice,
				Ok:     true,
			}
		}
	}

	if p, ccy, ok := PickCompareUnitPriceFromPriceTiers(strings.TrimSpace(in.MainlandPrice), bomQty); ok {
		return ComparePriceResult{Price: p, Ccy: ccy, Source: ComparePriceSourceMainlandPrice, Ok: true}
	}
	if p, ccy, ok := PickCompareUnitPriceFromPriceTiers(strings.TrimSpace(in.HkPrice), bomQty); ok {
		return ComparePriceResult{Price: p, Ccy: ccy, Source: ComparePriceSourceHkPrice, Ok: true}
	}
	if parseTierStrings {
		if p, ccy, ok := PickCompareUnitPriceFromPriceTiers(strings.TrimSpace(in.PriceTiers), bomQty); ok {
			return ComparePriceResult{Price: p, Ccy: ccy, Source: ComparePriceSourcePriceTiersParsed, Ok: true}
		}
	}

	return ComparePriceResult{}
}
