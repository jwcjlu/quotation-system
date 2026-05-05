package data

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"caichip/internal/biz"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// BOMSearchTaskRepo 仅保留 Agent 任务回写 bom_search_task / bom_quote_cache(+bom_quote_item) 所需方法（无 DB 时为部分 no-op）。
type BOMSearchTaskRepo struct {
	db    *gorm.DB
	alias biz.AliasLookup
}

// NewBOMSearchTaskRepo ...
func NewBOMSearchTaskRepo(d *Data, alias biz.AliasLookup) *BOMSearchTaskRepo {
	if d == nil || d.DB == nil {
		return &BOMSearchTaskRepo{alias: alias}
	}
	return &BOMSearchTaskRepo{db: d.DB, alias: alias}
}

// DBOk ...
func (r *BOMSearchTaskRepo) DBOk() bool {
	return r != nil && r.db != nil
}

func (r *BOMSearchTaskRepo) UpsertManualQuote(ctx context.Context, gapID uint64, row biz.AgentQuoteRow) error {
	if !r.DBOk() {
		return ErrSearchTaskNotFound
	}
	var gap BomLineGap
	if err := r.db.WithContext(ctx).Where("id = ?", gapID).First(&gap).Error; err != nil {
		return err
	}
	var session BomSession
	if err := r.db.WithContext(ctx).Where("id = ?", gap.SessionID).First(&session).Error; err != nil {
		return err
	}
	row.Seq = 1
	if strings.TrimSpace(row.QueryModel) == "" {
		row.QueryModel = gap.Mpn
	}
	quotesJSON, err := json.Marshal([]biz.AgentQuoteRow{row})
	if err != nil {
		return err
	}
	mpnNorm := normalizeMPNForSearchTask(gap.Mpn)
	if mpnNorm == "" || mpnNorm == "-" {
		mpnNorm = normalizeMPNForSearchTask(row.Model)
	}
	bizDate := session.BizDate.Format("2006-01-02")
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := r.upsertQuoteCache(ctx, tx, mpnNorm, "manual", bizDate, "ok", quotesJSON, nil); err != nil {
			return err
		}
		var cacheID uint64
		if err := tx.Model(&BomQuoteCache{}).
			Where("mpn_norm = ? AND platform_id = ? AND biz_date = ?", mpnNorm, "manual", session.BizDate).
			Select("id").
			Take(&cacheID).Error; err != nil {
			return err
		}
		if err := tx.Model(&BomQuoteCache{}).
			Where("mpn_norm = ? AND platform_id = ? AND biz_date = ?", mpnNorm, "manual", session.BizDate).
			Updates(map[string]any{
				"source_type": "manual",
				"session_id":  gap.SessionID,
				"line_id":     gap.LineID,
			}).Error; err != nil {
			return err
		}
		return tx.Model(&BomQuoteItem{}).
			Where("quote_id = ?", cacheID).
			Updates(map[string]any{
				"source_type": "manual",
				"session_id":  gap.SessionID,
				"line_id":     gap.LineID,
			}).Error
	})
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

// sanitizeForLegacyMySQLUTF8 过滤 4-byte rune，兼容 utf8/utf8mb3 列。
// 说明：若库已是 utf8mb4，此清洗会损失少量字符，但可避免整批写入失败。
func sanitizeForLegacyMySQLUTF8(s string) string {
	if s == "" || !utf8.ValidString(s) {
		return s
	}
	need := false
	for _, r := range s {
		if r > 0xFFFF {
			need = true
			break
		}
	}
	if !need {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if r <= 0xFFFF {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func (r *BOMSearchTaskRepo) manufacturerCanonicalPtrForQuote(ctx context.Context, manufacturer string) (*string, error) {
	if r == nil || r.alias == nil {
		return nil, nil
	}
	id, hit, err := biz.ResolveManufacturerCanonical(ctx, manufacturer, r.alias)
	if err != nil {
		// 报价落库不应因别名基础设施抖动而整体失败：降级为未写入 canonical。
		return nil, nil
	}
	if !hit {
		return nil, nil
	}
	cp := id
	return &cp, nil
}

func (r *BOMSearchTaskRepo) upsertQuoteCache(ctx context.Context, x *gorm.DB, mpnNorm, platformID, dateStr, outcome string, quotesJSON, noMpnDetail []byte) error {
	bd, err := time.ParseInLocation("2006-01-02", dateStr, time.Local)
	if err != nil {
		return err
	}
	var nd interface{}
	if len(noMpnDetail) > 0 {
		nd = noMpnDetail
	}
	now := time.Now()
	cache := &BomQuoteCache{
		MpnNorm:     mpnNorm,
		PlatformID:  platformID,
		BizDate:     bd,
		Outcome:     outcome,
		NoMpnDetail: noMpnDetail,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := x.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns: []clause.Column{
				{Name: "mpn_norm"},
				{Name: "platform_id"},
				{Name: "biz_date"},
			},
			DoUpdates: clause.Assignments(map[string]interface{}{
				"outcome":       outcome,
				"no_mpn_detail": nd,
				"updated_at":    gorm.Expr("CURRENT_TIMESTAMP(3)"),
			}),
		}).
		Create(cache).Error; err != nil {
		return err
	}
	var cacheID uint64
	if err := x.WithContext(ctx).
		Model(&BomQuoteCache{}).
		Where("mpn_norm = ? AND platform_id = ? AND biz_date = ?", mpnNorm, platformID, bd).
		Select("id").
		Take(&cacheID).Error; err != nil {
		return err
	}
	if cacheID == 0 {
		return errors.New("upsertQuoteCache: cache id not found after upsert")
	}
	if err := x.WithContext(ctx).Where("quote_id = ?", cacheID).Delete(&BomQuoteItem{}).Error; err != nil {
		return err
	}
	if len(quotesJSON) == 0 {
		return nil
	}
	var rows []biz.AgentQuoteRow
	if err := json.Unmarshal(quotesJSON, &rows); err != nil {
		return err
	}
	items := make([]BomQuoteItem, 0, len(rows))
	for i := range rows {
		row := rows[i]
		mfr := sanitizeForLegacyMySQLUTF8(row.Manufacturer)
		canonPtr, _ := r.manufacturerCanonicalPtrForQuote(ctx, mfr)
		manufacturerReviewStatus := biz.MfrReviewPending
		if canonPtr != nil && len(*canonPtr) > 0 {
			manufacturerReviewStatus = biz.MfrReviewAccepted
		}
		items = append(items, BomQuoteItem{
			QuoteID:                  cacheID,
			Model:                    sanitizeForLegacyMySQLUTF8(row.Model),
			Manufacturer:             mfr,
			ManufacturerCanonicalID:  canonPtr,
			Stock:                    sanitizeForLegacyMySQLUTF8(row.Stock),
			Package:                  sanitizeForLegacyMySQLUTF8(row.Package),
			Desc:                     sanitizeForLegacyMySQLUTF8(row.Desc),
			MOQ:                      sanitizeForLegacyMySQLUTF8(row.MOQ),
			LeadTime:                 sanitizeForLegacyMySQLUTF8(row.LeadTime),
			PriceTiers:               sanitizeForLegacyMySQLUTF8(row.PriceTiers),
			HKPrice:                  sanitizeForLegacyMySQLUTF8(row.HKPrice),
			MainlandPrice:            sanitizeForLegacyMySQLUTF8(row.MainlandPrice),
			QueryModel:               sanitizeForLegacyMySQLUTF8(row.QueryModel),
			DatasheetURL:             sanitizeForLegacyMySQLUTF8(row.DatasheetURL),
			CreatedAt:                now,
			UpdatedAt:                now,
			ManufacturerReviewStatus: manufacturerReviewStatus,
		})
	}
	if len(items) > 0 {
		if err := x.WithContext(ctx).Create(&items).Error; err != nil {
			return err
		}
	}
	return nil
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

// LoadQuoteCacheByMergeKey 读取报价缓存（明细来自 t_bom_quote_item）；无行则 ok=false。
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
	type cacheRow struct {
		ID          uint64 `gorm:"column:id"`
		Outcome     string `gorm:"column:outcome"`
		NoMpnDetail []byte `gorm:"column:no_mpn_detail"`
	}
	var row cacheRow
	err = r.db.WithContext(ctx).
		Model(&BomQuoteCache{}).
		Select("id, outcome, no_mpn_detail").
		Where("mpn_norm = ? AND platform_id = ? AND biz_date = ?", mpnNorm, platformID, bd).
		Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	qj, err := r.loadQuoteRowsJSONByCacheID(ctx, row.ID)
	if err != nil {
		return nil, false, err
	}
	nd := row.NoMpnDetail
	return &biz.QuoteCacheSnapshot{
		Outcome:     row.Outcome,
		QuotesJSON:  qj,
		NoMpnDetail: nd,
	}, true, nil
}

// LoadQuoteCachesForKeys 同一 biz_date 下批量读取 t_bom_quote_cache 并装配 t_bom_quote_item，减少配单 N×M 次往返。
func (r *BOMSearchTaskRepo) LoadQuoteCachesForKeys(ctx context.Context, bizDate time.Time, pairs []biz.MpnPlatformPair) (map[string]*biz.QuoteCacheSnapshot, error) {
	out := make(map[string]*biz.QuoteCacheSnapshot)
	if !r.DBOk() || len(pairs) == 0 {
		return out, nil
	}
	dateStr := bizDate.Format("2006-01-02")
	bd, err := time.ParseInLocation("2006-01-02", dateStr, time.Local)
	if err != nil {
		return nil, err
	}
	seen := make(map[string]struct{}, len(pairs))
	var uniq []biz.MpnPlatformPair
	for _, p := range pairs {
		mn := normalizeMPNForSearchTask(p.MpnNorm)
		pid := strings.TrimSpace(p.PlatformID)
		k := mn + "\x00" + pid
		if _, ok := seen[k]; ok {
			continue
		}
		seen[k] = struct{}{}
		uniq = append(uniq, biz.MpnPlatformPair{MpnNorm: mn, PlatformID: pid})
	}
	if len(uniq) == 0 {
		return out, nil
	}
	q := r.db.WithContext(ctx).Table(TableBomQuoteCache).Where("biz_date = ?", bd)
	{
		var parts []string
		var args []interface{}
		for _, p := range uniq {
			parts = append(parts, "(mpn_norm = ? AND platform_id = ?)")
			args = append(args, p.MpnNorm, p.PlatformID)
		}
		q = q.Where(strings.Join(parts, " OR "), args...)
	}
	type cacheRow struct {
		ID          uint64 `gorm:"column:id"`
		MpnNorm     string `gorm:"column:mpn_norm"`
		PlatformID  string `gorm:"column:platform_id"`
		Outcome     string `gorm:"column:outcome"`
		NoMpnDetail []byte `gorm:"column:no_mpn_detail"`
	}
	var rows []cacheRow
	if err := q.Find(&rows).Error; err != nil {
		return nil, err
	}
	quoteIDs := make([]uint64, 0, len(rows))
	for i := range rows {
		if rows[i].ID != 0 {
			quoteIDs = append(quoteIDs, rows[i].ID)
		}
	}
	quoteJSONByID, err := r.loadQuoteRowsJSONByCacheIDs(ctx, quoteIDs)
	if err != nil {
		return nil, err
	}
	for i := range rows {
		row := &rows[i]
		key := row.MpnNorm + "\x00" + row.PlatformID
		qj := quoteJSONByID[row.ID]
		if len(qj) == 0 {
			qj = []byte("[]")
		}
		nd := row.NoMpnDetail
		out[key] = &biz.QuoteCacheSnapshot{
			Outcome:     row.Outcome,
			QuotesJSON:  qj,
			NoMpnDetail: nd,
		}
	}
	return out, nil
}

type quoteItemRow struct {
	QuoteID                 uint64  `gorm:"column:quote_id"`
	Model                   string  `gorm:"column:model"`
	Manufacturer            string  `gorm:"column:manufacturer"`
	ManufacturerCanonicalID *string `gorm:"column:manufacturer_canonical_id"`
	Package                 string  `gorm:"column:package"`
	Desc                    string  `gorm:"column:desc"`
	Stock                   string  `gorm:"column:stock"`
	MOQ                     string  `gorm:"column:moq"`
	PriceTiers              string  `gorm:"column:price_tiers"`
	HKPrice                 string  `gorm:"column:hk_price"`
	MainlandPrice           string  `gorm:"column:mainland_price"`
	LeadTime                string  `gorm:"column:lead_time"`
	QueryModel              string  `gorm:"column:query_model"`
	DatasheetURL            string  `gorm:"column:datasheet_url"`
}

func (r *BOMSearchTaskRepo) loadQuoteRowsJSONByCacheID(ctx context.Context, quoteID uint64) ([]byte, error) {
	if quoteID == 0 {
		return []byte("[]"), nil
	}
	rowsByID, err := r.loadQuoteRowsJSONByCacheIDs(ctx, []uint64{quoteID})
	if err != nil {
		return nil, err
	}
	if raw := rowsByID[quoteID]; len(raw) > 0 {
		return raw, nil
	}
	return []byte("[]"), nil
}

func (r *BOMSearchTaskRepo) loadQuoteRowsJSONByCacheIDs(ctx context.Context, quoteIDs []uint64) (map[uint64][]byte, error) {
	out := make(map[uint64][]byte, len(quoteIDs))
	if len(quoteIDs) == 0 {
		return out, nil
	}
	seen := make(map[uint64]struct{}, len(quoteIDs))
	uniq := make([]uint64, 0, len(quoteIDs))
	for _, id := range quoteIDs {
		if id == 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		uniq = append(uniq, id)
	}
	if len(uniq) == 0 {
		return out, nil
	}
	var items []quoteItemRow
	if err := r.db.WithContext(ctx).
		Model(&BomQuoteItem{}).
		Select("quote_id, model, manufacturer, manufacturer_canonical_id, package, `desc`, stock, moq, price_tiers, hk_price, mainland_price, lead_time, query_model, datasheet_url").
		Where("quote_id IN ?", uniq).
		Order("quote_id ASC, id ASC").
		Find(&items).Error; err != nil {
		return nil, err
	}
	return quoteItemRowsJSONByCacheID(uniq, items)
}

func quoteItemRowsJSONByCacheID(quoteIDs []uint64, items []quoteItemRow) (map[uint64][]byte, error) {
	grouped := make(map[uint64][]biz.AgentQuoteRow, len(quoteIDs))
	for _, id := range quoteIDs {
		if id != 0 {
			grouped[id] = nil
		}
	}
	for _, it := range items {
		rows := grouped[it.QuoteID]
		rows = append(rows, biz.AgentQuoteRow{
			Seq:                     len(rows) + 1,
			Model:                   it.Model,
			Manufacturer:            it.Manufacturer,
			ManufacturerCanonicalID: it.ManufacturerCanonicalID,
			Package:                 it.Package,
			Desc:                    it.Desc,
			Stock:                   it.Stock,
			MOQ:                     it.MOQ,
			PriceTiers:              it.PriceTiers,
			HKPrice:                 it.HKPrice,
			MainlandPrice:           it.MainlandPrice,
			LeadTime:                it.LeadTime,
			QueryModel:              it.QueryModel,
			DatasheetURL:            it.DatasheetURL,
		})
		grouped[it.QuoteID] = rows
	}
	out := make(map[uint64][]byte, len(grouped))
	for quoteID, rows := range grouped {
		if len(rows) == 0 {
			out[quoteID] = []byte("[]")
			continue
		}
		raw, err := json.Marshal(rows)
		if err != nil {
			return nil, err
		}
		out[quoteID] = raw
	}
	return out, nil
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
	err := r.db.WithContext(ctx).
		Model(&BomSearchTask{}).
		Select("mpn_norm, platform_id, biz_date").
		Where("session_id = ? AND state = ?", sessionID, "pending").
		Group("mpn_norm, platform_id, biz_date").
		Find(&rows).Error
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
