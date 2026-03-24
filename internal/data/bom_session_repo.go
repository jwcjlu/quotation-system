package data

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"caichip/internal/biz"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// NewBOMSessionRepo 会话仓储；无 DB 时返回不可用桩。
func NewBOMSessionRepo(d *Data) biz.BOMSessionRepo {
	if d == nil || d.DB == nil {
		return bomSessionNoopRepo{}
	}
	return &bomSessionSQLRepo{db: d.DB}
}

type bomSessionNoopRepo struct{}

func (bomSessionNoopRepo) Create(ctx context.Context, s *biz.BOMSession) error {
	return biz.ErrBOMSessionStoreUnavailable
}

func (bomSessionNoopRepo) GetByID(ctx context.Context, id string) (*biz.BOMSession, error) {
	return nil, biz.ErrBOMSessionStoreUnavailable
}

func (bomSessionNoopRepo) ReplaceSessionLines(ctx context.Context, sessionID string, parseMode string, lines []*biz.BOMSessionLine) error {
	return biz.ErrBOMSessionStoreUnavailable
}

func (bomSessionNoopRepo) ListSessionLines(ctx context.Context, sessionID string) ([]*biz.BOMSessionLine, error) {
	return nil, biz.ErrBOMSessionStoreUnavailable
}

func (bomSessionNoopRepo) CountSessionLines(ctx context.Context, sessionID string) (int, error) {
	return 0, biz.ErrBOMSessionStoreUnavailable
}

func (bomSessionNoopRepo) UpdatePlatformSelection(ctx context.Context, sessionID string, platformIDs []string, expectedRevision int) (int, error) {
	return 0, biz.ErrBOMSessionStoreUnavailable
}

type bomSessionSQLRepo struct {
	db *gorm.DB
}

func (r *bomSessionSQLRepo) Create(ctx context.Context, s *biz.BOMSession) error {
	id := strings.TrimSpace(s.ID)
	if id == "" {
		id = uuid.New().String()
	}
	s.ID = id
	if s.Status == "" {
		s.Status = "draft"
	}
	if s.SelectionRevision == 0 {
		s.SelectionRevision = 1
	}
	raw, err := json.Marshal(s.PlatformIDs)
	if err != nil {
		return err
	}
	titlePtr := strPtrOrNil(s.Title)
	parsePtr := strPtrOrNil(s.ParseMode)
	storePtr := strPtrOrNil(s.StorageFileKey)
	q := `INSERT INTO bom_session (id, title, status, biz_date, selection_revision, platform_ids, parse_mode, storage_file_key)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)`
	return r.db.WithContext(ctx).Exec(q,
		id, titlePtr, s.Status, s.BizDate.Format("2006-01-02"), s.SelectionRevision, raw, parsePtr, storePtr,
	).Error
}

func normalizePlatformIDs(platformIDs []string) ([]string, error) {
	seen := make(map[string]struct{})
	var ids []string
	for _, p := range platformIDs {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if _, ok := seen[p]; ok {
			continue
		}
		seen[p] = struct{}{}
		ids = append(ids, p)
	}
	if len(ids) == 0 {
		return nil, biz.ErrBOMSessionPlatformsEmpty
	}
	return ids, nil
}

func (r *bomSessionSQLRepo) UpdatePlatformSelection(ctx context.Context, sessionID string, platformIDs []string, expectedRevision int) (int, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return 0, biz.ErrBOMSessionNotFound
	}
	ids, err := normalizePlatformIDs(platformIDs)
	if err != nil {
		return 0, err
	}
	raw, err := json.Marshal(ids)
	if err != nil {
		return 0, err
	}

	tx := r.db.WithContext(ctx).Begin()
	if tx.Error != nil {
		return 0, tx.Error
	}
	defer func() { _ = tx.Rollback() }()

	var curRev int
	err = tx.Raw(`
SELECT selection_revision FROM bom_session WHERE id = ? FOR UPDATE`, sessionID).Row().Scan(&curRev)
	if err == sql.ErrNoRows {
		return 0, biz.ErrBOMSessionNotFound
	}
	if err != nil {
		return 0, err
	}

	if expectedRevision != 0 && curRev != expectedRevision {
		return 0, biz.ErrBOMSessionRevisionConflict
	}

	newRev := curRev + 1
	err = tx.Exec(`
UPDATE bom_session SET platform_ids = ?, selection_revision = ?, updated_at = CURRENT_TIMESTAMP(3)
WHERE id = ?`, raw, newRev, sessionID).Error
	if err != nil {
		return 0, err
	}
	if err := tx.Commit().Error; err != nil {
		return 0, err
	}
	return newRev, nil
}

func strPtrOrNil(s string) interface{} {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	return s
}

func (r *bomSessionSQLRepo) GetByID(ctx context.Context, id string) (*biz.BOMSession, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, biz.ErrBOMSessionNotFound
	}
	var (
		sid                                  string
		title, status, parseMode, storageKey sql.NullString
		bizDate                              time.Time
		rev                                  int
		platformRaw                          []byte
		createdAt, updatedAt                 time.Time
	)
	var bizStr, cStr, uStr sql.NullString
	q := `SELECT id, title, status, biz_date, selection_revision, platform_ids, parse_mode, storage_file_key, created_at, updated_at
			FROM bom_session WHERE id = ?`
	err := r.db.WithContext(ctx).Raw(q, id).Row().Scan(
		&sid, &title, &status, &bizStr, &rev, &platformRaw, &parseMode, &storageKey, &cStr, &uStr)
	if err == sql.ErrNoRows {
		return nil, biz.ErrBOMSessionNotFound
	}
	if err != nil {
		return nil, err
	}
	if bizStr.Valid {
		t, err := parseDateTimeForScan(bizStr.String)
		if err != nil {
			return nil, fmt.Errorf("biz_date: %w", err)
		}
		bizDate = t
	}
	if cStr.Valid {
		t, err := parseDateTimeForScan(cStr.String)
		if err != nil {
			return nil, fmt.Errorf("created_at: %w", err)
		}
		createdAt = t
	}
	if uStr.Valid {
		t, err := parseDateTimeForScan(uStr.String)
		if err != nil {
			return nil, fmt.Errorf("updated_at: %w", err)
		}
		updatedAt = t
	}
	out := &biz.BOMSession{
		ID:                sid,
		Status:            status.String,
		BizDate:           bizDate,
		SelectionRevision: rev,
		CreatedAt:         createdAt,
		UpdatedAt:         updatedAt,
	}
	if title.Valid {
		out.Title = title.String
	}
	if parseMode.Valid {
		out.ParseMode = parseMode.String
	}
	if storageKey.Valid {
		out.StorageFileKey = storageKey.String
	}
	if len(platformRaw) > 0 {
		_ = json.Unmarshal(platformRaw, &out.PlatformIDs)
	}
	return out, nil
}

func (r *bomSessionSQLRepo) ListSessionLines(ctx context.Context, sessionID string) ([]*biz.BOMSessionLine, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil, nil
	}
	q := `SELECT id, line_no, raw_text, mpn, mfr, package, qty, extra_json FROM bom_session_line WHERE session_id = ? ORDER BY line_no`
	rows, err := r.db.WithContext(ctx).Raw(q, sessionID).Rows()
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*biz.BOMSessionLine
	for rows.Next() {
		var ln biz.BOMSessionLine
		var raw, mfr, pkg sql.NullString
		var qty sql.NullFloat64
		var extra []byte
		if err := rows.Scan(&ln.ID, &ln.LineNo, &raw, &ln.MPN, &mfr, &pkg, &qty, &extra); err != nil {
			return nil, err
		}
		if raw.Valid {
			ln.RawText = raw.String
		}
		if mfr.Valid {
			ln.MFR = mfr.String
		}
		if pkg.Valid {
			ln.Package = pkg.String
		}
		if qty.Valid {
			v := qty.Float64
			ln.Qty = &v
		}
		ln.ExtraJSON = extra
		out = append(out, &ln)
	}
	return out, rows.Err()
}

func (r *bomSessionSQLRepo) CountSessionLines(ctx context.Context, sessionID string) (int, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" || r.db == nil {
		return 0, nil
	}
	var n int
	err := r.db.WithContext(ctx).Raw(`SELECT COUNT(*) FROM bom_session_line WHERE session_id = ?`, sessionID).Row().Scan(&n)
	if err != nil {
		return 0, err
	}
	return n, nil
}

func (r *bomSessionSQLRepo) ReplaceSessionLines(ctx context.Context, sessionID string, parseMode string, lines []*biz.BOMSessionLine) error {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil
	}
	tx := r.db.WithContext(ctx).Begin()
	if tx.Error != nil {
		return tx.Error
	}
	defer func() { _ = tx.Rollback() }()

	err := tx.Exec(`DELETE FROM bom_session_line WHERE session_id = ?`, sessionID).Error
	if err != nil {
		return err
	}

	for _, line := range lines {
		if line == nil {
			continue
		}
		var raw, mfr, pkg interface{}
		if strings.TrimSpace(line.RawText) != "" {
			raw = line.RawText
		}
		if strings.TrimSpace(line.MFR) != "" {
			mfr = line.MFR
		}
		if strings.TrimSpace(line.Package) != "" {
			pkg = line.Package
		}
		var qty interface{}
		if line.Qty != nil {
			qty = *line.Qty
		}
		var extra interface{}
		if len(line.ExtraJSON) > 0 {
			extra = line.ExtraJSON
		}
		err = tx.Exec(`
				INSERT INTO bom_session_line (session_id, line_no, raw_text, mpn, mfr, package, qty, extra_json)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			sessionID, line.LineNo, raw, line.MPN, mfr, pkg, qty, extra).Error
		if err != nil {
			return err
		}
	}

	pm := strPtrOrNil(parseMode)
	err = tx.Exec(`UPDATE bom_session SET parse_mode = ?, updated_at = CURRENT_TIMESTAMP(3) WHERE id = ?`, pm, sessionID).Error
	if err != nil {
		return err
	}
	return tx.Commit().Error
}
