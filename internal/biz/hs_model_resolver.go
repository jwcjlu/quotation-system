package biz

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

// HsDatasheetCandidate 是候选 datasheet 源。
type HsDatasheetCandidate struct {
	ID           uint64
	DatasheetURL string
	UpdatedAt    time.Time
}

// HsDatasheetDownloadChecker 用于判定 URL 是否可下载。
type HsDatasheetDownloadChecker interface {
	CanDownload(ctx context.Context, url string) bool
}

// HsDatasheetAssetDownloader 负责执行 datasheet 下载。
type HsDatasheetAssetDownloader interface {
	CanDownload(ctx context.Context, url string) bool
	Download(ctx context.Context, model, manufacturer, datasheetURL string) (*HsDatasheetAssetRecord, error)
}

// HsModelResolver 负责 datasheet 选源。
type HsModelResolver struct {
	checker              HsDatasheetDownloadChecker
	downloader           HsDatasheetAssetDownloader
	assetRepo            HsDatasheetAssetRepo
	taskRepo             HsModelTaskRepo
	recoRepo             HsModelRecommendationRepo
	mappingRepo          HsModelMappingRepo
	featuresRepo         HsModelFeaturesRepo
	extractor            HsModelFeatureExtractor
	prefilter            HsModelCandidatePrefilter
	recommender          HsModelCandidateRecommender
	autoConfirmThreshold float64
	runIDGenerator       func() string
	maxStageRetries      int
	observer             HsResolveObserver
	mfrCanon             *ManufacturerCanonicalizer
}

func NewHsModelResolver(checker HsDatasheetDownloadChecker) *HsModelResolver {
	return &HsModelResolver{
		checker:              checker,
		autoConfirmThreshold: 0.9,
		maxStageRetries:      2,
		observer:             noopHsResolveObserver{},
		runIDGenerator: func() string {
			return fmt.Sprintf("run-%d", time.Now().UnixNano())
		},
	}
}

// WithAssetPersistence 注入下载器与资产仓储，用于闭环落库。
func (r *HsModelResolver) WithAssetPersistence(downloader HsDatasheetAssetDownloader, assetRepo HsDatasheetAssetRepo) *HsModelResolver {
	if r == nil {
		return nil
	}
	r.downloader = downloader
	r.assetRepo = assetRepo
	return r
}

func (r *HsModelResolver) WithStateMachine(taskRepo HsModelTaskRepo, recoRepo HsModelRecommendationRepo, mappingRepo HsModelMappingRepo) *HsModelResolver {
	if r == nil {
		return nil
	}
	r.taskRepo = taskRepo
	r.recoRepo = recoRepo
	r.mappingRepo = mappingRepo
	return r
}

// WithFeaturesRepo 注入特征落库；nil 或 DBOk=false 时跳过 t_hs_model_features 写入。
func (r *HsModelResolver) WithFeaturesRepo(repo HsModelFeaturesRepo) *HsModelResolver {
	if r == nil {
		return nil
	}
	r.featuresRepo = repo
	return r
}

type HsModelFeatureExtractor interface {
	Extract(ctx context.Context, model, manufacturer string, asset *HsDatasheetAssetRecord) (HsPrefilterInput, error)
}

type HsModelCandidatePrefilter interface {
	Prefilter(ctx context.Context, input HsPrefilterInput) ([]HsItemCandidate, error)
}

type HsModelCandidateRecommender interface {
	Recommend(ctx context.Context, input HsPrefilterInput, candidates []HsItemCandidate, limit int) ([]HsItemCandidate, error)
}

func (r *HsModelResolver) WithFeatureExtractor(extractor HsModelFeatureExtractor) *HsModelResolver {
	if r == nil {
		return nil
	}
	r.extractor = extractor
	return r
}

func (r *HsModelResolver) WithCandidateRecommender(recommender HsModelCandidateRecommender) *HsModelResolver {
	if r == nil {
		return nil
	}
	r.recommender = recommender
	return r
}

func (r *HsModelResolver) WithCandidatePrefilter(prefilter HsModelCandidatePrefilter) *HsModelResolver {
	if r == nil {
		return nil
	}
	r.prefilter = prefilter
	return r
}

func (r *HsModelResolver) WithAutoConfirmThreshold(threshold float64) *HsModelResolver {
	if r == nil {
		return nil
	}
	if threshold < 0 {
		threshold = 0
	}
	if threshold > 1 {
		threshold = 1
	}
	r.autoConfirmThreshold = threshold
	return r
}

func (r *HsModelResolver) WithRunIDGenerator(gen func() string) *HsModelResolver {
	if r == nil || gen == nil {
		return r
	}
	r.runIDGenerator = gen
	return r
}

func (r *HsModelResolver) WithMaxStageRetries(maxRetries int) *HsModelResolver {
	if r == nil {
		return nil
	}
	if maxRetries < 0 {
		maxRetries = 0
	}
	r.maxStageRetries = maxRetries
	return r
}

func (r *HsModelResolver) WithObserver(observer HsResolveObserver) *HsModelResolver {
	if r == nil {
		return nil
	}
	if observer == nil {
		r.observer = noopHsResolveObserver{}
		return r
	}
	r.observer = observer
	return r
}

// WithManufacturerCanonicalizer 注入厂牌别名解析；lookup 为 nil 时视为无别名能力（按未命中处理）。
func (r *HsModelResolver) WithManufacturerCanonicalizer(lookup AliasLookup) *HsModelResolver {
	if r == nil {
		return nil
	}
	r.mfrCanon = NewManufacturerCanonicalizer(lookup)
	return r
}

// SelectBestDatasheetSource 选择规则：
// 1) datasheet_url 非空且可下载；
// 2) updated_at 最新优先；
// 3) 若仍冲突按 id 倒序。
func (r *HsModelResolver) SelectBestDatasheetSource(ctx context.Context, candidates []HsDatasheetCandidate) *HsDatasheetCandidate {
	var best *HsDatasheetCandidate
	for i := range candidates {
		cur := candidates[i]
		cur.DatasheetURL = strings.TrimSpace(cur.DatasheetURL)
		if cur.DatasheetURL == "" {
			continue
		}
		if !r.canDownloadCandidate(ctx, cur.DatasheetURL) {
			continue
		}
		if best == nil {
			tmp := cur
			best = &tmp
			continue
		}
		if cur.UpdatedAt.After(best.UpdatedAt) || (cur.UpdatedAt.Equal(best.UpdatedAt) && cur.ID > best.ID) {
			tmp := cur
			best = &tmp
		}
	}
	return best
}

// ResolveAndPersistDatasheet 执行「选源 -> 下载 -> 写资产表」闭环。
func (r *HsModelResolver) ResolveAndPersistDatasheet(ctx context.Context, model, manufacturer string, candidates []HsDatasheetCandidate) (*HsDatasheetAssetRecord, error) {
	if r == nil || r.downloader == nil || r.assetRepo == nil {
		return nil, errors.New("hs model resolver: downloader/repo not configured")
	}
	chosen := r.SelectBestDatasheetSource(ctx, candidates)
	if chosen == nil {
		return nil, nil
	}
	record, dlErr := r.downloader.Download(ctx, model, manufacturer, chosen.DatasheetURL)
	if record == nil {
		record = &HsDatasheetAssetRecord{
			Model:          strings.TrimSpace(model),
			Manufacturer:   strings.TrimSpace(manufacturer),
			DatasheetURL:   strings.TrimSpace(chosen.DatasheetURL),
			DownloadStatus: "failed",
		}
	}
	if dlErr != nil {
		record.DownloadStatus = "failed"
		if strings.TrimSpace(record.ErrorMsg) == "" {
			record.ErrorMsg = dlErr.Error()
		}
		if strings.TrimSpace(record.DatasheetURL) == "" {
			record.DatasheetURL = strings.TrimSpace(chosen.DatasheetURL)
		}
	}
	if saveErr := r.assetRepo.Save(ctx, record); saveErr != nil {
		return record, saveErr
	}
	return record, dlErr
}

func (r *HsModelResolver) canDownloadCandidate(ctx context.Context, url string) bool {
	if r == nil {
		return false
	}
	// 统一判定来源：优先使用 downloader.CanDownload；未注入 downloader 时回退 checker。
	if r.downloader != nil {
		return r.downloader.CanDownload(ctx, url)
	}
	if r.checker != nil {
		return r.checker.CanDownload(ctx, url)
	}
	return false
}

// ResolveByModel 负责按型号执行 datasheet->extract->recommend 闭环，并维护 run 级状态。
func (r *HsModelResolver) ResolveByModel(ctx context.Context, req HsModelResolveRequest) (*HsModelTaskRecord, error) {
	startedAt := time.Now()
	if r == nil || r.taskRepo == nil || r.recoRepo == nil || r.mappingRepo == nil || r.extractor == nil || r.prefilter == nil || r.recommender == nil {
		return nil, errors.New("hs model resolver: state machine dependencies not configured (extractor/prefilter/recommender)")
	}
	n := req.normalized()
	if n.Model == "" || n.Manufacturer == "" || n.RequestTraceID == "" {
		return nil, ErrHsResolverInvalidRequest
	}

	if !n.ForceRefresh {
		existing, err := r.taskRepo.GetByRequestTraceID(ctx, n.Model, n.Manufacturer, n.RequestTraceID)
		if err != nil {
			return nil, err
		}
		if existing != nil {
			r.emitLog("resolve.idempotent_hit", map[string]any{
				"model":             n.Model,
				"manufacturer":      n.Manufacturer,
				"task_id":           existing.RunID,
				"run_id":            existing.RunID,
				"stage":             existing.Stage,
				"datasheet_url":     "",
				"datasheet_path":    "",
				"extract_model":     n.FeaturesVersion,
				"recommend_model":   n.RecommendModel,
				"candidate_count":   0,
				"best_score":        existing.BestScore,
				"final_status":      existing.ResultStatus,
				"error_code":        "",
				"hs_resolve_total":  1,
				"decision_mode":     "idempotent_reuse",
				"duration_ms_total": time.Since(startedAt).Milliseconds(),
			})
			return existing, nil
		}
	}
	confirmed, err := r.mappingRepo.GetConfirmedByModelManufacturer(ctx, n.Model, n.Manufacturer)
	if err != nil {
		return nil, err
	}
	if confirmed != nil && strings.TrimSpace(confirmed.CodeTS) != "" {
		task := &HsModelTaskRecord{
			Model:          n.Model,
			Manufacturer:   n.Manufacturer,
			RequestTraceID: n.RequestTraceID,
			RunID:          r.pickRunID(n),
			TaskStatus:     HsTaskStatusSuccess,
			ResultStatus:   HsResultStatusConfirmed,
			Stage:          HsTaskStageCompleted,
			BestCodeTS:     strings.TrimSpace(confirmed.CodeTS),
			BestScore:      confirmed.Confidence,
		}
		if err := r.taskRepo.Save(ctx, task); err != nil {
			return nil, err
		}
		r.recordMetric("hs_resolve_total", 1)
		r.recordMetric("hs_resolve_auto_accept_ratio", 1)
		r.recordMetric("hs_resolve_stage_latency_ms", float64(time.Since(startedAt).Milliseconds()), "stage", HsTaskStageCompleted)
		r.emitLog("resolve.mapping_fast_path", map[string]any{
			"model":           n.Model,
			"manufacturer":    n.Manufacturer,
			"task_id":         task.RunID,
			"run_id":          task.RunID,
			"stage":           HsTaskStageCompleted,
			"datasheet_url":   "",
			"datasheet_path":  "",
			"extract_model":   n.FeaturesVersion,
			"recommend_model": n.RecommendModel,
			"candidate_count": 0,
			"best_score":      task.BestScore,
			"final_status":    task.ResultStatus,
			"error_code":      "",
		})
		return task, nil
	}

	task := &HsModelTaskRecord{
		Model:          n.Model,
		Manufacturer:   n.Manufacturer,
		RequestTraceID: n.RequestTraceID,
		RunID:          r.pickRunID(n),
		TaskStatus:     HsTaskStatusRunning,
		ResultStatus:   HsResultStatusPendingReview,
		Stage:          HsTaskStageDatasheet,
		AttemptCount:   1,
	}
	if err := r.taskRepo.Save(ctx, task); err != nil {
		return nil, err
	}

	var (
		asset *HsDatasheetAssetRecord
		input HsPrefilterInput
		cands []HsItemCandidate
	)
	if err := r.retryStage(ctx, task, HsTaskStageDatasheet, func() error {
		got, dsErr := r.resolveDatasheetAsset(ctx, n)
		if dsErr != nil || got == nil || strings.TrimSpace(got.DownloadStatus) != "ok" {
			if dsErr != nil {
				return dsErr
			}
			return errors.New("datasheet not available")
		}
		asset = got
		return nil
	}, HsTaskStageDatasheetFailed); err != nil {
		r.recordMetric("hs_resolve_total", 1)
		r.recordMetric("hs_resolve_stage_latency_ms", float64(time.Since(startedAt).Milliseconds()), "stage", HsTaskStageDatasheetFailed)
		r.emitLog("resolve.failed", map[string]any{
			"model":           n.Model,
			"manufacturer":    n.Manufacturer,
			"task_id":         task.RunID,
			"run_id":          task.RunID,
			"stage":           task.Stage,
			"datasheet_url":   "",
			"datasheet_path":  "",
			"extract_model":   n.FeaturesVersion,
			"recommend_model": n.RecommendModel,
			"candidate_count": 0,
			"best_score":      0.0,
			"final_status":    task.ResultStatus,
			"error_code":      "DATASHEET_FAILED",
		})
		return task, err
	}

	if err := r.retryStage(ctx, task, HsTaskStageExtract, func() error {
		got, extractErr := r.extractor.Extract(ctx, n.Model, n.Manufacturer, asset)
		if extractErr != nil {
			return extractErr
		}
		input = got
		return nil
	}, HsTaskStageExtractFailed); err != nil {
		r.recordMetric("hs_resolve_total", 1)
		r.recordMetric("hs_resolve_stage_latency_ms", float64(time.Since(startedAt).Milliseconds()), "stage", HsTaskStageExtractFailed)
		r.emitLog("resolve.failed", map[string]any{
			"model":           n.Model,
			"manufacturer":    n.Manufacturer,
			"task_id":         task.RunID,
			"run_id":          task.RunID,
			"stage":           task.Stage,
			"datasheet_url":   safeAssetURL(asset),
			"datasheet_path":  safeAssetPath(asset),
			"extract_model":   n.FeaturesVersion,
			"recommend_model": n.RecommendModel,
			"candidate_count": 0,
			"best_score":      0.0,
			"final_status":    task.ResultStatus,
			"error_code":      "EXTRACT_FAILED",
		})
		return task, err
	}

	if strings.TrimSpace(input.TechCategory) == "" && len(input.TechCategoryRanked) == 0 {
		task.TaskStatus = HsTaskStatusFailed
		task.ResultStatus = HsResultStatusRejected
		task.Stage = HsTaskStageExtractFailed
		task.LastError = ErrHsResolverNoTechCategory.Error()
		_ = r.taskRepo.Save(ctx, task)
		r.recordMetric("hs_resolve_total", 1)
		r.recordMetric("hs_resolve_stage_latency_ms", float64(time.Since(startedAt).Milliseconds()), "stage", HsTaskStageExtractFailed)
		r.emitLog("resolve.failed", map[string]any{
			"model":           n.Model,
			"manufacturer":    n.Manufacturer,
			"task_id":         task.RunID,
			"run_id":          task.RunID,
			"stage":           task.Stage,
			"datasheet_url":   safeAssetURL(asset),
			"datasheet_path":  safeAssetPath(asset),
			"extract_model":   n.FeaturesVersion,
			"recommend_model": n.RecommendModel,
			"candidate_count": 0,
			"best_score":      0.0,
			"final_status":    task.ResultStatus,
			"error_code":      "NO_TECH_CATEGORY",
		})
		return task, ErrHsResolverNoTechCategory
	}

	if err := r.persistHsModelFeatures(ctx, n, asset, &input); err != nil {
		task.TaskStatus = HsTaskStatusFailed
		task.ResultStatus = HsResultStatusRejected
		task.Stage = HsTaskStageExtractFailed
		task.LastError = err.Error()
		_ = r.taskRepo.Save(ctx, task)
		r.recordMetric("hs_resolve_total", 1)
		r.recordMetric("hs_resolve_stage_latency_ms", float64(time.Since(startedAt).Milliseconds()), "stage", HsTaskStageExtractFailed)
		r.emitLog("resolve.failed", map[string]any{
			"model":           n.Model,
			"manufacturer":    n.Manufacturer,
			"task_id":         task.RunID,
			"run_id":          task.RunID,
			"stage":           task.Stage,
			"datasheet_url":   safeAssetURL(asset),
			"datasheet_path":  safeAssetPath(asset),
			"extract_model":   n.FeaturesVersion,
			"recommend_model": n.RecommendModel,
			"candidate_count": 0,
			"best_score":      0.0,
			"final_status":      task.ResultStatus,
			"error_code":        "FEATURES_SAVE_FAILED",
		})
		return task, err
	}

	recommendTop := n.RecommendationTop
	if err := r.retryStage(ctx, task, HsTaskStageRecommend, func() error {
		prefiltered, preErr := r.prefilter.Prefilter(ctx, input)
		if preErr != nil {
			return preErr
		}
		got, recoErr := r.recommender.Recommend(ctx, input, prefiltered, recommendTop)
		if recoErr != nil {
			return recoErr
		}
		if len(got) == 0 {
			return errors.New("recommend no candidate")
		}
		cands = got
		return nil
	}, HsTaskStageRecommendFailed); err != nil {
		r.recordMetric("hs_resolve_total", 1)
		r.recordMetric("hs_resolve_stage_latency_ms", float64(time.Since(startedAt).Milliseconds()), "stage", HsTaskStageRecommendFailed)
		r.emitLog("resolve.failed", map[string]any{
			"model":           n.Model,
			"manufacturer":    n.Manufacturer,
			"task_id":         task.RunID,
			"run_id":          task.RunID,
			"stage":           task.Stage,
			"datasheet_url":   safeAssetURL(asset),
			"datasheet_path":  safeAssetPath(asset),
			"extract_model":   n.FeaturesVersion,
			"recommend_model": n.RecommendModel,
			"candidate_count": 0,
			"best_score":      0.0,
			"final_status":    task.ResultStatus,
			"error_code":      "RECOMMEND_FAILED",
		})
		return task, err
	}
	sort.SliceStable(cands, func(i, j int) bool { return cands[i].Score > cands[j].Score })
	topN := min(len(cands), 3)
	mCanonPtr, err := r.manufacturerCanonicalPtr(ctx, n.Manufacturer)
	if err != nil {
		task.TaskStatus = HsTaskStatusFailed
		task.ResultStatus = HsResultStatusRejected
		task.Stage = HsTaskStageRecommendFailed
		task.LastError = err.Error()
		_ = r.taskRepo.Save(ctx, task)
		r.recordMetric("hs_resolve_total", 1)
		r.recordMetric("hs_resolve_stage_latency_ms", float64(time.Since(startedAt).Milliseconds()), "stage", HsTaskStageRecommendFailed)
		r.emitLog("resolve.failed", map[string]any{
			"model":             n.Model,
			"manufacturer":      n.Manufacturer,
			"task_id":           task.RunID,
			"run_id":            task.RunID,
			"stage":             task.Stage,
			"datasheet_url":     safeAssetURL(asset),
			"datasheet_path":    safeAssetPath(asset),
			"extract_model":     n.FeaturesVersion,
			"recommend_model":   n.RecommendModel,
			"candidate_count":   len(cands),
			"best_score":        0.0,
			"final_status":      task.ResultStatus,
			"error_code":        "MFR_CANONICAL_LOOKUP_FAILED",
		})
		return task, err
	}
	inputSnapshot, _ := json.Marshal(input)
	audits := make([]HsModelRecommendationRecord, 0, topN)
	for i := 0; i < topN; i++ {
		audits = append(audits, HsModelRecommendationRecord{
			Model:                   n.Model,
			Manufacturer:          n.Manufacturer,
			ManufacturerCanonicalID: mCanonPtr,
			RunID:                   task.RunID,
			CandidateRank:     uint8(i + 1),
			CodeTS:            cands[i].CodeTS,
			GName:             cands[i].GName,
			Score:             cands[i].Score,
			Reason:            hsRecoReasonOrDefault(cands[i].Reason),
			InputSnapshotJSON: inputSnapshot,
			RecommendModel:    n.RecommendModel,
			RecommendVersion:  n.RecommendVersion,
		})
	}
	if err := r.recoRepo.SaveTopN(ctx, audits); err != nil {
		task.TaskStatus = HsTaskStatusFailed
		task.ResultStatus = HsResultStatusRejected
		task.Stage = HsTaskStageRecommendFailed
		task.LastError = err.Error()
		_ = r.taskRepo.Save(ctx, task)
		r.recordMetric("hs_resolve_total", 1)
		r.recordMetric("hs_resolve_stage_latency_ms", float64(time.Since(startedAt).Milliseconds()), "stage", HsTaskStageRecommendFailed)
		r.emitLog("resolve.failed", map[string]any{
			"model":           n.Model,
			"manufacturer":    n.Manufacturer,
			"task_id":         task.RunID,
			"run_id":          task.RunID,
			"stage":           task.Stage,
			"datasheet_url":   safeAssetURL(asset),
			"datasheet_path":  safeAssetPath(asset),
			"extract_model":   n.FeaturesVersion,
			"recommend_model": n.RecommendModel,
			"candidate_count": len(cands),
			"best_score":      0.0,
			"final_status":    task.ResultStatus,
			"error_code":      "AUDIT_SAVE_FAILED",
		})
		return task, err
	}

	best := cands[0]
	task.BestScore = best.Score
	task.BestCodeTS = best.CodeTS
	task.Stage = HsTaskStageCompleted
	task.TaskStatus = HsTaskStatusSuccess
	task.LastError = ""
	task.ResultStatus = HsResultStatusPendingReview
	mappingStatus := HsResultStatusPendingReview
	if best.Score >= r.autoConfirmThreshold {
		task.ResultStatus = HsResultStatusConfirmed
		mappingStatus = HsResultStatusConfirmed
	}
	if err := r.mappingRepo.Save(ctx, &HsModelMappingRecord{
		Model:                   n.Model,
		Manufacturer:            n.Manufacturer,
		ManufacturerCanonicalID: mCanonPtr,
		CodeTS:                  best.CodeTS,
		Source:                  "llm_auto",
		Confidence:              best.Score,
		Status:                  mappingStatus,
		FeaturesVersion:         n.FeaturesVersion,
		RecommendationVersion:   n.RecommendVersion,
	}); err != nil {
		task.TaskStatus = HsTaskStatusFailed
		task.ResultStatus = HsResultStatusRejected
		task.Stage = HsTaskStageRecommendFailed
		task.LastError = err.Error()
		_ = r.taskRepo.Save(ctx, task)
		r.recordMetric("hs_resolve_total", 1)
		r.recordMetric("hs_resolve_stage_latency_ms", float64(time.Since(startedAt).Milliseconds()), "stage", HsTaskStageRecommendFailed)
		r.emitLog("resolve.failed", map[string]any{
			"model":           n.Model,
			"manufacturer":    n.Manufacturer,
			"task_id":         task.RunID,
			"run_id":          task.RunID,
			"stage":           task.Stage,
			"datasheet_url":   safeAssetURL(asset),
			"datasheet_path":  safeAssetPath(asset),
			"extract_model":   n.FeaturesVersion,
			"recommend_model": n.RecommendModel,
			"candidate_count": len(cands),
			"best_score":      best.Score,
			"final_status":    task.ResultStatus,
			"error_code":      "MAPPING_SAVE_FAILED",
		})
		return task, err
	}
	if err := r.taskRepo.Save(ctx, task); err != nil {
		return nil, err
	}
	r.recordMetric("hs_resolve_total", 1)
	if mappingStatus == HsResultStatusConfirmed {
		r.recordMetric("hs_resolve_auto_accept_ratio", 1)
	} else {
		r.recordMetric("hs_resolve_auto_accept_ratio", 0)
	}
	r.recordMetric("hs_resolve_stage_latency_ms", float64(time.Since(startedAt).Milliseconds()), "stage", HsTaskStageCompleted)
	r.emitLog("resolve.completed", map[string]any{
		"model":           n.Model,
		"manufacturer":    n.Manufacturer,
		"task_id":         task.RunID,
		"run_id":          task.RunID,
		"stage":           task.Stage,
		"datasheet_url":   safeAssetURL(asset),
		"datasheet_path":  safeAssetPath(asset),
		"extract_model":   n.FeaturesVersion,
		"recommend_model": n.RecommendModel,
		"candidate_count": len(cands),
		"best_score":      best.Score,
		"final_status":    task.ResultStatus,
		"error_code":      "",
	})
	return task, nil
}

func (r *HsModelResolver) manufacturerCanonicalPtr(ctx context.Context, manufacturer string) (*string, error) {
	if r == nil {
		return nil, nil
	}
	canon := r.mfrCanon
	if canon == nil {
		canon = NewManufacturerCanonicalizer(nil)
	}
	id, hit, err := canon.Resolve(ctx, manufacturer)
	if err != nil {
		return nil, err
	}
	if !hit {
		return nil, nil
	}
	return &id, nil
}

func (r *HsModelResolver) retryStage(ctx context.Context, task *HsModelTaskRecord, stage string, fn func() error, failedStage string) error {
	maxAttempts := r.maxStageRetries + 1
	if maxAttempts < 1 {
		maxAttempts = 1
	}
	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		task.Stage = stage
		task.AttemptCount = attempt
		if err := r.taskRepo.Save(ctx, task); err != nil {
			return err
		}
		if err := fn(); err == nil {
			return nil
		} else {
			lastErr = err
			task.LastError = err.Error()
		}
	}
	if lastErr == nil {
		lastErr = errors.New("unknown stage failure")
	}
	task.TaskStatus = HsTaskStatusFailed
	task.ResultStatus = HsResultStatusRejected
	task.Stage = failedStage
	task.LastError = lastErr.Error()
	_ = r.taskRepo.Save(ctx, task)
	return lastErr
}

func (r *HsModelResolver) pickRunID(req HsModelResolveRequest) string {
	if strings.TrimSpace(req.RunID) != "" {
		return strings.TrimSpace(req.RunID)
	}
	return r.runIDGenerator()
}

func (r *HsModelResolver) resolveDatasheetAsset(ctx context.Context, req HsModelResolveRequest) (*HsDatasheetAssetRecord, error) {
	if r.downloader != nil && r.assetRepo != nil {
		return r.ResolveAndPersistDatasheet(ctx, req.Model, req.Manufacturer, req.DatasheetCands)
	}
	chosen := r.SelectBestDatasheetSource(ctx, req.DatasheetCands)
	if chosen == nil {
		return nil, errors.New("datasheet not available")
	}
	return &HsDatasheetAssetRecord{
		Model:          req.Model,
		Manufacturer:   req.Manufacturer,
		DatasheetURL:   strings.TrimSpace(chosen.DatasheetURL),
		DownloadStatus: "ok",
	}, nil
}

func (r *HsModelResolver) recordMetric(name string, value float64, labels ...string) {
	if r == nil || r.observer == nil {
		return
	}
	r.observer.RecordMetric(name, value, labels...)
}

func (r *HsModelResolver) emitLog(event string, fields map[string]any) {
	if r == nil || r.observer == nil {
		return
	}
	r.observer.EmitLog(event, fields)
}

func safeAssetURL(asset *HsDatasheetAssetRecord) string {
	if asset == nil {
		return ""
	}
	return strings.TrimSpace(asset.DatasheetURL)
}

func safeAssetPath(asset *HsDatasheetAssetRecord) string {
	if asset == nil {
		return ""
	}
	return strings.TrimSpace(asset.LocalPath)
}

func hsRecoReasonOrDefault(s string) string {
	s = strings.TrimSpace(s)
	if s != "" {
		return s
	}
	return "auto recommend"
}
