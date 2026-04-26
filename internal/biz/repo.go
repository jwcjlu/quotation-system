package biz

import (
	"context"
	"errors"
	"time"
)

// ErrDispatchLeaseMismatch 调度结果上报时 lease 与当前不一致或非 leased 态。
var ErrDispatchLeaseMismatch = errors.New("dispatch: lease mismatch or task not leased")

// ErrBOMSessionRevisionMismatch PutPlatforms 时 expected_revision 与库内不一致。
var ErrBOMSessionRevisionMismatch = errors.New("bom_session: selection_revision mismatch")

// ErrBOMImportStatusTransitionInvalid 会话导入状态非法迁移。
var ErrBOMImportStatusTransitionInvalid = errors.New("bom_session: invalid import status transition")

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
	LoadLeasedTask(ctx context.Context, taskID, leaseID string) (*DispatchLeasedTask, error)
	RequeueLeased(ctx context.Context, taskID, leaseID string, nextAttempt int, nextClaimAt time.Time, lastError string) error
	FailLeasedTerminal(ctx context.Context, taskID, leaseID, resultStatus, lastError string, finishedAt time.Time) error
	ListStaleLeasedTasks(ctx context.Context, now, offlineBefore time.Time) ([]StaleDispatchTask, error)
	PullAndLeaseForAgent(ctx context.Context, queue, agentID string, meta *AgentSchedulingMeta, running []RunningTaskReport, max int, leaseExtraSec int32) ([]TaskMessage, error)
	ListLeasedTasksByAgent(ctx context.Context, agentID string) ([]LeasedDispatchTaskRow, error)
}

// AgentRegistryRepo Agent 元数据（心跳、match 用快照）。
type AgentRegistryRepo interface {
	DBOk() bool
	UpsertTaskHeartbeat(ctx context.Context, agentID, queue, hostname string, scripts []InstalledScript, tags []string) error
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
	ListSearchTaskStatusRows(ctx context.Context, sessionID string) ([]SearchTaskStatusRow, error)
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
	LoadQuoteCachesForKeys(ctx context.Context, bizDate time.Time, pairs []MpnPlatformPair) (map[string]*QuoteCacheSnapshot, error)
	DistinctPendingMergeKeysForSession(ctx context.Context, sessionID string) ([]MergeKey, error)
	UpsertManualQuote(ctx context.Context, gapID uint64, row AgentQuoteRow) error
}

type BOMLineGapRepo interface {
	DBOk() bool
	UpsertOpenGaps(ctx context.Context, gaps []BOMLineGap) error
	ListLineGaps(ctx context.Context, sessionID string, statuses []string) ([]BOMLineGap, error)
	GetLineGap(ctx context.Context, gapID uint64) (*BOMLineGap, error)
	UpdateLineGapStatus(ctx context.Context, gapID uint64, fromStatus string, toStatus string, actor string, note string) error
	SelectLineGapSubstitute(ctx context.Context, gapID uint64, actor string, substituteMpn string, reason string) error
}

type BOMMatchRunRepo interface {
	DBOk() bool
	CreateMatchRun(ctx context.Context, sessionID string, selectionRevision int, currency string, createdBy string, items []BOMMatchResultItemDraft) (uint64, int, error)
	ListMatchRuns(ctx context.Context, sessionID string) ([]BOMMatchRunView, error)
	GetMatchRun(ctx context.Context, runID uint64) (*BOMMatchRunView, []BOMMatchResultItemDraft, error)
	SupersedePreviousRuns(ctx context.Context, sessionID string, keepRunID uint64) error
}

type BOMMatchRunView struct {
	ID                  uint64
	RunNo               int
	SessionID           string
	SelectionRevision   int
	Status              string
	LineTotal           int
	MatchedLineCount    int
	UnresolvedLineCount int
	TotalAmount         float64
	Currency            string
	CreatedBy           string
	CreatedAt           time.Time
	SavedAt             *time.Time
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
	ImportStatus      string
	ImportProgress    int
	ImportStage       string
	ImportMessage     string
	ImportErrorCode   string
	ImportError       string
	ImportUpdatedAt   *time.Time
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
	TryStartImport(ctx context.Context, sessionID, startedMessage string) (started bool, err error)
	UpdateImportState(ctx context.Context, sessionID string, patch BOMImportStatePatch) error
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

// AliasLookup 厂牌别名表查询：alias_norm 与 NormalizeMfrString 输出一致时命中。
// ok=true 且 err=nil 表示命中；ok=false 且 err=nil 表示无行；err!=nil 表示数据库等基础设施错误。
type AliasLookup interface {
	CanonicalID(ctx context.Context, aliasNorm string) (canonicalID string, ok bool, err error)
}

// ManufacturerCanonicalDisplay 厂牌 canonical 下拉一行。
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

// HsModelMappingRecord 型号到 code_ts 的映射记录。
type HsModelMappingRecord struct {
	Model                   string
	Manufacturer            string
	ManufacturerCanonicalID *string
	CodeTS                  string
	Source                  string
	Confidence              float64
	Status                  string
	FeaturesVersion         string
	RecommendationVersion   string
	CreatedAt               time.Time
	UpdatedAt               time.Time
}

// HsDatasheetAssetRecord datasheet 资产记录。
type HsDatasheetAssetRecord struct {
	ID             uint64
	Model          string
	Manufacturer   string
	DatasheetURL   string
	LocalPath      string
	SHA256         string
	DownloadStatus string
	ErrorMsg       string
	UpdatedAt      time.Time
}

// HsModelFeaturesRecord datasheet 抽取结构化特征。
type HsModelFeaturesRecord struct {
	ID                      uint64
	Model                   string
	Manufacturer            string
	ManufacturerCanonicalID *string
	AssetID                 uint64
	TechCategory            string
	TechCategoryRankedJSON  []byte
	ComponentName           string
	PackageForm             string
	KeySpecsJSON            []byte
	RawExtractJSON          []byte
	ExtractModel            string
	ExtractVersion          string
	CreatedAt               time.Time
}

// HsTechCategoryRank 有序备选类目（设计 §8.1，至多 3 条）。
type HsTechCategoryRank struct {
	Rank         int
	TechCategory string
	Confidence   float64
}

// HsPrefilterInput 候选预筛输入特征。
type HsPrefilterInput struct {
	TechCategory       string
	TechCategoryRanked []HsTechCategoryRank
	ComponentName      string
	PackageForm        string
	KeySpecs           map[string]string
}

// HsPrefilterScoreDetail 预筛评分明细（供审计）。
type HsPrefilterScoreDetail struct {
	TechCategoryMatched  bool
	ComponentNameMatched bool
	PackageFormMatched   bool
	KeySpecsMatched      []string
	KeySpecsMissed       []string
}

// HsItemCandidate HS 候选条目（含评分与明细）。
type HsItemCandidate struct {
	CodeTS        string
	GName         string
	Unit1         string
	Unit2         string
	ControlMark   string
	SourceCoreHS6 string
	RawJSON       []byte
	Score         float64
	Reason        string
	ScoreDetail   HsPrefilterScoreDetail
}

// HsItemQueryRepo 从 t_hs_item 按规则检索候选。
type HsItemQueryRepo interface {
	DBOk() bool
	QueryCandidatesByRules(ctx context.Context, input HsPrefilterInput, limit int) ([]HsItemCandidate, error)
}

// HsMetaRecord 一行 HS 元数据（配置侧）。
type HsMetaRecord struct {
	ID            uint64
	Category      string
	ComponentName string
	CoreHS6       string
	Description   string
	Enabled       bool
	SortOrder     int32
	UpdatedAt     time.Time
}

// HsMetaListFilter 元数据列表查询条件。
type HsMetaListFilter struct {
	Page, PageSize int32
	Category       string
	ComponentName  string
	CoreHS6        string
	Enabled        *bool
}

// HsMetaRepo t_hs_meta 读写（无 DB 时 DBOk=false，写操作失败）。
type HsMetaRepo interface {
	DBOk() bool
	List(ctx context.Context, filter HsMetaListFilter) ([]HsMetaRecord, int64, error)
	Create(ctx context.Context, row *HsMetaRecord) error
	Update(ctx context.Context, row *HsMetaRecord) error
	Delete(ctx context.Context, id uint64) error
	CountByCoreAndComponent(ctx context.Context, coreHS6, componentName string, excludeID uint64) (int64, error)
}

// HsModelRecommendationRecord 单轮推荐候选审计记录。
type HsModelRecommendationRecord struct {
	Model                   string
	Manufacturer            string
	ManufacturerCanonicalID *string
	RunID                   string
	CandidateRank           uint8
	CodeTS                  string
	GName                   string
	Score                   float64
	Reason                  string
	InputSnapshotJSON       []byte
	RecommendModel          string
	RecommendVersion        string
	CreatedAt               time.Time
}

// HsModelMappingRepo 持久化最终映射（仅仓储职责，不承载业务判定）。
type HsModelMappingRepo interface {
	DBOk() bool
	GetConfirmedByModelManufacturer(ctx context.Context, model, manufacturer string) (*HsModelMappingRecord, error)
	Save(ctx context.Context, row *HsModelMappingRecord) error
}

// HsTaxRateDailyRepo t_hs_tax_rate_daily 读写（按自然日缓存）。
type HsTaxRateDailyRepo interface {
	DBOk() bool
	GetManyByBizDate(ctx context.Context, bizDate time.Time, codeTSList []string) (map[string]*HsTaxRateDailyRecord, error)
	Upsert(ctx context.Context, row *HsTaxRateDailyRecord) error
}

// TaxRateAPIFetcher 进口税率外呼（由 data 适配 HsTaxRateAPIRepo）。
type TaxRateAPIFetcher interface {
	FetchByCodeTS(ctx context.Context, codeTS string, pageSize int) (*TaxRateFetchResult, error)
}

// HsManualDatasheetUploadRecord 暂存上传元数据（不含二进制）。
type HsManualDatasheetUploadRecord struct {
	UploadID     string
	LocalPath    string
	SHA256       string
	ExpiresAt    time.Time
	OwnerSubject string
	ConsumedAt   *time.Time
}

// HsManualDatasheetUploadRepo 用户上传 PDF staging 表。
type HsManualDatasheetUploadRepo interface {
	DBOk() bool
	Create(ctx context.Context, row *HsManualDatasheetUploadRecord) error
	GetByUploadID(ctx context.Context, uploadID string) (*HsManualDatasheetUploadRecord, error)
	MarkConsumed(ctx context.Context, uploadID string) error
	DeleteExpiredBefore(ctx context.Context, t time.Time) (int64, error)
}

// HsDatasheetAssetRepo 持久化 datasheet 资产。
type HsDatasheetAssetRepo interface {
	DBOk() bool
	GetLatestByModelManufacturer(ctx context.Context, model, manufacturer string) (*HsDatasheetAssetRecord, error)
	Save(ctx context.Context, row *HsDatasheetAssetRecord) error
}

// HsModelFeaturesRepo 持久化抽取特征。
type HsModelFeaturesRepo interface {
	DBOk() bool
	Create(ctx context.Context, row *HsModelFeaturesRecord) (uint64, error)
}

// HsModelRecommendationRepo 持久化推荐审计结果。
type HsModelRecommendationRepo interface {
	DBOk() bool
	SaveTopN(ctx context.Context, rows []HsModelRecommendationRecord) error
	ListByRunID(ctx context.Context, runID string) ([]HsModelRecommendationRecord, error)
}
