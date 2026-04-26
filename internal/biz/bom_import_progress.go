package biz

import "strings"

const (
	// BOMImportStatusIdle 尚未开始导入。
	BOMImportStatusIdle = "idle"
	// BOMImportStatusParsing 正在解析导入文件。
	BOMImportStatusParsing = "parsing"
	// BOMImportStatusReady 导入完成且可进入后续流程。
	BOMImportStatusReady = "ready"
	// BOMImportStatusFailed 导入失败。
	BOMImportStatusFailed = "failed"
)

const (
	// BOMImportStageValidating 校验输入。
	BOMImportStageValidating = "validating"
	// BOMImportStageHeaderInfer 解析表头映射。
	BOMImportStageHeaderInfer = "header_infer"
	// BOMImportStageChunkParsing 分块解析。
	BOMImportStageChunkParsing = "chunk_parsing"
	// BOMImportStagePersisting 持久化解析结果。
	BOMImportStagePersisting = "persisting"
	// BOMImportStageDone 导入完成。
	BOMImportStageDone = "done"
	// BOMImportStageFailed 导入失败结束态。
	BOMImportStageFailed = "failed"
)

// BOMImportStatePatch 会话导入状态更新参数。
type BOMImportStatePatch struct {
	Status    string
	Progress  int
	Stage     string
	Message   *string
	ErrorCode *string
	Error     *string
}

// ClampProgress 限制导入进度区间到 [0,100]。
func ClampProgress(v int) int {
	if v < 0 {
		return 0
	}
	if v > 100 {
		return 100
	}
	return v
}

// NormalizeImportStatus 归一化未知状态到 idle。
func NormalizeImportStatus(v string) string {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case BOMImportStatusIdle, BOMImportStatusParsing, BOMImportStatusReady, BOMImportStatusFailed:
		return strings.ToLower(strings.TrimSpace(v))
	default:
		return BOMImportStatusIdle
	}
}

// NormalizeImportStage 归一化未知阶段到 validating。
func NormalizeImportStage(v string) string {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case BOMImportStageValidating,
		BOMImportStageHeaderInfer,
		BOMImportStageChunkParsing,
		BOMImportStagePersisting,
		BOMImportStageDone,
		BOMImportStageFailed:
		return strings.ToLower(strings.TrimSpace(v))
	default:
		return BOMImportStageValidating
	}
}

// IsImportStatusTransitionAllowed 判断导入状态迁移是否合法。
func IsImportStatusTransitionAllowed(from, to string) bool {
	_ = NormalizeImportStatus(from)
	to = NormalizeImportStatus(to)
	if to == BOMImportStatusIdle {
		return false
	}
	return true
}
