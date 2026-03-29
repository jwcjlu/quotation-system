package biz

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"
)

// LineMatchInput drives single-line multi-platform quote pick from cached quotes_json (design §1–§3).
//
// Matching V1 (strict):
//   - quotes_json: JSON array of AgentQuoteRow (same shape as bom_quote_cache). Invalid JSON or empty array → no candidate.
//   - Model: NormalizeMPNForBOMSearch(bom_mpn) must equal NormalizeMPNForBOMSearch(quote.model); empty model rows are skipped.
//   - Package: if BomPackage is non-empty after TrimSpace, quote.package must match after NormalizeMfrString on both sides
//     (trim → NFKC → ASCII upper); empty BomPackage → no package constraint.
//   - Manufacturer: TrimSpace(BomMfr)=="" → no mfr filter (§2.5). Else: quote must have non-empty manufacturer (§2.6);
//     both sides resolved via ResolveManufacturerCanonical; canonical IDs must be equal. If BOM mfr non-empty but BOM side
//     misses alias table → entire line returns Ok=false (strict §2.3). Quote side miss → skip row.
//   - Params/desc: V1 not compared (no extra BOM fields on this struct).
type LineMatchInput struct {
	BomMpn          string
	BomPackage      string // empty = no package constraint
	BomMfr          string // empty = no manufacturer constraint (§2.5)
	BomQty          int
	PlatformID      string
	QuotesJSON      []byte
	BizDate         time.Time
	RequestDay      time.Time
	BaseCCY         string
	RoundingMode    string // QuantizeUnitPriceBase mode; see bom_match_sort
	ParseTierStrings bool
}

// LineMatchPick is the best quote for one BOM line on one platform cache row, after filters, price extract, FX, and sort (§1.10).
type LineMatchPick struct {
	RowIndex           int
	Row                AgentQuoteRow
	UnitPriceBase      float64
	OriginalPrice      float64
	OriginalCCY        string
	ComparePriceSource ComparePriceSource
	FxMeta             FXMeta
	Ok                 bool
	Reason             string // set when Ok is false
}

const (
	lineMatchReasonQuotesInvalid          = "quotes_json_invalid"
	lineMatchReasonNoQuotes               = "no_quotes"
	lineMatchReasonBomManufacturerMiss    = "bom_manufacturer_alias_miss"
	lineMatchReasonNoCandidate            = "no_matching_quote"
	lineMatchReasonNoComparePrice         = "no_compare_price_after_filters"
	lineMatchReasonFXUnavailable          = "fx_unavailable_all_candidates"
)

// PickBestQuoteForLine filters cached quotes, extracts compare price, converts to base_ccy, and picks the best row by MatchSortKey.
// Errors are reserved for programmer mistakes (e.g. BomQty<=0, empty BaseCCY). Business outcomes use Ok=false and Reason.
func PickBestQuoteForLine(ctx context.Context, in LineMatchInput, fx FXRateLookup, alias AliasLookup) (LineMatchPick, error) {
	if in.BomQty <= 0 {
		return LineMatchPick{}, errors.New("bom line match: bom_qty must be > 0")
	}
	if strings.TrimSpace(in.BaseCCY) == "" {
		return LineMatchPick{}, errors.New("bom line match: base_ccy required")
	}

	bomMfrTrim := strings.TrimSpace(in.BomMfr)
	var bomCanonID string
	if bomMfrTrim != "" {
		id, hit := ResolveManufacturerCanonical(ctx, in.BomMfr, alias)
		if !hit {
			return LineMatchPick{Ok: false, Reason: lineMatchReasonBomManufacturerMiss}, nil
		}
		bomCanonID = id
	}

	var rows []AgentQuoteRow
	if err := json.Unmarshal(in.QuotesJSON, &rows); err != nil {
		return LineMatchPick{Ok: false, Reason: lineMatchReasonQuotesInvalid}, nil
	}
	if len(rows) == 0 {
		return LineMatchPick{Ok: false, Reason: lineMatchReasonNoQuotes}, nil
	}

	platformID := in.PlatformID
	defaultUnitCcy := DefaultQuoteCCY(platformID)

	var (
		bestIdx   int
		bestRow   AgentQuoteRow
		bestKey   MatchSortKey
		bestBase  float64
		bestOrig  float64
		bestCcy   string
		bestSrc   ComparePriceSource
		bestFX    FXMeta
		found     bool
		anyPrice  bool
		anyFXFail bool
	)

	for i := range rows {
		row := rows[i]
		if !lineMatchRowPasses(ctx, in, bomCanonID, bomMfrTrim != "", row, alias) {
			continue
		}

		moq := parseMoqDigits(row.MOQ)
		qIn := QuotePriceInput{
			UnitPrice:     0,
			MainlandPrice: row.MainlandPrice,
			HkPrice:       row.HKPrice,
			PriceTiers:    row.PriceTiers,
			Moq:           moq,
			Stock:         row.Stock,
		}
		cp := ExtractCompareUnitPrice(qIn, platformID, in.BomQty, in.ParseTierStrings, defaultUnitCcy)
		if !cp.Ok {
			continue
		}
		anyPrice = true

		base, meta, err := ToBaseCCY(ctx, cp.Price, cp.Ccy, in.BaseCCY, in.BizDate, in.RequestDay, fx)
		if err != nil {
			if errors.Is(err, ErrFXRateNotFound) {
				anyFXFail = true
			}
			continue
		}

		leadDays := MatchLeadDaysUnknown
		if d, ok := ParseLeadDays(row.LeadTime, platformID); ok {
			leadDays = d
		}
		stockVal := int64(0)
		if sq, ok := ParseCompareStock(row.Stock); ok {
			stockVal = sq
		}
		key := MatchSortKey{
			UnitPriceBaseQuantized: QuantizeUnitPriceBase(in.RoundingMode, base),
			LeadDays:               leadDays,
			StockParsed:            stockVal,
			PlatformID:             NormalizePlatformID(platformID),
		}

		if !found || LessMatchCandidate(key, bestKey) {
			found = true
			bestIdx = i
			bestRow = row
			bestKey = key
			bestBase = base
			bestOrig = cp.Price
			bestCcy = cp.Ccy
			bestSrc = cp.Source
			bestFX = meta
		}
	}

	if !found {
		if anyPrice && anyFXFail {
			return LineMatchPick{Ok: false, Reason: lineMatchReasonFXUnavailable}, nil
		}
		if !anyPrice {
			return LineMatchPick{Ok: false, Reason: lineMatchReasonNoComparePrice}, nil
		}
		return LineMatchPick{Ok: false, Reason: lineMatchReasonNoCandidate}, nil
	}

	return LineMatchPick{
		RowIndex:           bestIdx,
		Row:                bestRow,
		UnitPriceBase:      bestBase,
		OriginalPrice:      bestOrig,
		OriginalCCY:        bestCcy,
		ComparePriceSource: bestSrc,
		FxMeta:             bestFX,
		Ok:                 true,
	}, nil
}

func lineMatchRowPasses(ctx context.Context, in LineMatchInput, bomCanonID string, bomMfrRequired bool, row AgentQuoteRow, alias AliasLookup) bool {
	if strings.TrimSpace(row.Model) == "" {
		return false
	}
	if NormalizeMPNForBOMSearch(in.BomMpn) != NormalizeMPNForBOMSearch(row.Model) {
		return false
	}
	if pkg := strings.TrimSpace(in.BomPackage); pkg != "" {
		if NormalizeMfrString(row.Package) != NormalizeMfrString(pkg) {
			return false
		}
	}
	if !bomMfrRequired {
		return true
	}
	if strings.TrimSpace(row.Manufacturer) == "" {
		return false
	}
	qCanon, qHit := ResolveManufacturerCanonical(ctx, row.Manufacturer, alias)
	if !qHit || qCanon != bomCanonID {
		return false
	}
	return true
}

// parseMoqDigits takes the first positive integer literal from moq string (e.g. "1", "MOQ 100"); 0 = unknown / not set.
func parseMoqDigits(s string) int {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	start := -1
	for i, r := range s {
		if r >= '0' && r <= '9' {
			if start < 0 {
				start = i
			}
		} else if start >= 0 {
			v, err := strconv.Atoi(s[start:i])
			if err == nil && v > 0 {
				return v
			}
			start = -1
		}
	}
	if start >= 0 {
		v, err := strconv.Atoi(s[start:])
		if err == nil && v > 0 {
			return v
		}
	}
	return 0
}
