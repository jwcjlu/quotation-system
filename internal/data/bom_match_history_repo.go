package data

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"caichip/internal/biz"

	"gorm.io/gorm"
)

// NewBOMMatchHistoryRepo 配单快照仓储。
func NewBOMMatchHistoryRepo(d *Data) biz.BOMMatchHistoryRepo {
	if d == nil || d.DB == nil {
		return bomMatchHistoryNoopRepo{}
	}
	return &bomMatchHistorySQLRepo{db: d.DB}
}

type bomMatchHistoryNoopRepo struct{}

func (bomMatchHistoryNoopRepo) SaveSnapshot(ctx context.Context, sessionID string, strategy string, payloadJSON []byte) error {
	return nil
}

func (bomMatchHistoryNoopRepo) List(ctx context.Context, offset, limit int) ([]*biz.MatchHistoryRow, int, error) {
	return nil, 0, nil
}

func (bomMatchHistoryNoopRepo) GetByID(ctx context.Context, id int64) (*biz.MatchHistoryDetail, error) {
	return nil, biz.ErrMatchHistoryNotFound
}

type bomMatchHistorySQLRepo struct {
	db *gorm.DB
}

type snapshotPayload struct {
	TotalAmount float64          `json:"total_amount"`
	Items       []*biz.MatchItem `json:"items"`
}

func (r *bomMatchHistorySQLRepo) SaveSnapshot(ctx context.Context, sessionID string, strategy string, payloadJSON []byte) error {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" || len(payloadJSON) == 0 {
		return nil
	}
	var next int
	err := r.db.WithContext(ctx).Raw(
		`SELECT COALESCE(MAX(version), 0) + 1 FROM bom_match_result WHERE session_id = ?`, sessionID,
	).Row().Scan(&next)
	if err != nil {
		return err
	}
	return r.db.WithContext(ctx).Exec(
		`INSERT INTO bom_match_result (session_id, version, strategy, payload_json) VALUES (?, ?, ?, ?)`,
		sessionID, next, strPtrOrNil(strategy), payloadJSON,
	).Error
}

func (r *bomMatchHistorySQLRepo) List(ctx context.Context, offset, limit int) ([]*biz.MatchHistoryRow, int, error) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}
	var total int
	if err := r.db.WithContext(ctx).Raw(`SELECT COUNT(*) FROM bom_match_result`).Row().Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := r.db.WithContext(ctx).Raw(
		`SELECT id, session_id, version, strategy, created_at, payload_json FROM bom_match_result ORDER BY created_at DESC LIMIT ? OFFSET ?`,
		limit, offset,
	).Rows()
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []*biz.MatchHistoryRow
	for rows.Next() {
		var row biz.MatchHistoryRow
		var payload []byte
		var strat sql.NullString
		var cStr sql.NullString
		if err := rows.Scan(&row.ID, &row.SessionID, &row.Version, &strat, &cStr, &payload); err != nil {
			return nil, 0, err
		}
		if cStr.Valid {
			t, err := parseDateTimeForScan(cStr.String)
			if err != nil {
				return nil, 0, fmt.Errorf("created_at: %w", err)
			}
			row.CreatedAt = t
		}
		if strat.Valid {
			row.Strategy = strat.String
		}
		row.TotalAmount = extractTotalAmount(payload)
		out = append(out, &row)
	}
	return out, total, rows.Err()
}

func extractTotalAmount(payload []byte) float64 {
	var p snapshotPayload
	if err := json.Unmarshal(payload, &p); err != nil {
		return 0
	}
	return p.TotalAmount
}

func (r *bomMatchHistorySQLRepo) GetByID(ctx context.Context, id int64) (*biz.MatchHistoryDetail, error) {
	var d biz.MatchHistoryDetail
	var payload []byte
	var strat sql.NullString
	var cStr sql.NullString
	err := r.db.WithContext(ctx).Raw(
		`SELECT id, session_id, version, strategy, created_at, payload_json FROM bom_match_result WHERE id = ?`, id,
	).Row().Scan(&d.ID, &d.SessionID, &d.Version, &strat, &cStr, &payload)
	if err == nil && cStr.Valid {
		d.CreatedAt, err = parseDateTimeForScan(cStr.String)
		if err != nil {
			err = fmt.Errorf("created_at: %w", err)
		}
	}
	if err == sql.ErrNoRows {
		return nil, biz.ErrMatchHistoryNotFound
	}
	if err != nil {
		return nil, err
	}
	if strat.Valid {
		d.Strategy = strat.String
	}
	var p snapshotPayload
	if err := json.Unmarshal(payload, &p); err != nil {
		return nil, err
	}
	d.TotalAmount = p.TotalAmount
	d.Items = p.Items
	return &d, nil
}
