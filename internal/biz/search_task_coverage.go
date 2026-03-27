package biz

// SearchTaskMissingEntry 期望存在但未找到 bom_search_task 行的一项（只读检查）。
type SearchTaskMissingEntry struct {
	LineID     string
	LineNo     int32
	MpnNorm    string
	PlatformID string
	Reason     string // 如 no_task_row
}

// SearchTaskCoverageReport 会话下行×平台 与 bom_search_task 对齐情况（只读）。
type SearchTaskCoverageReport struct {
	Consistent        bool
	Missing           []SearchTaskMissingEntry
	OrphanTaskCount   int
	ExpectedTaskCount int
	ExistingTaskCount int
}
