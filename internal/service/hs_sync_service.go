package service

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"time"

	"caichip/internal/biz"
)

type hsSyncJob struct {
	ID            int64
	TriggerType   string
	Status        string
	CoreHS6       []string
	CreatedBy     string
	StartedAt     time.Time
	FinishedAt    time.Time
	ResultSummary map[string]any
}

// HsSyncService 提供 /api/hs/sync* 与 /api/hs/items* 的应用层能力。
type HsSyncService struct {
	metaRepo      biz.HsMetaRepo
	itemRepo      biz.HsItemReadRepo
	itemWriteRepo biz.HsItemWriteRepo
	queryRepo     biz.HsQueryAPIRepo
	mu            sync.RWMutex
	nextID        int64
	jobs          []hsSyncJob
}

func NewHsSyncService(metaRepo biz.HsMetaRepo, itemRepo biz.HsItemReadRepo, itemWriteRepo biz.HsItemWriteRepo, queryRepo biz.HsQueryAPIRepo) *HsSyncService {
	return &HsSyncService{
		metaRepo:      metaRepo,
		itemRepo:      itemRepo,
		itemWriteRepo: itemWriteRepo,
		queryRepo:     queryRepo,
		nextID:        1,
		jobs:          make([]hsSyncJob, 0, 16),
	}
}

func (s *HsSyncService) Run(ctx context.Context, mode string, selected []string, createdBy string) (map[string]any, error) {
	mode = strings.TrimSpace(mode)
	if mode != "all_enabled" && mode != "selected" {
		return nil, errors.New("mode 仅支持 all_enabled 或 selected")
	}
	coreHS6, err := s.resolveCoreHS6(ctx, mode, selected)
	if err != nil {
		return nil, err
	}
	if s.itemWriteRepo == nil || !s.itemWriteRepo.DBOk() {
		return nil, errors.New("hs_item 写入仓储不可用")
	}
	if s.queryRepo == nil {
		return nil, errors.New("hs_query_api 仓储不可用")
	}
	coreSuccess := 0
	coreFailed := 0
	totalRows := 0
	failedCoreHS6 := make([]string, 0)
	for i := range coreHS6 {
		rows, fetchErr := s.queryRepo.FetchAllByCoreHS6(ctx, coreHS6[i])
		if fetchErr != nil {
			coreFailed++
			failedCoreHS6 = append(failedCoreHS6, coreHS6[i])
			continue
		}
		if saveErr := s.itemWriteRepo.UpsertByCodeTS(ctx, rows); saveErr != nil {
			coreFailed++
			failedCoreHS6 = append(failedCoreHS6, coreHS6[i])
			continue
		}
		coreSuccess++
		totalRows += len(rows)
	}
	status := "success"
	if coreFailed > 0 && coreSuccess > 0 {
		status = "partial_success"
	}
	if coreSuccess == 0 && coreFailed > 0 {
		status = "failed"
	}
	now := time.Now()
	s.mu.Lock()
	jobID := s.nextID
	s.nextID++
	job := hsSyncJob{
		ID:          jobID,
		TriggerType: "manual",
		Status:      status,
		CoreHS6:     append([]string(nil), coreHS6...),
		CreatedBy:   strings.TrimSpace(createdBy),
		StartedAt:   now,
		FinishedAt:  now,
		ResultSummary: map[string]any{
			"core_total":      len(coreHS6),
			"core_success":    coreSuccess,
			"core_failed":     coreFailed,
			"row_total":       totalRows,
			"failed_core_hs6": failedCoreHS6,
		},
	}
	s.jobs = append([]hsSyncJob{job}, s.jobs...)
	s.mu.Unlock()

	return map[string]any{
		"job_id":         jobID,
		"trigger_type":   job.TriggerType,
		"status":         job.Status,
		"started_at":     job.StartedAt.UTC().Format(time.RFC3339Nano),
		"finished_at":    job.FinishedAt.UTC().Format(time.RFC3339Nano),
		"core_hs6":       coreHS6,
		"result_summary": job.ResultSummary,
	}, nil
}

func (s *HsSyncService) ListJobs(page, pageSize int32) map[string]any {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}
	s.mu.RLock()
	total := len(s.jobs)
	start := int((page - 1) * pageSize)
	if start > total {
		start = total
	}
	end := start + int(pageSize)
	if end > total {
		end = total
	}
	rows := append([]hsSyncJob(nil), s.jobs[start:end]...)
	s.mu.RUnlock()

	items := make([]map[string]any, 0, len(rows))
	for i := range rows {
		items = append(items, map[string]any{
			"id":               rows[i].ID,
			"trigger_type":     rows[i].TriggerType,
			"status":           rows[i].Status,
			"started_at":       rows[i].StartedAt.UTC().Format(time.RFC3339Nano),
			"finished_at":      rows[i].FinishedAt.UTC().Format(time.RFC3339Nano),
			"request_snapshot": map[string]any{"core_hs6": rows[i].CoreHS6},
			"result_summary":   rows[i].ResultSummary,
		})
	}
	return map[string]any{
		"items": items,
		"total": total,
	}
}

func (s *HsSyncService) JobDetail(id int64) (map[string]any, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for i := range s.jobs {
		if s.jobs[i].ID != id {
			continue
		}
		return map[string]any{
			"id":               s.jobs[i].ID,
			"trigger_type":     s.jobs[i].TriggerType,
			"status":           s.jobs[i].Status,
			"created_by":       s.jobs[i].CreatedBy,
			"started_at":       s.jobs[i].StartedAt.UTC().Format(time.RFC3339Nano),
			"finished_at":      s.jobs[i].FinishedAt.UTC().Format(time.RFC3339Nano),
			"request_snapshot": map[string]any{"core_hs6": s.jobs[i].CoreHS6},
			"result_summary":   s.jobs[i].ResultSummary,
		}, true
	}
	return nil, false
}

func (s *HsSyncService) ListItems(ctx context.Context, filter biz.HsItemListFilter) (map[string]any, error) {
	if s.itemRepo == nil || !s.itemRepo.DBOk() {
		return map[string]any{"items": []map[string]any{}, "total": 0}, nil
	}
	rows, total, err := s.itemRepo.List(ctx, filter)
	if err != nil {
		return nil, err
	}
	items := make([]map[string]any, 0, len(rows))
	for i := range rows {
		items = append(items, map[string]any{
			"code_ts":         rows[i].CodeTS,
			"g_name":          rows[i].GName,
			"unit_1":          rows[i].Unit1,
			"unit_2":          rows[i].Unit2,
			"control_mark":    rows[i].ControlMark,
			"source_core_hs6": rows[i].SourceCoreHS6,
		})
	}
	return map[string]any{
		"items": items,
		"total": total,
	}, nil
}

func (s *HsSyncService) ItemDetail(ctx context.Context, codeTS string) (map[string]any, bool, error) {
	if s.itemRepo == nil || !s.itemRepo.DBOk() {
		return nil, false, nil
	}
	row, err := s.itemRepo.GetByCodeTS(ctx, codeTS)
	if err != nil {
		return nil, false, err
	}
	if row == nil {
		return nil, false, nil
	}
	return map[string]any{
		"code_ts":         row.CodeTS,
		"g_name":          row.GName,
		"unit_1":          row.Unit1,
		"unit_2":          row.Unit2,
		"control_mark":    row.ControlMark,
		"source_core_hs6": row.SourceCoreHS6,
		"raw_json":        parseRawJSON(row.RawJSON),
	}, true, nil
}

func (s *HsSyncService) resolveCoreHS6(ctx context.Context, mode string, selected []string) ([]string, error) {
	if mode == "selected" {
		out := make([]string, 0, len(selected))
		seen := make(map[string]struct{}, len(selected))
		for i := range selected {
			v := strings.TrimSpace(selected[i])
			if err := biz.ValidateCoreHS6(v); err != nil {
				return nil, err
			}
			if _, ok := seen[v]; ok {
				continue
			}
			seen[v] = struct{}{}
			out = append(out, v)
		}
		if len(out) == 0 {
			return nil, errors.New("selected 模式下 core_hs6 不能为空")
		}
		return out, nil
	}
	if s.metaRepo == nil || !s.metaRepo.DBOk() {
		return []string{}, nil
	}
	enabled := true
	page := int32(1)
	pageSize := int32(200)
	out := make([]string, 0, 64)
	seen := map[string]struct{}{}
	for {
		rows, total, err := s.metaRepo.List(ctx, biz.HsMetaListFilter{
			Page:     page,
			PageSize: pageSize,
			Enabled:  &enabled,
		})
		if err != nil {
			return nil, err
		}
		for i := range rows {
			v := strings.TrimSpace(rows[i].CoreHS6)
			if v == "" {
				continue
			}
			if _, ok := seen[v]; ok {
				continue
			}
			seen[v] = struct{}{}
			out = append(out, v)
		}
		if int64(page*pageSize) >= total {
			break
		}
		page++
	}
	return out, nil
}

func parseRawJSON(raw []byte) any {
	if len(raw) == 0 {
		return map[string]any{}
	}
	var out any
	if err := json.Unmarshal(raw, &out); err != nil {
		return string(raw)
	}
	return out
}
