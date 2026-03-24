package data

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"caichip/internal/biz"
	"caichip/internal/conf"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// BOMSearchTaskRepo bom_search_task / bom_quote_cache 访问（无 DB 时为 no-op）。
type BOMSearchTaskRepo struct {
	db       *gorm.DB
	dispatch *DispatchTaskRepo
	bc       *conf.Bootstrap
}

// NewBOMSearchTaskRepo ...
func NewBOMSearchTaskRepo(d *Data, dispatch *DispatchTaskRepo, bc *conf.Bootstrap) *BOMSearchTaskRepo {
	if d == nil || d.DB == nil {
		return &BOMSearchTaskRepo{bc: bc}
	}
	return &BOMSearchTaskRepo{db: d.DB, dispatch: dispatch, bc: bc}
}

func mysqlDispatchEnabled(bc *conf.Bootstrap) bool {
	if bc == nil || bc.Agent == nil {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(bc.Agent.DispatchStore), "mysql")
}

// DBOk ...
func (r *BOMSearchTaskRepo) DBOk() bool {
	return r != nil && r.db != nil
}

var _ biz.BOMSearchTaskEnsurer = (*BOMSearchTaskRepo)(nil)

var (
	// ErrSearchTaskNotFound 会话下无对应搜索任务行。
	ErrSearchTaskNotFound = errors.New("bom_search_task not found")
	// ErrSearchTaskCaichipMismatch 回传的 caichip_task_id 与库内不一致。
	ErrSearchTaskCaichipMismatch = errors.New("bom_search_task caichip_task_id mismatch")
)

// SearchTaskRow 任务一行（API 聚合用）。
type SearchTaskRow struct {
	MpnNorm           string
	PlatformID        string
	State             string
	AutoAttempt       int
	ManualAttempt     int
	LastError         sql.NullString
	SelectionRevision int
}

// BOMSearchTaskRowKey 按 caichip_task_id 定位 bom 搜索行（Agent TaskResult 回写缓存用）。
type BOMSearchTaskRowKey struct {
	SessionID  string
	MpnNorm    string
	PlatformID string
	BizDate    time.Time
}

// LoadSearchTaskByCaichipTaskID 根据调度下发的 caichip_task_id（cloud uuid）查一行；无匹配返回 (nil, nil)。
func (r *BOMSearchTaskRepo) LoadSearchTaskByCaichipTaskID(ctx context.Context, caichipTaskID string) (*BOMSearchTaskRowKey, error) {
	if !r.DBOk() {
		return nil, nil
	}
	caichipTaskID = strings.TrimSpace(caichipTaskID)
	if caichipTaskID == "" {
		return nil, nil
	}
	var sid, mpn, pid string
	var bizStr sql.NullString
	err := r.db.WithContext(ctx).Raw(`
SELECT session_id, mpn_norm, platform_id, biz_date
FROM bom_search_task WHERE caichip_task_id = ? LIMIT 1`, caichipTaskID).Row().Scan(&sid, &mpn, &pid, &bizStr)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if err := scanBizDateValid(bizStr); err != nil {
		return nil, err
	}
	bd, err := parseDateTimeForScan(bizStr.String)
	if err != nil {
		return nil, err
	}
	return &BOMSearchTaskRowKey{
		SessionID:  sid,
		MpnNorm:    mpn,
		PlatformID: pid,
		BizDate:    bd,
	}, nil
}

func scanBizDateValid(bizStr sql.NullString) error {
	if !bizStr.Valid || strings.TrimSpace(bizStr.String) == "" {
		return fmt.Errorf("bom_search_task biz_date missing")
	}
	return nil
}

func (r *BOMSearchTaskRepo) loadSessionTaskMeta(ctx context.Context, sessionID string) (bizDate time.Time, rev int, platforms []string, err error) {
	if r.db == nil {
		return time.Time{}, 0, nil, sql.ErrNoRows
	}
	sessionID = strings.TrimSpace(sessionID)
	var platformRaw []byte
	var bizStr sql.NullString
	err = r.db.WithContext(ctx).Raw(`
SELECT biz_date, selection_revision, platform_ids
FROM bom_session WHERE id = ?`, sessionID).Row().Scan(&bizStr, &rev, &platformRaw)
	if err != nil {
		return time.Time{}, 0, nil, err
	}
	if bizStr.Valid {
		bizDate, err = parseDateTimeForScan(bizStr.String)
		if err != nil {
			return time.Time{}, 0, nil, fmt.Errorf("biz_date: %w", err)
		}
	}
	if len(platformRaw) > 0 {
		_ = json.Unmarshal(platformRaw, &platforms)
	}
	if platforms == nil {
		platforms = []string{}
	}
	return bizDate, rev, platforms, nil
}

// EnsureTasksForSession 按当前会话 platform_ids × 行 MPN 写入/刷新 bom_search_task（幂等 upsert）。
func (r *BOMSearchTaskRepo) EnsureTasksForSession(ctx context.Context, sessionID string) error {
	if !r.DBOk() {
		return nil
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil
	}
	bizDate, rev, platforms, err := r.loadSessionTaskMeta(ctx, sessionID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return biz.ErrBOMSessionNotFound
		}
		return err
	}
	if len(platforms) == 0 {
		return nil
	}

	var lines []string
	rows, qErr := r.db.WithContext(ctx).Raw(`
SELECT mpn FROM bom_session_line WHERE session_id = ? ORDER BY line_no`, sessionID).Rows()
	if qErr != nil {
		return qErr
	}
	defer rows.Close()
	for rows.Next() {
		var mpn string
		if err = rows.Scan(&mpn); err != nil {
			return err
		}
		lines = append(lines, biz.NormalizeMPNForTask(mpn))
	}
	if err = rows.Err(); err != nil {
		return err
	}

	dateStr := bizDate.Format("2006-01-02")
	for _, mpnNorm := range lines {
		for _, pid := range platforms {
			p := strings.TrimSpace(pid)
			if p == "" {
				continue
			}
			taskCloudID := uuid.NewString()
			err = r.db.WithContext(ctx).Exec(`
INSERT INTO bom_search_task (session_id, mpn_norm, platform_id, biz_date, state, selection_revision, caichip_task_id)
VALUES (?, ?, ?, ?, 'pending', ?, ?)
ON DUPLICATE KEY UPDATE
  selection_revision = VALUES(selection_revision),
  updated_at = CURRENT_TIMESTAMP(3),
  caichip_task_id = IFNULL(caichip_task_id, VALUES(caichip_task_id))`,
				sessionID, mpnNorm, p, dateStr, rev, taskCloudID).Error
			if err != nil {
				return err
			}
			if mysqlDispatchEnabled(r.bc) && r.dispatch != nil && r.dispatch.DBOk() {
				var cid sql.NullString
				qErr := r.db.WithContext(ctx).Raw(`
SELECT caichip_task_id FROM bom_search_task
WHERE session_id = ? AND mpn_norm = ? AND platform_id = ? AND biz_date = ?`,
					sessionID, mpnNorm, p, dateStr).Row().Scan(&cid)
				if qErr == nil && cid.Valid && strings.TrimSpace(cid.String) != "" {
					scriptID, e2 := r.lookupPlatformScriptID(ctx, p)
					if e2 == nil && scriptID != "" {
						entryFile := fmt.Sprintf("%s_crawler.py", scriptID)
						qt := &biz.QueuedTask{
							TaskMessage: biz.TaskMessage{
								TaskID:    strings.TrimSpace(cid.String),
								ScriptID:  scriptID,
								Version:   "1.0.0",
								Attempt:   1,
								EntryFile: &entryFile,
								Argv:      []string{"--model", mpnNorm, "--parse-workers", "8"},
								Params:    map[string]interface{}{},
							},
							Queue: "default",
						}
						_ = r.dispatch.EnqueuePending(ctx, qt)
					}
				}
			}
		}
	}
	return nil
}

// TaskAgg 按 state 聚合。
type TaskAgg struct {
	Total       int
	PendingLike int // pending + dispatched + running
	Succeeded   int // succeeded_quotes + succeeded_no_mpn
	FailedLike  int // failed + cancelled
	ByState     map[string]int
}

// AggregateTasksForSession 当前会话下所有搜索任务按 state 计数。
func (r *BOMSearchTaskRepo) AggregateTasksForSession(ctx context.Context, sessionID string, bizDate time.Time) (*TaskAgg, error) {
	if !r.DBOk() {
		return &TaskAgg{ByState: map[string]int{}}, nil
	}
	sessionID = strings.TrimSpace(sessionID)
	dateStr := bizDate.Format("2006-01-02")
	rows, err := r.db.WithContext(ctx).Raw(`
SELECT state, COUNT(*) FROM bom_search_task
WHERE session_id = ? AND biz_date = ?
GROUP BY state`, sessionID, dateStr).Rows()
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	agg := &TaskAgg{ByState: map[string]int{}}
	for rows.Next() {
		var st string
		var n int
		if err = rows.Scan(&st, &n); err != nil {
			return nil, err
		}
		st = strings.TrimSpace(strings.ToLower(st))
		agg.ByState[st] = n
		agg.Total += n
		switch st {
		case "pending", "dispatched", "running":
			agg.PendingLike += n
		case "succeeded_quotes", "succeeded_no_mpn":
			agg.Succeeded += n
		case "failed", "cancelled":
			agg.FailedLike += n
		default:
			agg.PendingLike += n
		}
	}
	return agg, rows.Err()
}

// ListTasksForSession 列出会话在业务日下的全部任务。
func (r *BOMSearchTaskRepo) ListTasksForSession(ctx context.Context, sessionID string, bizDate time.Time) ([]SearchTaskRow, error) {
	if !r.DBOk() {
		return nil, nil
	}
	sessionID = strings.TrimSpace(sessionID)
	dateStr := bizDate.Format("2006-01-02")
	rows, err := r.db.WithContext(ctx).Raw(`
SELECT mpn_norm, platform_id, state, auto_attempt, manual_attempt, last_error, selection_revision
FROM bom_search_task
WHERE session_id = ? AND biz_date = ?`, sessionID, dateStr).Rows()
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []SearchTaskRow
	for rows.Next() {
		var t SearchTaskRow
		if err = rows.Scan(&t.MpnNorm, &t.PlatformID, &t.State, &t.AutoAttempt, &t.ManualAttempt, &t.LastError, &t.SelectionRevision); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// QuoteCacheOutcome 报价缓存结果摘要。
func (r *BOMSearchTaskRepo) QuoteCacheOutcome(ctx context.Context, mpnNorm, platformID string, bizDate time.Time) (outcome string, err error) {
	if !r.DBOk() {
		return "", nil
	}
	dateStr := bizDate.Format("2006-01-02")
	mpnNorm = biz.NormalizeMPNForTask(mpnNorm)
	platformID = strings.TrimSpace(platformID)
	var oc sql.NullString
	err = r.db.WithContext(ctx).Raw(`
SELECT outcome FROM bom_quote_cache
WHERE mpn_norm = ? AND platform_id = ? AND biz_date = ?`, mpnNorm, platformID, dateStr).Row().Scan(&oc)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	if oc.Valid {
		return strings.TrimSpace(oc.String), nil
	}
	return "", nil
}

// UpsertQuoteCache 写入/更新 bom_quote_cache（主键 mpn_norm, platform_id, biz_date）。
func (r *BOMSearchTaskRepo) UpsertQuoteCache(ctx context.Context, mpnNorm, platformID string, bizDate time.Time, outcome string, quotesJSON, noMpnDetail []byte) error {
	if !r.DBOk() {
		return nil
	}
	dateStr := bizDate.Format("2006-01-02")
	mpnNorm = biz.NormalizeMPNForTask(mpnNorm)
	platformID = strings.TrimSpace(platformID)
	outcome = strings.TrimSpace(outcome)
	if outcome == "" {
		outcome = "ok"
	}
	return r.upsertQuoteCacheExec(ctx, r.db, mpnNorm, platformID, dateStr, outcome, quotesJSON, noMpnDetail)
}

func (r *BOMSearchTaskRepo) upsertQuoteCacheExec(ctx context.Context, x *gorm.DB, mpnNorm, platformID, dateStr, outcome string, quotesJSON, noMpnDetail []byte) error {
	var qj, nd interface{}
	if len(quotesJSON) > 0 {
		qj = quotesJSON
	}
	if len(noMpnDetail) > 0 {
		nd = noMpnDetail
	}
	return x.WithContext(ctx).Exec(`
INSERT INTO bom_quote_cache (mpn_norm, platform_id, biz_date, outcome, quotes_json, no_mpn_detail)
VALUES (?, ?, ?, ?, ?, ?)
ON DUPLICATE KEY UPDATE
  outcome = VALUES(outcome),
  quotes_json = VALUES(quotes_json),
  no_mpn_detail = VALUES(no_mpn_detail),
  updated_at = CURRENT_TIMESTAMP(3)`, mpnNorm, platformID, dateStr, outcome, qj, nd).Error
}

// FinalizeSearchTask 事务内校验任务行、更新 state（并视状态 UPSERT 报价缓存）。
func (r *BOMSearchTaskRepo) FinalizeSearchTask(ctx context.Context, sessionID, mpnNorm, platformID string, bizDate time.Time, caichipTaskID, state string, lastErr *string, quoteOutcome string, quotesJSON, noMpnDetail []byte) error {
	if !r.DBOk() {
		return sql.ErrNoRows
	}
	sessionID = strings.TrimSpace(sessionID)
	mpnNorm = biz.NormalizeMPNForTask(mpnNorm)
	platformID = strings.TrimSpace(platformID)
	dateStr := bizDate.Format("2006-01-02")
	st := strings.TrimSpace(strings.ToLower(state))

	tx := r.db.WithContext(ctx).Begin()
	if tx.Error != nil {
		return tx.Error
	}
	defer func() { _ = tx.Rollback() }()

	var existing sql.NullString
	scanErr := tx.Raw(`
SELECT caichip_task_id FROM bom_search_task
WHERE session_id = ? AND mpn_norm = ? AND platform_id = ? AND biz_date = ?
FOR UPDATE`, sessionID, mpnNorm, platformID, dateStr).Row().Scan(&existing)
	if errors.Is(scanErr, sql.ErrNoRows) {
		return ErrSearchTaskNotFound
	}
	if scanErr != nil {
		return scanErr
	}

	cid := strings.TrimSpace(caichipTaskID)
	if cid != "" && existing.Valid && strings.TrimSpace(existing.String) != "" {
		if strings.TrimSpace(existing.String) != cid {
			return ErrSearchTaskCaichipMismatch
		}
	}

	var lastErrArg interface{}
	if lastErr != nil {
		lastErrArg = *lastErr
	}

	if err := tx.Exec(`
UPDATE bom_search_task SET
  state = ?,
  last_error = ?,
  updated_at = CURRENT_TIMESTAMP(3),
  auto_attempt = auto_attempt + 1
WHERE session_id = ? AND mpn_norm = ? AND platform_id = ? AND biz_date = ?`,
		st, lastErrArg, sessionID, mpnNorm, platformID, dateStr).Error; err != nil {
		return err
	}

	switch st {
	case "succeeded_quotes":
		oc := strings.TrimSpace(quoteOutcome)
		if oc == "" {
			oc = "ok"
		}
		if err := r.upsertQuoteCacheExec(ctx, tx, mpnNorm, platformID, dateStr, oc, quotesJSON, noMpnDetail); err != nil {
			return err
		}
	case "succeeded_no_mpn":
		if err := r.upsertQuoteCacheExec(ctx, tx, mpnNorm, platformID, dateStr, "no_mpn_match", nil, noMpnDetail); err != nil {
			return err
		}
	}
	return tx.Commit().Error
}

// LoadSucceededQuoteRowsForSession 列出 succeeded_quotes 且已有关联缓存行的报价 JSON（供配单聚合）。
func (r *BOMSearchTaskRepo) LoadSucceededQuoteRowsForSession(ctx context.Context, sessionID string, bizDate time.Time) ([]biz.SessionQuoteRawRow, error) {
	if !r.DBOk() {
		return nil, nil
	}
	sessionID = strings.TrimSpace(sessionID)
	dateStr := bizDate.Format("2006-01-02")
	rows, err := r.db.WithContext(ctx).Raw(`
SELECT t.mpn_norm, t.platform_id, c.quotes_json
FROM bom_search_task t
INNER JOIN bom_quote_cache c
  ON c.mpn_norm = t.mpn_norm AND c.platform_id = t.platform_id AND c.biz_date = t.biz_date
WHERE t.session_id = ? AND t.biz_date = ? AND t.state = 'succeeded_quotes'`, sessionID, dateStr).Rows()
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []biz.SessionQuoteRawRow
	for rows.Next() {
		var row biz.SessionQuoteRawRow
		var qj []byte
		if err = rows.Scan(&row.MpnNorm, &row.PlatformID, &qj); err != nil {
			return nil, err
		}
		row.QuotesJSON = qj
		out = append(out, row)
	}
	return out, rows.Err()
}

func (r *BOMSearchTaskRepo) lookupPlatformScriptID(ctx context.Context, platformID string) (string, error) {
	platformID = strings.TrimSpace(platformID)
	if platformID == "" || r.db == nil {
		return "", sql.ErrNoRows
	}
	var sid string
	err := r.db.WithContext(ctx).Raw(`
SELECT script_id FROM bom_platform_script WHERE platform_id = ? AND enabled = 1`, platformID).Row().Scan(&sid)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(sid), nil
}

// BumpManualRetry 将任务置回 pending 并 manual_attempt+1。
func (r *BOMSearchTaskRepo) BumpManualRetry(ctx context.Context, sessionID, mpnNorm, platformID string) error {
	if !r.DBOk() {
		return nil
	}
	sessionID = strings.TrimSpace(sessionID)
	mpnNorm = biz.NormalizeMPNForTask(mpnNorm)
	platformID = strings.TrimSpace(platformID)
	if sessionID == "" || mpnNorm == "" || platformID == "" {
		return nil
	}
	bizDate, _, _, err := r.loadSessionTaskMeta(ctx, sessionID)
	if err != nil {
		return err
	}
	dateStr := bizDate.Format("2006-01-02")
	return r.db.WithContext(ctx).Exec(`
UPDATE bom_search_task SET
  manual_attempt = manual_attempt + 1,
  state = 'pending',
  last_error = NULL,
  updated_at = CURRENT_TIMESTAMP(3)
WHERE session_id = ? AND mpn_norm = ? AND platform_id = ? AND biz_date = ?`,
		sessionID, mpnNorm, platformID, dateStr).Error
}
