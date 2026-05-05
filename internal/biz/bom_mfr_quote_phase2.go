package biz

import (
	"errors"
	"strings"
)

// ErrParentLineMissingManufacturerCanonical 阶段二提交时父行缺少 canonical（不变量 / REQ-S2-002）。
var ErrParentLineMissingManufacturerCanonical = errors.New("parent line has no manufacturer_canonical_id")

// QuoteItemEligibleForPhase2ReviewList 设计 §5.1：在「父行 mfr 非空、父行 canonical 非空、报价 manufacturer 非空、
// 报价 manufacturer_review_status=pending」已由 data 列表与调用方过滤后，判断是否应出现在阶段二待人工列表。
// 若报价侧尚无 manufacturer_canonical_id，且别名解析命中并与父行 canonical 一致，则视为已对齐、不入队。
func QuoteItemEligibleForPhase2ReviewList(
	demandCanonicalID string,
	quoteManufacturer string,
	quoteItemManufacturerCanonicalID *string,
	resolvedQuoteCanonical string,
	resolvedFromAlias bool,
) bool {
	if strings.TrimSpace(quoteManufacturer) == "" {
		return false
	}
	if quoteItemManufacturerCanonicalID != nil && strings.TrimSpace(*quoteItemManufacturerCanonicalID) != "" {
		return false
	}
	d := strings.TrimSpace(demandCanonicalID)
	if d == "" {
		return false
	}
	if resolvedFromAlias && strings.TrimSpace(resolvedQuoteCanonical) == d {
		return false
	}
	return true
}

// RequireParentManufacturerCanonicalForQuoteMfrReview 阶段二 accept/reject/改判前：父行须仍具备 canonical（与现 service 行为一致）。
func RequireParentManufacturerCanonicalForQuoteMfrReview(parentManufacturerCanonicalID *string) error {
	if parentManufacturerCanonicalID == nil || strings.TrimSpace(*parentManufacturerCanonicalID) == "" {
		return ErrParentLineMissingManufacturerCanonical
	}
	return nil
}
