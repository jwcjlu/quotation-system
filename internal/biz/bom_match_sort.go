package biz

import (
	"fmt"
	"math"
	"strconv"
)

// MatchLeadDaysUnknown is used in MatchSortKey when lead time cannot be parsed (+∞ semantics, §1.10).
const MatchLeadDaysUnknown = 1 << 30

// MatchSortKey is the deterministic tie-break ordering for BOM match candidates after unit_price_base quantization (§1.10).
type MatchSortKey struct {
	UnitPriceBaseQuantized int64
	LeadDays               int
	StockParsed            int64
	PlatformID             string
}

// QuantizeUnitPriceBase converts unit_price_base to an int64 sort key.
//   - "minor_unit": round(unitPriceBase * 1e6) — fixed-scale minor representation (V1 code path; config may later map to true 分 *100).
//   - "decimal6": format to 6 decimal places, then micro-units int64 (distinct from direct float rounding in edge cases).
func QuantizeUnitPriceBase(mode string, unitPriceBase float64) int64 {
	switch mode {
	case "minor_unit":
		return int64(math.Round(unitPriceBase * 1e6))
	case "decimal6":
		s := fmt.Sprintf("%.6f", unitPriceBase)
		v, err := strconv.ParseFloat(s, 64)
		if err != nil {
			return 0
		}
		return int64(math.Round(v * 1e6))
	default:
		return int64(math.Round(unitPriceBase * 1e6))
	}
}

// LessMatchCandidate reports whether a should rank before b (a is better for auto-selection).
// Order: lower UnitPriceBaseQuantized; tie → lower LeadDays; tie → higher StockParsed; tie → lower PlatformID lexicographic.
func LessMatchCandidate(a, b MatchSortKey) bool {
	if a.UnitPriceBaseQuantized != b.UnitPriceBaseQuantized {
		return a.UnitPriceBaseQuantized < b.UnitPriceBaseQuantized
	}
	if a.LeadDays != b.LeadDays {
		return a.LeadDays < b.LeadDays
	}
	if a.StockParsed != b.StockParsed {
		return a.StockParsed > b.StockParsed
	}
	return a.PlatformID < b.PlatformID
}
