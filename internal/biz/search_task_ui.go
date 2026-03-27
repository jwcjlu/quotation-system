package biz

import "strings"

// Search UI 四态（与 DB 多状态映射，用于 API / 前端展示）。
const (
	SearchUIStatePending = "pending"   // 待搜索
	SearchUISearching    = "searching" // 搜索中
	SearchUISucceeded    = "succeeded" // 搜索成功（含无型号命中）
	SearchUIFailed       = "failed"    // 搜索失败（含取消）
	SearchUIMissing      = "missing"   // 应有任务但未生成行（仅 coverage / gap 用）
)

// MapSearchTaskStateToQuad 将 bom_search_task.state 映射为四态（及扩展 missing 由调用方设置）。
func MapSearchTaskStateToQuad(dbState string) string {
	st := strings.ToLower(strings.TrimSpace(dbState))
	switch st {
	case "pending":
		return SearchUIStatePending
	case "dispatched", "running":
		return SearchUISearching
	case "succeeded_quotes", "succeeded_no_mpn":
		return SearchUISucceeded
	case "failed", "cancelled":
		return SearchUIFailed
	default:
		return SearchUIStatePending
	}
}
