package data

import (
	"database/sql"
	"time"
)

// CaichipDispatchTask 对应 t_caichip_dispatch_task。
type CaichipDispatchTask struct {
	ID               uint64         `gorm:"column:id;primaryKey;autoIncrement;index:idx_dispatch_claim,priority:3"`
	TaskID           string         `gorm:"column:task_id;size:128;uniqueIndex:uk_dispatch_task_id"`
	Queue            string         `gorm:"column:queue;size:128;not null;default:default;index:idx_dispatch_claim,priority:1"`
	ScriptID         string         `gorm:"column:script_id;size:128;not null"`
	Version          string         `gorm:"column:version;size:64;not null"`
	RequiredTags     []byte         `gorm:"column:required_tags;type:json"`
	EntryFile        sql.NullString `gorm:"column:entry_file;size:512"`
	TimeoutSec       int            `gorm:"column:timeout_sec;not null;default:300"`
	ParamsJSON       []byte         `gorm:"column:params_json;type:json"`
	ArgvJSON         []byte         `gorm:"column:argv_json;type:json"`
	Attempt          int            `gorm:"column:attempt;not null;default:1"`
	State            string         `gorm:"column:state;size:32;not null;default:pending;index:idx_dispatch_claim,priority:2;index:idx_dispatch_state_updated,priority:1;index:idx_dispatch_leased_agent,priority:2"`
	LeaseID          sql.NullString `gorm:"column:lease_id;size:64"`
	LeasedToAgentID  sql.NullString `gorm:"column:leased_to_agent_id;size:64;index:idx_dispatch_leased_agent,priority:1"`
	LeasedAt         *time.Time     `gorm:"column:leased_at;precision:3"`
	LeaseDeadlineAt  *time.Time     `gorm:"column:lease_deadline_at;precision:3"`
	NextClaimAt      *time.Time     `gorm:"column:next_claim_at;precision:3"`
	FinishedAt       *time.Time     `gorm:"column:finished_at;precision:3"`
	ResultStatus     sql.NullString `gorm:"column:result_status;size:32"`
	LastError        sql.NullString `gorm:"column:last_error;type:text"`
	RetryMax         int            `gorm:"column:retry_max;not null;default:0"`
	RetryBackoffJSON []byte         `gorm:"column:retry_backoff_json;type:json"`
	CreatedAt        time.Time      `gorm:"column:created_at;precision:3;autoCreateTime"`
	UpdatedAt        time.Time      `gorm:"column:updated_at;precision:3;autoUpdateTime;index:idx_dispatch_state_updated,priority:2"`
}

func (CaichipDispatchTask) TableName() string { return TableCaichipDispatchTask }

// CaichipAgent 对应 t_caichip_agent。
type CaichipAgent struct {
	ID                  uint64     `gorm:"column:id;primaryKey;autoIncrement"`
	AgentID             string     `gorm:"column:agent_id;size:64;uniqueIndex:uk_caichip_agent_agent_id;not null"`
	Queue               string     `gorm:"column:queue;size:128;not null;default:default"`
	Hostname            *string    `gorm:"column:hostname;size:256"` // 空串不入库时用 nil
	LastTaskHeartbeatAt *time.Time `gorm:"column:last_task_heartbeat_at;precision:3"`
	// AgentStatus 任务心跳维度：online / offline / unknown（列名 agent_status，避免与 SQL 保留字 status 冲突）
	AgentStatus string    `gorm:"column:agent_status;size:16;not null;default:unknown"`
	CreatedAt   time.Time `gorm:"column:created_at;precision:3;autoCreateTime"`
	UpdatedAt   time.Time `gorm:"column:updated_at;precision:3;autoUpdateTime"`
}

func (CaichipAgent) TableName() string { return TableCaichipAgent }

// CaichipAgentTag 对应 t_caichip_agent_tag。
type CaichipAgentTag struct {
	AgentID string `gorm:"column:agent_id;size:64;primaryKey"`
	Tag     string `gorm:"column:tag;size:256;primaryKey"`
}

func (CaichipAgentTag) TableName() string { return TableCaichipAgentTag }

// CaichipAgentInstalledScript 对应 t_caichip_agent_installed_script。
type CaichipAgentInstalledScript struct {
	AgentID   string    `gorm:"column:agent_id;size:64;primaryKey"`
	ScriptID  string    `gorm:"column:script_id;size:128;primaryKey"`
	Version   string    `gorm:"column:version;size:64;not null"`
	EnvStatus string    `gorm:"column:env_status;size:32;not null"`
	UpdatedAt time.Time `gorm:"column:updated_at;precision:3"`
}

func (CaichipAgentInstalledScript) TableName() string { return TableCaichipAgentInstalledScript }

// CaichipAgentScriptAuth Agent × script_id 站点登录凭据（密码密文）。
type CaichipAgentScriptAuth struct {
	ID             uint64    `gorm:"column:id;primaryKey;autoIncrement"`
	AgentID        string    `gorm:"column:agent_id;size:64;not null;uniqueIndex:uk_agent_script_auth,priority:1"`
	ScriptID       string    `gorm:"column:script_id;size:128;not null;uniqueIndex:uk_agent_script_auth,priority:2"`
	Username       string    `gorm:"column:username;size:256;not null"`
	PasswordCipher string    `gorm:"column:password_cipher;type:text;not null"`
	CreatedAt      time.Time `gorm:"column:created_at;precision:3;autoCreateTime"`
	UpdatedAt      time.Time `gorm:"column:updated_at;precision:3;autoUpdateTime"`
}

func (CaichipAgentScriptAuth) TableName() string { return TableCaichipAgentScriptAuth }

// BomSearchTask 对应 t_bom_search_task。
type BomSearchTask struct {
	ID                uint64         `gorm:"column:id;primaryKey;autoIncrement"`
	SessionID         string         `gorm:"column:session_id;size:36;not null;uniqueIndex:uk_bom_search;index:idx_bom_search_session_state,priority:1;index:idx_bom_search_mpn,priority:1"`
	MpnNorm           string         `gorm:"column:mpn_norm;size:256;not null;uniqueIndex:uk_bom_search;index:idx_bom_search_mpn,priority:2"`
	PlatformID        string         `gorm:"column:platform_id;size:32;not null;uniqueIndex:uk_bom_search;index:idx_bom_search_mpn,priority:3"`
	BizDate           time.Time      `gorm:"column:biz_date;type:date;not null;uniqueIndex:uk_bom_search;index:idx_bom_search_mpn,priority:4"`
	State             string         `gorm:"column:state;size:32;not null;default:pending;index:idx_bom_search_session_state,priority:2"`
	AutoAttempt       int            `gorm:"column:auto_attempt;not null;default:0"`
	ManualAttempt     int            `gorm:"column:manual_attempt;not null;default:0"`
	SelectionRevision int            `gorm:"column:selection_revision;not null"`
	CaichipTaskID     sql.NullString `gorm:"column:caichip_task_id;size:128;index:idx_bom_search_caichip_task"`
	LastError         sql.NullString `gorm:"column:last_error;type:text"`
	CreatedAt         time.Time      `gorm:"column:created_at;precision:3"`
	UpdatedAt         time.Time      `gorm:"column:updated_at;precision:3"`
}

func (BomSearchTask) TableName() string { return TableBomSearchTask }

// BomQuoteCache 对应 t_bom_quote_cache。
type BomQuoteCache struct {
	ID          uint64    `gorm:"column:id;primaryKey;autoIncrement"`
	MpnNorm     string    `gorm:"column:mpn_norm;size:256;not null;uniqueIndex:uk_bom_quote_cache_merge,priority:1"`
	PlatformID  string    `gorm:"column:platform_id;size:32;not null;uniqueIndex:uk_bom_quote_cache_merge,priority:2"`
	BizDate     time.Time `gorm:"column:biz_date;type:date;not null;uniqueIndex:uk_bom_quote_cache_merge,priority:3"`
	Outcome     string    `gorm:"column:outcome;size:32;not null"`
	QuotesJSON  []byte    `gorm:"column:quotes_json;type:json"`
	NoMpnDetail []byte    `gorm:"column:no_mpn_detail;type:json"`
	CreatedAt   time.Time `gorm:"column:created_at;precision:3;autoCreateTime"`
	UpdatedAt   time.Time `gorm:"column:updated_at;precision:3;autoUpdateTime;index:idx_bom_quote_cache_updated"`
}

func (BomQuoteCache) TableName() string { return TableBomQuoteCache }

// BomQuoteItem 对应 t_bom_quote_item（报价明细）。
type BomQuoteItem struct {
	ID                      uint64    `gorm:"column:id;primaryKey;autoIncrement"`
	QuoteID                 uint64    `gorm:"column:quote_id;not null;index:idx_bom_quote_item_quote_id"`
	Model                   string    `gorm:"column:model;size:255;not null;default:''"`
	Manufacturer            string    `gorm:"column:manufacturer;size:255;not null;default:''"`
	ManufacturerCanonicalID *string   `gorm:"column:manufacturer_canonical_id;size:128"`
	Stock                   string    `gorm:"column:stock;size:64;not null;default:''"`
	Package                 string    `gorm:"column:package;size:128;not null;default:''"`
	Desc                    string    `gorm:"column:desc;size:512;not null;default:''"`
	MOQ                     string    `gorm:"column:moq;size:64;not null;default:''"`
	LeadTime                string    `gorm:"column:lead_time;size:128;not null;default:''"`
	PriceTiers              string    `gorm:"column:price_tiers;type:text"`
	HKPrice                 string    `gorm:"column:hk_price;type:text"`
	MainlandPrice           string    `gorm:"column:mainland_price;type:text"`
	QueryModel              string    `gorm:"column:query_model;size:255;not null;default:''"`
	DatasheetURL            string    `gorm:"column:datasheet_url;type:text"`
	CreatedAt               time.Time `gorm:"column:created_at;precision:3;autoCreateTime"`
	UpdatedAt               time.Time `gorm:"column:updated_at;precision:3;autoUpdateTime"`
}

func (BomQuoteItem) TableName() string { return TableBomQuoteItem }

// BomSession 对应 t_bom_session。
type BomSession struct {
	ID                string    `gorm:"column:id;size:36;primaryKey"`
	Title             *string   `gorm:"column:title;size:256"`
	CustomerName      *string   `gorm:"column:customer_name;size:256"`
	ContactPhone      *string   `gorm:"column:contact_phone;size:64"`
	ContactEmail      *string   `gorm:"column:contact_email;size:256"`
	ContactExtra      *string   `gorm:"column:contact_extra;size:512"`
	Status            string    `gorm:"column:status;size:32;not null;default:draft;index:idx_bom_session_status"`
	ReadinessMode     string    `gorm:"column:readiness_mode;size:16;not null;default:lenient"`
	BizDate           time.Time `gorm:"column:biz_date;type:date;not null;index:idx_bom_session_biz_date"`
	SelectionRevision int       `gorm:"column:selection_revision;not null;default:1"`
	PlatformIDs       string    `gorm:"column:platform_ids;type:json;not null"`
	ParseMode         *string   `gorm:"column:parse_mode;size:32"`
	StorageFileKey    *string   `gorm:"column:storage_file_key;size:512"`
	CreatedAt         time.Time `gorm:"column:created_at;precision:3"`
	UpdatedAt         time.Time `gorm:"column:updated_at;precision:3;index:idx_bom_session_updated"`
}

func (BomSession) TableName() string { return TableBomSession }

// BomSessionLine 对应 t_bom_session_line。
type BomSessionLine struct {
	ID        int64     `gorm:"column:id;primaryKey;autoIncrement"`
	SessionID string    `gorm:"column:session_id;size:36;not null;index:idx_bom_line_session;uniqueIndex:uk_session_line;index:idx_bom_line_mpn,priority:1"`
	LineNo    int       `gorm:"column:line_no;not null;uniqueIndex:uk_session_line"`
	RawText   *string   `gorm:"column:raw_text;type:text"`
	Mpn       string    `gorm:"column:mpn;size:256;not null;index:idx_bom_line_mpn,priority:2"`
	Mfr       *string   `gorm:"column:mfr;size:256"`
	Package   *string   `gorm:"column:package;size:128"`
	Qty       *float64  `gorm:"column:qty;type:decimal(18,4)"`
	ExtraJSON []byte    `gorm:"column:extra_json;type:json"`
	CreatedAt time.Time `gorm:"column:created_at;precision:3"`
}

func (BomSessionLine) TableName() string { return TableBomSessionLine }

// BomMergeInflight 合并键与在途调度 task_id（设计 §3.5）；表 t_bom_merge_inflight。
type BomMergeInflight struct {
	MpnNorm    string    `gorm:"column:mpn_norm;size:256;primaryKey"`
	PlatformID string    `gorm:"column:platform_id;size:32;primaryKey"`
	BizDate    time.Time `gorm:"column:biz_date;type:date;primaryKey"`
	TaskID     string    `gorm:"column:task_id;size:128;not null;index:idx_bom_merge_inflight_task"`
	CreatedAt  time.Time `gorm:"column:created_at;precision:3"`
}

func (BomMergeInflight) TableName() string { return TableBomMergeInflight }

// BomMergeProxyWait BOM 合并键代理获取失败退避（策略 B）；表 t_bom_merge_proxy_wait。
type BomMergeProxyWait struct {
	MpnNorm       string     `gorm:"column:mpn_norm;size:256;primaryKey"`
	PlatformID    string     `gorm:"column:platform_id;size:32;primaryKey"`
	BizDate       time.Time  `gorm:"column:biz_date;type:date;primaryKey"`
	NextRetryAt   time.Time  `gorm:"column:next_retry_at;precision:3;not null;index:idx_bom_merge_proxy_wait_next"`
	Attempt       int        `gorm:"column:attempt;not null;default:0"`
	LastError     string     `gorm:"column:last_error;type:text"`
	FirstFailedAt *time.Time `gorm:"column:first_failed_at;precision:3"`
	CreatedAt     time.Time  `gorm:"column:created_at;precision:3;autoCreateTime"`
	UpdatedAt     time.Time  `gorm:"column:updated_at;precision:3;autoUpdateTime"`
}

func (BomMergeProxyWait) TableName() string { return TableBomMergeProxyWait }

// BomPlatformScript 对应 t_bom_platform_script（平台与 Agent 脚本映射）。
type BomPlatformScript struct {
	PlatformID    string    `gorm:"column:platform_id;size:32;primaryKey"`
	ScriptID      string    `gorm:"column:script_id;size:128;not null"`
	DisplayName   *string   `gorm:"column:display_name;size:128"`
	Enabled       bool      `gorm:"column:enabled;not null;default:true"`
	RunParamsJSON []byte    `gorm:"column:run_params;type:json"`
	UpdatedAt     time.Time `gorm:"column:updated_at;precision:3;autoUpdateTime"`
}

func (BomPlatformScript) TableName() string { return TableBomPlatformScript }

// BomManufacturerAlias 对应 t_bom_manufacturer_alias（厂牌别名 → 规范 ID）。
type BomManufacturerAlias struct {
	ID          uint64    `gorm:"column:id;primaryKey;autoIncrement"`
	CanonicalID string    `gorm:"column:canonical_id;size:128;not null;index:idx_bom_mfr_canonical_id"`
	DisplayName string    `gorm:"column:display_name;size:512;not null"`
	Alias       string    `gorm:"column:alias;size:512;not null"`
	AliasNorm   string    `gorm:"column:alias_norm;size:512;not null;uniqueIndex:uk_bom_mfr_alias_norm"`
	CreatedAt   time.Time `gorm:"column:created_at;precision:3;autoCreateTime"`
	UpdatedAt   time.Time `gorm:"column:updated_at;precision:3;autoUpdateTime"`
}

func (BomManufacturerAlias) TableName() string { return TableBomManufacturerAlias }

// BomFxRate 对应 t_bom_fx_rate（配单换汇汇率）。
type BomFxRate struct {
	ID           uint64    `gorm:"column:id;primaryKey;autoIncrement"`
	FromCcy      string    `gorm:"column:from_ccy;size:3;not null;uniqueIndex:uk_bom_fx_rate,priority:1;index:idx_bom_fx_rate_lookup,priority:1"`
	ToCcy        string    `gorm:"column:to_ccy;size:3;not null;uniqueIndex:uk_bom_fx_rate,priority:2;index:idx_bom_fx_rate_lookup,priority:2"`
	BizDate      time.Time `gorm:"column:biz_date;type:date;not null;uniqueIndex:uk_bom_fx_rate,priority:3;index:idx_bom_fx_rate_lookup,priority:3"`
	Rate         float64   `gorm:"column:rate;type:decimal(24,10);not null"`
	Source       string    `gorm:"column:source;size:64;not null;default:manual;uniqueIndex:uk_bom_fx_rate,priority:4"`
	TableVersion string    `gorm:"column:table_version;size:64;not null;default:'';uniqueIndex:uk_bom_fx_rate,priority:5"`
	CreatedAt    time.Time `gorm:"column:created_at;precision:3;autoCreateTime"`
	UpdatedAt    time.Time `gorm:"column:updated_at;precision:3;autoUpdateTime"`
}

func (BomFxRate) TableName() string { return TableBomFxRate }

// HsPolicyVersion 对应 t_hs_policy_version（按生效日版本化阈值）。
type HsPolicyVersion struct {
	ID                       uint64    `gorm:"column:id;primaryKey;autoIncrement"`
	VersionID                string    `gorm:"column:version_id;size:64;not null;uniqueIndex:uk_hs_policy_version_id"`
	EffectiveFrom            time.Time `gorm:"column:effective_from;type:date;not null;index:idx_hs_policy_effective"`
	AutoPassConfidenceMin    float64   `gorm:"column:auto_pass_confidence_min;type:decimal(10,4);not null"`
	AutoPassCompletenessMin  float64   `gorm:"column:auto_pass_completeness_min;type:decimal(10,4);not null"`
	AutoPassTopGapMin        float64   `gorm:"column:auto_pass_top_gap_min;type:decimal(10,4);not null"`
	QuickReviewTopGapMin     float64   `gorm:"column:quick_review_top_gap_min;type:decimal(10,4);not null"`
	QuickReviewConfidenceMin float64   `gorm:"column:quick_review_confidence_min;type:decimal(10,4);not null"`
	ForceReviewConfidenceMax float64   `gorm:"column:force_review_confidence_max;type:decimal(10,4);not null"`
	ForceReviewCompleteness  float64   `gorm:"column:force_review_completeness;type:decimal(10,4);not null"`
	Enabled                  bool      `gorm:"column:enabled;not null;default:true;index:idx_hs_policy_effective"`
	CreatedAt                time.Time `gorm:"column:created_at;precision:3;autoCreateTime"`
	UpdatedAt                time.Time `gorm:"column:updated_at;precision:3;autoUpdateTime"`
}

func (HsPolicyVersion) TableName() string { return TableHsPolicyVersion }

// HsCase 对应 t_hs_case（历史归类案例库）。
type HsCase struct {
	ID            uint64    `gorm:"column:id;primaryKey;autoIncrement"`
	Model         string    `gorm:"column:model;size:256;index:idx_hs_case_model"`
	ProductNameCN string    `gorm:"column:product_name_cn;size:512;index:idx_hs_case_name"`
	Manufacturer  string    `gorm:"column:manufacturer;size:256"`
	Package       string    `gorm:"column:package;size:128"`
	HSCode        string    `gorm:"column:hs_code;size:16;not null;index:idx_hs_case_hs_code"`
	Title         string    `gorm:"column:title;size:512"`
	Reason        string    `gorm:"column:reason;type:text"`
	EvidenceJSON  []byte    `gorm:"column:evidence_json;type:json"`
	SourceTrust   float64   `gorm:"column:source_trust;type:decimal(10,4);not null;default:1"`
	CreatedAt     time.Time `gorm:"column:created_at;precision:3;autoCreateTime"`
	UpdatedAt     time.Time `gorm:"column:updated_at;precision:3;autoUpdateTime"`
}

func (HsCase) TableName() string { return TableHsCase }

// HsReviewDecision 对应 t_hs_review_decision（复核闭环记录）。
type HsReviewDecision struct {
	ID              uint64    `gorm:"column:id;primaryKey;autoIncrement"`
	RequestKey      string    `gorm:"column:request_key;size:512;not null;index:idx_hs_review_req_key"`
	FinalHSCode     string    `gorm:"column:final_hs_code;size:16;not null"`
	ReviewRequired  bool      `gorm:"column:review_required;not null"`
	ReviewReasons   string    `gorm:"column:review_reasons;type:json"`
	PolicyVersionID string    `gorm:"column:policy_version_id;size:64;not null;index:idx_hs_review_policy"`
	CreatedAt       time.Time `gorm:"column:created_at;precision:3;autoCreateTime"`
	UpdatedAt       time.Time `gorm:"column:updated_at;precision:3;autoUpdateTime"`
}

func (HsReviewDecision) TableName() string { return TableHsReviewDecision }

// HsModelMapping 保存型号+厂牌到 HS code_ts 的最终映射。
type HsModelMapping struct {
	ID                      uint64    `gorm:"column:id;primaryKey;autoIncrement"`
	Model                   string    `gorm:"column:model;size:128;not null;uniqueIndex:uk_hs_model_mapping_model_mfr,priority:1"`
	Manufacturer            string    `gorm:"column:manufacturer;size:128;not null;uniqueIndex:uk_hs_model_mapping_model_mfr,priority:2"`
	ManufacturerCanonicalID *string   `gorm:"column:manufacturer_canonical_id;size:128"`
	CodeTS                  string    `gorm:"column:code_ts;type:char(10);not null;check:chk_hs_model_mapping_code_ts,code_ts REGEXP '^[0-9]{10}$';index:idx_hs_model_mapping_code_ts"`
	Source                  string    `gorm:"column:source;type:enum('manual','llm_auto');not null;default:llm_auto"`
	Confidence              float64   `gorm:"column:confidence;type:decimal(5,4)"`
	Status                  string    `gorm:"column:status;type:enum('confirmed','pending_review','rejected');not null;default:pending_review;index:idx_hs_model_mapping_status"`
	FeaturesVersion         string    `gorm:"column:features_version;size:64;not null;default:''"`
	RecommendationVersion   string    `gorm:"column:recommendation_version;size:64;not null;default:''"`
	CreatedAt               time.Time `gorm:"column:created_at;precision:3;autoCreateTime"`
	UpdatedAt               time.Time `gorm:"column:updated_at;precision:3;autoUpdateTime"`
}

func (HsModelMapping) TableName() string { return TableHsModelMapping }

// HsDatasheetAsset 记录 datasheet 下载与落地资产。
type HsDatasheetAsset struct {
	ID             uint64    `gorm:"column:id;primaryKey;autoIncrement"`
	Model          string    `gorm:"column:model;size:128;not null;index:idx_hs_datasheet_asset_model_mfr,priority:1"`
	Manufacturer   string    `gorm:"column:manufacturer;size:128;not null;index:idx_hs_datasheet_asset_model_mfr,priority:2"`
	DatasheetURL   string    `gorm:"column:datasheet_url;size:1024;not null;default:''"`
	LocalPath      string    `gorm:"column:local_path;size:512;not null;default:''"`
	SHA256         string    `gorm:"column:sha256;type:char(64);not null;default:''"`
	DownloadStatus string    `gorm:"column:download_status;type:enum('ok','failed');not null;default:failed"`
	ErrorMsg       string    `gorm:"column:error_msg;size:512;not null;default:''"`
	UpdatedAt      time.Time `gorm:"column:updated_at;precision:3;autoUpdateTime"`
}

func (HsDatasheetAsset) TableName() string { return TableHsDatasheetAsset }

// HsManualDatasheetUpload 用户上传 PDF 暂存行（Resolve 消费后标记 consumed）。
type HsManualDatasheetUpload struct {
	ID           uint64     `gorm:"column:id;primaryKey;autoIncrement"`
	UploadID     string     `gorm:"column:upload_id;size:64;not null;uniqueIndex:uk_hs_manual_upload_id"`
	LocalPath    string     `gorm:"column:local_path;size:512;not null"`
	SHA256       string     `gorm:"column:sha256;type:char(64);not null"`
	ExpiresAt    time.Time  `gorm:"column:expires_at;precision:3;not null;index:idx_hs_manual_upload_expires"`
	OwnerSubject *string    `gorm:"column:owner_subject;size:128"`
	ConsumedAt   *time.Time `gorm:"column:consumed_at;precision:3"`
	CreatedAt    time.Time  `gorm:"column:created_at;precision:3;autoCreateTime"`
}

func (HsManualDatasheetUpload) TableName() string { return TableHsManualDatasheetUpload }

// HsModelFeatures 保存 datasheet 抽取后的结构化特征。
type HsModelFeatures struct {
	ID                      uint64           `gorm:"column:id;primaryKey;autoIncrement"`
	Model                   string           `gorm:"column:model;size:128;not null;index:idx_hs_model_features_model_mfr,priority:1"`
	Manufacturer            string           `gorm:"column:manufacturer;size:128;not null;index:idx_hs_model_features_model_mfr,priority:2"`
	ManufacturerCanonicalID *string          `gorm:"column:manufacturer_canonical_id;size:128"`
	AssetID                 uint64           `gorm:"column:asset_id;not null;index:idx_hs_model_features_asset_id"`
	Asset                   HsDatasheetAsset `gorm:"foreignKey:AssetID;references:ID;constraint:OnDelete:RESTRICT"`
	TechCategory            string           `gorm:"column:tech_category;size:64;not null;default:''"`
	TechCategoryRankedJSON  []byte           `gorm:"column:tech_category_ranked_json;type:json"`
	ComponentName           string           `gorm:"column:component_name;size:128;not null;default:''"`
	PackageForm             string           `gorm:"column:package_form;size:64;not null;default:''"`
	KeySpecsJSON            []byte           `gorm:"column:key_specs_json;type:json"`
	RawExtractJSON          []byte           `gorm:"column:raw_extract_json;type:json"`
	ExtractModel            string           `gorm:"column:extract_model;size:64;not null;default:''"`
	ExtractVersion          string           `gorm:"column:extract_version;size:64;not null;default:''"`
	CreatedAt               time.Time        `gorm:"column:created_at;precision:3;autoCreateTime"`
}

func (HsModelFeatures) TableName() string { return TableHsModelFeatures }

// HsModelRecommendation 保存每次推荐的 TopN 审计轨迹。
type HsModelRecommendation struct {
	ID                      uint64    `gorm:"column:id;primaryKey;autoIncrement"`
	Model                   string    `gorm:"column:model;size:128;not null;index:idx_hs_model_reco_model_mfr_created,priority:1"`
	Manufacturer            string    `gorm:"column:manufacturer;size:128;not null;index:idx_hs_model_reco_model_mfr_created,priority:2"`
	ManufacturerCanonicalID *string   `gorm:"column:manufacturer_canonical_id;size:128"`
	RunID                   string    `gorm:"column:run_id;size:384;not null;index:idx_hs_model_reco_run_id;uniqueIndex:uk_hs_model_reco_run_rank,priority:1"`
	CandidateRank           uint8     `gorm:"column:candidate_rank;type:tinyint unsigned;not null;uniqueIndex:uk_hs_model_reco_run_rank,priority:2"`
	CodeTS                  string    `gorm:"column:code_ts;type:char(10);not null;check:chk_hs_model_reco_code_ts,code_ts REGEXP '^[0-9]{10}$'"`
	GName                   string    `gorm:"column:g_name;size:512;not null;default:''"`
	Score                   float64   `gorm:"column:score;type:decimal(5,4)"`
	Reason                  string    `gorm:"column:reason;size:1024;not null;default:''"`
	InputSnapshotJSON       []byte    `gorm:"column:input_snapshot_json;type:json"`
	RecommendModel          string    `gorm:"column:recommend_model;size:64;not null;default:''"`
	RecommendVersion        string    `gorm:"column:recommend_version;size:64;not null;default:''"`
	CreatedAt               time.Time `gorm:"column:created_at;precision:3;autoCreateTime;index:idx_hs_model_reco_model_mfr_created,priority:3,sort:desc"`
}

func (HsModelRecommendation) TableName() string { return TableHsModelRecommendation }

// HsModelTask 保存按型号解析任务状态（设计 §6）。
type HsModelTask struct {
	RunID          string    `gorm:"column:run_id;size:512;primaryKey"`
	Model          string    `gorm:"column:model;size:128;not null;index:idx_hs_model_task_model_mfr_updated,priority:1;index:idx_hs_model_task_req,priority:1"`
	Manufacturer   string    `gorm:"column:manufacturer;size:128;not null;index:idx_hs_model_task_model_mfr_updated,priority:2;index:idx_hs_model_task_req,priority:2"`
	RequestTraceID string    `gorm:"column:request_trace_id;size:256;not null;default:'';index:idx_hs_model_task_req,priority:3"`
	TaskStatus     string    `gorm:"column:task_status;size:32;not null"`
	ResultStatus   string    `gorm:"column:result_status;size:32;not null"`
	Stage          string    `gorm:"column:stage;size:64;not null;default:''"`
	AttemptCount   int       `gorm:"column:attempt_count;not null;default:0"`
	LastError      string    `gorm:"column:last_error;type:text"`
	BestScore      float64   `gorm:"column:best_score;type:decimal(8,4);not null;default:0"`
	BestCodeTS     string    `gorm:"column:best_code_ts;type:char(10);not null;default:''"`
	UpdatedAt      time.Time `gorm:"column:updated_at;precision:3;autoUpdateTime;index:idx_hs_model_task_model_mfr_updated,priority:3,sort:desc"`
}

func (HsModelTask) TableName() string { return TableHsModelTask }
