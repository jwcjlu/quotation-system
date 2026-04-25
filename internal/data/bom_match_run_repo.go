package data

import (
	"context"
	"time"

	"caichip/internal/biz"

	"gorm.io/gorm"
)

type BOMMatchRunRepo struct{ data *Data }

func NewBOMMatchRunRepo(d *Data) *BOMMatchRunRepo { return &BOMMatchRunRepo{data: d} }
func (r *BOMMatchRunRepo) DBOk() bool             { return r != nil && r.data != nil && r.data.DB != nil }

func (r *BOMMatchRunRepo) CreateMatchRun(ctx context.Context, sessionID string, selectionRevision int, currency string, createdBy string, items []biz.BOMMatchResultItemDraft) (uint64, int, error) {
	var runID uint64
	var runNo int
	err := r.data.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var maxNo int
		if err := tx.Model(&BomMatchRun{}).Where("session_id = ?", sessionID).
			Select("COALESCE(MAX(run_no), 0)").Scan(&maxNo).Error; err != nil {
			return err
		}
		runNo = maxNo + 1
		total, matched, unresolved := biz.SummarizeMatchRunItems(items)
		now := time.Now()
		run := BomMatchRun{
			RunNo: runNo, SessionID: sessionID, SelectionRevision: selectionRevision,
			Status: biz.MatchRunSaved, Source: "manual_save", LineTotal: len(items),
			MatchedLineCount: matched, UnresolvedLineCount: unresolved,
			TotalAmount: total, Currency: currency, CreatedBy: nullableString(createdBy),
			SavedAt: &now,
		}
		if err := tx.Create(&run).Error; err != nil {
			return err
		}
		runID = run.ID
		rows := make([]BomMatchResultItem, 0, len(items))
		for _, it := range items {
			rows = append(rows, mapBizMatchItemDraftToData(run.ID, sessionID, it))
		}
		if len(rows) > 0 {
			if err := tx.Create(&rows).Error; err != nil {
				return err
			}
		}
		return nil
	})
	return runID, runNo, err
}

func (r *BOMMatchRunRepo) ListMatchRuns(ctx context.Context, sessionID string) ([]biz.BOMMatchRunView, error) {
	var rows []BomMatchRun
	if err := r.data.DB.WithContext(ctx).Where("session_id = ?", sessionID).
		Order("run_no DESC").Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]biz.BOMMatchRunView, 0, len(rows))
	for _, row := range rows {
		out = append(out, mapDataMatchRunToBiz(row))
	}
	return out, nil
}

func (r *BOMMatchRunRepo) GetMatchRun(ctx context.Context, runID uint64) (*biz.BOMMatchRunView, []biz.BOMMatchResultItemDraft, error) {
	var run BomMatchRun
	if err := r.data.DB.WithContext(ctx).Where("id = ?", runID).First(&run).Error; err != nil {
		return nil, nil, err
	}
	var rows []BomMatchResultItem
	if err := r.data.DB.WithContext(ctx).Where("run_id = ?", runID).
		Order("line_no ASC, id ASC").Find(&rows).Error; err != nil {
		return nil, nil, err
	}
	view := mapDataMatchRunToBiz(run)
	items := make([]biz.BOMMatchResultItemDraft, 0, len(rows))
	for _, row := range rows {
		items = append(items, mapDataMatchItemToBizDraft(row))
	}
	return &view, items, nil
}

func (r *BOMMatchRunRepo) SupersedePreviousRuns(ctx context.Context, sessionID string, keepRunID uint64) error {
	now := time.Now()
	return r.data.DB.WithContext(ctx).Model(&BomMatchRun{}).
		Where("session_id = ? AND id <> ? AND status = ?", sessionID, keepRunID, biz.MatchRunSaved).
		Updates(map[string]any{"status": biz.MatchRunSuperseded, "superseded_at": &now}).Error
}

func mapBizMatchItemDraftToData(runID uint64, sessionID string, it biz.BOMMatchResultItemDraft) BomMatchResultItem {
	return BomMatchResultItem{
		RunID: runID, SessionID: sessionID, LineID: it.LineID, LineNo: it.LineNo,
		SourceType: it.SourceType, MatchStatus: it.MatchStatus, GapID: optionalUint64(it.GapID),
		QuoteItemID: optionalUint64(it.QuoteItemID), PlatformID: it.PlatformID, DemandMpn: it.DemandMpn,
		DemandMfr: it.DemandMfr, DemandPackage: it.DemandPackage, DemandQty: optionalFloat64(it.DemandQty),
		MatchedMpn: it.MatchedMpn, MatchedMfr: it.MatchedMfr, MatchedPackage: it.MatchedPackage,
		Stock: optionalInt64(it.Stock), LeadTime: it.LeadTime, UnitPrice: optionalFloat64(it.UnitPrice),
		Subtotal: optionalFloat64(it.Subtotal), Currency: it.Currency, OriginalMpn: nullableString(it.OriginalMpn),
		SubstituteMpn: nullableString(it.SubstituteMpn), SubstituteReason: nullableString(it.SubstituteReason),
		CodeTS: it.CodeTS, ControlMark: it.ControlMark, ImportTaxOrdinaryRate: it.ImportTaxImpOrdinaryRate,
		ImportTaxDiscountRate: it.ImportTaxImpDiscountRate, ImportTaxTempRate: it.ImportTaxImpTempRate,
		SnapshotJSON: it.SnapshotJSON,
	}
}

func mapDataMatchRunToBiz(row BomMatchRun) biz.BOMMatchRunView {
	return biz.BOMMatchRunView{
		ID: row.ID, RunNo: row.RunNo, SessionID: row.SessionID, SelectionRevision: row.SelectionRevision,
		Status: row.Status, LineTotal: row.LineTotal, MatchedLineCount: row.MatchedLineCount,
		UnresolvedLineCount: row.UnresolvedLineCount, TotalAmount: row.TotalAmount,
		Currency: row.Currency, CreatedBy: derefString(row.CreatedBy), CreatedAt: row.CreatedAt, SavedAt: row.SavedAt,
	}
}

func mapDataMatchItemToBizDraft(row BomMatchResultItem) biz.BOMMatchResultItemDraft {
	return biz.BOMMatchResultItemDraft{
		LineID: row.LineID, LineNo: row.LineNo, SourceType: row.SourceType, MatchStatus: row.MatchStatus,
		GapID: derefUint64(row.GapID), QuoteItemID: derefUint64(row.QuoteItemID), PlatformID: row.PlatformID,
		DemandMpn: row.DemandMpn, DemandMfr: row.DemandMfr, DemandPackage: row.DemandPackage,
		DemandQty: derefFloat64(row.DemandQty), MatchedMpn: row.MatchedMpn, MatchedMfr: row.MatchedMfr,
		MatchedPackage: row.MatchedPackage, Stock: derefInt64(row.Stock), LeadTime: row.LeadTime,
		UnitPrice: derefFloat64(row.UnitPrice), Subtotal: derefFloat64(row.Subtotal), Currency: row.Currency,
		OriginalMpn: derefString(row.OriginalMpn), SubstituteMpn: derefString(row.SubstituteMpn),
		SubstituteReason: derefString(row.SubstituteReason), CodeTS: row.CodeTS, ControlMark: row.ControlMark,
		ImportTaxImpOrdinaryRate: row.ImportTaxOrdinaryRate, ImportTaxImpDiscountRate: row.ImportTaxDiscountRate,
		ImportTaxImpTempRate: row.ImportTaxTempRate, SnapshotJSON: row.SnapshotJSON,
	}
}

func optionalUint64(v uint64) *uint64 {
	if v == 0 {
		return nil
	}
	return &v
}

func optionalInt64(v int64) *int64 {
	if v == 0 {
		return nil
	}
	return &v
}

func optionalFloat64(v float64) *float64 {
	if v == 0 {
		return nil
	}
	return &v
}

func derefUint64(v *uint64) uint64 {
	if v == nil {
		return 0
	}
	return *v
}

func derefInt64(v *int64) int64 {
	if v == nil {
		return 0
	}
	return *v
}

func derefFloat64(v *float64) float64 {
	if v == nil {
		return 0
	}
	return *v
}

var _ biz.BOMMatchRunRepo = (*BOMMatchRunRepo)(nil)
