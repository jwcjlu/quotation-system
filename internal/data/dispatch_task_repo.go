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

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ErrDispatchTaskNoDB 未配置数据库或 DB 为 nil。
var ErrDispatchTaskNoDB = errors.New("dispatch task repo: database not configured")

const (
	dispatchStatePending   = "pending"
	dispatchStateLeased    = "leased"
	dispatchStateFinished  = "finished"
	dispatchStateCancelled = "cancelled"
)

// DispatchTaskRepo 访问调度队列表 `caichip_dispatch_task`（MySQL 5.7+；SKIP LOCKED 需 8.0.1+/MariaDB 10.6+，否则自动降级）。
type DispatchTaskRepo struct {
	db         *gorm.DB
	skipLocked bool
}

// NewDispatchTaskRepo 无 DB　时返回零值 Repo（DBOk()==false）。
func NewDispatchTaskRepo(d *Data) *DispatchTaskRepo {
	if d == nil || d.DB == nil {
		return &DispatchTaskRepo{}
	}
	return &DispatchTaskRepo{db: d.DB, skipLocked: d.mysqlSkipLocked}
}

// DBOk 是否已连接数据库。
func (r *DispatchTaskRepo) DBOk() bool {
	return r != nil && r.db != nil
}

// Ping 用于健康检查。
func (r *DispatchTaskRepo) Ping(ctx context.Context) error {
	if !r.DBOk() {
		return ErrDispatchTaskNoDB
	}
	sqlDB, err := r.db.DB()
	if err != nil {
		return err
	}
	return sqlDB.PingContext(ctx)
}

func caichipDispatchRowFromQueuedTask(t *biz.QueuedTask) (*CaichipDispatchTask, error) {
	if t == nil {
		return nil, errors.New("dispatch enqueue: nil task")
	}
	q := strings.TrimSpace(t.Queue)
	if q == "" {
		q = "default"
	}
	taskID := strings.TrimSpace(t.TaskID)
	if taskID == "" {
		return nil, errors.New("dispatch enqueue: task_id required")
	}
	att := t.Attempt
	if att <= 0 {
		att = 1
	}
	tagsJSON := []byte("null")
	if len(t.RequiredTags) > 0 {
		b, err := json.Marshal(t.RequiredTags)
		if err != nil {
			return nil, err
		}
		tagsJSON = b
	}
	var paramsJSON []byte
	if len(t.Params) > 0 {
		b, err := json.Marshal(t.Params)
		if err != nil {
			return nil, err
		}
		paramsJSON = b
	}
	timeout := t.TimeoutSec
	if timeout <= 0 {
		timeout = 300
	}
	var entry sql.NullString
	if t.EntryFile != nil && *t.EntryFile != "" {
		entry = sql.NullString{String: *t.EntryFile, Valid: true}
	}
	var argvJSON []byte
	if len(t.Argv) > 0 {
		b, err := json.Marshal(t.Argv)
		if err != nil {
			return nil, err
		}
		argvJSON = b
	}
	return &CaichipDispatchTask{
		TaskID:       taskID,
		Queue:        q,
		ScriptID:     strings.TrimSpace(t.ScriptID),
		Version:      strings.TrimSpace(t.Version),
		RequiredTags: tagsJSON,
		EntryFile:    entry,
		TimeoutSec:   timeout,
		ParamsJSON:   paramsJSON,
		ArgvJSON:     argvJSON,
		Attempt:      att,
		State:        dispatchStatePending,
	}, nil
}

func enqueuePendingWithDB(db *gorm.DB, ctx context.Context, t *biz.QueuedTask) error {
	row, err := caichipDispatchRowFromQueuedTask(t)
	if err != nil {
		return err
	}
	return db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "task_id"}},
		DoUpdates: clause.Assignments(map[string]interface{}{
			"updated_at": gorm.Expr("CURRENT_TIMESTAMP(3)"),
		}),
	}).Create(row).Error
}

// EnqueuePending 幂等入队（已存在相同 task_id 则不动状态）。
func (r *DispatchTaskRepo) EnqueuePending(ctx context.Context, t *biz.QueuedTask) error {
	if !r.DBOk() {
		return ErrDispatchTaskNoDB
	}
	return enqueuePendingWithDB(r.db, ctx, t)
}

// EnqueuePendingTx 在已有事务内入队（与 bom_merge_inflight / bom_search_task 同事务）。
func (r *DispatchTaskRepo) EnqueuePendingTx(ctx context.Context, tx *gorm.DB, t *biz.QueuedTask) error {
	if !r.DBOk() || tx == nil {
		return ErrDispatchTaskNoDB
	}
	return enqueuePendingWithDB(tx, ctx, t)
}

// ReclaimStaleLeases 将过期 lease 或 holder 已离线的任务打回 pending。
func (r *DispatchTaskRepo) ReclaimStaleLeases(ctx context.Context, now, offlineBefore time.Time) (int64, error) {
	if !r.DBOk() {
		return 0, ErrDispatchTaskNoDB
	}
	sub := r.db.Session(&gorm.Session{NewDB: true})
	cond := sub.Where("d.lease_deadline_at IS NOT NULL AND d.lease_deadline_at < ?", now).
		Or("a.agent_id IS NOT NULL AND a.last_task_heartbeat_at IS NOT NULL AND a.last_task_heartbeat_at < ?", offlineBefore).
		Or("a.agent_id IS NULL AND d.leased_to_agent_id IS NOT NULL AND d.leased_at < ?", offlineBefore)

	var ids []uint64
	err := r.db.WithContext(ctx).Table(fmt.Sprintf("%s AS d", TableCaichipDispatchTask)).
		Select("d.id").
		Joins(fmt.Sprintf("LEFT JOIN %s AS a ON a.agent_id = d.leased_to_agent_id", TableCaichipAgent)).
		Where("d.state = ?", dispatchStateLeased).
		Where(cond).
		Pluck("d.id", &ids).Error
	if err != nil {
		return 0, err
	}
	if len(ids) == 0 {
		return 0, nil
	}
	res := r.db.WithContext(ctx).Model(&CaichipDispatchTask{}).
		Where("id IN ?", ids).
		Updates(map[string]interface{}{
			"state":              dispatchStatePending,
			"lease_id":           gorm.Expr("NULL"),
			"leased_to_agent_id": gorm.Expr("NULL"),
			"leased_at":          gorm.Expr("NULL"),
			"lease_deadline_at":  gorm.Expr("NULL"),
			"attempt":            gorm.Expr("attempt + 1"),
			"updated_at":         gorm.Expr("CURRENT_TIMESTAMP(3)"),
		})
	return res.RowsAffected, res.Error
}

// FinishLeased 租约正确则标记 finished；已 finished 幂等返回 nil；lease 不对返回 biz.ErrDispatchLeaseMismatch。
func (r *DispatchTaskRepo) FinishLeased(ctx context.Context, taskID, leaseID, resultStatus string) error {
	if !r.DBOk() {
		return ErrDispatchTaskNoDB
	}
	taskID = strings.TrimSpace(taskID)
	leaseID = strings.TrimSpace(leaseID)
	res := r.db.WithContext(ctx).Model(&CaichipDispatchTask{}).
		Where("task_id = ? AND state = ? AND lease_id = ?", taskID, dispatchStateLeased, leaseID).
		Updates(map[string]interface{}{
			"state":         dispatchStateFinished,
			"finished_at":   gorm.Expr("CURRENT_TIMESTAMP(3)"),
			"result_status": strings.TrimSpace(resultStatus),
			"updated_at":    gorm.Expr("CURRENT_TIMESTAMP(3)"),
		})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected > 0 {
		_ = r.db.WithContext(ctx).Where("task_id = ?", taskID).Delete(&BomMergeInflight{})
		return nil
	}
	var row CaichipDispatchTask
	err := r.db.WithContext(ctx).Where("task_id = ?", taskID).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if strings.EqualFold(strings.TrimSpace(row.State), dispatchStateFinished) {
		_ = r.db.WithContext(ctx).Where("task_id = ?", taskID).Delete(&BomMergeInflight{})
		return nil
	}
	return biz.ErrDispatchLeaseMismatch
}

// PullAndLeaseForAgent 短事务内 SKIP LOCKED + match + 租约。
func (r *DispatchTaskRepo) PullAndLeaseForAgent(ctx context.Context, queue, agentID string, meta *biz.AgentSchedulingMeta, running []biz.RunningTaskReport, max int, leaseExtraSec int32) ([]biz.TaskMessage, error) {
	if !r.DBOk() {
		return nil, ErrDispatchTaskNoDB
	}
	if max <= 0 {
		max = 8
	}
	queue = strings.TrimSpace(queue)
	if queue == "" {
		queue = "default"
	}
	agentID = strings.TrimSpace(agentID)
	if agentID == "" {
		return nil, errors.New("dispatch pull: agent_id required")
	}
	claimLimit := max * 15
	if claimLimit < 32 {
		claimLimit = 32
	}
	if claimLimit > 200 {
		claimLimit = 200
	}
	busyTask, busyScript := runningBusySets(running)

	tx := r.db.WithContext(ctx).Begin(&sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if tx.Error != nil {
		return nil, tx.Error
	}
	defer func() { _ = tx.Rollback() }()

	q := tx.Model(&CaichipDispatchTask{}).
		Select("id").
		Where("queue = ? AND state = ?", queue, dispatchStatePending).
		Order("id ASC").
		Limit(claimLimit)
	if r.skipLocked {
		q = q.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"})
	} else {
		q = q.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	var ids []uint64
	if err := q.Pluck("id", &ids).Error; err != nil {
		return nil, err
	}

	var out []biz.TaskMessage
	for _, id := range ids {
		if len(out) >= max {
			break
		}
		var d CaichipDispatchTask
		if err := tx.Where("id = ?", id).First(&d).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				continue
			}
			return nil, err
		}
		qtask := dispatchModelToQueued(&d)
		if !biz.MatchTaskForAgent(meta, qtask) {
			continue
		}
		if _, ok := busyTask[qtask.TaskID]; ok {
			continue
		}
		if _, ok := busyScript[qtask.ScriptID]; ok {
			continue
		}
		leaseID := uuid.NewString()
		timeoutSec := d.TimeoutSec
		if timeoutSec <= 0 {
			timeoutSec = 300
		}
		extra := int(leaseExtraSec)
		if extra < 0 {
			extra = 0
		}
		deadline := time.Now().Add(time.Duration(timeoutSec+extra) * time.Second)

		up := tx.Model(&CaichipDispatchTask{}).
			Where("id = ? AND state = ?", id, dispatchStatePending).
			Updates(map[string]interface{}{
				"state":              dispatchStateLeased,
				"lease_id":           leaseID,
				"leased_to_agent_id": agentID,
				"leased_at":          gorm.Expr("CURRENT_TIMESTAMP(3)"),
				"lease_deadline_at":  deadline,
				"attempt":            d.Attempt,
				"updated_at":         gorm.Expr("CURRENT_TIMESTAMP(3)"),
			})
		if up.Error != nil {
			return nil, up.Error
		}
		if up.RowsAffected != 1 {
			continue
		}
		msg := biz.TaskMessage{
			TaskID:     d.TaskID,
			ScriptID:   d.ScriptID,
			Version:    d.Version,
			TimeoutSec: timeoutSec,
			LeaseID:    leaseID,
			Attempt:    d.Attempt,
			Params:     decodeDispatchParamsJSON(d.ParamsJSON),
		}
		if d.EntryFile.Valid && d.EntryFile.String != "" {
			s := d.EntryFile.String
			msg.EntryFile = &s
		}
		if len(d.ArgvJSON) > 0 && string(d.ArgvJSON) != "null" {
			_ = json.Unmarshal(d.ArgvJSON, &msg.Argv)
		}
		out = append(out, msg)
	}
	if err := tx.Commit().Error; err != nil {
		return nil, err
	}
	return out, nil
}

func decodeDispatchParamsJSON(raw []byte) map[string]interface{} {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	var m map[string]interface{}
	if json.Unmarshal(raw, &m) != nil || len(m) == 0 {
		return nil
	}
	return m
}

func runningBusySets(running []biz.RunningTaskReport) (taskIDs, scriptIDs map[string]struct{}) {
	taskIDs = make(map[string]struct{})
	scriptIDs = make(map[string]struct{})
	for _, r := range running {
		if r.TaskID != "" {
			taskIDs[r.TaskID] = struct{}{}
		}
		if r.ScriptID != "" {
			scriptIDs[r.ScriptID] = struct{}{}
		}
	}
	return taskIDs, scriptIDs
}

func dispatchModelToQueued(d *CaichipDispatchTask) *biz.QueuedTask {
	if d == nil {
		return nil
	}
	t := &biz.QueuedTask{
		TaskMessage: biz.TaskMessage{
			TaskID:     d.TaskID,
			ScriptID:   d.ScriptID,
			Version:    d.Version,
			TimeoutSec: d.TimeoutSec,
			Attempt:    d.Attempt,
		},
		Queue: d.Queue,
	}
	if d.EntryFile.Valid && d.EntryFile.String != "" {
		s := d.EntryFile.String
		t.EntryFile = &s
	}
	t.Params = decodeDispatchParamsJSON(d.ParamsJSON)
	if len(d.ArgvJSON) > 0 && string(d.ArgvJSON) != "null" {
		_ = json.Unmarshal(d.ArgvJSON, &t.Argv)
	}
	if len(d.RequiredTags) > 0 && string(d.RequiredTags) != "null" {
		_ = json.Unmarshal(d.RequiredTags, &t.RequiredTags)
	}
	return t
}

var _ biz.DispatchTaskRepo = (*DispatchTaskRepo)(nil)
