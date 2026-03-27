package biz

import (
	"encoding/json"
	"strings"
)

// AgentQuoteRow 各平台采集脚本 stdout 中统一的报价数组元素结构（JSON 对象字段一致）。
type AgentQuoteRow struct {
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
	QueryModel    string `json:"query_model,omitempty"`
}

type taskStdoutEnvelope struct {
	Error   string          `json:"error"`
	Results []AgentQuoteRow `json:"results"`
}

// ParseTaskStdoutQuotes 将 Agent 任务 stdout 中的 JSON 解析为报价 JSON 字节（写入 bom_quote_cache.quotes_json）。
// 顶层支持：① JSON 数组；② {"error":"","results":[...]}（error 非空则拒绝）。
func ParseTaskStdoutQuotes(stdout string) (quotesJSON []byte, ok bool) {
	s := strings.TrimSpace(stdout)
	if s == "" {
		return nil, false
	}
	if s[0] != '[' && s[0] != '{' {
		return nil, false
	}

	var rows []AgentQuoteRow
	switch s[0] {
	case '[':
		if err := json.Unmarshal([]byte(s), &rows); err != nil {
			return nil, false
		}
	case '{':
		var env taskStdoutEnvelope
		if err := json.Unmarshal([]byte(s), &env); err != nil {
			return nil, false
		}
		if strings.TrimSpace(env.Error) != "" {
			return nil, false
		}
		if env.Results == nil {
			env.Results = []AgentQuoteRow{}
		}
		rows = env.Results
	default:
		return nil, false
	}

	if !agentQuoteRowsValid(rows) {
		return nil, false
	}
	out, err := json.Marshal(rows)
	if err != nil {
		return nil, false
	}
	return out, true
}

func agentQuoteRowsValid(rows []AgentQuoteRow) bool {
	for _, r := range rows {
		if strings.TrimSpace(r.Model) == "" {
			return false
		}
	}
	return true
}
