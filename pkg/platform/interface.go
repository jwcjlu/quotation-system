package platform

// Quote 平台报价统一结构
type Quote struct {
	Platform      string // ickey | szlcsc | icgoo
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

// SearchRequest 单次搜索请求（用于批量接口）
type SearchRequest struct {
	Model    string
	Quantity int
}

// Searcher 平台搜索接口
type Searcher interface {
	Name() string
	Search(model string, quantity int) ([]*Quote, error)
}

// BatchSearcher 支持批量多型号搜索的接口，平台内部可多线程并行
type BatchSearcher interface {
	Searcher
	SearchBatch(reqs []SearchRequest) (map[string][]*Quote, error)
}
