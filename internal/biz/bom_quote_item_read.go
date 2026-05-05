package biz

// BomQuoteItemReadRow 关联报价缓存平台后的 t_bom_quote_item 只读行（运营审查）。
type BomQuoteItemReadRow struct {
	PlatformID              string
	QuoteID                 uint64
	ItemID                  uint64
	Model                   string
	Manufacturer            string
	ManufacturerCanonicalID   string
	ManufacturerReviewStatus  string
	Package                   string
	Stock                   string
	Desc                    string
	MOQ                     string
	LeadTime                string
	PriceTiers              string
	HKPrice                 string
	MainlandPrice           string
	QueryModel              string
	DatasheetURL            string
	SourceType              string
	SessionID               string
	LineID                  int64
}
