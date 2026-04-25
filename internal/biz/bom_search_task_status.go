package biz

import (
	"strings"
	"time"
)

const (
	SearchTaskUIStatePending   = "pending"
	SearchTaskUIStateSearching = "searching"
	SearchTaskUIStateSucceeded = "succeeded"
	SearchTaskUIStateNoData    = "no_data"
	SearchTaskUIStateFailed    = "failed"
	SearchTaskUIStateSkipped   = "skipped"
	SearchTaskUIStateCancelled = "cancelled"
	SearchTaskUIStateMissing   = "missing"
)

type SearchTaskRetryMode string

const (
	SearchTaskRetryBatchAnomaly SearchTaskRetryMode = "batch_anomaly"
	SearchTaskRetrySingleManual SearchTaskRetryMode = "single_manual"
)

type SearchTaskStatusSummary struct {
	Total     int
	Pending   int
	Searching int
	Succeeded int
	NoData    int
	Failed    int
	Skipped   int
	Cancelled int
	Missing   int
	Retryable int
}

type SearchTaskStatusRow struct {
	LineID             uint64
	LineNo             int
	MpnRaw             string
	MpnNorm            string
	PlatformID         string
	PlatformName       string
	SearchTaskID       uint64
	SearchTaskState    string
	SearchUIState      string
	Retryable          bool
	RetryBlockedReason string
	DispatchTaskID     string
	DispatchTaskState  string
	DispatchAgentID    string
	DispatchResult     string
	LeaseDeadlineAt    *time.Time
	Attempt            int
	RetryMax           int
	UpdatedAt          *time.Time
	LastError          string
}

func NormalizeBOMSearchTaskState(state string) string {
	normalized := strings.ToLower(strings.TrimSpace(state))
	if normalized == "" {
		return SearchTaskUIStateMissing
	}
	return normalized
}

func MapBOMSearchTaskUIState(state string) string {
	switch NormalizeBOMSearchTaskState(state) {
	case "pending", "retry_backoff":
		return SearchTaskUIStatePending
	case "running", SearchTaskUIStateSearching:
		return SearchTaskUIStateSearching
	case "succeeded":
		return SearchTaskUIStateSucceeded
	case "no_result", SearchTaskUIStateNoData:
		return SearchTaskUIStateNoData
	case "failed_retryable", "failed_terminal", SearchTaskUIStateFailed:
		return SearchTaskUIStateFailed
	case "skipped":
		return SearchTaskUIStateSkipped
	case "cancelled":
		return SearchTaskUIStateCancelled
	case SearchTaskUIStateMissing:
		return SearchTaskUIStateMissing
	default:
		return SearchTaskUIStateFailed
	}
}

func CanRetryBOMSearchTask(state string, mode SearchTaskRetryMode) (bool, string) {
	normalized := NormalizeBOMSearchTaskState(state)

	switch normalized {
	case "failed_retryable", "failed_terminal", SearchTaskUIStateMissing:
		switch mode {
		case SearchTaskRetryBatchAnomaly, SearchTaskRetrySingleManual:
			return true, ""
		default:
			return false, "不支持的重试模式"
		}
	case "no_result":
		if mode == SearchTaskRetrySingleManual {
			return true, ""
		}
		if mode == SearchTaskRetryBatchAnomaly {
			return false, "无结果任务不纳入批量异常重试，可单条手动重试"
		}
		return false, "不支持的重试模式"
	case "pending", "retry_backoff":
		return false, "任务仍在等待执行，暂不能重试"
	case "running", SearchTaskUIStateSearching:
		return false, "任务正在搜索中，暂不能重试"
	case "succeeded":
		return false, "任务已成功，无需重试"
	case "skipped":
		return false, "任务已跳过，不能重试"
	case "cancelled":
		return false, "任务已取消，不能重试"
	default:
		return false, "未知任务状态，不能重试"
	}
}

func BuildSearchTaskStatusSummary(rows []SearchTaskStatusRow) SearchTaskStatusSummary {
	var summary SearchTaskStatusSummary
	summary.Total = len(rows)

	for _, row := range rows {
		uiState := strings.ToLower(strings.TrimSpace(row.SearchUIState))
		if uiState == "" {
			uiState = MapBOMSearchTaskUIState(row.SearchTaskState)
		}

		switch uiState {
		case SearchTaskUIStatePending:
			summary.Pending++
		case SearchTaskUIStateSearching:
			summary.Searching++
		case SearchTaskUIStateSucceeded:
			summary.Succeeded++
		case SearchTaskUIStateNoData:
			summary.NoData++
		case SearchTaskUIStateFailed:
			summary.Failed++
		case SearchTaskUIStateSkipped:
			summary.Skipped++
		case SearchTaskUIStateCancelled:
			summary.Cancelled++
		case SearchTaskUIStateMissing:
			summary.Missing++
		default:
			summary.Failed++
		}

		if row.Retryable {
			summary.Retryable++
		}
	}

	return summary
}
