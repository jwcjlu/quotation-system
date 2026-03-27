package data

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"caichip/internal/biz"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// BOMSearchTaskRepo 仅保留 Agent 任务回写 bom_search_task / bom_quote_cache 所需方法（无 DB 时为部分 no-op）。
type BOMSearchTaskRepo struct {
	db *gorm.DB
}

// NewBOMSearchTaskRepo ...
func NewBOMSearchTaskRepo(d *Data) *BOMSearchTaskRepo {
	if d == nil || d.DB == nil {
		return &BOMSearchTaskRepo{}
	}
	return &BOMSearchTaskRepo{db: d.DB}
}

// DBOk ...
func (r *BOMSearchTaskRepo) DBOk() bool {
	return r != nil && r.db != nil
}

var (
	// ErrSearchTaskNotFound 会话下无对应搜索任务行。
	ErrSearchTaskNotFound = errors.New("bom_search_task not found")
	// ErrSearchTaskCaichipMismatch 回传的 caichip_task_id 与库内不一致。
	ErrSearchTaskCaichipMismatch = errors.New("bom_search_task caichip_task_id mismatch")
)

// LoadSearchTaskByCaichipTaskID 根据调度下发的 caichip_task_id（cloud uuid）查一行；无匹配返回 (nil, nil)。
func (r *BOMSearchTaskRepo) LoadSearchTaskByCaichipTaskID(ctx context.Context, caichipTaskID string) (*biz.BOMSearchTaskLookup, error) {
	if !r.DBOk() {
		return nil, nil
	}
	caichipTaskID = strings.TrimSpace(caichipTaskID)
	if caichipTaskID == "" {
		return nil, nil
	}
	var task BomSearchTask
	err := r.db.WithContext(ctx).Where("caichip_task_id = ?", caichipTaskID).First(&task).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if task.BizDate.IsZero() {
		return nil, errors.New("bom_search_task biz_date missing")
	}
	return &biz.BOMSearchTaskLookup{
		SessionID:  task.SessionID,
		MpnNorm:    task.MpnNorm,
		PlatformID: task.PlatformID,
		BizDate:    task.BizDate,
	}, nil
}

var _ biz.BOMSearchTaskRepo = (*BOMSearchTaskRepo)(nil)

func normalizeMPNForSearchTask(mpn string) string {
	m := strings.TrimSpace(mpn)
	if m == "" {
		return "-"
	}
	return strings.ToUpper(m)
}

func (r *BOMSearchTaskRepo) upsertQuoteCache(ctx context.Context, x *gorm.DB, mpnNorm, platformID, dateStr, outcome string, quotesJSON, noMpnDetail []byte) error {
	bd, err := time.ParseInLocation("2006-01-02", dateStr, time.Local)
	if err != nil {
		return err
	}
	var qj, nd interface{}
	if len(quotesJSON) > 0 {
		qj = quotesJSON
	}
	if len(noMpnDetail) > 0 {
		nd = noMpnDetail
	}
	cache := BomQuoteCache{
		MpnNorm:     mpnNorm,
		PlatformID:  platformID,
		BizDate:     bd,
		Outcome:     outcome,
		QuotesJSON:  quotesJSON,
		NoMpnDetail: noMpnDetail,
	}
	return x.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{
			{Name: "mpn_norm"},
			{Name: "platform_id"},
			{Name: "biz_date"},
		},
		DoUpdates: clause.Assignments(map[string]interface{}{
			"outcome":       outcome,
			"quotes_json":   qj,
			"no_mpn_detail": nd,
			"updated_at":    gorm.Expr("CURRENT_TIMESTAMP(3)"),
		}),
	}).Create(&cache).Error
}

// FinalizeSearchTask 事务内校验任务行、更新 state（并视状态 UPSERT 报价缓存）。
func (r *BOMSearchTaskRepo) FinalizeSearchTask(ctx context.Context, sessionID, mpnNorm, platformID string, bizDate time.Time, caichipTaskID, state string, lastErr *string, quoteOutcome string, quotesJSON, noMpnDetail []byte) error {
	if !r.DBOk() {
		return ErrSearchTaskNotFound
	}
	sessionID = strings.TrimSpace(sessionID)
	mpnNorm = normalizeMPNForSearchTask(mpnNorm)
	platformID = strings.TrimSpace(platformID)
	dateStr := bizDate.Format("2006-01-02")
	st := strings.TrimSpace(strings.ToLower(state))

	tx := r.db.WithContext(ctx).Begin()
	if tx.Error != nil {
		return tx.Error
	}
	defer func() { _ = tx.Rollback() }()

	var task BomSearchTask
	err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("session_id = ? AND mpn_norm = ? AND platform_id = ? AND biz_date = ?", sessionID, mpnNorm, platformID, dateStr).
		First(&task).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrSearchTaskNotFound
	}
	if err != nil {
		return err
	}

	cid := strings.TrimSpace(caichipTaskID)
	if cid != "" && task.CaichipTaskID.Valid && strings.TrimSpace(task.CaichipTaskID.String) != "" {
		if strings.TrimSpace(task.CaichipTaskID.String) != cid {
			return ErrSearchTaskCaichipMismatch
		}
	}

	var lastErrArg interface{}
	if lastErr != nil {
		lastErrArg = *lastErr
	}

	upd := map[string]interface{}{
		"state":      st,
		"last_error": lastErrArg,
		"updated_at": gorm.Expr("CURRENT_TIMESTAMP(3)"),
	}
	if st == "failed_retryable" || st == "failed_terminal" {
		upd["auto_attempt"] = gorm.Expr("auto_attempt + 1")
	}
	if err := tx.Model(&BomSearchTask{}).
		Where("session_id = ? AND mpn_norm = ? AND platform_id = ? AND biz_date = ?", sessionID, mpnNorm, platformID, dateStr).
		Updates(upd).Error; err != nil {
		return err
	}

	switch st {
	case "succeeded", "succeeded_quotes":
		oc := strings.TrimSpace(quoteOutcome)
		if oc == "" {
			oc = "ok"
		}
		if err := r.upsertQuoteCache(ctx, tx, mpnNorm, platformID, dateStr, oc, quotesJSON, noMpnDetail); err != nil {
			return err
		}
	case "no_result", "succeeded_no_mpn":
		if err := r.upsertQuoteCache(ctx, tx, mpnNorm, platformID, dateStr, "no_mpn_match", nil, noMpnDetail); err != nil {
			return err
		}
	}
	return tx.Commit().Error
}

func (r *BOMSearchTaskRepo) snapshotRows(rows []BomSearchTask) []biz.TaskReadinessSnapshot {
	out := make([]biz.TaskReadinessSnapshot, 0, len(rows))
	for _, t := range rows {
		out = append(out, biz.TaskReadinessSnapshot{
			MpnNorm:    t.MpnNorm,
			PlatformID: t.PlatformID,
			State:      strings.ToLower(strings.TrimSpace(t.State)),
		})
	}
	return out
}

// ListTasksForSession 会话下全部搜索任务（就绪聚合用）。
func (r *BOMSearchTaskRepo) ListTasksForSession(ctx context.Context, sessionID string) ([]biz.TaskReadinessSnapshot, error) {
	if !r.DBOk() {
		return nil, nil
	}
	var rows []BomSearchTask
	err := r.db.WithContext(ctx).Where("session_id = ?", strings.TrimSpace(sessionID)).Find(&rows).Error
	if err != nil {
		return nil, err
	}
	return r.snapshotRows(rows), nil
}

// ListActiveBySession state IN (pending, running, failed_retryable)。
func (r *BOMSearchTaskRepo) ListActiveBySession(ctx context.Context, sessionID string) ([]biz.TaskReadinessSnapshot, error) {
	if !r.DBOk() {
		return nil, nil
	}
	var rows []BomSearchTask
	err := r.db.WithContext(ctx).
		Where("session_id = ? AND state IN ?", strings.TrimSpace(sessionID), []string{"pending", "running", "failed_retryable"}).
		Find(&rows).Error
	if err != nil {
		return nil, err
	}
	return r.snapshotRows(rows), nil
}

// CancelBySessionPlatform 未完成平台任务标为 cancelled（设计 §5 会话级关闭平台）。
func (r *BOMSearchTaskRepo) CancelBySessionPlatform(ctx context.Context, sessionID, platformID string) (int64, error) {
	if !r.DBOk() {
		return 0, nil
	}
	tx := r.db.WithContext(ctx).Model(&BomSearchTask{}).
		Where("session_id = ? AND platform_id = ? AND state IN ?", strings.TrimSpace(sessionID), strings.TrimSpace(platformID),
			[]string{"pending", "running", "failed_retryable"}).
		Updates(map[string]interface{}{
			"state":      "cancelled",
			"updated_at": gorm.Expr("CURRENT_TIMESTAMP(3)"),
		})
	return tx.RowsAffected, tx.Error
}

// MarkSkippedBySessionPlatform 未完成平台任务标为 skipped。
func (r *BOMSearchTaskRepo) MarkSkippedBySessionPlatform(ctx context.Context, sessionID, platformID string) (int64, error) {
	if !r.DBOk() {
		return 0, nil
	}
	tx := r.db.WithContext(ctx).Model(&BomSearchTask{}).
		Where("session_id = ? AND platform_id = ? AND state IN ?", strings.TrimSpace(sessionID), strings.TrimSpace(platformID),
			[]string{"pending", "running", "failed_retryable"}).
		Updates(map[string]interface{}{
			"state":      "skipped",
			"updated_at": gorm.Expr("CURRENT_TIMESTAMP(3)"),
		})
	return tx.RowsAffected, tx.Error
}

// CancelTasksBySessionMpnNorm 删除/改行时作废该型号下全部平台任务。
func (r *BOMSearchTaskRepo) CancelTasksBySessionMpnNorm(ctx context.Context, sessionID, mpnNorm string) error {
	if !r.DBOk() {
		return nil
	}
	mn := normalizeMPNForSearchTask(mpnNorm)
	return r.db.WithContext(ctx).Model(&BomSearchTask{}).
		Where("session_id = ? AND mpn_norm = ?", strings.TrimSpace(sessionID), mn).
		Updates(map[string]interface{}{
			"state":      "cancelled",
			"updated_at": gorm.Expr("CURRENT_TIMESTAMP(3)"),
		}).Error
}

// CancelAllTasksBySession 导入全量替换前作废全部任务行（设计 §4，不物理删）。
func (r *BOMSearchTaskRepo) CancelAllTasksBySession(ctx context.Context, sessionID string) error {
	if !r.DBOk() {
		return nil
	}
	return r.db.WithContext(ctx).Model(&BomSearchTask{}).
		Where("session_id = ?", strings.TrimSpace(sessionID)).
		Updates(map[string]interface{}{
			"state":      "cancelled",
			"updated_at": gorm.Expr("CURRENT_TIMESTAMP(3)"),
		}).Error
}

// UpsertPendingTasks 为 (mpn_norm, platform) 写入 pending；冲突则重置为 pending 并清调度键。
func (r *BOMSearchTaskRepo) UpsertPendingTasks(ctx context.Context, sessionID string, bizDate time.Time, selectionRevision int, pairs []biz.MpnPlatformPair) error {
	if !r.DBOk() || len(pairs) == 0 {
		return nil
	}
	sessionID = strings.TrimSpace(sessionID)
	for _, p := range pairs {
		mn := normalizeMPNForSearchTask(p.MpnNorm)
		pid := strings.TrimSpace(p.PlatformID)
		if mn == "" || mn == "-" || pid == "" {
			continue
		}
		row := BomSearchTask{
			SessionID:         sessionID,
			MpnNorm:           mn,
			PlatformID:        pid,
			BizDate:           bizDate,
			State:             "pending",
			AutoAttempt:       0,
			ManualAttempt:     0,
			SelectionRevision: selectionRevision,
			CreatedAt:         time.Now(),
			UpdatedAt:         time.Now(),
		}
		if err := r.db.WithContext(ctx).Clauses(clause.OnConflict{
			Columns: []clause.Column{
				{Name: "session_id"},
				{Name: "mpn_norm"},
				{Name: "platform_id"},
				{Name: "biz_date"},
			},
			DoUpdates: clause.Assignments(map[string]interface{}{
				"state":              "pending",
				"selection_revision": selectionRevision,
				"last_error":         gorm.Expr("NULL"),
				"caichip_task_id":    gorm.Expr("NULL"),
				"updated_at":         gorm.Expr("CURRENT_TIMESTAMP(3)"),
			}),
		}).Create(&row).Error; err != nil {
			return err
		}
	}
	return nil
}

// GetTaskStateBySessionKey 返回当前任务 state，无行则 ("", nil)。
func (r *BOMSearchTaskRepo) GetTaskStateBySessionKey(ctx context.Context, sessionID, mpnNorm, platformID string, bizDate time.Time) (string, error) {
	if !r.DBOk() {
		return "", nil
	}
	sessionID = strings.TrimSpace(sessionID)
	mpnNorm = normalizeMPNForSearchTask(mpnNorm)
	platformID = strings.TrimSpace(platformID)
	dateStr := bizDate.Format("2006-01-02")
	var task BomSearchTask
	err := r.db.WithContext(ctx).
		Where("session_id = ? AND mpn_norm = ? AND platform_id = ? AND biz_date = ?", sessionID, mpnNorm, platformID, dateStr).
		First(&task).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return strings.ToLower(strings.TrimSpace(task.State)), nil
}

// UpdateTaskStateBySessionKey 仅更新 state（重试/管理用）。
func (r *BOMSearchTaskRepo) UpdateTaskStateBySessionKey(ctx context.Context, sessionID, mpnNorm, platformID string, bizDate time.Time, state string) error {
	if !r.DBOk() {
		return nil
	}
	sessionID = strings.TrimSpace(sessionID)
	mpnNorm = normalizeMPNForSearchTask(mpnNorm)
	platformID = strings.TrimSpace(platformID)
	st := strings.ToLower(strings.TrimSpace(state))
	dateStr := bizDate.Format("2006-01-02")
	return r.db.WithContext(ctx).Model(&BomSearchTask{}).
		Where("session_id = ? AND mpn_norm = ? AND platform_id = ? AND biz_date = ?", sessionID, mpnNorm, platformID, dateStr).
		Updates(map[string]interface{}{
			"state":      st,
			"updated_at": gorm.Expr("CURRENT_TIMESTAMP(3)"),
		}).Error
}

func taskToLookup(t *BomSearchTask) biz.BOMSearchTaskLookup {
	return biz.BOMSearchTaskLookup{
		SessionID:  t.SessionID,
		MpnNorm:    t.MpnNorm,
		PlatformID: t.PlatformID,
		BizDate:    t.BizDate,
	}
}

// ListSearchTaskLookupsByCaichipTaskID 同 caichip_task_id 的全部业务行（fan-out）。
func (r *BOMSearchTaskRepo) ListSearchTaskLookupsByCaichipTaskID(ctx context.Context, caichipTaskID string) ([]biz.BOMSearchTaskLookup, error) {
	if !r.DBOk() {
		return nil, nil
	}
	caichipTaskID = strings.TrimSpace(caichipTaskID)
	if caichipTaskID == "" {
		return nil, nil
	}
	var rows []BomSearchTask
	err := r.db.WithContext(ctx).Where("caichip_task_id = ?", caichipTaskID).Find(&rows).Error
	if err != nil {
		return nil, err
	}
	out := make([]biz.BOMSearchTaskLookup, 0, len(rows))
	for i := range rows {
		if rows[i].BizDate.IsZero() {
			continue
		}
		out = append(out, taskToLookup(&rows[i]))
	}
	return out, nil
}

// ListPendingLookupsByMergeKey 合并键下仍为 pending 的任务（含多会话）。
func (r *BOMSearchTaskRepo) ListPendingLookupsByMergeKey(ctx context.Context, mpnNorm, platformID string, bizDate time.Time) ([]biz.BOMSearchTaskLookup, error) {
	if !r.DBOk() {
		return nil, nil
	}
	mpnNorm = normalizeMPNForSearchTask(mpnNorm)
	platformID = strings.TrimSpace(platformID)
	dateStr := bizDate.Format("2006-01-02")
	var rows []BomSearchTask
	err := r.db.WithContext(ctx).
		Where("mpn_norm = ? AND platform_id = ? AND biz_date = ? AND state = ?", mpnNorm, platformID, dateStr, "pending").
		Find(&rows).Error
	if err != nil {
		return nil, err
	}
	out := make([]biz.BOMSearchTaskLookup, 0, len(rows))
	for i := range rows {
		if rows[i].BizDate.IsZero() {
			continue
		}
		out = append(out, taskToLookup(&rows[i]))
	}
	return out, nil
}

// LoadQuoteCacheByMergeKey 读取报价缓存；无行则 ok=false。
func (r *BOMSearchTaskRepo) LoadQuoteCacheByMergeKey(ctx context.Context, mpnNorm, platformID string, bizDate time.Time) (*biz.QuoteCacheSnapshot, bool, error) {
	if !r.DBOk() {
		return nil, false, nil
	}
	mpnNorm = normalizeMPNForSearchTask(mpnNorm)
	platformID = strings.TrimSpace(platformID)
	dateStr := bizDate.Format("2006-01-02")
	bd, err := time.ParseInLocation("2006-01-02", dateStr, time.Local)
	if err != nil {
		return nil, false, err
	}
	var row BomQuoteCache
	err = r.db.WithContext(ctx).
		Where("mpn_norm = ? AND platform_id = ? AND biz_date = ?", mpnNorm, platformID, bd).
		First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	qj := row.QuotesJSON
	nd := row.NoMpnDetail
	return &biz.QuoteCacheSnapshot{
		Outcome:     row.Outcome,
		QuotesJSON:  qj,
		NoMpnDetail: nd,
	}, true, nil
}

// DistinctPendingMergeKeysForSession 会话内 pending 任务涉及的合并键去重。
func (r *BOMSearchTaskRepo) DistinctPendingMergeKeysForSession(ctx context.Context, sessionID string) ([]biz.MergeKey, error) {
	if !r.DBOk() {
		return nil, nil
	}
	sessionID = strings.TrimSpace(sessionID)
	type keyRow struct {
		MpnNorm    string    `gorm:"column:mpn_norm"`
		PlatformID string    `gorm:"column:platform_id"`
		BizDate    time.Time `gorm:"column:biz_date"`
	}
	var rows []keyRow
	err := r.db.WithContext(ctx).Raw(fmt.Sprintf(`
SELECT mpn_norm, platform_id, biz_date FROM %s
WHERE session_id = ? AND state = 'pending'
GROUP BY mpn_norm, platform_id, biz_date`, TableBomSearchTask), sessionID).Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	out := make([]biz.MergeKey, 0, len(rows))
	for _, row := range rows {
		out = append(out, biz.MergeKey{
			MpnNorm:    row.MpnNorm,
			PlatformID: row.PlatformID,
			BizDate:    row.BizDate,
		})
	}
	return out, nil
}
