package biz

import "time"

// BOM 领域模型
type BOM struct {
	ID        string
	CreatedAt time.Time
	Items     []*BOMItem
}

// BOMItem BOM 物料项
type BOMItem struct {
	Index        int
	Raw          string
	Model        string
	Manufacturer string
	Package      string
	Quantity     int
	Params       string
}

// ItemQuotes 物料报价聚合
type ItemQuotes struct {
	Model    string
	Quantity int
	Quotes   []*Quote
}

// Quote 平台报价
type Quote struct {
	Platform      string  `json:"platform"`
	MatchedModel  string  `json:"matched_model"`
	Manufacturer  string  `json:"manufacturer"`
	Package       string  `json:"package"` // 封装，用于型号/封装/厂牌匹配筛选
	Description   string  `json:"description"`
	Stock         int64   `json:"stock"`
	LeadTime      string  `json:"lead_time"`
	MOQ           int32   `json:"moq"`
	Increment     int32   `json:"increment"`
	PriceTiers    string  `json:"price_tiers"`
	HKPrice       string  `json:"hk_price"`
	MainlandPrice string  `json:"mainland_price"`
	UnitPrice     float64 `json:"unit_price"`
	Subtotal      float64 `json:"subtotal"`
}

// MatchItem 配单结果项
type MatchItem struct {
	Index              int      `json:"index"`
	Model              string   `json:"model"`
	Quantity           int      `json:"quantity"`
	MatchedModel       string   `json:"matched_model"`
	Manufacturer       string   `json:"manufacturer"`
	Platform           string   `json:"platform"`
	LeadTime           string   `json:"lead_time"`
	Stock              int64    `json:"stock"`
	UnitPrice          float64  `json:"unit_price"`
	Subtotal           float64  `json:"subtotal"`
	MatchStatus        string   `json:"match_status"`
	AllQuotes          []*Quote `json:"all_quotes"`
	DemandManufacturer string   `json:"demand_manufacturer"` // 需求厂牌（解析自 BOM）
	DemandPackage      string   `json:"demand_package"`      // 需求封装（解析自 BOM）
}
