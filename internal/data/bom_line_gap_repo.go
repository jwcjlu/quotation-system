package data

import (
	"context"
	"strings"
	"time"

	"caichip/internal/biz"

	"gorm.io/gorm/clause"
)

type BOMLineGapRepo struct{ data *Data }

func NewBOMLineGapRepo(d *Data) *BOMLineGapRepo { return &BOMLineGapRepo{data: d} }
func (r *BOMLineGapRepo) DBOk() bool            { return r != nil && r.data != nil && r.data.DB != nil }

func (r *BOMLineGapRepo) UpsertOpenGaps(ctx context.Context, gaps []biz.BOMLineGap) error {
	if len(gaps) == 0 {
		return nil
	}
	seen := make(map[string]struct{})
	rows := make([]BomLineGap, 0, len(gaps))
	for _, g := range gaps {
		g.Status = biz.LineGapOpen
		active := g.ActiveKey()
		if active == "" {
			continue
		}
		if _, ok := seen[active]; ok {
			continue
		}
		seen[active] = struct{}{}
		rows = append(rows, BomLineGap{
			SessionID: g.SessionID, LineID: g.LineID, LineNo: g.LineNo, Mpn: g.Mpn,
			GapType: g.GapType, ReasonCode: g.ReasonCode, ReasonDetail: nullableString(g.ReasonDetail),
			ResolutionStatus: biz.LineGapOpen, ActiveKey: active,
		})
	}
	if len(rows) == 0 {
		return nil
	}
	return r.data.DB.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "active_key"}},
		DoUpdates: clause.AssignmentColumns([]string{"line_no", "mpn", "reason_code", "reason_detail", "updated_at"}),
	}).Create(&rows).Error
}

func (r *BOMLineGapRepo) ListLineGaps(ctx context.Context, sessionID string, statuses []string) ([]biz.BOMLineGap, error) {
	var rows []BomLineGap
	q := r.data.DB.WithContext(ctx).Where("session_id = ?", sessionID)
	if len(statuses) > 0 {
		q = q.Where("resolution_status IN ?", statuses)
	}
	if err := q.Order("line_no ASC, id ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]biz.BOMLineGap, 0, len(rows))
	for _, row := range rows {
		out = append(out, mapDataGapToBiz(row))
	}
	return out, nil
}

func (r *BOMLineGapRepo) GetLineGap(ctx context.Context, gapID uint64) (*biz.BOMLineGap, error) {
	var row BomLineGap
	if err := r.data.DB.WithContext(ctx).Where("id = ?", gapID).First(&row).Error; err != nil {
		return nil, err
	}
	gap := mapDataGapToBiz(row)
	return &gap, nil
}

func (r *BOMLineGapRepo) UpdateLineGapStatus(ctx context.Context, gapID uint64, fromStatus string, toStatus string, actor string, note string) error {
	now := time.Now()
	return r.data.DB.WithContext(ctx).Model(&BomLineGap{}).
		Where("id = ? AND resolution_status = ?", gapID, fromStatus).
		Updates(map[string]any{
			"resolution_status": toStatus,
			"active_key":        "",
			"resolved_by":       nullableString(actor),
			"resolved_at":       &now,
			"resolution_note":   nullableString(note),
		}).Error
}

func (r *BOMLineGapRepo) SelectLineGapSubstitute(ctx context.Context, gapID uint64, actor string, substituteMpn string, reason string) error {
	now := time.Now()
	return r.data.DB.WithContext(ctx).Model(&BomLineGap{}).
		Where("id = ? AND resolution_status = ?", gapID, biz.LineGapOpen).
		Updates(map[string]any{
			"resolution_status": biz.LineGapSubstituteSelected,
			"active_key":        "",
			"resolved_by":       nullableString(actor),
			"resolved_at":       &now,
			"substitute_mpn":    nullableString(substituteMpn),
			"substitute_reason": nullableString(reason),
		}).Error
}

func mapDataGapToBiz(row BomLineGap) biz.BOMLineGap {
	return biz.BOMLineGap{
		ID: row.ID, SessionID: row.SessionID, LineID: row.LineID, LineNo: row.LineNo,
		Mpn: row.Mpn, GapType: row.GapType, ReasonCode: row.ReasonCode,
		ReasonDetail: derefString(row.ReasonDetail), Status: row.ResolutionStatus,
		ResolutionNote: derefString(row.ResolutionNote), SubstituteMpn: derefString(row.SubstituteMpn),
		SubstituteReason: derefString(row.SubstituteReason),
	}
}

func nullableString(s string) *string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	return &s
}

func derefString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

var _ biz.BOMLineGapRepo = (*BOMLineGapRepo)(nil)
