package data

import (
	"context"
	"strings"

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

func (r *BomManufacturerCleaningRepo) BackfillSessionManufacturerCanonical(ctx context.Context, sessionID, aliasNorm, canonicalID string, overwrite bool) (biz.ManufacturerCleaningResult, error) {
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
	quotes, err := r.listQuoteItemsForCleaning(ctx, sessionID)
	if err != nil {
		return out, err
	}
	for _, item := range quotes {
		if biz.NormalizeMfrString(item.Manufacturer) != aliasNorm {
			continue
		}
		if !overwrite && item.ManufacturerCanonicalID != nil {
			continue
		}
		updated, err := r.updateQuoteItemCanonical(ctx, item.ID, canonicalID, overwrite)
		if err != nil {
			return out, err
		}
		out.QuoteItemUpdated += updated
	}
	return out, nil
}

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
	quotes, err := r.listQuoteItemsForCleaning(ctx, sessionID)
	if err != nil {
		return out, err
	}
	for _, item := range quotes {
		if item.ManufacturerCanonicalID != nil {
			continue
		}
		updated, err := r.applyKnownManufacturer(ctx, biz.NormalizeMfrString(item.Manufacturer), func(canonicalID string) (int64, error) {
			return r.updateQuoteItemCanonical(ctx, item.ID, canonicalID, false)
		})
		if err != nil {
			return out, err
		}
		out.QuoteItemUpdated += updated
	}
	return out, nil
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

func (r *BomManufacturerCleaningRepo) listQuoteItemsForCleaning(ctx context.Context, sessionID string) ([]BomQuoteItem, error) {
	var rows []BomQuoteItem
	err := r.db.WithContext(ctx).Model(&BomQuoteItem{}).Where("session_id = ?", sessionID).Find(&rows).Error
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

func (r *BomManufacturerCleaningRepo) updateQuoteItemCanonical(ctx context.Context, id uint64, canonicalID string, overwrite bool) (int64, error) {
	q := r.db.WithContext(ctx).Model(&BomQuoteItem{}).Where("id = ?", id)
	if !overwrite {
		q = q.Where("manufacturer_canonical_id IS NULL")
	}
	res := q.Update("manufacturer_canonical_id", canonicalID)
	return res.RowsAffected, res.Error
}

var _ biz.BomManufacturerCleaningRepo = (*BomManufacturerCleaningRepo)(nil)
