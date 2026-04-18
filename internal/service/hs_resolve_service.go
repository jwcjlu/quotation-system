package service

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	v1 "caichip/api/bom/v1"
	"caichip/internal/biz"
	"caichip/internal/conf"
	"caichip/internal/data"

	kerrors "github.com/go-kratos/kratos/v2/errors"
	"github.com/go-kratos/kratos/v2/log"
)

const hsResolveDecisionMode = "auto_top1_with_top3_audit"

type hsResolveRunner interface {
	ResolveByModel(ctx context.Context, req biz.HsModelResolveRequest) (*biz.HsModelTaskRecord, error)
}

type hsTaskQuery interface {
	GetByRunID(ctx context.Context, runID string) (*biz.HsModelTaskRecord, error)
	GetLatestByModelManufacturer(ctx context.Context, model, manufacturer string) (*biz.HsModelTaskRecord, error)
}

type hsRecommendationLister interface {
	ListByRunID(ctx context.Context, runID string) ([]biz.HsModelRecommendationRecord, error)
}

type hsDatasheetSource interface {
	GetLatestByModelManufacturer(ctx context.Context, model, manufacturer string) (*biz.HsDatasheetAssetRecord, error)
}

// hsBomDatasheetLister 由 BOM 报价明细提供多行 datasheet 候选（设计 §4.2）。
type hsBomDatasheetLister interface {
	ListQuoteDatasheetCandidates(ctx context.Context, model, manufacturer string) ([]biz.HsDatasheetCandidate, error)
}

type hsConfirmer interface {
	Confirm(ctx context.Context, req biz.HsModelConfirmRequest) (*biz.HsModelConfirmResult, error)
}

type HsResolveService struct {
	resolver    hsResolveRunner
	taskQuery   hsTaskQuery
	recoRepo    hsRecommendationLister
	datasheet   hsDatasheetSource
	confirmer   hsConfirmer
	syncTimeout time.Duration
	log         *log.Helper

	mu           sync.RWMutex
	taskMap      map[string]*biz.HsModelTaskRecord
	recommendTop int
}

type hsMemoryTaskRepo struct {
	mu      sync.RWMutex
	byRunID map[string]biz.HsModelTaskRecord
	byReq   map[string]string
	latest  map[string]string
}

func newHsMemoryTaskRepo() *hsMemoryTaskRepo {
	return &hsMemoryTaskRepo{
		byRunID: make(map[string]biz.HsModelTaskRecord),
		byReq:   make(map[string]string),
		latest:  make(map[string]string),
	}
}

func (r *hsMemoryTaskRepo) reqKey(model, manufacturer, traceID string) string {
	return strings.TrimSpace(model) + "|" + strings.TrimSpace(manufacturer) + "|" + strings.TrimSpace(traceID)
}

func (r *hsMemoryTaskRepo) latestKey(model, manufacturer string) string {
	return strings.TrimSpace(model) + "|" + strings.TrimSpace(manufacturer)
}

func (r *hsMemoryTaskRepo) GetByRequestTraceID(_ context.Context, model, manufacturer, requestTraceID string) (*biz.HsModelTaskRecord, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	runID, ok := r.byReq[r.reqKey(model, manufacturer, requestTraceID)]
	if !ok {
		return nil, nil
	}
	row, ok := r.byRunID[runID]
	if !ok {
		return nil, nil
	}
	cp := row
	return &cp, nil
}

func (r *hsMemoryTaskRepo) GetByRunID(_ context.Context, runID string) (*biz.HsModelTaskRecord, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	row, ok := r.byRunID[strings.TrimSpace(runID)]
	if !ok {
		return nil, nil
	}
	cp := row
	return &cp, nil
}

func (r *hsMemoryTaskRepo) GetLatestByModelManufacturer(_ context.Context, model, manufacturer string) (*biz.HsModelTaskRecord, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	runID, ok := r.latest[r.latestKey(model, manufacturer)]
	if !ok {
		return nil, nil
	}
	row, ok := r.byRunID[runID]
	if !ok {
		return nil, nil
	}
	cp := row
	return &cp, nil
}

func (r *hsMemoryTaskRepo) Save(_ context.Context, row *biz.HsModelTaskRecord) error {
	if row == nil || strings.TrimSpace(row.RunID) == "" {
		return nil
	}
	cp := *row
	cp.UpdatedAt = time.Now()
	r.mu.Lock()
	r.byRunID[cp.RunID] = cp
	if strings.TrimSpace(cp.RequestTraceID) != "" {
		r.byReq[r.reqKey(cp.Model, cp.Manufacturer, cp.RequestTraceID)] = cp.RunID
	}
	r.latest[r.latestKey(cp.Model, cp.Manufacturer)] = cp.RunID
	r.mu.Unlock()
	return nil
}

type hsMemoryConfirmRepo struct {
	mu   sync.RWMutex
	byID map[string]biz.HsModelConfirmResult
}

func newHsMemoryConfirmRepo() *hsMemoryConfirmRepo {
	return &hsMemoryConfirmRepo{byID: make(map[string]biz.HsModelConfirmResult)}
}

func (r *hsMemoryConfirmRepo) GetByConfirmRequestID(_ context.Context, confirmRequestID string) (*biz.HsModelConfirmResult, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	row, ok := r.byID[strings.TrimSpace(confirmRequestID)]
	if !ok {
		return nil, nil
	}
	cp := row
	return &cp, nil
}

func (r *hsMemoryConfirmRepo) Save(_ context.Context, row *biz.HsModelConfirmResult) error {
	if row == nil || strings.TrimSpace(row.ConfirmRequestID) == "" {
		return nil
	}
	cp := *row
	r.mu.Lock()
	r.byID[cp.ConfirmRequestID] = cp
	r.mu.Unlock()
	return nil
}

type hsAlwaysDownloadChecker struct{}

func (hsAlwaysDownloadChecker) CanDownload(_ context.Context, _ string) bool { return true }

type hsPassthroughDownloader struct{}

func (hsPassthroughDownloader) CanDownload(_ context.Context, _ string) bool { return true }

func (hsPassthroughDownloader) Download(_ context.Context, model, manufacturer, datasheetURL string) (*biz.HsDatasheetAssetRecord, error) {
	return &biz.HsDatasheetAssetRecord{
		Model:          strings.TrimSpace(model),
		Manufacturer:   strings.TrimSpace(manufacturer),
		DatasheetURL:   strings.TrimSpace(datasheetURL),
		DownloadStatus: "ok",
	}, nil
}

type hsSimpleFeatureExtractor struct{}

func (hsSimpleFeatureExtractor) Extract(_ context.Context, model, _ string, _ *biz.HsDatasheetAssetRecord) (biz.HsPrefilterInput, error) {
	return biz.HsPrefilterInput{
		ComponentName: strings.TrimSpace(model),
		TechCategory:  "其他",
		KeySpecs:      map[string]string{},
	}, nil
}

type hsSimpleRecommender struct{}

type hsSimplePrefilter struct{}

func (hsSimplePrefilter) Prefilter(_ context.Context, _ biz.HsPrefilterInput) ([]biz.HsItemCandidate, error) {
	return []biz.HsItemCandidate{
		{CodeTS: "0000000001", GName: "synthetic candidate", Score: 0.95},
		{CodeTS: "0000000002", GName: "synthetic candidate", Score: 0.94},
		{CodeTS: "0000000003", GName: "synthetic candidate", Score: 0.93},
	}, nil
}

func (hsSimpleRecommender) Recommend(_ context.Context, _ biz.HsPrefilterInput, candidates []biz.HsItemCandidate, limit int) ([]biz.HsItemCandidate, error) {
	if limit <= 0 {
		limit = 3
	}
	if len(candidates) == 0 {
		return nil, nil
	}
	if limit > len(candidates) {
		limit = len(candidates)
	}
	out := make([]biz.HsItemCandidate, 0, limit)
	out = append(out, candidates[:limit]...)
	return out, nil
}

type hsResolveLogObserver struct {
	log *log.Helper
}

func (o hsResolveLogObserver) RecordMetric(name string, value float64, labels ...string) {
	if o.log == nil {
		return
	}
	kv := []any{"metric_name", name, "value", value}
	for i := 0; i+1 < len(labels); i += 2 {
		kv = append(kv, labels[i], labels[i+1])
	}
	args := append([]any{"hs_resolve_metric"}, kv...)
	o.log.Info(args...)
}

func (o hsResolveLogObserver) EmitLog(event string, fields map[string]any) {
	if o.log == nil {
		return
	}
	kv := make([]any, 0, len(fields)*2+2)
	kv = append(kv, "event", event)
	for k, v := range fields {
		kv = append(kv, k, v)
	}
	args := append([]any{"hs_resolve_event"}, kv...)
	o.log.Info(args...)
}

func NewHsResolveService(
	resolver hsResolveRunner,
	taskQuery hsTaskQuery,
	recoRepo hsRecommendationLister,
	datasheet hsDatasheetSource,
	confirmer hsConfirmer,
	syncTimeout time.Duration,
) *HsResolveService {
	if syncTimeout <= 0 {
		syncTimeout = 8 * time.Second
	}
	return &HsResolveService{
		resolver:     resolver,
		taskQuery:    taskQuery,
		recoRepo:     recoRepo,
		datasheet:    datasheet,
		confirmer:    confirmer,
		syncTimeout:  syncTimeout,
		taskMap:      make(map[string]*biz.HsModelTaskRecord),
		recommendTop: 3,
	}
}

// NewDefaultHsResolveService 用于 Wire 注入的默认实现；当前仅保证 API 层可用性。
func NewDefaultHsResolveService(
	c *conf.Bootstrap,
	logger log.Logger,
	mappingRepo *data.HsModelMappingRepo,
	assetRepo *data.HsDatasheetAssetRepo,
	recoRepo *data.HsModelRecommendationRepo,
	itemQueryRepo *data.HsItemQueryRepo,
	hsTaskRepo *data.HsModelTaskRepo,
	openAIChat *data.OpenAIChat,
	featuresRepo *data.HsModelFeaturesRepo,
	mfrAliasLookup biz.AliasLookup,
) *HsResolveService {
	hsCfg := biz.NewHsResolveConfig(c)
	helper := log.NewHelper(logger)
	observer := hsResolveLogObserver{log: helper}
	taskRepo := biz.HsModelTaskRepo(newHsMemoryTaskRepo())
	if hsTaskRepo != nil && hsTaskRepo.DBOk() {
		taskRepo = hsTaskRepo
	}
	confirmRepo := newHsMemoryConfirmRepo()
	bomQuoteItemDatasheetSource := data.NewHsBomQuoteItemDatasheetSourceFromAssetRepo(assetRepo)
	var resolver hsResolveRunner
	if mappingRepo != nil && assetRepo != nil && recoRepo != nil && itemQueryRepo != nil && openAIChat != nil {
		extractClient := data.NewHsLLMExtractClient(openAIChat)
		recommendClient := data.NewHsLLMRecommendClient(openAIChat)
		extractor := data.NewHsLLMFeatureExtractor(extractClient)
		recommender := data.NewHsLLMCandidateRecommender(recommendClient)
		prefilter := biz.NewHsCandidatePrefilter(itemQueryRepo, biz.HsPrefilterUnboundedCap)
		assetDir := filepath.Join(os.TempDir(), "caichip", "hs_datasheets")
		downloader := data.NewHsDatasheetDownloader(assetDir, http.DefaultClient)
		br := biz.NewHsModelResolver(hsAlwaysDownloadChecker{}).
			WithAssetPersistence(downloader, assetRepo).
			WithStateMachine(taskRepo, recoRepo, mappingRepo).
			WithFeatureExtractor(extractor).
			WithCandidatePrefilter(prefilter).
			WithCandidateRecommender(recommender).
			WithAutoConfirmThreshold(hsCfg.AutoAcceptThreshold).
			WithObserver(observer).
			WithMaxStageRetries(hsCfg.ResolveRetryMax).
			WithManufacturerCanonicalizer(mfrAliasLookup)
		if featuresRepo != nil && featuresRepo.DBOk() {
			br = br.WithFeaturesRepo(featuresRepo)
		}
		resolver = br
	}
	confirmer := biz.NewHsModelConfirmService(taskRepo, recoRepo, mappingRepo, confirmRepo).
		WithObserver(observer).
		WithManufacturerCanonicalizer(mfrAliasLookup)
	svc := NewHsResolveService(
		resolver,
		taskRepo,
		recoRepo,
		bomQuoteItemDatasheetSource,
		confirmer,
		time.Duration(hsCfg.SyncTimeoutMs)*time.Millisecond,
	)
	svc.setRecommendTop(hsCfg.MaxCandidates)
	svc.log = helper
	return svc
}

func (s *HsResolveService) ResolveByModel(ctx context.Context, req *v1.HsResolveByModelRequest) (*v1.HsResolveByModelReply, error) {
	model := strings.TrimSpace(req.GetModel())
	manufacturer := strings.TrimSpace(req.GetManufacturer())
	traceID := strings.TrimSpace(req.GetRequestTraceId())
	if model == "" || manufacturer == "" || traceID == "" {
		return nil, kerrors.BadRequest("HS_RESOLVE_BAD_REQUEST", "model, manufacturer, request_trace_id are required")
	}
	if s.resolver == nil {
		return nil, kerrors.ServiceUnavailable("HS_RESOLVE_DISABLED", "hs resolve runner is not configured")
	}

	runID := s.makeRunID(model, manufacturer, traceID, req.GetForceRefresh())
	resolveReq := biz.HsModelResolveRequest{
		Model:             model,
		Manufacturer:      manufacturer,
		RequestTraceID:    traceID,
		RunID:             runID,
		ForceRefresh:      req.GetForceRefresh(),
		RecommendationTop: s.getRecommendTop(),
		DatasheetCands:    s.buildDatasheetCandidates(ctx, model, manufacturer),
	}
	taskID := runID
	runCh := make(chan struct {
		task *biz.HsModelTaskRecord
		err  error
	}, 1)

	go func() {
		task, err := s.resolver.ResolveByModel(context.WithoutCancel(ctx), resolveReq)
		if task != nil {
			s.setTask(taskID, task)
		}
		runCh <- struct {
			task *biz.HsModelTaskRecord
			err  error
		}{task: task, err: err}
	}()

	select {
	case out := <-runCh:
		if out.err != nil && out.task == nil {
			s.logResolveEvent("resolve.failed", map[string]any{
				"model":        model,
				"manufacturer": manufacturer,
				"task_id":      taskID,
				"run_id":       taskID,
				"stage":        "service_resolve",
				"final_status": biz.HsResultStatusRejected,
				"error_code":   "HS_RESOLVE_FAILED",
			})
			return nil, out.err
		}
		reply, err := s.buildResolveReply(ctx, taskID, out.task)
		if err != nil {
			return nil, err
		}
		// Keep async semantics consistent: unfinished task should still be accepted.
		reply.Accepted = out.task == nil || out.task.TaskStatus == biz.HsTaskStatusRunning
		s.logResolveEvent("resolve.reply", map[string]any{
			"model":        model,
			"manufacturer": manufacturer,
			"task_id":      taskID,
			"run_id":       reply.GetRunId(),
			"stage":        "service_resolve",
			"final_status": reply.GetResultStatus(),
			"error_code":   reply.GetErrorCode(),
		})
		return reply, nil
	case <-time.After(s.syncTimeout):
		s.logResolveEvent("resolve.accepted_async", map[string]any{
			"model":        model,
			"manufacturer": manufacturer,
			"task_id":      taskID,
			"run_id":       taskID,
			"stage":        "service_timeout",
			"final_status": biz.HsResultStatusPendingReview,
			"error_code":   "",
		})
		return &v1.HsResolveByModelReply{
			Accepted:     true,
			TaskId:       taskID,
			DecisionMode: hsResolveDecisionMode,
			TaskStatus:   biz.HsTaskStatusRunning,
			ResultStatus: biz.HsResultStatusPendingReview,
		}, nil
	}
}

func (s *HsResolveService) GetResolveTask(ctx context.Context, req *v1.HsResolveTaskRequest) (*v1.HsResolveTaskReply, error) {
	taskID := strings.TrimSpace(req.GetTaskId())
	if taskID == "" {
		return nil, kerrors.BadRequest("HS_RESOLVE_BAD_TASK_ID", "task_id is required")
	}
	task := s.getTask(taskID)
	if task == nil && s.taskQuery != nil {
		var err error
		task, err = s.taskQuery.GetByRunID(ctx, taskID)
		if err != nil {
			return nil, err
		}
	}
	if task == nil {
		return &v1.HsResolveTaskReply{
			TaskId:       taskID,
			DecisionMode: hsResolveDecisionMode,
			TaskStatus:   biz.HsTaskStatusRunning,
			ResultStatus: biz.HsResultStatusPendingReview,
		}, nil
	}
	out, err := s.buildTaskReply(ctx, taskID, task)
	if err != nil {
		return nil, err
	}
	s.logResolveEvent("resolve.task_polled", map[string]any{
		"task_id":      taskID,
		"run_id":       out.GetRunId(),
		"stage":        "service_poll",
		"final_status": out.GetResultStatus(),
		"error_code":   out.GetErrorCode(),
	})
	return out, nil
}

func (s *HsResolveService) ConfirmResolve(ctx context.Context, req *v1.HsResolveConfirmRequest) (*v1.HsResolveConfirmReply, error) {
	if s.confirmer == nil {
		return nil, kerrors.ServiceUnavailable("HS_RESOLVE_CONFIRM_DISABLED", "hs confirm service is not configured")
	}
	runID := strings.TrimSpace(req.GetRunId())
	if runID == "" {
		return nil, kerrors.BadRequest("HS_RESOLVE_CONFIRM_BAD_REQUEST", "run_id is required")
	}
	model := strings.TrimSpace(req.GetModel())
	manufacturer := strings.TrimSpace(req.GetManufacturer())
	if model == "" || manufacturer == "" {
		return nil, kerrors.BadRequest("HS_RESOLVE_CONFIRM_BAD_REQUEST", "model and manufacturer are required")
	}
	if s.taskQuery == nil {
		return nil, kerrors.ServiceUnavailable("HS_RESOLVE_CONFIRM_DISABLED", "task query is not configured")
	}
	task, err := s.taskQuery.GetByRunID(ctx, runID)
	if err != nil {
		return nil, err
	}
	if task == nil {
		return nil, kerrors.NotFound("HS_RESOLVE_RUN_NOT_FOUND", "run_id not found")
	}
	if task.Model != model || task.Manufacturer != manufacturer {
		return nil, kerrors.Conflict("HS_RESOLVE_CONFIRM_MODEL_MISMATCH", "run_id does not match model/manufacturer")
	}
	in := biz.HsModelConfirmRequest{
		RunID:            runID,
		CandidateRank:    uint8(req.GetCandidateRank()),
		ExpectedCodeTS:   strings.TrimSpace(req.GetExpectedCodeTs()),
		ConfirmRequestID: strings.TrimSpace(req.GetConfirmRequestId()),
	}
	res, err := s.confirmer.Confirm(ctx, in)
	if err != nil {
		if errors.Is(err, biz.ErrHsResolverConfirmRunNotLatest) {
			return nil, kerrors.Conflict("HS_RESOLVE_CONFIRM_RUN_NOT_LATEST", err.Error())
		}
		if errors.Is(err, biz.ErrHsResolverConfirmTupleMismatch) {
			return nil, kerrors.Conflict("HS_RESOLVE_CONFIRM_TUPLE_MISMATCH", err.Error())
		}
		if errors.Is(err, biz.ErrHsResolverInvalidRequest) {
			return nil, kerrors.BadRequest("HS_RESOLVE_CONFIRM_BAD_REQUEST", err.Error())
		}
		return nil, err
	}
	s.logResolveEvent("resolve.confirmed", map[string]any{
		"model":        model,
		"manufacturer": manufacturer,
		"task_id":      runID,
		"run_id":       runID,
		"stage":        "service_confirm",
		"final_status": biz.HsResultStatusConfirmed,
		"error_code":   "",
	})
	return &v1.HsResolveConfirmReply{
		RunId:            res.RunID,
		CandidateRank:    uint32(res.CandidateRank),
		CodeTs:           res.CodeTS,
		ConfirmRequestId: res.ConfirmRequestID,
		TaskStatus:       biz.HsTaskStatusSuccess,
		ResultStatus:     biz.HsResultStatusConfirmed,
		DecisionMode:     hsResolveDecisionMode,
	}, nil
}

func (s *HsResolveService) GetResolveHistory(ctx context.Context, req *v1.HsResolveHistoryRequest) (*v1.HsResolveHistoryReply, error) {
	runID := strings.TrimSpace(req.GetRunId())
	model := strings.TrimSpace(req.GetModel())
	manufacturer := strings.TrimSpace(req.GetManufacturer())
	var (
		task *biz.HsModelTaskRecord
		err  error
	)
	if s.taskQuery != nil && runID != "" {
		task, err = s.taskQuery.GetByRunID(ctx, runID)
		if err != nil {
			return nil, err
		}
	}
	if task == nil && runID != "" {
		task = s.getTask(runID)
	}
	if task == nil && runID == "" {
		if model == "" || manufacturer == "" {
			return nil, kerrors.BadRequest("HS_RESOLVE_HISTORY_BAD_REQUEST", "run_id or model+manufacturer is required")
		}
		if s.taskQuery == nil {
			return nil, kerrors.ServiceUnavailable("HS_RESOLVE_HISTORY_DISABLED", "task query is not configured")
		}
		task, err = s.taskQuery.GetLatestByModelManufacturer(ctx, model, manufacturer)
		if err != nil {
			return nil, err
		}
	}
	if task == nil {
		return &v1.HsResolveHistoryReply{}, nil
	}
	cands, err := s.loadCandidates(ctx, task.RunID)
	if err != nil {
		return nil, err
	}
	return &v1.HsResolveHistoryReply{
		Items: []*v1.HsResolveHistoryItem{
			{
				RunId:        task.RunID,
				DecisionMode: hsResolveDecisionMode,
				TaskStatus:   task.TaskStatus,
				ResultStatus: task.ResultStatus,
				BestCodeTs:   task.BestCodeTS,
				BestScore:    task.BestScore,
				Candidates:   cands,
			},
		},
	}, nil
}

func (s *HsResolveService) buildResolveReply(ctx context.Context, taskID string, task *biz.HsModelTaskRecord) (*v1.HsResolveByModelReply, error) {
	if task == nil {
		return &v1.HsResolveByModelReply{
			Accepted:     true,
			TaskId:       taskID,
			DecisionMode: hsResolveDecisionMode,
			TaskStatus:   biz.HsTaskStatusRunning,
			ResultStatus: biz.HsResultStatusPendingReview,
		}, nil
	}
	cands, err := s.loadCandidates(ctx, task.RunID)
	if err != nil {
		return nil, err
	}
	reply := &v1.HsResolveByModelReply{
		Accepted:     false,
		TaskId:       taskID,
		RunId:        task.RunID,
		DecisionMode: hsResolveDecisionMode,
		TaskStatus:   task.TaskStatus,
		ResultStatus: task.ResultStatus,
		BestCodeTs:   task.BestCodeTS,
		BestScore:    task.BestScore,
		Candidates:   cands,
	}
	if task.TaskStatus == biz.HsTaskStatusFailed {
		reply.ErrorCode = "HS_RESOLVE_FAILED"
		reply.ErrorMessage = task.LastError
	}
	return reply, nil
}

func (s *HsResolveService) buildTaskReply(ctx context.Context, taskID string, task *biz.HsModelTaskRecord) (*v1.HsResolveTaskReply, error) {
	cands, err := s.loadCandidates(ctx, task.RunID)
	if err != nil {
		return nil, err
	}
	reply := &v1.HsResolveTaskReply{
		TaskId:       taskID,
		RunId:        task.RunID,
		DecisionMode: hsResolveDecisionMode,
		TaskStatus:   task.TaskStatus,
		ResultStatus: task.ResultStatus,
		BestCodeTs:   task.BestCodeTS,
		BestScore:    task.BestScore,
		Candidates:   cands,
	}
	if task.TaskStatus == biz.HsTaskStatusFailed {
		reply.ErrorCode = "HS_RESOLVE_FAILED"
		reply.ErrorMessage = task.LastError
	}
	return reply, nil
}

func (s *HsResolveService) loadCandidates(ctx context.Context, runID string) ([]*v1.HsResolveCandidate, error) {
	if strings.TrimSpace(runID) == "" || s.recoRepo == nil {
		return nil, nil
	}
	rows, err := s.recoRepo.ListByRunID(ctx, runID)
	if err != nil {
		return nil, err
	}
	out := make([]*v1.HsResolveCandidate, 0, len(rows))
	for i := range rows {
		out = append(out, &v1.HsResolveCandidate{
			CandidateRank: uint32(rows[i].CandidateRank),
			CodeTs:        rows[i].CodeTS,
			Score:         rows[i].Score,
			Reason:        rows[i].Reason,
		})
	}
	return out, nil
}

func (s *HsResolveService) getTask(taskID string) *biz.HsModelTaskRecord {
	s.mu.RLock()
	defer s.mu.RUnlock()
	task := s.taskMap[taskID]
	if task == nil {
		return nil
	}
	cp := *task
	return &cp
}

func (s *HsResolveService) setTask(taskID string, task *biz.HsModelTaskRecord) {
	if task == nil {
		return
	}
	cp := *task
	s.mu.Lock()
	s.taskMap[taskID] = &cp
	s.taskMap[task.RunID] = &cp
	s.mu.Unlock()
}

func (s *HsResolveService) setRecommendTop(v int) {
	if s == nil || v <= 0 {
		return
	}
	s.mu.Lock()
	s.recommendTop = v
	s.mu.Unlock()
}

func (s *HsResolveService) getRecommendTop() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.recommendTop <= 0 {
		return 3
	}
	return s.recommendTop
}

func (s *HsResolveService) makeTaskID(model, manufacturer, traceID string) string {
	return strings.TrimSpace(model) + "|" + strings.TrimSpace(manufacturer) + "|" + strings.TrimSpace(traceID)
}

func (s *HsResolveService) makeRunID(model, manufacturer, traceID string, forceRefresh bool) string {
	base := s.makeTaskID(model, manufacturer, traceID)
	if !forceRefresh {
		return base
	}
	return fmt.Sprintf("%s|refresh-%d", base, time.Now().UnixNano())
}

func (s *HsResolveService) buildDatasheetCandidates(ctx context.Context, model, manufacturer string) []biz.HsDatasheetCandidate {
	if s.datasheet == nil {
		return nil
	}
	if lister, ok := s.datasheet.(hsBomDatasheetLister); ok {
		cands, err := lister.ListQuoteDatasheetCandidates(ctx, model, manufacturer)
		if err != nil || len(cands) == 0 {
			return nil
		}
		return cands
	}
	asset, err := s.datasheet.GetLatestByModelManufacturer(ctx, model, manufacturer)
	if err != nil || asset == nil || strings.TrimSpace(asset.DatasheetURL) == "" {
		return nil
	}
	return []biz.HsDatasheetCandidate{
		{
			ID:           asset.ID,
			DatasheetURL: strings.TrimSpace(asset.DatasheetURL),
			UpdatedAt:    asset.UpdatedAt,
		},
	}
}

func (s *HsResolveService) logResolveEvent(event string, fields map[string]any) {
	if s == nil || s.log == nil {
		return
	}
	kv := make([]any, 0, len(fields)*2+2)
	kv = append(kv, "event", event)
	for k, v := range fields {
		kv = append(kv, k, v)
	}
	args := append([]any{"hs_resolve_event"}, kv...)
	s.log.Info(args...)
}

var _ v1.HsResolveServiceHTTPServer = (*HsResolveService)(nil)
