package data

import (
	"context"
	"errors"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// BomMergeProxyWaitRepo t_bom_merge_proxy_wait 读写。
type BomMergeProxyWaitRepo struct {
	db *gorm.DB
}

// NewBomMergeProxyWaitRepo ...
func NewBomMergeProxyWaitRepo(d *Data) *BomMergeProxyWaitRepo {
	if d == nil || d.DB == nil {
		return &BomMergeProxyWaitRepo{}
	}
	return &BomMergeProxyWaitRepo{db: d.DB}
}

// DBOk ...
func (r *BomMergeProxyWaitRepo) DBOk() bool {
	return r != nil && r.db != nil
}

// Get 按合并键读取一行；无行返回 (nil, nil)。
func (r *BomMergeProxyWaitRepo) Get(ctx context.Context, mpnNorm, platformID string, bizDate time.Time) (*BomMergeProxyWait, error) {
	if !r.DBOk() {
		return nil, nil
	}
	var row BomMergeProxyWait
	err := r.db.WithContext(ctx).
		Where("mpn_norm = ? AND platform_id = ? AND biz_date = ?", mpnNorm, platformID, bizDate.Format("2006-01-02")).
		First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &row, nil
}

// UpsertAfterFailure 记录/更新退避行。
func (r *BomMergeProxyWaitRepo) UpsertAfterFailure(ctx context.Context, row *BomMergeProxyWait) error {
	if !r.DBOk() || row == nil {
		return errors.New("bom_merge_proxy_wait: no db")
	}
	return r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{
			{Name: "mpn_norm"},
			{Name: "platform_id"},
			{Name: "biz_date"},
		},
		DoUpdates: clause.Assignments(map[string]interface{}{
			"next_retry_at":   row.NextRetryAt,
			"attempt":         row.Attempt,
			"last_error":      row.LastError,
			"first_failed_at": row.FirstFailedAt,
			"updated_at":      gorm.Expr("CURRENT_TIMESTAMP(3)"),
		}),
	}).Create(row).Error
}

// Delete 幂等删除。
func (r *BomMergeProxyWaitRepo) Delete(ctx context.Context, mpnNorm, platformID string, bizDate time.Time) error {
	if !r.DBOk() {
		return nil
	}
	return r.db.WithContext(ctx).
		Where("mpn_norm = ? AND platform_id = ? AND biz_date = ?", mpnNorm, platformID, bizDate.Format("2006-01-02")).
		Delete(&BomMergeProxyWait{}).Error
}

// ListDue 到期行，按 next_retry_at 升序。
func (r *BomMergeProxyWaitRepo) ListDue(ctx context.Context, limit int) ([]BomMergeProxyWait, error) {
	if !r.DBOk() {
		return nil, nil
	}
	if limit <= 0 {
		limit = 32
	}
	var rows []BomMergeProxyWait
	err := r.db.WithContext(ctx).
		Where("next_retry_at <= CURRENT_TIMESTAMP(3)").
		Order("next_retry_at ASC").
		Limit(limit).
		Find(&rows).Error
	return rows, err
}

// FailPendingTasksForMergeKey 将仍 pending 且未附着 caichip_task_id 的任务标为 failed_terminal。
func (r *BomMergeProxyWaitRepo) FailPendingTasksForMergeKey(ctx context.Context, mpnNorm, platformID string, bizDate time.Time, lastErr string) (int64, error) {
	if !r.DBOk() {
		return 0, nil
	}
	mpnNorm = strings.TrimSpace(mpnNorm)
	platformID = strings.TrimSpace(platformID)
	dateStr := bizDate.Format("2006-01-02")
	res := r.db.WithContext(ctx).Model(&BomSearchTask{}).
		Where("mpn_norm = ? AND platform_id = ? AND biz_date = ? AND state = ? AND (caichip_task_id IS NULL OR caichip_task_id = '')",
			mpnNorm, platformID, dateStr, "pending").
		Updates(map[string]interface{}{
			"state":      "failed_terminal",
			"last_error": lastErr,
			"updated_at": gorm.Expr("CURRENT_TIMESTAMP(3)"),
		})
	return res.RowsAffected, res.Error
}
