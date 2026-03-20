package szlcsc

import (
	"caichip/pkg/platform"
)

// Client 立创商城搜索客户端（stub）
type Client struct {
	searchURL string
	timeout   int
}

// NewClient 创建立创商城客户端
func NewClient(searchURL string, timeout int) *Client {
	return &Client{
		searchURL: searchURL,
		timeout:   timeout,
	}
}

// Name 实现 platform.Searcher
func (c *Client) Name() string {
	return "szlcsc"
}

// Search 搜索型号报价（stub）
func (c *Client) Search(model string, quantity int) ([]*platform.Quote, error) {
	unitPrice := 1.2
	return []*platform.Quote{
		{
			Platform:     "szlcsc",
			MatchedModel: model,
			Manufacturer: "N/A",
			Package:      "N/A",
			Description:  "Stub result for " + model,
			Stock:        500,
			LeadTime:     "3-5工作日",
			MOQ:          1,
			UnitPrice:    unitPrice,
			Subtotal:     unitPrice * float64(quantity),
		},
	}, nil
}

var _ platform.Searcher = (*Client)(nil)
