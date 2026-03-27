package biz

import (
	"errors"
	"strings"
)

// ErrInvalidTaskTransition 状态机收到非法 (from, event) 组合。
var ErrInvalidTaskTransition = errors.New("bom_search_task: invalid transition")

// BomSearchTaskTransition 按设计 spec §3.2 Mermaid 计算下一状态；from/event 不区分大小写，返回的 to 为小写。
func BomSearchTaskTransition(from, event string) (to string, err error) {
	from = strings.ToLower(strings.TrimSpace(from))
	event = strings.ToLower(strings.TrimSpace(event))
	if from == "" || event == "" {
		return "", ErrInvalidTaskTransition
	}
	to, ok := bomSearchTaskTransitionTable[from][event]
	if !ok {
		return "", ErrInvalidTaskTransition
	}
	return to, nil
}

// bomSearchTaskTransitionTable 仅含设计文档明确列出的转移；终态 cancelled/skipped 无出边（除非后续产品扩展迁移脚本）。
var bomSearchTaskTransitionTable = map[string]map[string]string{
	"pending": {
		"claim_dispatch": "running",
		"bom_revoke":     "cancelled",
		"user_skip":      "skipped",
	},
	"running": {
		"result_ok_with_quotes": "succeeded",
		"result_ok_empty":       "no_result",
		"error_retryable":       "failed_retryable",
		"error_terminal":        "failed_terminal",
		"bom_revoke":            "cancelled",
		"user_skip_force":       "skipped",
	},
	"failed_retryable": {
		"retry_backoff":      "pending",
		"attempts_exhausted": "failed_terminal",
		"bom_revoke":         "cancelled",
	},
	"succeeded": {
		"line_deleted_platform_removed": "cancelled",
	},
	"no_result": {
		"line_deleted_platform_removed": "cancelled",
	},
}
