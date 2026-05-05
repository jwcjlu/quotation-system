package service

import (
	"strings"

	"caichip/internal/biz"
	"caichip/internal/data"
)

// phase2QuoteItemMfrReviewPrelude 阶段二列表：父行与报价子行在调用别名解析前的共同过滤（与 QuoteItemEligibleForPhase2ReviewList 前置条件一致）。
func phase2QuoteItemMfrReviewPrelude(it biz.MfrReviewQuoteItem, lineByID map[int64]data.BomSessionLine) (line data.BomSessionLine, quoteMfr, normKey string, ok bool) {
	if it.LineID == nil {
		return
	}
	line, ok = lineByID[*it.LineID]
	if !ok {
		return
	}
	if strings.TrimSpace(derefStrPtr(line.Mfr)) == "" {
		ok = false
		return
	}
	if line.ManufacturerCanonicalID == nil || strings.TrimSpace(*line.ManufacturerCanonicalID) == "" {
		ok = false
		return
	}
	quoteMfr = strings.TrimSpace(it.Manufacturer)
	if quoteMfr == "" {
		ok = false
		return
	}
	if it.ManufacturerCanonicalID != nil && strings.TrimSpace(*it.ManufacturerCanonicalID) != "" {
		ok = false
		return
	}
	normKey = biz.NormalizeMfrString(quoteMfr)
	if normKey == "" {
		ok = false
		return
	}
	ok = true
	return
}
