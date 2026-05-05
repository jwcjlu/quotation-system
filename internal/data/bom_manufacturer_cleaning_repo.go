package data

import (
	"context"
	"strings"
	"time"

	"caichip/internal/biz"

	"gorm.io/gorm"
)

type BomManufacturerCleaningRepo struct {
	db    *gorm.DB
	alias biz.AliasLookup
}

func NewBomManufacturerCleaningRepo(d *Data, alias biz.AliasLookup) *BomManufacturerCleaningRepo {
	if d == nil || d.DB == nil {
		return &BomManufacturerCleaningRepo{alias: alias}
	}
	return &BomManufacturerCleaningRepo{db: d.DB, alias: alias}
}

func (r *BomManufacturerCleaningRepo) DBOk() bool {
	return r != nil && r.db != nil && r.alias != nil
}

// BackfillSessionLineManufacturerCanonical 阶段一：仅更新 t_bom_session_line（与别名审批所选 canonical 对齐）。
// 报价明细 t_bom_quote_item 不在此路径回填（两阶段清洗 — 阶段二单独评审）。
func (r *BomManufacturerCleaningRepo) BackfillSessionLineManufacturerCanonical(ctx context.Context, sessionID, aliasNorm, canonicalID string, overwrite bool) (biz.ManufacturerCleaningResult, error) {
	var out biz.ManufacturerCleaningResult
	if !r.DBOk() {
		return out, gorm.ErrInvalidDB
	}
	sessionID = strings.TrimSpace(sessionID)
	aliasNorm = strings.TrimSpace(aliasNorm)
	canonicalID = strings.TrimSpace(canonicalID)
	if sessionID == "" || aliasNorm == "" || canonicalID == "" {
		return out, nil
	}
	lines, err := r.listSessionLinesForCleaning(ctx, sessionID)
	if err != nil {
		return out, err
	}
	for _, line := range lines {
		if biz.NormalizeMfrString(derefStr(line.Mfr)) != aliasNorm {
			continue
		}
		if !overwrite && line.ManufacturerCanonicalID != nil {
			continue
		}
		updated, err := r.updateSessionLineCanonical(ctx, line.ID, canonicalID, overwrite)
		if err != nil {
			return out, err
		}
		out.SessionLineUpdated += updated
	}
	return out, nil
}

// ApplyKnownAliasesToSession 仅对需求行按别名表补 canonical；不修改 t_bom_quote_item。
func (r *BomManufacturerCleaningRepo) ApplyKnownAliasesToSession(ctx context.Context, sessionID string) (biz.ManufacturerCleaningResult, error) {
	var out biz.ManufacturerCleaningResult
	if !r.DBOk() {
		return out, gorm.ErrInvalidDB
	}
	sessionID = strings.TrimSpace(sessionID)
	lines, err := r.listSessionLinesForCleaning(ctx, sessionID)
	if err != nil {
		return out, err
	}
	for _, line := range lines {
		if line.ManufacturerCanonicalID != nil {
			continue
		}
		updated, err := r.applyKnownManufacturer(ctx, biz.NormalizeMfrString(derefStr(line.Mfr)), func(canonicalID string) (int64, error) {
			return r.updateSessionLineCanonical(ctx, line.ID, canonicalID, false)
		})
		if err != nil {
			return out, err
		}
		out.SessionLineUpdated += updated
	}
	return out, nil
}

func (r *BomManufacturerCleaningRepo) ListMfrReviewQuoteItems(ctx context.Context, sessionID string) ([]biz.MfrReviewQuoteItem, error) {
	if r.db == nil {
		return nil, gorm.ErrInvalidDB
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil, nil
	}
	var rows []BomQuoteItem
	err := r.db.WithContext(ctx).Model(&BomQuoteItem{}).Where("session_id = ? AND manufacturer_review_status = ?", sessionID, biz.MfrReviewPending).Find(&rows).Error
	if err != nil {
		return nil, err
	}
	out := make([]biz.MfrReviewQuoteItem, 0, len(rows))
	for _, it := range rows {
		out = append(out, biz.MfrReviewQuoteItem{
			ID:                       it.ID,
			LineID:                   it.LineID,
			Manufacturer:             it.Manufacturer,
			ManufacturerCanonicalID:  it.ManufacturerCanonicalID,
			ManufacturerReviewStatus: it.ManufacturerReviewStatus,
		})
	}
	return out, nil
}

func (r *BomManufacturerCleaningRepo) LoadMfrReviewQuoteItem(ctx context.Context, sessionID string, quoteItemID uint64) (*biz.MfrReviewQuoteItem, error) {
	if r.db == nil {
		return nil, gorm.ErrInvalidDB
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" || quoteItemID == 0 {
		return nil, nil
	}
	var it BomQuoteItem
	err := r.db.WithContext(ctx).Where("id = ? AND session_id = ?", quoteItemID, sessionID).First(&it).Error
	if err != nil {
		return nil, err
	}
	out := &biz.MfrReviewQuoteItem{
		ID:                       it.ID,
		LineID:                   it.LineID,
		Manufacturer:             it.Manufacturer,
		ManufacturerCanonicalID:  it.ManufacturerCanonicalID,
		ManufacturerReviewStatus: it.ManufacturerReviewStatus,
	}
	return out, nil
}

func (r *BomManufacturerCleaningRepo) UpdateQuoteItemManufacturerReview(ctx context.Context, quoteItemID uint64, status string, canonicalID *string, reason *string) error {
	if r.db == nil {
		return gorm.ErrInvalidDB
	}
	updates := map[string]any{
		"manufacturer_review_status": status,
	}
	if canonicalID != nil {
		updates["manufacturer_canonical_id"] = *canonicalID
	} else {
		updates["manufacturer_canonical_id"] = nil
	}
	if reason != nil {
		updates["manufacturer_review_reason"] = *reason
	} else {
		updates["manufacturer_review_reason"] = nil
	}
	now := time.Now().UTC()
	updates["manufacturer_reviewed_at"] = &now
	return r.db.WithContext(ctx).Model(&BomQuoteItem{}).Where("id = ?", quoteItemID).Updates(updates).Error
}

func (r *BomManufacturerCleaningRepo) applyKnownManufacturer(ctx context.Context, aliasNorm string, update func(string) (int64, error)) (int64, error) {
	if aliasNorm == "" {
		return 0, nil
	}
	canonicalID, ok, err := r.alias.CanonicalID(ctx, aliasNorm)
	if err != nil || !ok {
		return 0, err
	}
	return update(canonicalID)
}

func (r *BomManufacturerCleaningRepo) listSessionLinesForCleaning(ctx context.Context, sessionID string) ([]BomSessionLine, error) {
	var rows []BomSessionLine
	err := r.db.WithContext(ctx).Model(&BomSessionLine{}).Where("session_id = ?", sessionID).Find(&rows).Error
	return rows, err
}

func (r *BomManufacturerCleaningRepo) updateSessionLineCanonical(ctx context.Context, id int64, canonicalID string, overwrite bool) (int64, error) {
	q := r.db.WithContext(ctx).Model(&BomSessionLine{}).Where("id = ?", id)
	if !overwrite {
		q = q.Where("manufacturer_canonical_id IS NULL")
	}
	res := q.Update("manufacturer_canonical_id", canonicalID)
	return res.RowsAffected, res.Error
}

var _ biz.BomManufacturerCleaningRepo = (*BomManufacturerCleaningRepo)(nil)
