package data

import (
	"context"
	"errors"
	"strings"

	"caichip/internal/biz"

	"gorm.io/gorm"
)

// HsModelTaskRepo 实现 biz.HsModelTaskRepo（GORM，跨进程可见）。
type HsModelTaskRepo struct {
	d *Data
}

func NewHsModelTaskRepo(d *Data) *HsModelTaskRepo {
	return &HsModelTaskRepo{d: d}
}

func (r *HsModelTaskRepo) DBOk() bool {
	return r != nil && r.d != nil && r.d.DB != nil
}

func (r *HsModelTaskRepo) GetByRequestTraceID(ctx context.Context, model, manufacturer, requestTraceID string) (*biz.HsModelTaskRecord, error) {
	if !r.DBOk() {
		return nil, nil
	}
	model = strings.TrimSpace(model)
	manufacturer = strings.TrimSpace(manufacturer)
	requestTraceID = strings.TrimSpace(requestTraceID)
	if model == "" || manufacturer == "" || requestTraceID == "" {
		return nil, nil
	}
	var row HsModelTask
	err := r.d.DB.WithContext(ctx).
		Where("model = ? AND manufacturer = ? AND request_trace_id = ?", model, manufacturer, requestTraceID).
		Order("updated_at DESC").
		Limit(1).
		First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return hsModelTaskToBiz(&row), nil
}

func (r *HsModelTaskRepo) GetByRunID(ctx context.Context, runID string) (*biz.HsModelTaskRecord, error) {
	if !r.DBOk() {
		return nil, nil
	}
	runID = strings.TrimSpace(runID)
	if runID == "" {
		return nil, nil
	}
	var row HsModelTask
	err := r.d.DB.WithContext(ctx).Where("run_id = ?", runID).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return hsModelTaskToBiz(&row), nil
}

func (r *HsModelTaskRepo) GetLatestByModelManufacturer(ctx context.Context, model, manufacturer string) (*biz.HsModelTaskRecord, error) {
	if !r.DBOk() {
		return nil, nil
	}
	model = strings.TrimSpace(model)
	manufacturer = strings.TrimSpace(manufacturer)
	if model == "" || manufacturer == "" {
		return nil, nil
	}
	var row HsModelTask
	err := r.d.DB.WithContext(ctx).
		Where("model = ? AND manufacturer = ?", model, manufacturer).
		Order("updated_at DESC").
		Limit(1).
		First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return hsModelTaskToBiz(&row), nil
}

func (r *HsModelTaskRepo) Save(ctx context.Context, row *biz.HsModelTaskRecord) error {
	if !r.DBOk() || row == nil {
		return gorm.ErrInvalidDB
	}
	runID := strings.TrimSpace(row.RunID)
	if runID == "" {
		return nil
	}
	m := HsModelTask{
		RunID:          runID,
		Model:          strings.TrimSpace(row.Model),
		Manufacturer:   strings.TrimSpace(row.Manufacturer),
		RequestTraceID: strings.TrimSpace(row.RequestTraceID),
		TaskStatus:     strings.TrimSpace(row.TaskStatus),
		ResultStatus:   strings.TrimSpace(row.ResultStatus),
		Stage:          strings.TrimSpace(row.Stage),
		AttemptCount:   row.AttemptCount,
		LastError:      row.LastError,
		BestScore:      row.BestScore,
		BestCodeTS:     strings.TrimSpace(row.BestCodeTS),
	}
	return r.d.DB.WithContext(ctx).Save(&m).Error
}

func hsModelTaskToBiz(row *HsModelTask) *biz.HsModelTaskRecord {
	if row == nil {
		return nil
	}
	return &biz.HsModelTaskRecord{
		Model:          row.Model,
		Manufacturer:   row.Manufacturer,
		RequestTraceID: row.RequestTraceID,
		RunID:          row.RunID,
		TaskStatus:     row.TaskStatus,
		ResultStatus:   row.ResultStatus,
		Stage:          row.Stage,
		AttemptCount:   row.AttemptCount,
		LastError:      row.LastError,
		BestScore:      row.BestScore,
		BestCodeTS:     row.BestCodeTS,
		UpdatedAt:      row.UpdatedAt,
	}
}

var _ biz.HsModelTaskRepo = (*HsModelTaskRepo)(nil)
