package data

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"

	"caichip/internal/biz"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// ErrDispatchTaskNoDB 未配置数据库或 DB 为 nil。
var ErrDispatchTaskNoDB = errors.New("dispatch task repo: database not configured")

// ErrDispatchLeaseMismatch 结果上报时 lease 与当前不一致或非 leased 态。
var ErrDispatchLeaseMismatch = errors.New("dispatch: lease mismatch or task not leased")

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

// EnqueuePending 幂等入队（已存在相同 task_id 则不动状态）。
func (r *DispatchTaskRepo) EnqueuePending(ctx context.Context, t *biz.QueuedTask) error {
	if !r.DBOk() {
		return ErrDispatchTaskNoDB
	}
	if t == nil {
		return errors.New("dispatch enqueue: nil task")
	}
	q := strings.TrimSpace(t.Queue)
	if q == "" {
		q = "default"
	}
	taskID := strings.TrimSpace(t.TaskID)
	if taskID == "" {
		return errors.New("dispatch enqueue: task_id required")
	}
	att := t.Attempt
	if att <= 0 {
		att = 1
	}
	tagsJSON := []byte("null")
	if len(t.RequiredTags) > 0 {
		b, err := json.Marshal(t.RequiredTags)
		if err != nil {
			return err
		}
		tagsJSON = b
	}
	var paramsJSON any
	if len(t.Params) > 0 {
		b, err := json.Marshal(t.Params)
		if err != nil {
			return err
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
	var argvJSON any
	if len(t.Argv) > 0 {
		b, err := json.Marshal(t.Argv)
		if err != nil {
			return err
		}
		argvJSON = b
	}
	return r.db.WithContext(ctx).Exec(`
INSERT INTO caichip_dispatch_task (
  task_id, queue, script_id, version, required_tags, entry_file, timeout_sec, params_json, argv_json, attempt, state
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'pending')
ON DUPLICATE KEY UPDATE updated_at = updated_at`,
		taskID, q, strings.TrimSpace(t.ScriptID), strings.TrimSpace(t.Version),
		tagsJSON, entry, timeout, paramsJSON, argvJSON, att,
	).Error
}

// ReclaimStaleLeases 将过期 lease 或 holder 已离线的任务打回 pending。
func (r *DispatchTaskRepo) ReclaimStaleLeases(ctx context.Context, now, offlineBefore time.Time) (int64, error) {
	if !r.DBOk() {
		return 0, ErrDispatchTaskNoDB
	}
	g := r.db.WithContext(ctx).Exec(`
UPDATE caichip_dispatch_task d
LEFT JOIN caichip_agent a ON a.agent_id = d.leased_to_agent_id
SET d.state = 'pending',
    d.lease_id = NULL,
    d.leased_to_agent_id = NULL,
    d.leased_at = NULL,
    d.lease_deadline_at = NULL,
    d.attempt = d.attempt + 1,
    d.updated_at = CURRENT_TIMESTAMP(3)
WHERE d.state = 'leased'
AND (
  (d.lease_deadline_at IS NOT NULL AND d.lease_deadline_at < ?)
  OR (a.agent_id IS NOT NULL AND a.last_task_heartbeat_at IS NOT NULL AND a.last_task_heartbeat_at < ?)
  OR (a.agent_id IS NULL AND d.leased_to_agent_id IS NOT NULL AND d.leased_at < ?)
)`, now, offlineBefore, offlineBefore)
	return g.RowsAffected, g.Error
}

// FinishLeased 租约正确则标记 finished；已 finished 幂等返回 nil；lease 不对返回 ErrDispatchLeaseMismatch。
func (r *DispatchTaskRepo) FinishLeased(ctx context.Context, taskID, leaseID, resultStatus string) error {
	if !r.DBOk() {
		return ErrDispatchTaskNoDB
	}
	taskID = strings.TrimSpace(taskID)
	leaseID = strings.TrimSpace(leaseID)
	g := r.db.WithContext(ctx).Exec(`
UPDATE caichip_dispatch_task
SET state = 'finished', finished_at = CURRENT_TIMESTAMP(3), result_status = ?, updated_at = CURRENT_TIMESTAMP(3)
WHERE task_id = ? AND state = 'leased' AND lease_id = ?`,
		strings.TrimSpace(resultStatus), taskID, leaseID)
	if g.Error != nil {
		return g.Error
	}
	if g.RowsAffected > 0 {
		return nil
	}
	var state string
	err := r.db.WithContext(ctx).Raw(`SELECT state FROM caichip_dispatch_task WHERE task_id = ?`, taskID).Row().Scan(&state)
	if err == sql.ErrNoRows {
		return nil
	}
	if err != nil {
		return err
	}
	if strings.EqualFold(strings.TrimSpace(state), dispatchStateFinished) {
		return nil
	}
	return ErrDispatchLeaseMismatch
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

	lockClause := "FOR UPDATE"
	if r.skipLocked {
		lockClause = "FOR UPDATE SKIP LOCKED"
	}
	rows, err := tx.Raw(`
SELECT id FROM caichip_dispatch_task
WHERE queue = ? AND state = 'pending'
ORDER BY id ASC
LIMIT `+strconv.Itoa(claimLimit)+`
`+lockClause, queue).Rows()
	if err != nil {
		return nil, err
	}
	var ids []int64
	for rows.Next() {
		var id int64
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return nil, err
		}
		ids = append(ids, id)
	}
	_ = rows.Close()
	if err = rows.Err(); err != nil {
		return nil, err
	}

	var out []biz.TaskMessage
	for _, id := range ids {
		if len(out) >= max {
			break
		}
		drow, err := r.loadRowByIDTx(ctx, tx, id)
		if err != nil {
			return nil, err
		}
		if drow == nil {
			continue
		}
		qtask := dispatchRowToQueued(drow)
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
		timeoutSec := drow.TimeoutSec
		if timeoutSec <= 0 {
			timeoutSec = 300
		}
		extra := int(leaseExtraSec)
		if extra < 0 {
			extra = 0
		}
		deadline := time.Now().Add(time.Duration(timeoutSec+extra) * time.Second)

		up := tx.Exec(`
UPDATE caichip_dispatch_task
SET state = 'leased', lease_id = ?, leased_to_agent_id = ?, leased_at = CURRENT_TIMESTAMP(3),
    lease_deadline_at = ?, attempt = ?, updated_at = CURRENT_TIMESTAMP(3)
WHERE id = ? AND state = 'pending'`,
			leaseID, agentID, deadline, drow.Attempt, id)
		if up.Error != nil {
			return nil, up.Error
		}
		if up.RowsAffected != 1 {
			continue
		}
		msg := biz.TaskMessage{
			TaskID:     drow.TaskID,
			ScriptID:   drow.ScriptID,
			Version:    drow.Version,
			TimeoutSec: timeoutSec,
			LeaseID:    leaseID,
			Attempt:    drow.Attempt,
			Params:     decodeDispatchParamsJSON(drow.ParamsJSON),
		}
		if drow.EntryFile.Valid {
			s := drow.EntryFile.String
			msg.EntryFile = &s
		}
		if len(drow.ArgvJSON) > 0 && string(drow.ArgvJSON) != "null" {
			_ = json.Unmarshal(drow.ArgvJSON, &msg.Argv)
		}
		out = append(out, msg)
	}
	if err := tx.Commit().Error; err != nil {
		return nil, err
	}
	return out, nil
}

type dispatchTaskRow struct {
	ID           int64
	TaskID       string
	Queue        string
	ScriptID     string
	Version      string
	TimeoutSec   int
	Attempt      int
	EntryFile    sql.NullString
	RequiredTags []byte
	ParamsJSON   []byte
	ArgvJSON     []byte
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

func (r *DispatchTaskRepo) loadRowByIDTx(ctx context.Context, tx *gorm.DB, id int64) (*dispatchTaskRow, error) {
	_ = r
	row := tx.WithContext(ctx).Raw(`
SELECT id, task_id, queue, script_id, version, timeout_sec, attempt, entry_file, required_tags, params_json, argv_json
FROM caichip_dispatch_task WHERE id = ?`, id).Row()
	var d dispatchTaskRow
	var entry sql.NullString
	err := row.Scan(&d.ID, &d.TaskID, &d.Queue, &d.ScriptID, &d.Version, &d.TimeoutSec, &d.Attempt, &entry, &d.RequiredTags, &d.ParamsJSON, &d.ArgvJSON)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	d.EntryFile = entry
	return &d, nil
}

func dispatchRowToQueued(d *dispatchTaskRow) *biz.QueuedTask {
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
