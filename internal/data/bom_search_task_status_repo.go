package data

import (
	"context"
	"database/sql"
	"strings"
	"time"

	"caichip/internal/biz"
)

func (r *BOMSearchTaskRepo) ListSearchTaskStatusRows(ctx context.Context, sessionID string) ([]biz.SearchTaskStatusRow, error) {
	if !r.DBOk() {
		return nil, nil
	}
	sessionID = strings.TrimSpace(sessionID)
	var tasks []BomSearchTask
	if err := r.db.WithContext(ctx).
		Where("session_id = ?", sessionID).
		Order("mpn_norm ASC, platform_id ASC").
		Find(&tasks).Error; err != nil {
		return nil, err
	}

	taskIDs := make([]string, 0, len(tasks))
	for _, task := range tasks {
		taskID := nullStringValue(task.CaichipTaskID)
		if taskID != "" {
			taskIDs = append(taskIDs, taskID)
		}
	}
	dispatchByID, err := r.loadDispatchTasksByTaskID(ctx, taskIDs)
	if err != nil {
		return nil, err
	}

	out := make([]biz.SearchTaskStatusRow, 0, len(tasks))
	for _, task := range tasks {
		row := biz.SearchTaskStatusRow{
			SearchTaskID:    task.ID,
			MpnNorm:         strings.TrimSpace(task.MpnNorm),
			PlatformID:      strings.TrimSpace(task.PlatformID),
			SearchTaskState: strings.ToLower(strings.TrimSpace(task.State)),
			Attempt:         task.AutoAttempt + task.ManualAttempt,
			DispatchTaskID:  nullStringValue(task.CaichipTaskID),
			LastError:       nullStringValue(task.LastError),
			UpdatedAt:       timePtr(task.UpdatedAt),
		}
		if dispatch, ok := dispatchByID[row.DispatchTaskID]; ok {
			fillDispatchTaskStatus(&row, dispatch)
		}
		out = append(out, row)
	}
	return out, nil
}

func (r *BOMSearchTaskRepo) loadDispatchTasksByTaskID(ctx context.Context, taskIDs []string) (map[string]CaichipDispatchTask, error) {
	out := make(map[string]CaichipDispatchTask, len(taskIDs))
	if len(taskIDs) == 0 {
		return out, nil
	}
	var rows []CaichipDispatchTask
	if err := r.db.WithContext(ctx).Where("task_id IN ?", taskIDs).Find(&rows).Error; err != nil {
		return nil, err
	}
	for _, row := range rows {
		out[strings.TrimSpace(row.TaskID)] = row
	}
	return out, nil
}

func fillDispatchTaskStatus(row *biz.SearchTaskStatusRow, dispatch CaichipDispatchTask) {
	row.DispatchTaskState = strings.ToLower(strings.TrimSpace(dispatch.State))
	row.DispatchAgentID = nullStringValue(dispatch.LeasedToAgentID)
	row.DispatchResult = nullStringValue(dispatch.ResultStatus)
	row.LeaseDeadlineAt = dispatch.LeaseDeadlineAt
	row.Attempt = dispatch.Attempt
	row.RetryMax = dispatch.RetryMax
	if row.LastError == "" {
		row.LastError = nullStringValue(dispatch.LastError)
	}
	if dispatch.UpdatedAt.After(valueTime(row.UpdatedAt)) {
		row.UpdatedAt = timePtr(dispatch.UpdatedAt)
	}
}

func nullStringValue(v sql.NullString) string {
	if !v.Valid {
		return ""
	}
	return strings.TrimSpace(v.String)
}

func timePtr(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	v := t
	return &v
}

func valueTime(t *time.Time) time.Time {
	if t == nil {
		return time.Time{}
	}
	return *t
}
