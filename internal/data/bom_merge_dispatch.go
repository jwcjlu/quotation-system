package data

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"caichip/internal/biz"

	"github.com/go-sql-driver/mysql"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// BomMergeDispatch 实现设计 §3.5：合并键调度（缓存短路 / 在途复用 / 新抓入队 + 事务内挂接）。
type BomMergeDispatch struct {
	db       *gorm.DB
	dispatch *DispatchTaskRepo
	search   *BOMSearchTaskRepo
	session  *BomSessionRepo
	scripts  *AgentScriptPackageRepo
}

// NewBomMergeDispatch ...
func NewBomMergeDispatch(d *Data, disp *DispatchTaskRepo, search *BOMSearchTaskRepo, session *BomSessionRepo, scripts *AgentScriptPackageRepo) *BomMergeDispatch {
	if d == nil || d.DB == nil {
		return &BomMergeDispatch{}
	}
	return &BomMergeDispatch{
		db:       d.DB,
		dispatch: disp,
		search:   search,
		session:  session,
		scripts:  scripts,
	}
}

var _ biz.MergeDispatchExecutor = (*BomMergeDispatch)(nil)

// DBOk ...
func (m *BomMergeDispatch) DBOk() bool {
	return m != nil && m.db != nil && m.dispatch != nil && m.dispatch.DBOk() && m.search != nil && m.search.DBOk()
}

var errRetryMergeDispatch = errors.New("bom merge: retry transaction")

func isMySQLDuplicateKey(err error) bool {
	if err == nil {
		return false
	}
	var me *mysql.MySQLError
	return errors.As(err, &me) && me.Number == 1062
}

func attachPendingBOMTasks(tx *gorm.DB, taskID, mpnNorm, platformID, dateStr string) (int64, error) {
	res := tx.Model(&BomSearchTask{}).
		Where("mpn_norm = ? AND platform_id = ? AND biz_date = ? AND state = ? AND (caichip_task_id IS NULL OR caichip_task_id = '')",
			mpnNorm, platformID, dateStr, "pending").
		Updates(map[string]interface{}{
			"caichip_task_id": taskID,
			"state":           "running",
			"updated_at":      gorm.Expr("CURRENT_TIMESTAMP(3)"),
		})
	return res.RowsAffected, res.Error
}

func (m *BomMergeDispatch) buildQueuedTask(ctx context.Context, taskID, mpnNorm, platformID string) (*biz.QueuedTask, error) {
	pid := strings.TrimSpace(platformID)
	var scriptID string
	_ = m.db.WithContext(ctx).Table(TableBomPlatformScript).
		Select("script_id").
		Where("platform_id = ? AND enabled = 1", pid).
		Limit(1).
		Scan(&scriptID).Error
	if strings.TrimSpace(scriptID) == "" {
		scriptID = pid
	}
	version := "0.0.1"
	file := fmt.Sprintf("%s_crawler.py", pid)
	if m.scripts != nil && m.scripts.DBOk() {
		pkg, err := m.scripts.GetPublished(ctx, scriptID)
		if err == nil && pkg != nil {
			if v := strings.TrimSpace(pkg.Version); v != "" {
				version = v
			}
			if ef := strings.TrimSpace(pkg.EntryFile); ef != "" {
				file = filepath.Base(ef)
			}
		}
	}
	return &biz.QueuedTask{
		TaskMessage: biz.TaskMessage{
			TaskID:   taskID,
			ScriptID: scriptID,
			Version:  version,
			Attempt:  1,
			Params: map[string]interface{}{
				"mpn_norm":    mpnNorm,
				"platform_id": pid,
			},
			Argv:      []string{"--model", mpnNorm, "--parse-workers", "8"},
			EntryFile: &file,
		},
		Queue: "default",
	}, nil
}

func finalizePendingFromCache(ctx context.Context, search *BOMSearchTaskRepo, L biz.BOMSearchTaskLookup, snap *biz.QuoteCacheSnapshot) error {
	oc := strings.ToLower(strings.TrimSpace(snap.Outcome))
	if oc == "no_mpn_match" || oc == "no_result" {
		return search.FinalizeSearchTask(ctx, L.SessionID, L.MpnNorm, L.PlatformID, L.BizDate, "", "no_result", nil, "", nil, snap.NoMpnDetail)
	}
	qo := strings.TrimSpace(snap.Outcome)
	if qo == "" {
		qo = "ok"
	}
	return search.FinalizeSearchTask(ctx, L.SessionID, L.MpnNorm, L.PlatformID, L.BizDate, "", "succeeded", nil, qo, snap.QuotesJSON, snap.NoMpnDetail)
}

// TryDispatchMergeKey 对单合并键执行 A/B/C 分支。
func (m *BomMergeDispatch) TryDispatchMergeKey(ctx context.Context, mpnNorm, platformID string, bizDate time.Time) error {
	if !m.DBOk() {
		return nil
	}
	mpnNorm = normalizeMPNForSearchTask(mpnNorm)
	platformID = strings.TrimSpace(platformID)
	if mpnNorm == "" || mpnNorm == "-" || platformID == "" {
		return nil
	}
	dateStr := bizDate.Format("2006-01-02")
	bd, err := time.ParseInLocation("2006-01-02", dateStr, time.Local)
	if err != nil {
		return err
	}

	snap, hit, err := m.search.LoadQuoteCacheByMergeKey(ctx, mpnNorm, platformID, bd)
	if err != nil {
		return err
	}
	if hit && snap != nil {
		lookups, err := m.search.ListPendingLookupsByMergeKey(ctx, mpnNorm, platformID, bd)
		if err != nil {
			return err
		}
		sessions := make(map[string]struct{})
		for _, L := range lookups {
			if err := finalizePendingFromCache(ctx, m.search, L, snap); err != nil {
				return err
			}
			sessions[L.SessionID] = struct{}{}
		}
		if m.session != nil && m.session.DBOk() {
			for sid := range sessions {
				_ = biz.TryMarkSessionDataReady(ctx, m.session, m.search, sid)
			}
		}
		return nil
	}

	for attempt := 0; attempt < 6; attempt++ {
		err := m.tryDispatchMergeKeyTx(ctx, mpnNorm, platformID, bd, dateStr)
		if err == nil {
			return nil
		}
		if errors.Is(err, errRetryMergeDispatch) {
			continue
		}
		return err
	}
	return errors.New("bom merge: exceeded retry")
}

func (m *BomMergeDispatch) tryDispatchMergeKeyTx(ctx context.Context, mpnNorm, platformID string, bd time.Time, dateStr string) error {
	return m.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// 清理已终态调度对应的 inflight 行
		if err := tx.Exec(fmt.Sprintf(`
DELETE mi FROM %s mi
INNER JOIN %s d ON mi.task_id = d.task_id
WHERE d.state IN (?, ?)`, TableBomMergeInflight, TableCaichipDispatchTask), dispatchStateFinished, dispatchStateCancelled).Error; err != nil {
			return err
		}

		var inflight BomMergeInflight
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("mpn_norm = ? AND platform_id = ? AND biz_date = ?", mpnNorm, platformID, bd).
			First(&inflight).Error
		if err == nil {
			var drow CaichipDispatchTask
			e2 := tx.Where("task_id = ? AND state IN ?", inflight.TaskID, []string{dispatchStatePending, dispatchStateLeased}).
				First(&drow).Error
			if e2 == nil {
				_, err := attachPendingBOMTasks(tx, inflight.TaskID, mpnNorm, platformID, dateStr)
				return err
			}
			if err := tx.Delete(&inflight).Error; err != nil {
				return err
			}
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}

		tid := uuid.NewString()
		now := time.Now()
		row := BomMergeInflight{
			MpnNorm:    mpnNorm,
			PlatformID: platformID,
			BizDate:    bd,
			TaskID:     tid,
			CreatedAt:  now,
		}
		if err := tx.Create(&row).Error; err != nil {
			if isMySQLDuplicateKey(err) {
				return errRetryMergeDispatch
			}
			return err
		}
		qt, err := m.buildQueuedTask(ctx, tid, mpnNorm, platformID)
		if err != nil {
			return err
		}
		if err := m.dispatch.EnqueuePendingTx(ctx, tx, qt); err != nil {
			return err
		}
		_, err = attachPendingBOMTasks(tx, tid, mpnNorm, platformID, dateStr)
		return err
	})
}

// TryDispatchPendingKeysForSession 对会话内每个 distinct 合并键尝试调度。
func (m *BomMergeDispatch) TryDispatchPendingKeysForSession(ctx context.Context, sessionID string) error {
	if !m.DBOk() {
		return nil
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil
	}
	keys, err := m.search.DistinctPendingMergeKeysForSession(ctx, sessionID)
	if err != nil {
		return err
	}
	for _, k := range keys {
		if err := m.TryDispatchMergeKey(ctx, k.MpnNorm, k.PlatformID, k.BizDate); err != nil {
			return err
		}
	}
	return nil
}
