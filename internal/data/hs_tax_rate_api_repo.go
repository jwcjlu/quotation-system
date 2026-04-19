package data

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"caichip/internal/conf"
)

const defaultTaxRateAPIURL = "https://www.singlewindow.cn/access/ui/1776225848566/TaxRate001?cw2qEsfh=0RC85aAlqWxhpXxTC7L8RPvSD78IFIygNX76nZ..oW84veAvzBCmtkMhaH6HI98Ok416joJPci1c0uNbipH1.cjWAmm1NHnnQprMNjZoZsmIDbKy3z0uT6a"

type HsTaxRateAPIRepo struct {
	apiURL     string
	httpClient *http.Client
}

func NewHsTaxRateAPIRepo(c *conf.Bootstrap) *HsTaxRateAPIRepo {
	apiURL := defaultTaxRateAPIURL
	if v := strings.TrimSpace(os.Getenv("CAICHIP_HS_TAX_RATE_API_URL")); v != "" {
		apiURL = v
	}
	timeout := 15 * time.Second
	if c != nil && c.GetHsResolveSyncTimeoutMs() > 0 {
		timeout = time.Duration(c.GetHsResolveSyncTimeoutMs()) * time.Millisecond
	}
	return &HsTaxRateAPIRepo{
		apiURL:     apiURL,
		httpClient: &http.Client{Timeout: timeout},
	}
}

// HsTaxRateAPIItem 对应 TaxRate001 响应中 data.data[] 的单条记录。
type HsTaxRateAPIItem struct {
	ImpDiscountRate string `json:"impDiscountRate"`
	GName           string `json:"gName"`
	CodeTS          string `json:"codeTs"`
	ImpTempRate     string `json:"impTempRate"`
	ImpOrdinaryRate string `json:"impOrdinaryRate"`
}

// HsTaxRateFetchResult 对应 TaxRate001 响应中的根级 data 对象（含列表 data 数组）。
type HsTaxRateFetchResult struct {
	Statue     int64              `json:"statue"` // 接口字段名为 statue
	Message    string             `json:"message"`
	TotalCount int64              `json:"totalCount"`
	Items      []HsTaxRateAPIItem `json:"data"`
}

type hsTaxRateAPIEnvelope struct {
	Status    string          `json:"status"`
	IndexInfo string          `json:"indexinfo"`
	Data      json.RawMessage `json:"data"`
}

// FetchByCodeTS 调用税率接口（TaxRate001）并解析为结构化结果。
func (r *HsTaxRateAPIRepo) FetchByCodeTS(ctx context.Context, codeTS string, pageSize int) (*HsTaxRateFetchResult, error) {
	codeTS = strings.TrimSpace(codeTS)
	if codeTS == "" {
		return nil, errors.New("tax_rate_api: code_ts is required")
	}
	if pageSize <= 0 {
		pageSize = 10
	}
	payloadData, err := json.Marshal(map[string]any{
		"gName":    "",
		"codeTs":   codeTS,
		"nextPage": 1,
		"pageSize": pageSize,
	})
	if err != nil {
		return nil, err
	}
	payload, err := json.Marshal(map[string]any{
		"Head": map[string]any{},
		"Data": string(payloadData),
	})
	if err != nil {
		return nil, err
	}
	apiURL := strings.TrimSpace(r.apiURL)
	if apiURL == "" {
		apiURL = defaultTaxRateAPIURL
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	client := r.httpClient
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("tax_rate_api http status=%d body=%s", resp.StatusCode, string(body))
	}
	return decodeTaxRateBody(body)
}

func decodeTaxRateBody(body []byte) (*HsTaxRateFetchResult, error) {
	var env hsTaxRateAPIEnvelope
	if err := json.Unmarshal(body, &env); err != nil {
		return nil, err
	}
	if st := strings.ToLower(strings.TrimSpace(env.Status)); st != "" && st != "success" {
		return nil, fmt.Errorf("tax_rate_api status=%s", env.Status)
	}
	inner, err := unmarshalTaxRateInnerPayload(env.Data)
	if err != nil {
		return nil, err
	}
	if inner == nil {
		return &HsTaxRateFetchResult{}, nil
	}
	return inner, nil
}

func unmarshalTaxRateInnerPayload(raw json.RawMessage) (*HsTaxRateFetchResult, error) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || string(raw) == "null" {
		return &HsTaxRateFetchResult{}, nil
	}
	// 部分环境下 data 为 JSON 字符串，需二次解析。
	if raw[0] == '"' {
		var s string
		if err := json.Unmarshal(raw, &s); err != nil {
			return nil, fmt.Errorf("tax_rate_api data string: %w", err)
		}
		s = strings.TrimSpace(s)
		if s == "" {
			return &HsTaxRateFetchResult{}, nil
		}
		return unmarshalTaxRateInnerPayload(json.RawMessage(s))
	}
	var out HsTaxRateFetchResult
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("tax_rate_api inner data: %w", err)
	}
	return &out, nil
}
