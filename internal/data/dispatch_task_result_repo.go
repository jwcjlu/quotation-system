package data

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"caichip/internal/biz"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func (r *DispatchTaskRepo) SubmitLeasedResult(ctx context.Context, in *biz.TaskResultIn) error {
	if in == nil {
		return errors.New("dispatch submit result: nil input")
	}
	status := strings.TrimSpace(in.Status)
	if strings.EqualFold(status, "success") {
		return r.FinishLeased(ctx, in.TaskID, in.LeaseID, status)
	}
	if !r.DBOk() {
		return ErrDispatchTaskNoDB
	}
	taskID := strings.TrimSpace(in.TaskID)
	leaseID := strings.TrimSpace(in.LeaseID)
	if taskID == "" {
		return errors.New("dispatch submit result: task_id required")
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var row CaichipDispatchTask
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("task_id = ?", taskID).
			First(&row).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		if strings.EqualFold(row.State, dispatchStateFinished) ||
			strings.EqualFold(row.State, dispatchStateFailedTerminal) ||
			strings.EqualFold(row.State, dispatchStateCancelled) {
			return nil
		}
		if !strings.EqualFold(row.State, dispatchStateLeased) ||
			!row.LeaseID.Valid ||
			strings.TrimSpace(row.LeaseID.String) != leaseID {
			return biz.ErrDispatchLeaseMismatch
		}

		now := time.Now()
		lastErr := sql.NullString{String: strings.TrimSpace(in.ErrorMessage), Valid: strings.TrimSpace(in.ErrorMessage) != ""}
		policy := dispatchRetryPolicyFromRow(&row)
		delay, shouldRetry := policy.DelayForFailedAttempt(row.Attempt)
		if shouldRetry {
			nextClaimAt := now.Add(delay)
			return tx.Model(&CaichipDispatchTask{}).
				Where("id = ?", row.ID).
				Updates(map[string]interface{}{
					"state":              dispatchStatePending,
					"attempt":            row.Attempt + 1,
					"lease_id":           gorm.Expr("NULL"),
					"leased_to_agent_id": gorm.Expr("NULL"),
					"leased_at":          gorm.Expr("NULL"),
					"lease_deadline_at":  gorm.Expr("NULL"),
					"next_claim_at":      nextClaimAt,
					"finished_at":        gorm.Expr("NULL"),
					"result_status":      gorm.Expr("NULL"),
					"last_error":         lastErr,
					"updated_at":         gorm.Expr("CURRENT_TIMESTAMP(3)"),
				}).Error
		}

		err = tx.Model(&CaichipDispatchTask{}).
			Where("id = ?", row.ID).
			Updates(map[string]interface{}{
				"state":              dispatchStateFailedTerminal,
				"lease_id":           gorm.Expr("NULL"),
				"leased_to_agent_id": gorm.Expr("NULL"),
				"leased_at":          gorm.Expr("NULL"),
				"lease_deadline_at":  gorm.Expr("NULL"),
				"next_claim_at":      gorm.Expr("NULL"),
				"finished_at":        now,
				"result_status":      dispatchStateFailedTerminal,
				"last_error":         lastErr,
				"updated_at":         gorm.Expr("CURRENT_TIMESTAMP(3)"),
			}).Error
		if err != nil {
			return err
		}
		return tx.Where("task_id = ?", taskID).Delete(&BomMergeInflight{}).Error
	})
}

func dispatchRetryPolicyFromRow(row *CaichipDispatchTask) biz.DispatchRetryPolicy {
	if row == nil {
		return biz.DispatchRetryPolicy{}
	}
	policy := biz.DispatchRetryPolicy{RetryMax: row.RetryMax}
	if len(row.RetryBackoffJSON) > 0 && string(row.RetryBackoffJSON) != "null" {
		_ = json.Unmarshal(row.RetryBackoffJSON, &policy.BackoffSec)
	}
	return policy
}
