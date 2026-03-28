package data

import (
	"database/sql"
	"time"
)

// CaichipDispatchTask 对应 t_caichip_dispatch_task。
type CaichipDispatchTask struct {
	ID              uint64         `gorm:"column:id;primaryKey;autoIncrement;index:idx_dispatch_claim,priority:3"`
	TaskID          string         `gorm:"column:task_id;size:128;uniqueIndex:uk_dispatch_task_id"`
	Queue           string         `gorm:"column:queue;size:128;not null;default:default;index:idx_dispatch_claim,priority:1"`
	ScriptID        string         `gorm:"column:script_id;size:128;not null"`
	Version         string         `gorm:"column:version;size:64;not null"`
	RequiredTags    []byte         `gorm:"column:required_tags;type:json"`
	EntryFile       sql.NullString `gorm:"column:entry_file;size:512"`
	TimeoutSec      int            `gorm:"column:timeout_sec;not null;default:300"`
	ParamsJSON      []byte         `gorm:"column:params_json;type:json"`
	ArgvJSON        []byte         `gorm:"column:argv_json;type:json"`
	Attempt         int            `gorm:"column:attempt;not null;default:1"`
	State           string         `gorm:"column:state;size:32;not null;default:pending;index:idx_dispatch_claim,priority:2;index:idx_dispatch_state_updated,priority:1;index:idx_dispatch_leased_agent,priority:2"`
	LeaseID         sql.NullString `gorm:"column:lease_id;size:64"`
	LeasedToAgentID sql.NullString `gorm:"column:leased_to_agent_id;size:64;index:idx_dispatch_leased_agent,priority:1"`
	LeasedAt        *time.Time     `gorm:"column:leased_at;precision:3"`
	LeaseDeadlineAt *time.Time     `gorm:"column:lease_deadline_at;precision:3"`
	FinishedAt      *time.Time     `gorm:"column:finished_at;precision:3"`
	ResultStatus    sql.NullString `gorm:"column:result_status;size:32"`
	LastError       sql.NullString `gorm:"column:last_error;type:text"`
	CreatedAt       time.Time      `gorm:"column:created_at;precision:3;autoCreateTime"`
	UpdatedAt       time.Time      `gorm:"column:updated_at;precision:3;autoUpdateTime;index:idx_dispatch_state_updated,priority:2"`
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
	MpnNorm     string    `gorm:"column:mpn_norm;size:256;primaryKey"`
	PlatformID  string    `gorm:"column:platform_id;size:32;primaryKey"`
	BizDate     time.Time `gorm:"column:biz_date;type:date;primaryKey"`
	Outcome     string    `gorm:"column:outcome;size:32;not null"`
	QuotesJSON  []byte    `gorm:"column:quotes_json;type:json"`
	NoMpnDetail []byte    `gorm:"column:no_mpn_detail;type:json"`
	CreatedAt   time.Time `gorm:"column:created_at;precision:3;autoCreateTime"`
	UpdatedAt   time.Time `gorm:"column:updated_at;precision:3;autoUpdateTime;index:idx_bom_quote_cache_updated"`
}

func (BomQuoteCache) TableName() string { return TableBomQuoteCache }

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

// BomPlatformScript 对应 t_bom_platform_script（平台与 Agent 脚本映射）。
type BomPlatformScript struct {
	PlatformID  string    `gorm:"column:platform_id;size:32;primaryKey"`
	ScriptID    string    `gorm:"column:script_id;size:128;not null"`
	DisplayName *string   `gorm:"column:display_name;size:128"`
	Enabled     bool      `gorm:"column:enabled;not null;default:true"`
	UpdatedAt   time.Time `gorm:"column:updated_at;precision:3;autoUpdateTime"`
}

func (BomPlatformScript) TableName() string { return TableBomPlatformScript }
