package biz

import (
	"context"
	"errors"
	"time"
)

// ErrDispatchLeaseMismatch 调度结果上报时 lease 与当前不一致或非 leased 态（由 data 层返回；service 可映射为 ErrLeaseReassigned）。
var ErrDispatchLeaseMismatch = errors.New("dispatch: lease mismatch or task not leased")

// ErrBOMSessionRevisionMismatch PutPlatforms 时 expected_revision 与库内不一致。
var ErrBOMSessionRevisionMismatch = errors.New("bom_session: selection_revision mismatch")

// AgentRegistrySummary 运维列表用 Agent 一行快照。
type AgentRegistrySummary struct {
	AgentID             string
	Queue               string
	Hostname            string
	LastTaskHeartbeatAt *time.Time
}

// AgentInstalledScriptRow 某 Agent 已安装脚本一行（含更新时间）。
type AgentInstalledScriptRow struct {
	ScriptID  string
	Version   string
	EnvStatus string
	UpdatedAt time.Time
}

// LeasedDispatchTaskRow 某 Agent 当前租约中的调度任务摘要。
type LeasedDispatchTaskRow struct {
	TaskID          string
	ScriptID        string
	Version         string
	LeasedAt        *time.Time
	LeaseDeadlineAt *time.Time
}

// DispatchTaskRepo 调度队列表持久化（caichip_dispatch_task）。
type DispatchTaskRepo interface {
	DBOk() bool
	Ping(ctx context.Context) error
	EnqueuePending(ctx context.Context, t *QueuedTask) error
	ReclaimStaleLeases(ctx context.Context, now, offlineBefore time.Time) (int64, error)
	FinishLeased(ctx context.Context, taskID, leaseID, resultStatus string) error
	PullAndLeaseForAgent(ctx context.Context, queue, agentID string, meta *AgentSchedulingMeta, running []RunningTaskReport, max int, leaseExtraSec int32) ([]TaskMessage, error)
	ListLeasedTasksByAgent(ctx context.Context, agentID string) ([]LeasedDispatchTaskRow, error)
}

// AgentRegistryRepo Agent 元数据（心跳、match 用快照）。
type AgentRegistryRepo interface {
	DBOk() bool
	UpsertTaskHeartbeat(ctx context.Context, agentID, queue, hostname string, scripts []InstalledScript, tags []string) error
	LoadSchedulingMeta(ctx context.Context, agentID string) (*AgentSchedulingMeta, error)
	ListAgentRegistrySummaries(ctx context.Context) ([]AgentRegistrySummary, error)
	ListInstalledScriptsForAgent(ctx context.Context, agentID string) ([]AgentInstalledScriptRow, error)
}

// BOMSearchTaskLookup 按 caichip_task_id 定位到的 bom_search_task 业务键。
type BOMSearchTaskLookup struct {
	SessionID  string
	MpnNorm    string
	PlatformID string
	BizDate    time.Time
}

// QuoteCacheSnapshot 报价缓存只读片段（合并键命中时用）。
type QuoteCacheSnapshot struct {
	Outcome     string
	QuotesJSON  []byte
	NoMpnDetail []byte
}

// BOMSearchTaskRepo BOM 搜索任务与报价缓存持久化。
type BOMSearchTaskRepo interface {
	DBOk() bool
	LoadSearchTaskByCaichipTaskID(ctx context.Context, caichipTaskID string) (*BOMSearchTaskLookup, error)
	FinalizeSearchTask(ctx context.Context, sessionID, mpnNorm, platformID string, bizDate time.Time, caichipTaskID, state string, lastErr *string, quoteOutcome string, quotesJSON, noMpnDetail []byte) error
	ListTasksForSession(ctx context.Context, sessionID string) ([]TaskReadinessSnapshot, error)
	ListActiveBySession(ctx context.Context, sessionID string) ([]TaskReadinessSnapshot, error)
	CancelBySessionPlatform(ctx context.Context, sessionID, platformID string) (int64, error)
	MarkSkippedBySessionPlatform(ctx context.Context, sessionID, platformID string) (int64, error)
	CancelAllTasksBySession(ctx context.Context, sessionID string) error
	CancelTasksBySessionMpnNorm(ctx context.Context, sessionID, mpnNorm string) error
	UpsertPendingTasks(ctx context.Context, sessionID string, bizDate time.Time, selectionRevision int, pairs []MpnPlatformPair) error
	GetTaskStateBySessionKey(ctx context.Context, sessionID, mpnNorm, platformID string, bizDate time.Time) (state string, err error)
	UpdateTaskStateBySessionKey(ctx context.Context, sessionID, mpnNorm, platformID string, bizDate time.Time, state string) error
	ListSearchTaskLookupsByCaichipTaskID(ctx context.Context, caichipTaskID string) ([]BOMSearchTaskLookup, error)
	ListPendingLookupsByMergeKey(ctx context.Context, mpnNorm, platformID string, bizDate time.Time) ([]BOMSearchTaskLookup, error)
	LoadQuoteCacheByMergeKey(ctx context.Context, mpnNorm, platformID string, bizDate time.Time) (*QuoteCacheSnapshot, bool, error)
	DistinctPendingMergeKeysForSession(ctx context.Context, sessionID string) ([]MergeKey, error)
}

// MpnPlatformPair 会话内待搜索的 (规范化型号, 平台)。
type MpnPlatformPair struct {
	MpnNorm    string
	PlatformID string
}

// BOMSessionView 会话只读快照。
type BOMSessionView struct {
	SessionID         string
	Title             string
	CustomerName      string
	ContactPhone      string
	ContactEmail      string
	ContactExtra      string
	Status            string
	ReadinessMode     string
	BizDate           time.Time
	PlatformIDs       []string
	SelectionRevision int
}

// BOMSessionLineView 会话行只读快照。
type BOMSessionLineView struct {
	ID     int64
	LineNo int
	Mpn    string
}

// BOMSessionRepo bom_session / bom_session_line 持久化。
type BOMSessionRepo interface {
	DBOk() bool
	CreateSession(ctx context.Context, title string, platformIDs []string, customerName, contactPhone, contactEmail, contactExtra, readinessMode *string) (sessionID string, bizDate time.Time, selectionRevision int, err error)
	GetSession(ctx context.Context, sessionID string) (*BOMSessionView, error)
	PatchSession(ctx context.Context, sessionID string, title, customerName, contactPhone, contactEmail, contactExtra, readinessMode *string) error
	PutPlatforms(ctx context.Context, sessionID string, platformIDs []string, expectedRevision int32) (newRevision int, err error)
	ListSessions(ctx context.Context, page, pageSize int32, status, bizDate, q string) (items []BOMSessionListItem, total int32, err error)
	ReplaceSessionLines(ctx context.Context, sessionID string, lines []BomImportLine, parseMode *string) (nextLineNo int, err error)
	ListSessionLines(ctx context.Context, sessionID string) ([]BOMSessionLineView, error)
	SetSessionStatus(ctx context.Context, sessionID, status string) error
	CreateSessionLine(ctx context.Context, sessionID, mpn, mfr, pkg string, qty *float64, rawText, extraJSON *string) (lineID int64, lineNo int32, newRevision int, err error)
	DeleteSessionLine(ctx context.Context, sessionID string, lineID int64) error
	UpdateSessionLine(ctx context.Context, sessionID string, lineID int64, mpn, mfr, pkg *string, qty *float64, rawText, extraJSON *string) (newRevision int, err error)
}

// BOMSessionListItem 列表行。
type BOMSessionListItem struct {
	SessionID    string
	Title        string
	CustomerName string
	Status       string
	BizDate      string
	UpdatedAt    string
	LineCount    int32
}

// PublishedScriptMeta 已发布脚本包摘要（供 Agent 同步比对，与 data 表字段对齐的最小集）。
type PublishedScriptMeta struct {
	ScriptID       string
	Version        string
	SHA256         string
	StorageRelPath string
	Status         string
}

// AgentScriptPublishedLister 列出已发布脚本包（仅同步所需）。
type AgentScriptPublishedLister interface {
	DBOk() bool
	ListPublishedScripts(ctx context.Context) ([]PublishedScriptMeta, error)
}
