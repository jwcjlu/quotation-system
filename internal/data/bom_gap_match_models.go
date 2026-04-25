package data

import "time"

type BomLineGap struct {
	ID               uint64     `gorm:"column:id;primaryKey;autoIncrement"`
	SessionID        string     `gorm:"column:session_id;size:36;not null;index:idx_bom_line_gap_session_status,priority:1;index:idx_bom_line_gap_line,priority:1"`
	LineID           int64      `gorm:"column:line_id;not null;index:idx_bom_line_gap_line,priority:2"`
	LineNo           int        `gorm:"column:line_no;not null"`
	Mpn              string     `gorm:"column:mpn;size:256;not null;default:''"`
	GapType          string     `gorm:"column:gap_type;size:64;not null"`
	ReasonCode       string     `gorm:"column:reason_code;size:64;not null;default:''"`
	ReasonDetail     *string    `gorm:"column:reason_detail;type:text"`
	ResolutionStatus string     `gorm:"column:resolution_status;size:32;not null;default:open;index:idx_bom_line_gap_session_status,priority:2"`
	ActiveKey        string     `gorm:"column:active_key;size:191;not null;default:'';uniqueIndex:uk_bom_line_gap_active"`
	ResolvedBy       *string    `gorm:"column:resolved_by;size:128"`
	ResolvedAt       *time.Time `gorm:"column:resolved_at;precision:3"`
	ResolutionNote   *string    `gorm:"column:resolution_note;type:text"`
	SubstituteMpn    *string    `gorm:"column:substitute_mpn;size:256"`
	SubstituteReason *string    `gorm:"column:substitute_reason;type:text"`
	CreatedAt        time.Time  `gorm:"column:created_at;precision:3;autoCreateTime"`
	UpdatedAt        time.Time  `gorm:"column:updated_at;precision:3;autoUpdateTime;index:idx_bom_line_gap_updated"`
}

func (BomLineGap) TableName() string { return TableBomLineGap }

type BomMatchRun struct {
	ID                  uint64     `gorm:"column:id;primaryKey;autoIncrement"`
	RunNo               int        `gorm:"column:run_no;not null;uniqueIndex:uk_bom_match_run_session_no,priority:2"`
	SessionID           string     `gorm:"column:session_id;size:36;not null;uniqueIndex:uk_bom_match_run_session_no,priority:1;index:idx_bom_match_run_session_status,priority:1"`
	SelectionRevision   int        `gorm:"column:selection_revision;not null"`
	Status              string     `gorm:"column:status;size:32;not null;default:saved;index:idx_bom_match_run_session_status,priority:2"`
	Source              string     `gorm:"column:source;size:32;not null;default:manual_save"`
	LineTotal           int        `gorm:"column:line_total;not null;default:0"`
	MatchedLineCount    int        `gorm:"column:matched_line_count;not null;default:0"`
	UnresolvedLineCount int        `gorm:"column:unresolved_line_count;not null;default:0"`
	TotalAmount         float64    `gorm:"column:total_amount;type:decimal(24,6);not null;default:0"`
	Currency            string     `gorm:"column:currency;size:8;not null;default:CNY"`
	CreatedBy           *string    `gorm:"column:created_by;size:128"`
	CreatedAt           time.Time  `gorm:"column:created_at;precision:3;autoCreateTime;index:idx_bom_match_run_created"`
	SavedAt             *time.Time `gorm:"column:saved_at;precision:3"`
	SupersededAt        *time.Time `gorm:"column:superseded_at;precision:3"`
}

func (BomMatchRun) TableName() string { return TableBomMatchRun }

type BomMatchResultItem struct {
	ID                    uint64    `gorm:"column:id;primaryKey;autoIncrement"`
	RunID                 uint64    `gorm:"column:run_id;not null;uniqueIndex:uk_bom_match_result_run_line,priority:1"`
	SessionID             string    `gorm:"column:session_id;size:36;not null;index:idx_bom_match_result_session_line,priority:1"`
	LineID                int64     `gorm:"column:line_id;not null;uniqueIndex:uk_bom_match_result_run_line,priority:2;index:idx_bom_match_result_session_line,priority:2"`
	LineNo                int       `gorm:"column:line_no;not null"`
	SourceType            string    `gorm:"column:source_type;size:32;not null"`
	MatchStatus           string    `gorm:"column:match_status;size:32;not null;default:''"`
	GapID                 *uint64   `gorm:"column:gap_id;index:idx_bom_match_result_gap"`
	QuoteItemID           *uint64   `gorm:"column:quote_item_id;index:idx_bom_match_result_quote_item"`
	PlatformID            string    `gorm:"column:platform_id;size:32;not null;default:''"`
	DemandMpn             string    `gorm:"column:demand_mpn;size:256;not null;default:''"`
	DemandMfr             string    `gorm:"column:demand_mfr;size:256;not null;default:''"`
	DemandPackage         string    `gorm:"column:demand_package;size:128;not null;default:''"`
	DemandQty             *float64  `gorm:"column:demand_qty;type:decimal(18,4)"`
	MatchedMpn            string    `gorm:"column:matched_mpn;size:256;not null;default:''"`
	MatchedMfr            string    `gorm:"column:matched_mfr;size:256;not null;default:''"`
	MatchedPackage        string    `gorm:"column:matched_package;size:128;not null;default:''"`
	Stock                 *int64    `gorm:"column:stock"`
	LeadTime              string    `gorm:"column:lead_time;size:128;not null;default:''"`
	UnitPrice             *float64  `gorm:"column:unit_price;type:decimal(24,6)"`
	Subtotal              *float64  `gorm:"column:subtotal;type:decimal(24,6)"`
	Currency              string    `gorm:"column:currency;size:8;not null;default:CNY"`
	OriginalMpn           *string   `gorm:"column:original_mpn;size:256"`
	SubstituteMpn         *string   `gorm:"column:substitute_mpn;size:256"`
	SubstituteReason      *string   `gorm:"column:substitute_reason;type:text"`
	CodeTS                string    `gorm:"column:code_ts;type:char(10);not null;default:''"`
	ControlMark           string    `gorm:"column:control_mark;size:64;not null;default:''"`
	ImportTaxOrdinaryRate string    `gorm:"column:import_tax_imp_ordinary_rate;size:64;not null;default:''"`
	ImportTaxDiscountRate string    `gorm:"column:import_tax_imp_discount_rate;size:64;not null;default:''"`
	ImportTaxTempRate     string    `gorm:"column:import_tax_imp_temp_rate;size:64;not null;default:''"`
	SnapshotJSON          []byte    `gorm:"column:snapshot_json;type:json"`
	CreatedAt             time.Time `gorm:"column:created_at;precision:3;autoCreateTime"`
}

func (BomMatchResultItem) TableName() string { return TableBomMatchResultItem }
