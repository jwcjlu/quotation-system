package ickey

import (
	"encoding/json"
	"path/filepath"
	"runtime"
	"testing"

	"caichip/pkg/platform"
)

func TestParseFirstPriceFromTiers(t *testing.T) {
	tests := []struct {
		tiers    string
		quantity int
		want     float64
	}{
		{"1+ ￥0.88 | 10+ ￥0.75 | 100+ ￥0.65", 5, 0.88},
		{"1+ ￥0.88 | 10+ ￥0.75 | 100+ ￥0.65", 50, 0.75},
		{"1+ ￥0.88 | 10+ ￥0.75 | 100+ ￥0.65", 200, 0.65},
		{"1+ ￥0.88 | 10+ ￥0.75", 100, 0.75},
		{"1+ ￥1.50", 1, 1.50},
		{"", 10, 0},
		{"N/A", 10, 0},
	}
	for _, tt := range tests {
		got := parseFirstPriceFromTiers(tt.tiers, tt.quantity)
		if got != tt.want {
			t.Errorf("parseFirstPriceFromTiers(%q, %d) = %v, want %v", tt.tiers, tt.quantity, got, tt.want)
		}
	}
}

func TestToQuote(t *testing.T) {
	c := NewClient("https://search.ickey.cn/", 15, "python", "ickey_crawler.py")
	r := &crawlerResult{
		Seq:           1,
		Model:         "SN74HC595PWR",
		Manufacturer:  "TI",
		Package:       "TSSOP-16",
		Desc:          "8-Bit Shift",
		Stock:         "10000",
		MOQ:           "1",
		PriceTiers:    "1+ ￥0.88",
		HKPrice:       "1+ $0.12",
		MainlandPrice: "1+ ￥0.88 | 10+ ￥0.75",
		LeadTime:      "7-9工作日",
	}
	q := c.toQuote(r, 5)
	if q.Platform != "ickey" {
		t.Errorf("Platform = %q, want ickey", q.Platform)
	}
	if q.MatchedModel != "SN74HC595PWR" {
		t.Errorf("MatchedModel = %q, want SN74HC595PWR", q.MatchedModel)
	}
	if q.Manufacturer != "TI" {
		t.Errorf("Manufacturer = %q, want TI", q.Manufacturer)
	}
	if q.Package != "TSSOP-16" {
		t.Errorf("Package = %q, want TSSOP-16", q.Package)
	}
	if q.Stock != 10000 {
		t.Errorf("Stock = %d, want 10000", q.Stock)
	}
	if q.UnitPrice != 0.88 {
		t.Errorf("UnitPrice = %v, want 0.88", q.UnitPrice)
	}
	if q.Subtotal != 0.88*5 {
		t.Errorf("Subtotal = %v, want %v", q.Subtotal, 0.88*5)
	}
}

func TestParseCrawlerJSON(t *testing.T) {
	body := `[
		{"seq":1,"model":"ABC123","manufacturer":"MPS","package":"SOT-23","desc":"","stock":"5000","moq":"10","price_tiers":"10+ ￥1.20","hk_price":"","mainland_price":"10+ ￥1.20","lead_time":"5-7工作日"}
	]`
	var results []crawlerResult
	if err := json.Unmarshal([]byte(body), &results); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("len(results) = %d, want 1", len(results))
	}
	if results[0].Model != "ABC123" || results[0].Manufacturer != "MPS" {
		t.Errorf("got Model=%q Manufacturer=%q", results[0].Model, results[0].Manufacturer)
	}
	if results[0].MainlandPrice != "10+ ￥1.20" {
		t.Errorf("MainlandPrice = %q", results[0].MainlandPrice)
	}
}

func TestSearch_WithStub(t *testing.T) {
	// 使用 testdata 中的 stub 脚本，不依赖真实爬虫（基于测试文件路径，不依赖 cwd）
	_, file, _, _ := runtime.Caller(0)
	testDir := filepath.Dir(file)
	stubPath := filepath.Join(testDir, "testdata", "ickey_stub.py")
	workDir := filepath.Dir(stubPath)
	scriptName := filepath.Base(stubPath)

	c := &Client{
		searchURL:     "https://search.ickey.cn/",
		timeout:       5,
		crawlerPath:   "python",
		crawlerScript: scriptName,
		workDir:       workDir,
	}

	quotes, err := c.Search("SN74HC595PWR", 10)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(quotes) != 1 {
		t.Fatalf("len(quotes) = %d, want 1", len(quotes))
	}
	q := quotes[0]
	if q.Platform != "ickey" || q.MatchedModel != "SN74HC595PWR" {
		t.Errorf("got Platform=%q MatchedModel=%q", q.Platform, q.MatchedModel)
	}
	if q.Manufacturer != "TI" {
		t.Errorf("Manufacturer = %q, want TI", q.Manufacturer)
	}
	// quantity=10 应取 10+ ￥0.75 档
	if q.UnitPrice != 0.75 {
		t.Errorf("UnitPrice = %v, want 0.75 (10+ tier)", q.UnitPrice)
	}
}

func TestSearchBatch_WithStub(t *testing.T) {

	c := &Client{
		searchURL:     "https://search.ickey.cn/",
		timeout:       300,
		crawlerPath:   "python",
		crawlerScript: "ickey_crawler.py",
		workDir:       "D:\\workspace\\caichip",
	}

	m, err := c.SearchBatch([]platform.SearchRequest{
		{Model: "SN74HC595PWR", Quantity: 10},
		{Model: "CC1310F128RHBR", Quantity: 5},
	})
	if err != nil {
		t.Fatalf("SearchBatch: %v", err)
	}
	if len(m) != 2 {
		t.Fatalf("len(m) = %d, want 2", len(m))
	}
	if len(m["SN74HC595PWR"]) != 1 || len(m["ABC123"]) != 1 {
		t.Errorf("got SN74HC595PWR=%d ABC123=%d", len(m["SN74HC595PWR"]), len(m["ABC123"]))
	}
	// quantity=10 取 10+ 档 0.75
	if m["SN74HC595PWR"][0].UnitPrice != 0.75 {
		t.Errorf("SN74HC595PWR UnitPrice = %v, want 0.75", m["SN74HC595PWR"][0].UnitPrice)
	}
	// quantity=5 取 1+ 档 0.88
	if m["ABC123"][0].UnitPrice != 0.88 {
		t.Errorf("ABC123 UnitPrice = %v, want 0.88", m["ABC123"][0].UnitPrice)
	}
}

// TestClientName 验证 Searcher 接口
func TestClientName(t *testing.T) {
	c := NewClient("", 0, "", "")
	if c.Name() != "ickey" {
		t.Errorf("Name() = %q, want ickey", c.Name())
	}
}

// 验证 platform.Searcher 接口实现
var _ platform.Searcher = (*Client)(nil)
