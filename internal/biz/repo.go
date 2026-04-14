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
var ErrHSPolicySourceUnavailable = errors.New("hs policy source unavailable")

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
	// MarkAgentsOfflineBefore 将「无任务心跳或心跳早于 cutoff」的 Agent 标为 offline（运维列表与库内状态一致）。
	MarkAgentsOfflineBefore(ctx context.Context, cutoff time.Time) (int64, error)
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
	// LoadQuoteCachesForKeys 按业务日批量加载报价缓存；返回 map 仅含命中行，键为 MpnNorm+"\x00"+PlatformID（与 Normalize 后一致）。
	LoadQuoteCachesForKeys(ctx context.Context, bizDate time.Time, pairs []MpnPlatformPair) (map[string]*QuoteCacheSnapshot, error)
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

// BomPlatformScript 采集平台配置（t_bom_platform_script）；platform_id 与 script_id 一一对应。
type BomPlatformScript struct {
	PlatformID    string
	ScriptID      string
	DisplayName   string
	Enabled       bool
	RunParamsJSON []byte
	UpdatedAt     time.Time
}

// BomPlatformScriptRepo 平台 run_params 与脚本映射。
type BomPlatformScriptRepo interface {
	DBOk() bool
	List(ctx context.Context) ([]BomPlatformScript, error)
	Get(ctx context.Context, platformID string) (*BomPlatformScript, error)
	Upsert(ctx context.Context, p *BomPlatformScript) error
	Delete(ctx context.Context, platformID string) error
}

// ManufacturerCanonicalDisplay 厂牌 canonical 下拉一行（与 t_bom_manufacturer_alias 聚合查询一致）。
type ManufacturerCanonicalDisplay struct {
	CanonicalID string
	DisplayName string
}

// BomManufacturerAliasRepo 厂牌别名表：点查 + 运维列表/写入。
type BomManufacturerAliasRepo interface {
	CanonicalID(ctx context.Context, aliasNorm string) (canonicalID string, ok bool, err error)
	DBOk() bool
	ListDistinctCanonicals(ctx context.Context, limit int) ([]ManufacturerCanonicalDisplay, error)
	CreateRow(ctx context.Context, canonicalID, displayName, alias, aliasNorm string) error
}

type HSClassifyPolicy struct {
	VersionID                string
	AutoPassConfidenceMin    float64
	AutoPassCompletenessMin  float64
	AutoPassTopGapMin        float64
	QuickReviewTopGapMin     float64
	QuickReviewConfidenceMin float64
	ForceReviewConfidenceMax float64
	ForceReviewCompleteness  float64
}

type HSReferenceCase struct {
	HSCode   string
	Title    string
	Reason   string
	Score    float64
	Evidence []string
}

type HSReviewWrite struct {
	RequestKey      string
	FinalHSCode     string
	ReviewRequired  bool
	ReviewReasons   []string
	PolicyVersionID string
}

type HSPolicyRepo interface {
	DBOk() bool
	LoadByDeclarationDate(ctx context.Context, declarationDate time.Time) (*HSClassifyPolicy, bool, error)
}

type HSCaseRepo interface {
	DBOk() bool
	SearchTopCases(ctx context.Context, req *HSClassifyRequest, topN int) ([]HSReferenceCase, error)
}

type HSReviewRepo interface {
	DBOk() bool
	SaveDecision(ctx context.Context, row HSReviewWrite) error
}
