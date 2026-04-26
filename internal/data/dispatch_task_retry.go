package data

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"caichip/internal/biz"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type staleDispatchLeaseRow struct {
	CaichipDispatchTask
	HolderLastTaskHeartbeatAt *time.Time `gorm:"column:holder_last_task_heartbeat_at"`
}

func dispatchRetryBackoffForQueuedTask(xs []int) []int {
	if len(xs) == 0 {
		return nil
	}
	out := make([]int, 0, len(xs))
	for _, x := range xs {
		if x <= 0 {
			return nil
		}
		out = append(out, x)
	}
	return out
}

func decodeDispatchRetryBackoff(raw []byte) []int {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	var xs []int
	if err := json.Unmarshal(raw, &xs); err != nil {
		return nil
	}
	return dispatchRetryBackoffForQueuedTask(xs)
}

func (r *DispatchTaskRepo) LoadLeasedTask(ctx context.Context, taskID, leaseID string) (*biz.DispatchLeasedTask, error) {
	if !r.DBOk() {
		return nil, ErrDispatchTaskNoDB
	}
	row, err := r.loadDispatchTaskByTaskID(ctx, taskID)
	if err != nil || row == nil {
		return nil, err
	}
	if isDispatchTaskTerminalState(row.State) {
		return nil, nil
	}
	if !strings.EqualFold(strings.TrimSpace(row.State), dispatchStateLeased) {
		return nil, biz.ErrDispatchLeaseMismatch
	}
	if leaseID != "" && (!row.LeaseID.Valid || row.LeaseID.String != leaseID) {
		return nil, biz.ErrDispatchLeaseMismatch
	}
	return &biz.DispatchLeasedTask{
		TaskID:          strings.TrimSpace(row.TaskID),
		LeaseID:         row.LeaseID.String,
		Attempt:         row.Attempt,
		RetryMax:        row.RetryMax,
		RetryBackoffSec: decodeDispatchRetryBackoff(row.RetryBackoffJSON),
	}, nil
}

func (r *DispatchTaskRepo) ListStaleLeasedTasks(ctx context.Context, now, offlineBefore time.Time) ([]biz.StaleDispatchTask, error) {
	if !r.DBOk() {
		return nil, ErrDispatchTaskNoDB
	}
	sub := r.db.Session(&gorm.Session{NewDB: true})
	cond := sub.Where("d.lease_deadline_at IS NOT NULL AND d.lease_deadline_at < ?", now).
		Or("a.agent_id IS NOT NULL AND a.last_task_heartbeat_at IS NOT NULL AND a.last_task_heartbeat_at < ?", offlineBefore).
		Or("a.agent_id IS NULL AND d.leased_to_agent_id IS NOT NULL AND d.leased_at < ?", offlineBefore)

	var rows []staleDispatchLeaseRow
	err := r.db.WithContext(ctx).
		Table(fmt.Sprintf("%s AS d", TableCaichipDispatchTask)).
		Select("d.*, a.last_task_heartbeat_at AS holder_last_task_heartbeat_at").
		Joins(fmt.Sprintf("LEFT JOIN %s AS a ON a.agent_id = d.leased_to_agent_id", TableCaichipAgent)).
		Where("d.state = ?", dispatchStateLeased).
		Where(cond).
		Order("d.id ASC").
		Clauses(clause.Locking{Strength: "UPDATE"}).
		Find(&rows).Error
	if err != nil {
		return nil, err
	}
	out := make([]biz.StaleDispatchTask, 0, len(rows))
	for _, row := range rows {
		if !row.LeaseID.Valid {
			continue
		}
		out = append(out, biz.StaleDispatchTask{
			DispatchLeasedTask: biz.DispatchLeasedTask{
				TaskID:          strings.TrimSpace(row.TaskID),
				LeaseID:         row.LeaseID.String,
				Attempt:         row.Attempt,
				RetryMax:        row.RetryMax,
				RetryBackoffSec: decodeDispatchRetryBackoff(row.RetryBackoffJSON),
			},
			FailureReason: staleDispatchFailureReason(row, now, offlineBefore),
		})
	}
	return out, nil
}

func (r *DispatchTaskRepo) RequeueLeased(ctx context.Context, taskID, leaseID string, nextAttempt int, nextClaimAt time.Time, lastError string) error {
	if !r.DBOk() {
		return ErrDispatchTaskNoDB
	}
	return r.updateLeasedTask(ctx, taskID, leaseID, map[string]interface{}{
		"state":              dispatchStatePending,
		"attempt":            nextAttempt,
		"next_claim_at":      nextClaimAt,
		"last_error":         strings.TrimSpace(lastError),
		"result_status":      gorm.Expr("NULL"),
		"finished_at":        gorm.Expr("NULL"),
		"lease_id":           gorm.Expr("NULL"),
		"leased_to_agent_id": gorm.Expr("NULL"),
		"leased_at":          gorm.Expr("NULL"),
		"lease_deadline_at":  gorm.Expr("NULL"),
		"updated_at":         gorm.Expr("CURRENT_TIMESTAMP(3)"),
	})
}

func (r *DispatchTaskRepo) FailLeasedTerminal(ctx context.Context, taskID, leaseID, resultStatus, lastError string, finishedAt time.Time) error {
	if !r.DBOk() {
		return ErrDispatchTaskNoDB
	}
	return r.updateLeasedTask(ctx, taskID, leaseID, map[string]interface{}{
		"state":              dispatchStateFailedTerminal,
		"result_status":      strings.TrimSpace(resultStatus),
		"finished_at":        finishedAt,
		"last_error":         strings.TrimSpace(lastError),
		"next_claim_at":      gorm.Expr("NULL"),
		"lease_id":           gorm.Expr("NULL"),
		"leased_to_agent_id": gorm.Expr("NULL"),
		"leased_at":          gorm.Expr("NULL"),
		"lease_deadline_at":  gorm.Expr("NULL"),
		"updated_at":         gorm.Expr("CURRENT_TIMESTAMP(3)"),
	})
}

func (r *DispatchTaskRepo) updateLeasedTask(ctx context.Context, taskID, leaseID string, fields map[string]interface{}) error {
	res := r.db.WithContext(ctx).
		Model(&CaichipDispatchTask{}).
		Where("task_id = ? AND state = ? AND lease_id = ?", strings.TrimSpace(taskID), dispatchStateLeased, strings.TrimSpace(leaseID)).
		Updates(fields)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected > 0 {
		return nil
	}
	return r.dispatchLeaseUpdateFallback(ctx, taskID)
}

func (r *DispatchTaskRepo) dispatchLeaseUpdateFallback(ctx context.Context, taskID string) error {
	row, err := r.loadDispatchTaskByTaskID(ctx, taskID)
	if err != nil || row == nil {
		return err
	}
	if isDispatchTaskTerminalState(row.State) {
		return nil
	}
	return biz.ErrDispatchLeaseMismatch
}

func (r *DispatchTaskRepo) loadDispatchTaskByTaskID(ctx context.Context, taskID string) (*CaichipDispatchTask, error) {
	var row CaichipDispatchTask
	err := r.db.WithContext(ctx).Where("task_id = ?", strings.TrimSpace(taskID)).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &row, nil
}

func staleDispatchFailureReason(row staleDispatchLeaseRow, now, offlineBefore time.Time) string {
	if row.LeaseDeadlineAt != nil && row.LeaseDeadlineAt.Before(now) {
		return biz.DispatchFailureReasonLeaseExpired
	}
	if row.HolderLastTaskHeartbeatAt != nil && row.HolderLastTaskHeartbeatAt.Before(offlineBefore) {
		return biz.DispatchFailureReasonAgentOfflineReclaimed
	}
	return biz.DispatchFailureReasonAgentOfflineReclaimed
}

func isDispatchTaskTerminalState(state string) bool {
	state = strings.TrimSpace(state)
	return strings.EqualFold(state, dispatchStateFinished) ||
		strings.EqualFold(state, dispatchStateCancelled) ||
		strings.EqualFold(state, dispatchStateFailedTerminal)
}
