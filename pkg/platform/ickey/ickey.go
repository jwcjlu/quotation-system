package ickey

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"caichip/pkg/platform"
)

// crawlerResult 爬虫返回的单条结果（与 ickey_crawler.py 输出一致）
// 字段必须导出（首字母大写）才能被 json.Unmarshal 填充
type crawlerResult struct {
	Seq           int    `json:"seq"`
	Model         string `json:"model"`
	Manufacturer  string `json:"manufacturer"`
	Package       string `json:"package"`
	Desc          string `json:"desc"`
	Stock         string `json:"stock"`
	MOQ           string `json:"moq"`
	PriceTiers    string `json:"price_tiers"`
	HKPrice       string `json:"hk_price"`
	MainlandPrice string `json:"mainland_price"`
	LeadTime      string `json:"lead_time"`
	QueryModel    string `json:"query_model"` // 多型号搜索时标记来源型号
}

// Client 云汉芯城搜索客户端，通过调用 ickey_crawler.py 获取报价
type Client struct {
	searchURL     string
	timeout       int
	crawlerPath   string // python 或 python3
	crawlerScript string // ickey_crawler.py 路径
	workDir       string // 脚本所在目录
}

// NewClient 创建 Ickey 客户端
func NewClient(searchURL string, timeout int, crawlerPath, crawlerScript string) *Client {
	if crawlerPath == "" {
		crawlerPath = "python"
	}
	if crawlerScript == "" {
		crawlerScript = "ickey_crawler.py"
	}
	workDir := ""
	if abs, err := filepath.Abs(crawlerScript); err == nil {
		workDir = filepath.Dir(abs)
		crawlerScript = filepath.Base(abs)
	}
	return &Client{
		searchURL:     searchURL,
		timeout:       timeout,
		crawlerPath:   crawlerPath,
		crawlerScript: crawlerScript,
		workDir:       workDir,
	}
}

// Name 实现 platform.Searcher
func (c *Client) Name() string {
	return "ickey"
}

// Search 调用 ickey_crawler.py 搜索型号报价（单型号）
func (c *Client) Search(model string, quantity int) ([]*platform.Quote, error) {
	m, err := c.SearchBatch([]platform.SearchRequest{{Model: model, Quantity: quantity}})
	if err != nil {
		return nil, err
	}
	return m[model], nil
}

// SearchBatch 多型号批量搜索，一次调用爬虫，爬虫内部多线程并行
// 返回 map[model][]*Quote，key 为请求的型号
func (c *Client) SearchBatch(reqs []platform.SearchRequest) (map[string][]*platform.Quote, error) {
	if len(reqs) == 0 {
		return nil, nil
	}

	models := make([]string, 0, len(reqs))
	qtyByModel := make(map[string]int)
	for _, r := range reqs {
		if r.Model == "" {
			continue
		}
		models = append(models, r.Model)
		if r.Quantity <= 0 {
			r.Quantity = 1
		}
		qtyByModel[r.Model] = r.Quantity
	}
	if len(models) == 0 {
		return nil, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(c.timeout+5)*time.Second)
	defer cancel()

	scriptPath := c.crawlerScript
	if c.workDir != "" {
		scriptPath = filepath.Join(c.workDir, c.crawlerScript)
	}

	// 逗号分隔多型号
	modelArg := strings.Join(models, ",")
	cmd := exec.CommandContext(ctx, c.crawlerPath, scriptPath, "--model", modelArg)
	if c.workDir != "" {
		cmd.Dir = c.workDir
	}

	body, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("ickey crawler: %w", err)
	}

	var results []crawlerResult
	if err := json.Unmarshal(body, &results); err != nil {
		return nil, fmt.Errorf("ickey crawler json: %w", err)
	}

	out := make(map[string][]*platform.Quote)
	for _, r := range results {
		qm := r.QueryModel
		if qm == "" {
			qm = r.Model
		}
		qty := qtyByModel[qm]
		if qty <= 0 {
			qty = 1
		}
		q := c.toQuote(&r, qty)
		out[qm] = append(out[qm], q)
	}
	return out, nil
}

func (c *Client) toQuote(r *crawlerResult, quantity int) *platform.Quote {
	stock, _ := strconv.ParseInt(r.Stock, 10, 64)
	moq, _ := strconv.Atoi(r.MOQ)
	if moq <= 0 {
		moq = 1
	}

	unitPrice := parseFirstPriceFromTiers(r.MainlandPrice, quantity)
	subtotal := unitPrice * float64(quantity)

	desc := r.Desc
	if r.Package != "" && r.Package != "N/A" {
		if desc != "" {
			desc = r.Package + " " + desc
		} else {
			desc = r.Package
		}
	}

	return &platform.Quote{
		Platform:      "ickey",
		MatchedModel:  r.Model,
		Manufacturer:  r.Manufacturer,
		Package:       r.Package,
		Description:   desc,
		Stock:         stock,
		LeadTime:      r.LeadTime,
		MOQ:           int32(moq),
		Increment:     1,
		PriceTiers:    r.PriceTiers,
		HKPrice:       r.HKPrice,
		MainlandPrice: r.MainlandPrice,
		UnitPrice:     unitPrice,
		Subtotal:      subtotal,
	}
}

// parseFirstPriceFromTiers 从价格梯度解析单价，如 "1+ ￥0.88 | 10+ ￥0.75" -> 按数量取对应档位
var priceTierRe = regexp.MustCompile(`(\d+)\+\s*[￥¥$]?\s*([\d.]+)`)

func parseFirstPriceFromTiers(tiers string, quantity int) float64 {
	matches := priceTierRe.FindAllStringSubmatch(tiers, -1)
	if len(matches) == 0 {
		return 0
	}
	// 取满足 quantity 的最低档位价格
	var bestPrice float64
	bestQty := -1
	for _, m := range matches {
		if len(m) < 3 {
			continue
		}
		qty, _ := strconv.Atoi(m[1])
		price, _ := strconv.ParseFloat(m[2], 64)
		if qty <= quantity && qty > bestQty {
			bestQty = qty
			bestPrice = price
		}
	}
	if bestPrice > 0 {
		return bestPrice
	}
	// 未找到满足数量的档位，取最低档
	price, _ := strconv.ParseFloat(matches[0][2], 64)
	return price
}

var (
	_ platform.Searcher      = (*Client)(nil)
	_ platform.BatchSearcher = (*Client)(nil)
)
