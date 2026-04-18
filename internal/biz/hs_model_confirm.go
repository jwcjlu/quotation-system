package biz

import (
	"context"
	"errors"
	"strings"
)

// HsModelConfirmRequest 人工确认请求。
type HsModelConfirmRequest struct {
	RunID            string
	CandidateRank    uint8
	ExpectedCodeTS   string
	ConfirmRequestID string
}

func (r HsModelConfirmRequest) normalized() HsModelConfirmRequest {
	r.RunID = strings.TrimSpace(r.RunID)
	r.ExpectedCodeTS = strings.TrimSpace(r.ExpectedCodeTS)
	r.ConfirmRequestID = strings.TrimSpace(r.ConfirmRequestID)
	return r
}

// HsModelConfirmResult 人工确认幂等结果。
type HsModelConfirmResult struct {
	RunID            string
	CandidateRank    uint8
	CodeTS           string
	ConfirmRequestID string
}

// HsModelConfirmRepo 保存确认请求幂等结果。
type HsModelConfirmRepo interface {
	GetByConfirmRequestID(ctx context.Context, confirmRequestID string) (*HsModelConfirmResult, error)
	Save(ctx context.Context, row *HsModelConfirmResult) error
}

type HsModelConfirmService struct {
	taskRepo    HsModelTaskRepo
	recoRepo    HsModelRecommendationRepo
	mappingRepo HsModelMappingRepo
	confirmRepo HsModelConfirmRepo
	observer    HsResolveObserver
	mfrCanon    *ManufacturerCanonicalizer
}

func NewHsModelConfirmService(taskRepo HsModelTaskRepo, recoRepo HsModelRecommendationRepo, mappingRepo HsModelMappingRepo, confirmRepo HsModelConfirmRepo) *HsModelConfirmService {
	return &HsModelConfirmService{
		taskRepo:    taskRepo,
		recoRepo:    recoRepo,
		mappingRepo: mappingRepo,
		confirmRepo: confirmRepo,
		observer:    noopHsResolveObserver{},
	}
}

func (s *HsModelConfirmService) WithObserver(observer HsResolveObserver) *HsModelConfirmService {
	if s == nil {
		return nil
	}
	if observer == nil {
		s.observer = noopHsResolveObserver{}
		return s
	}
	s.observer = observer
	return s
}

// WithManufacturerCanonicalizer 注入厂牌别名解析；lookup 为 nil 时视为无别名能力（按未命中处理）。
func (s *HsModelConfirmService) WithManufacturerCanonicalizer(lookup AliasLookup) *HsModelConfirmService {
	if s == nil {
		return nil
	}
	s.mfrCanon = NewManufacturerCanonicalizer(lookup)
	return s
}

// Confirm 在最新有效 run 上执行候选确认，并按 confirm_request_id 幂等。
func (s *HsModelConfirmService) Confirm(ctx context.Context, req HsModelConfirmRequest) (*HsModelConfirmResult, error) {
	if s == nil || s.taskRepo == nil || s.recoRepo == nil || s.mappingRepo == nil || s.confirmRepo == nil {
		return nil, errors.New("hs model confirm: dependencies not configured")
	}
	n := req.normalized()
	if n.RunID == "" || n.CandidateRank == 0 || n.ExpectedCodeTS == "" || n.ConfirmRequestID == "" {
		return nil, ErrHsResolverInvalidRequest
	}

	exists, err := s.confirmRepo.GetByConfirmRequestID(ctx, n.ConfirmRequestID)
	if err != nil {
		return nil, err
	}
	if exists != nil {
		return exists, nil
	}

	runTask, err := s.taskRepo.GetByRunID(ctx, n.RunID)
	if err != nil {
		return nil, err
	}
	if runTask == nil {
		return nil, ErrHsResolverInvalidRequest
	}
	latest, err := s.taskRepo.GetLatestByModelManufacturer(ctx, runTask.Model, runTask.Manufacturer)
	if err != nil {
		return nil, err
	}
	if latest == nil || latest.RunID != runTask.RunID {
		return nil, ErrHsResolverConfirmRunNotLatest
	}

	cands, err := s.recoRepo.ListByRunID(ctx, n.RunID)
	if err != nil {
		return nil, err
	}
	var picked *HsModelRecommendationRecord
	for i := range cands {
		if cands[i].CandidateRank == n.CandidateRank {
			picked = &cands[i]
			break
		}
	}
	if picked == nil || picked.CodeTS != n.ExpectedCodeTS {
		return nil, ErrHsResolverConfirmTupleMismatch
	}

	var canonPtr *string
	canon := s.mfrCanon
	if canon == nil {
		canon = NewManufacturerCanonicalizer(nil)
	}
	id, hit, err := canon.Resolve(ctx, runTask.Manufacturer)
	if err != nil {
		return nil, err
	}
	if hit {
		canonPtr = &id
	}

	if err := s.mappingRepo.Save(ctx, &HsModelMappingRecord{
		Model:                   runTask.Model,
		Manufacturer:            runTask.Manufacturer,
		ManufacturerCanonicalID: canonPtr,
		CodeTS:                  picked.CodeTS,
		Source:                  "manual",
		Confidence:              picked.Score,
		Status:                  HsResultStatusConfirmed,
	}); err != nil {
		return nil, err
	}

	runTask.ResultStatus = HsResultStatusConfirmed
	runTask.TaskStatus = HsTaskStatusSuccess
	runTask.Stage = HsTaskStageCompleted
	if err := s.taskRepo.Save(ctx, runTask); err != nil {
		return nil, err
	}

	result := &HsModelConfirmResult{
		RunID:            n.RunID,
		CandidateRank:    n.CandidateRank,
		CodeTS:           picked.CodeTS,
		ConfirmRequestID: n.ConfirmRequestID,
	}
	if err := s.confirmRepo.Save(ctx, result); err != nil {
		return nil, err
	}
	if s.observer != nil {
		s.observer.RecordMetric("hs_resolve_manual_override_total", 1)
		s.observer.EmitLog("resolve.manual_override", map[string]any{
			"model":           runTask.Model,
			"manufacturer":    runTask.Manufacturer,
			"task_id":         runTask.RunID,
			"run_id":          runTask.RunID,
			"stage":           HsTaskStageCompleted,
			"datasheet_url":   "",
			"datasheet_path":  "",
			"extract_model":   "",
			"recommend_model": "",
			"candidate_count": len(cands),
			"best_score":      picked.Score,
			"final_status":    runTask.ResultStatus,
			"error_code":      "",
		})
	}
	return result, nil
}
