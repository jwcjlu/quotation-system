package data

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"caichip/internal/biz"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// BomSessionRepo bom_session / bom_session_line。
type BomSessionRepo struct {
	db *gorm.DB
}

// NewBomSessionRepo ...
func NewBomSessionRepo(d *Data) *BomSessionRepo {
	if d == nil || d.DB == nil {
		return &BomSessionRepo{}
	}
	return &BomSessionRepo{db: d.DB}
}

// DBOk ...
func (r *BomSessionRepo) DBOk() bool {
	return r != nil && r.db != nil
}

var _ biz.BOMSessionRepo = (*BomSessionRepo)(nil)

func normalizeReadinessColumn(mode *string) string {
	if mode == nil {
		return biz.ReadinessLenient
	}
	v := strings.ToLower(strings.TrimSpace(*mode))
	if v == biz.ReadinessStrict {
		return biz.ReadinessStrict
	}
	return biz.ReadinessLenient
}

func (r *BomSessionRepo) CreateSession(ctx context.Context, title string, platformIDs []string, customerName, contactPhone, contactEmail, contactExtra, readinessMode *string) (sessionID string, bizDate time.Time, selectionRevision int, err error) {
	if !r.DBOk() {
		return "", time.Time{}, 0, gorm.ErrInvalidDB
	}
	ids := normalizePlatformSlice(platformIDs)
	b, err := json.Marshal(ids)
	if err != nil {
		return "", time.Time{}, 0, err
	}
	id := uuid.NewString()
	now := time.Now()
	bd := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	row := BomSession{
		ID:                id,
		Title:             strPtrOrNil(title),
		CustomerName:      customerName,
		ContactPhone:      contactPhone,
		ContactEmail:      contactEmail,
		ContactExtra:      contactExtra,
		Status:            "draft",
		ReadinessMode:     normalizeReadinessColumn(readinessMode),
		BizDate:           bd,
		SelectionRevision: 1,
		PlatformIDs:       string(b),
		ImportStatus:      biz.BOMImportStatusIdle,
		ImportProgress:    0,
		ImportStage:       biz.BOMImportStageValidating,
		CreatedAt:         now,
		UpdatedAt:         now,
	}
	if err := runGormOnceMoreOnBadConn(ctx, r.db, func(d *gorm.DB) error {
		return d.Create(&row).Error
	}); err != nil {
		return "", time.Time{}, 0, err
	}
	return id, bd, 1, nil
}

func (r *BomSessionRepo) GetSession(ctx context.Context, sessionID string) (*biz.BOMSessionView, error) {
	if !r.DBOk() {
		return nil, gorm.ErrInvalidDB
	}
	sessionID = strings.TrimSpace(sessionID)
	var row BomSession
	err := r.db.WithContext(ctx).Where("id = ?", sessionID).First(&row).Error
	if err != nil {
		return nil, err
	}
	pids, err := parsePlatformJSON(row.PlatformIDs)
	if err != nil {
		return nil, err
	}
	return &biz.BOMSessionView{
		SessionID:         row.ID,
		Title:             derefStr(row.Title),
		CustomerName:      derefStr(row.CustomerName),
		ContactPhone:      derefStr(row.ContactPhone),
		ContactEmail:      derefStr(row.ContactEmail),
		ContactExtra:      derefStr(row.ContactExtra),
		Status:            row.Status,
		ImportStatus:      row.ImportStatus,
		ImportProgress:    row.ImportProgress,
		ImportStage:       row.ImportStage,
		ImportMessage:     derefStr(row.ImportMessage),
		ImportErrorCode:   derefStr(row.ImportErrorCode),
		ImportError:       derefStr(row.ImportError),
		ImportUpdatedAt:   row.ImportUpdatedAt,
		ReadinessMode:     row.ReadinessMode,
		BizDate:           row.BizDate,
		PlatformIDs:       pids,
		SelectionRevision: row.SelectionRevision,
	}, nil
}

func (r *BomSessionRepo) PatchSession(ctx context.Context, sessionID string, title, customerName, contactPhone, contactEmail, contactExtra, readinessMode *string) error {
	if !r.DBOk() {
		return gorm.ErrInvalidDB
	}
	sessionID = strings.TrimSpace(sessionID)
	up := map[string]interface{}{"updated_at": gorm.Expr("CURRENT_TIMESTAMP(3)")}
	if title != nil {
		up["title"] = strPtrOrNil(*title)
	}
	if customerName != nil {
		up["customer_name"] = customerName
	}
	if contactPhone != nil {
		up["contact_phone"] = contactPhone
	}
	if contactEmail != nil {
		up["contact_email"] = contactEmail
	}
	if contactExtra != nil {
		up["contact_extra"] = contactExtra
	}
	if readinessMode != nil {
		up["readiness_mode"] = normalizeReadinessColumn(readinessMode)
	}
	return r.db.WithContext(ctx).Model(&BomSession{}).Where("id = ?", sessionID).Updates(up).Error
}

func (r *BomSessionRepo) PutPlatforms(ctx context.Context, sessionID string, platformIDs []string, expectedRevision int32) (newRevision int, err error) {
	if !r.DBOk() {
		return 0, gorm.ErrInvalidDB
	}
	sessionID = strings.TrimSpace(sessionID)
	ids := normalizePlatformSlice(platformIDs)
	b, err := json.Marshal(ids)
	if err != nil {
		return 0, err
	}
	tx := r.db.WithContext(ctx).Begin()
	if tx.Error != nil {
		return 0, tx.Error
	}
	defer func() { _ = tx.Rollback() }()

	var cur BomSession
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", sessionID).First(&cur).Error; err != nil {
		return 0, err
	}
	if int32(cur.SelectionRevision) != expectedRevision {
		return 0, biz.ErrBOMSessionRevisionMismatch
	}
	next := cur.SelectionRevision + 1
	if err := tx.Model(&BomSession{}).Where("id = ? AND selection_revision = ?", sessionID, cur.SelectionRevision).Updates(map[string]interface{}{
		"platform_ids":       string(b),
		"selection_revision": next,
		"updated_at":         gorm.Expr("CURRENT_TIMESTAMP(3)"),
	}).Error; err != nil {
		return 0, err
	}
	if err := tx.Commit().Error; err != nil {
		return 0, err
	}
	return next, nil
}

func (r *BomSessionRepo) ListSessions(ctx context.Context, page, pageSize int32, status, bizDate, q string) (items []biz.BOMSessionListItem, total int32, err error) {
	if !r.DBOk() {
		return nil, 0, gorm.ErrInvalidDB
	}
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}
	if pageSize > 200 {
		pageSize = 200
	}
	qb := r.db.WithContext(ctx).Model(&BomSession{})
	if strings.TrimSpace(status) != "" {
		qb = qb.Where("status = ?", strings.TrimSpace(status))
	}
	if strings.TrimSpace(bizDate) != "" {
		qb = qb.Where("biz_date = ?", strings.TrimSpace(bizDate))
	}
	if kw := strings.TrimSpace(q); kw != "" {
		like := "%" + kw + "%"
		qb = qb.Where("title LIKE ? OR customer_name LIKE ?", like, like)
	}
	var cnt int64
	if err := qb.Count(&cnt).Error; err != nil {
		return nil, 0, err
	}
	var rows []BomSession
	if err := qb.Order("updated_at DESC").Offset(int((page - 1) * pageSize)).Limit(int(pageSize)).Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	out := make([]biz.BOMSessionListItem, 0, len(rows))
	for _, row := range rows {
		var lc int64
		_ = r.db.WithContext(ctx).Model(&BomSessionLine{}).Where("session_id = ?", row.ID).Count(&lc).Error
		out = append(out, biz.BOMSessionListItem{
			SessionID:    row.ID,
			Title:        derefStr(row.Title),
			CustomerName: derefStr(row.CustomerName),
			Status:       row.Status,
			BizDate:      row.BizDate.Format("2006-01-02"),
			UpdatedAt:    row.UpdatedAt.Format(time.RFC3339Nano),
			LineCount:    int32(lc),
		})
	}
	return out, int32(cnt), nil
}

func (r *BomSessionRepo) ReplaceSessionLines(ctx context.Context, sessionID string, lines []biz.BomImportLine, parseMode *string) (nextLineNo int, err error) {
	if !r.DBOk() {
		return 0, gorm.ErrInvalidDB
	}
	sessionID = strings.TrimSpace(sessionID)
	tx := r.db.WithContext(ctx).Begin()
	if tx.Error != nil {
		return 0, tx.Error
	}
	defer func() { _ = tx.Rollback() }()

	var sess BomSession
	if err := tx.Where("id = ?", sessionID).First(&sess).Error; err != nil {
		return 0, err
	}
	nextRev := sess.SelectionRevision + 1
	if err := tx.Where("session_id = ?", sessionID).Delete(&BomSessionLine{}).Error; err != nil {
		return 0, err
	}
	maxNo := 0
	for _, ln := range lines {
		line := BomSessionLine{
			SessionID: sessionID,
			LineNo:    ln.LineNo,
			Mpn:       strings.TrimSpace(ln.Mpn),
			CreatedAt: time.Now(),
		}
		if ln.Mfr != "" {
			line.Mfr = &ln.Mfr
		}
		if ln.Package != "" {
			line.Package = &ln.Package
		}
		if ln.Qty != nil {
			line.Qty = ln.Qty
		}
		if len(ln.ExtraJSON) > 0 {
			line.ExtraJSON = ln.ExtraJSON
		}
		if ln.RawText != "" {
			rt := ln.RawText
			line.RawText = &rt
		}
		if err := tx.Create(&line).Error; err != nil {
			return 0, err
		}
		if line.LineNo > maxNo {
			maxNo = line.LineNo
		}
	}
	up := map[string]interface{}{
		"updated_at":         gorm.Expr("CURRENT_TIMESTAMP(3)"),
		"status":             "searching",
		"selection_revision": nextRev,
	}
	if parseMode != nil {
		up["parse_mode"] = *parseMode
	}
	if err := tx.Model(&BomSession{}).Where("id = ?", sessionID).Updates(up).Error; err != nil {
		return 0, err
	}
	if err := tx.Commit().Error; err != nil {
		return 0, err
	}
	return maxNo, nil
}

// ListSessionLinesFull 供 service 展示完整行（GetBOMLines）。
func (r *BomSessionRepo) ListSessionLinesFull(ctx context.Context, sessionID string) ([]BomSessionLine, error) {
	if !r.DBOk() {
		return nil, gorm.ErrInvalidDB
	}
	var rows []BomSessionLine
	err := r.db.WithContext(ctx).Where("session_id = ?", strings.TrimSpace(sessionID)).Order("line_no").Find(&rows).Error
	return rows, err
}

func (r *BomSessionRepo) ListSessionLines(ctx context.Context, sessionID string) ([]biz.BOMSessionLineView, error) {
	if !r.DBOk() {
		return nil, gorm.ErrInvalidDB
	}
	var rows []BomSessionLine
	err := r.db.WithContext(ctx).Where("session_id = ?", strings.TrimSpace(sessionID)).Order("line_no").Find(&rows).Error
	if err != nil {
		return nil, err
	}
	out := make([]biz.BOMSessionLineView, 0, len(rows))
	for _, row := range rows {
		out = append(out, biz.BOMSessionLineView{
			ID:     row.ID,
			LineNo: row.LineNo,
			Mpn:    row.Mpn,
		})
	}
	return out, nil
}

func (r *BomSessionRepo) SetSessionStatus(ctx context.Context, sessionID, status string) error {
	if !r.DBOk() {
		return gorm.ErrInvalidDB
	}
	return r.db.WithContext(ctx).Model(&BomSession{}).Where("id = ?", strings.TrimSpace(sessionID)).Updates(map[string]interface{}{
		"status":     strings.TrimSpace(status),
		"updated_at": gorm.Expr("CURRENT_TIMESTAMP(3)"),
	}).Error
}

func (r *BomSessionRepo) TryStartImport(ctx context.Context, sessionID, startedMessage string) (bool, error) {
	if !r.DBOk() {
		return false, gorm.ErrInvalidDB
	}
	sessionID = strings.TrimSpace(sessionID)
	msg := strings.TrimSpace(startedMessage)
	if msg == "" {
		msg = "import started"
	}
	tx := r.db.WithContext(ctx).Begin()
	if tx.Error != nil {
		return false, tx.Error
	}
	defer func() { _ = tx.Rollback() }()
	var cur BomSession
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", sessionID).First(&cur).Error; err != nil {
		return false, err
	}
	if strings.EqualFold(strings.TrimSpace(cur.ImportStatus), biz.BOMImportStatusParsing) {
		if err := tx.Commit().Error; err != nil {
			return false, err
		}
		return false, nil
	}
	if err := tx.Model(&BomSession{}).Where("id = ?", sessionID).Updates(map[string]interface{}{
		"import_status":     biz.BOMImportStatusParsing,
		"import_progress":   5,
		"import_stage":      biz.BOMImportStageValidating,
		"import_message":    msg,
		"import_error_code": nil,
		"import_error":      nil,
		"import_updated_at": gorm.Expr("CURRENT_TIMESTAMP(3)"),
		"updated_at":        gorm.Expr("CURRENT_TIMESTAMP(3)"),
	}).Error; err != nil {
		return false, err
	}
	if err := tx.Commit().Error; err != nil {
		return false, err
	}
	return true, nil
}

func (r *BomSessionRepo) UpdateImportState(ctx context.Context, sessionID string, patch biz.BOMImportStatePatch) error {
	if !r.DBOk() {
		return gorm.ErrInvalidDB
	}
	sessionID = strings.TrimSpace(sessionID)
	status := biz.NormalizeImportStatus(patch.Status)
	stage := biz.NormalizeImportStage(patch.Stage)
	progress := biz.ClampProgress(patch.Progress)
	progressExpr := gorm.Expr("GREATEST(COALESCE(import_progress, 0), ?)", progress)
	if status == biz.BOMImportStatusFailed {
		progressExpr = gorm.Expr("?", progress)
	} else if status == biz.BOMImportStatusParsing && stage == biz.BOMImportStageValidating {
		progressExpr = gorm.Expr("?", progress)
	}
	tx := r.db.WithContext(ctx).Begin()
	if tx.Error != nil {
		return tx.Error
	}
	defer func() { _ = tx.Rollback() }()
	var cur BomSession
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", sessionID).First(&cur).Error; err != nil {
		return err
	}
	if !biz.IsImportStatusTransitionAllowed(cur.ImportStatus, status) {
		return biz.ErrBOMImportStatusTransitionInvalid
	}
	updates := map[string]interface{}{
		"import_status":     status,
		"import_progress":   progressExpr,
		"import_stage":      stage,
		"import_message":    trimPtr(patch.Message),
		"import_error_code": trimPtr(patch.ErrorCode),
		"import_error":      trimPtr(patch.Error),
		"import_updated_at": gorm.Expr("CURRENT_TIMESTAMP(3)"),
		"updated_at":        gorm.Expr("CURRENT_TIMESTAMP(3)"),
	}
	if err := tx.Model(&BomSession{}).Where("id = ?", sessionID).Updates(updates).Error; err != nil {
		return err
	}
	return tx.Commit().Error
}

func normalizePlatformSlice(in []string) []string {
	seen := make(map[string]struct{})
	out := make([]string, 0, len(in))
	for _, p := range in {
		p = biz.NormalizePlatformID(p)
		if p == "" {
			continue
		}
		if _, ok := seen[p]; ok {
			continue
		}
		seen[p] = struct{}{}
		out = append(out, p)
	}
	return out
}

func parsePlatformJSON(raw string) ([]string, error) {
	var ids []string
	if strings.TrimSpace(raw) == "" {
		return ids, nil
	}
	if err := json.Unmarshal([]byte(raw), &ids); err != nil {
		return nil, err
	}
	out := normalizePlatformSlice(ids)
	return out, nil
}

func strPtrOrNil(s string) *string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	return &s
}

func derefStr(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func trimPtr(in *string) *string {
	if in == nil {
		return nil
	}
	v := strings.TrimSpace(*in)
	if v == "" {
		return nil
	}
	return &v
}

func (r *BomSessionRepo) CreateSessionLine(ctx context.Context, sessionID, mpn, mfr, pkg string, qty *float64, rawText, extraJSON *string) (lineID int64, lineNo int32, newRevision int, err error) {
	if !r.DBOk() {
		return 0, 0, 0, gorm.ErrInvalidDB
	}
	sessionID = strings.TrimSpace(sessionID)
	mpn = strings.TrimSpace(mpn)
	if mpn == "" {
		return 0, 0, 0, errors.New("bom_session_line: mpn required")
	}
	tx := r.db.WithContext(ctx).Begin()
	if tx.Error != nil {
		return 0, 0, 0, tx.Error
	}
	defer func() { _ = tx.Rollback() }()

	var sess BomSession
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", sessionID).First(&sess).Error; err != nil {
		return 0, 0, 0, err
	}
	var last BomSessionLine
	_ = tx.Where("session_id = ?", sessionID).Order("line_no DESC").First(&last).Error
	nextNo := 1
	if last.ID != 0 {
		nextNo = last.LineNo + 1
	}
	row := BomSessionLine{
		SessionID: sessionID,
		LineNo:    nextNo,
		Mpn:       mpn,
		CreatedAt: time.Now(),
	}
	if mfr != "" {
		row.Mfr = &mfr
	}
	if pkg != "" {
		row.Package = &pkg
	}
	if qty != nil {
		row.Qty = qty
	}
	if rawText != nil && *rawText != "" {
		row.RawText = rawText
	}
	if extraJSON != nil && *extraJSON != "" {
		row.ExtraJSON = []byte(*extraJSON)
	}
	if err := tx.Create(&row).Error; err != nil {
		return 0, 0, 0, err
	}
	nextRev := sess.SelectionRevision + 1
	if err := tx.Model(&BomSession{}).Where("id = ?", sessionID).Updates(map[string]interface{}{
		"selection_revision": nextRev,
		"updated_at":         gorm.Expr("CURRENT_TIMESTAMP(3)"),
	}).Error; err != nil {
		return 0, 0, 0, err
	}
	if err := tx.Commit().Error; err != nil {
		return 0, 0, 0, err
	}
	return row.ID, int32(row.LineNo), nextRev, nil
}

func (r *BomSessionRepo) DeleteSessionLine(ctx context.Context, sessionID string, lineID int64) error {
	if !r.DBOk() {
		return gorm.ErrInvalidDB
	}
	sessionID = strings.TrimSpace(sessionID)
	tx := r.db.WithContext(ctx).Begin()
	if tx.Error != nil {
		return tx.Error
	}
	defer func() { _ = tx.Rollback() }()

	var ln BomSessionLine
	if err := tx.Where("id = ? AND session_id = ?", lineID, sessionID).First(&ln).Error; err != nil {
		return err
	}
	if err := tx.Delete(&BomSessionLine{}, ln.ID).Error; err != nil {
		return err
	}
	var sess BomSession
	if err := tx.Where("id = ?", sessionID).First(&sess).Error; err != nil {
		return err
	}
	nextRev := sess.SelectionRevision + 1
	if err := tx.Model(&BomSession{}).Where("id = ?", sessionID).Updates(map[string]interface{}{
		"selection_revision": nextRev,
		"updated_at":         gorm.Expr("CURRENT_TIMESTAMP(3)"),
	}).Error; err != nil {
		return err
	}
	return tx.Commit().Error
}

func (r *BomSessionRepo) UpdateSessionLine(ctx context.Context, sessionID string, lineID int64, mpn, mfr, pkg *string, qty *float64, rawText, extraJSON *string) (newRevision int, err error) {
	if !r.DBOk() {
		return 0, gorm.ErrInvalidDB
	}
	sessionID = strings.TrimSpace(sessionID)
	tx := r.db.WithContext(ctx).Begin()
	if tx.Error != nil {
		return 0, tx.Error
	}
	defer func() { _ = tx.Rollback() }()

	var ln BomSessionLine
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND session_id = ?", lineID, sessionID).First(&ln).Error; err != nil {
		return 0, err
	}
	up := map[string]interface{}{}
	if mpn != nil {
		up["mpn"] = strings.TrimSpace(*mpn)
	}
	if mfr != nil {
		up["mfr"] = mfr
	}
	if pkg != nil {
		up["package"] = pkg
	}
	if qty != nil {
		up["qty"] = qty
	}
	if rawText != nil {
		up["raw_text"] = rawText
	}
	if extraJSON != nil {
		up["extra_json"] = []byte(*extraJSON)
	}
	if len(up) == 0 {
		_ = tx.Rollback()
		var sess BomSession
		if err := r.db.WithContext(ctx).Where("id = ?", sessionID).First(&sess).Error; err != nil {
			return 0, err
		}
		return sess.SelectionRevision, nil // 无字段变更，不递增 revision
	}
	if err := tx.Model(&BomSessionLine{}).Where("id = ?", lineID).Updates(up).Error; err != nil {
		return 0, err
	}
	var sess BomSession
	if err := tx.Where("id = ?", sessionID).First(&sess).Error; err != nil {
		return 0, err
	}
	nextRev := sess.SelectionRevision + 1
	if err := tx.Model(&BomSession{}).Where("id = ?", sessionID).Updates(map[string]interface{}{
		"selection_revision": nextRev,
		"updated_at":         gorm.Expr("CURRENT_TIMESTAMP(3)"),
	}).Error; err != nil {
		return 0, err
	}
	if err := tx.Commit().Error; err != nil {
		return 0, err
	}
	return nextRev, nil
}
