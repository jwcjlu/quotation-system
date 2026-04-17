package biz

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"
)

// LineMatchInput drives single-line multi-platform quote pick from cached quote rows (design §1–§3).
//
// Matching V1 (strict):
//   - quote_rows: AgentQuoteRow array loaded from t_bom_quote_item; empty array → no candidate.
//   - Model: NormalizeMPNForBOMSearch(bom_mpn) must equal NormalizeMPNForBOMSearch(quote.model); empty model rows are skipped.
//   - Package: if BomPackage is non-empty after TrimSpace, quote.package must match after NormalizeMfrString on both sides
//     (trim → NFKC → ASCII upper); empty BomPackage → no package constraint.
//   - Manufacturer: TrimSpace(BomMfr)=="" → no mfr filter (§2.5). Else: quote must have non-empty manufacturer (§2.6);
//     both sides resolved via ResolveManufacturerCanonical; canonical IDs must be equal. If BOM mfr non-empty but BOM side
//     misses alias table → entire line returns Ok=false (strict §2.3). Quote side miss → skip row.
//   - Params/desc: V1 not compared (no extra BOM fields on this struct).
//
// BomManufacturerResolveHint 由外层在一次配单中预先解析需求厂牌后传入，避免每个平台重复查别
// BomManufacturerResolveHint 由外层在一次配单中预先解析需求厂牌后传入，避免每个平台重复查别名表。
// Hit=false 表示已确认别名未命中（等价于 lineMatchReasonBomManufacturerMiss）；Hit=true 时使用 CanonID。
type BomManufacturerResolveHint struct {
	CanonID string
	Hit     bool
}

type LineMatchInput struct {
	BomMpn           string
	BomPackage       string // empty = no package constraint
	BomMfr           string // empty = no manufacturer constraint (§2.5)
	BomQty           int
	PlatformID       string
	QuoteRows        []AgentQuoteRow
	BizDate          time.Time
	RequestDay       time.Time
	BaseCCY          string
	RoundingMode     string // QuantizeUnitPriceBase mode; see bom_match_sort
	ParseTierStrings bool
	// BomMfrHint 非 nil 时跳过对 BomMfr 的 ResolveManufacturerCanonical（按 Hit/CanonID 解释）。
	BomMfrHint *BomManufacturerResolveHint
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
	// MfrMismatchQuoteManufacturers：BOM 有厂牌且已解析 canonical 时，本缓存内「型号+封装过关但厂牌未对齐」的报价 manufacturer 原文（去重）。
	MfrMismatchQuoteManufacturers []string
}

const (
	lineMatchReasonNoQuotes            = "no_quotes"
	lineMatchReasonBomManufacturerMiss = "bom_manufacturer_alias_miss"
	lineMatchReasonNoCandidate         = "no_matching_quote"
	lineMatchReasonNoComparePrice      = "no_compare_price_after_filters"
	lineMatchReasonFXUnavailable       = "fx_unavailable_all_candidates"
	// MfrMismatchEmptyPlaceholder 记入 MfrMismatchQuoteManufacturers，表示报价行 manufacturer 为空；不可作为审核入库别名。
	MfrMismatchEmptyPlaceholder = "(报价厂牌为空)"
)

// PickBestQuoteForLine filters cached quotes, extracts compare price, converts to base_ccy, and picks the best row by MatchSortKey.
// Errors: programmer mistakes (e.g. BomQty<=0, empty BaseCCY); FX/别名表查询等基础设施失败（非 ErrFXRateNotFound）。业务无匹配等用 Ok=false 与 Reason。
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
		if in.BomMfrHint != nil {
			if !in.BomMfrHint.Hit {
				return LineMatchPick{Ok: false, Reason: lineMatchReasonBomManufacturerMiss}, nil
			}
			bomCanonID = in.BomMfrHint.CanonID
		} else {
			id, hit, err := ResolveManufacturerCanonical(ctx, in.BomMfr, alias)
			if err != nil {
				return LineMatchPick{}, err
			}
			if !hit {
				return LineMatchPick{Ok: false, Reason: lineMatchReasonBomManufacturerMiss}, nil
			}
			bomCanonID = id
		}
	}

	rows := append([]AgentQuoteRow(nil), in.QuoteRows...)
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
	mfrSeen := make(map[string]struct{})
	var mfrMismatch []string
	addMfrMismatch := func(raw string) {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			raw = MfrMismatchEmptyPlaceholder
		}
		if _, ok := mfrSeen[raw]; ok {
			return
		}
		mfrSeen[raw] = struct{}{}
		mfrMismatch = append(mfrMismatch, raw)
	}
	bomMfrConstraint := bomMfrTrim != "" && bomCanonID != ""

	for i := range rows {
		row := rows[i]
		rowOK, err := lineMatchRowPasses(ctx, in, bomCanonID, bomMfrTrim != "", row, alias)
		if err != nil {
			return LineMatchPick{}, err
		}
		if bomMfrConstraint && quoteRowPassesModelAndPackage(in, row) && !rowOK {
			mfr := strings.TrimSpace(row.Manufacturer)
			if mfr == "" {
				addMfrMismatch("")
			} else {
				qCanon, qHit, qerr := ResolveManufacturerCanonical(ctx, row.Manufacturer, alias)
				if qerr != nil {
					return LineMatchPick{}, qerr
				}
				if !qHit || qCanon != bomCanonID {
					addMfrMismatch(row.Manufacturer)
				}
			}
		}
		if !rowOK {
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
				continue
			}
			return LineMatchPick{}, err
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
			return LineMatchPick{Ok: false, Reason: lineMatchReasonFXUnavailable, MfrMismatchQuoteManufacturers: mfrMismatch}, nil
		}
		if !anyPrice {
			return LineMatchPick{Ok: false, Reason: lineMatchReasonNoComparePrice, MfrMismatchQuoteManufacturers: mfrMismatch}, nil
		}
		return LineMatchPick{Ok: false, Reason: lineMatchReasonNoCandidate, MfrMismatchQuoteManufacturers: mfrMismatch}, nil
	}

	return LineMatchPick{
		RowIndex:                      bestIdx,
		Row:                           bestRow,
		UnitPriceBase:                 bestBase,
		OriginalPrice:                 bestOrig,
		OriginalCCY:                   bestCcy,
		ComparePriceSource:            bestSrc,
		FxMeta:                        bestFX,
		Ok:                            true,
		MfrMismatchQuoteManufacturers: mfrMismatch,
	}, nil
}

func lineMatchRowPasses(ctx context.Context, in LineMatchInput, bomCanonID string, bomMfrRequired bool, row AgentQuoteRow, alias AliasLookup) (bool, error) {
	if strings.TrimSpace(row.Model) == "" {
		return false, nil
	}
	if NormalizeMPNForBOMSearch(in.BomMpn) != NormalizeMPNForBOMSearch(row.Model) {
		return false, nil
	}
	if pkg := strings.TrimSpace(in.BomPackage); pkg != "" {
		if NormalizeMfrString(row.Package) != NormalizeMfrString(pkg) {
			return false, nil
		}
	}
	if !bomMfrRequired {
		return true, nil
	}
	if strings.TrimSpace(row.Manufacturer) == "" {
		return false, nil
	}
	qCanon, qHit, err := ResolveManufacturerCanonical(ctx, row.Manufacturer, alias)
	if err != nil {
		return false, err
	}
	if !qHit || qCanon != bomCanonID {
		return false, nil
	}
	return true, nil
}

// quoteRowPassesModelAndPackage 仅校验型号与封装（若有），不含厂牌；用于识别「厂牌不匹配」类跳过。
func quoteRowPassesModelAndPackage(in LineMatchInput, row AgentQuoteRow) bool {
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
