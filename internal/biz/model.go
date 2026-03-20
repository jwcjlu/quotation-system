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
	Platform      string
	MatchedModel  string
	Manufacturer  string
	Package       string // 封装，用于型号/封装/厂牌匹配筛选
	Description   string
	Stock         int64
	LeadTime      string
	MOQ           int32
	Increment     int32
	PriceTiers    string
	HKPrice       string
	MainlandPrice string
	UnitPrice     float64
	Subtotal      float64
}

// MatchItem 配单结果项
type MatchItem struct {
	Index              int
	Model              string
	Quantity           int
	MatchedModel       string
	Manufacturer       string
	Platform           string
	LeadTime           string
	Stock              int64
	UnitPrice          float64
	Subtotal           float64
	MatchStatus        string
	AllQuotes          []*Quote
	DemandManufacturer string // 需求厂牌（解析自 BOM）
	DemandPackage      string // 需求封装（解析自 BOM）
}
