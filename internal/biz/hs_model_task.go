package biz

import (
	"context"
	"errors"
	"strings"
	"time"
)

const (
	HsTaskStatusRunning = "running"
	HsTaskStatusSuccess = "success"
	HsTaskStatusFailed  = "failed"
)

const (
	HsResultStatusConfirmed     = "confirmed"
	HsResultStatusPendingReview = "pending_review"
	HsResultStatusRejected      = "rejected"
)

const (
	HsTaskStageDatasheet       = "datasheet"
	HsTaskStageExtract         = "extract"
	HsTaskStageRecommend       = "recommend"
	HsTaskStageCompleted       = "completed"
	HsTaskStageDatasheetFailed = "datasheet_failed"
	HsTaskStageExtractFailed   = "extract_failed"
	HsTaskStageRecommendFailed = "recommend_failed"
)

var (
	ErrHsResolverInvalidRequest       = errors.New("hs model resolver: invalid request")
	ErrHsResolverConfirmRunNotLatest  = errors.New("hs model resolver: confirm rejected, run is not latest")
	ErrHsResolverConfirmTupleMismatch = errors.New("hs model resolver: confirm candidate tuple mismatch")
)

// HsModelTaskRecord 表示一次按型号解析任务快照。
type HsModelTaskRecord struct {
	Model          string
	Manufacturer   string
	RequestTraceID string
	RunID          string
	TaskStatus     string
	ResultStatus   string
	Stage          string
	AttemptCount   int
	LastError      string
	BestScore      float64
	BestCodeTS     string
	UpdatedAt      time.Time
}

// HsModelTaskRepo 持久化任务状态（幂等、最新 run 判定）。
type HsModelTaskRepo interface {
	GetByRequestTraceID(ctx context.Context, model, manufacturer, requestTraceID string) (*HsModelTaskRecord, error)
	GetByRunID(ctx context.Context, runID string) (*HsModelTaskRecord, error)
	GetLatestByModelManufacturer(ctx context.Context, model, manufacturer string) (*HsModelTaskRecord, error)
	Save(ctx context.Context, row *HsModelTaskRecord) error
}

// HsModelResolveRequest 解析请求输入。
type HsModelResolveRequest struct {
	Model             string
	Manufacturer      string
	RequestTraceID    string
	RunID             string
	ForceRefresh      bool
	DatasheetCands    []HsDatasheetCandidate
	RecommendModel    string
	RecommendVersion  string
	FeaturesVersion   string
	RecommendationTop int
}

func (r HsModelResolveRequest) normalized() HsModelResolveRequest {
	r.Model = strings.TrimSpace(r.Model)
	r.Manufacturer = strings.TrimSpace(r.Manufacturer)
	r.RequestTraceID = strings.TrimSpace(r.RequestTraceID)
	r.RunID = strings.TrimSpace(r.RunID)
	if r.RecommendationTop <= 0 {
		r.RecommendationTop = 3
	}
	return r
}
