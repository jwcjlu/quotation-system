package kuaidaili

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// getDpsStaticToken 仅 secret_id + sign_type=token + signature=secret_token 时的 getdps（无 secret_key，无法使用官方 SDK 的令牌刷新流程）。
func getDpsStaticToken(ctx context.Context, cfg Config, num int, extra map[string]interface{}) ([]string, error) {
	base := strings.TrimSpace(cfg.BaseURL)
	if base == "" {
		base = "https://dps.kdlapi.com/api/getdps"
	}
	params := url.Values{}
	params.Set("secret_id", strings.TrimSpace(cfg.SecretID))
	params.Set("sign_type", "token")
	params.Set("signature", strings.TrimSpace(cfg.SecretToken))
	params.Set("num", strconv.Itoa(num))
	params.Set("format", "json")
	for k, v := range extra {
		if k == "format" {
			continue
		}
		params.Set(k, fmt.Sprint(v))
	}
	u, err := url.Parse(base)
	if err != nil {
		return nil, err
	}
	u.RawQuery = params.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	hc := &http.Client{Timeout: 20 * time.Second}
	resp, err := hc.Do(req)
	if err != nil {
		return nil, fmt.Errorf("kuaidaili http: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("kuaidaili http status %d: %s", resp.StatusCode, truncateErr(body))
	}
	var ar struct {
		Code int             `json:"code"`
		Msg  string          `json:"msg"`
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(body, &ar); err != nil {
		return nil, fmt.Errorf("kuaidaili json: %w", err)
	}
	if ar.Code != 0 {
		return nil, fmt.Errorf("kuaidaili code=%d msg=%s", ar.Code, strings.TrimSpace(ar.Msg))
	}
	var db struct {
		Count     int             `json:"count"`
		ProxyList json.RawMessage `json:"proxy_list"`
	}
	if len(ar.Data) > 0 && string(ar.Data) != `""` {
		if err := json.Unmarshal(ar.Data, &db); err != nil {
			return nil, fmt.Errorf("kuaidaili data: %w", err)
		}
	}
	if db.Count == 0 || len(db.ProxyList) == 0 {
		return nil, nil
	}
	var rawList []string
	if err := json.Unmarshal(db.ProxyList, &rawList); err != nil {
		return nil, fmt.Errorf("kuaidaili proxy_list: %w", err)
	}
	return rawList, nil
}

func truncateErr(b []byte) string {
	s := strings.TrimSpace(string(b))
	if len(s) > 200 {
		return s[:200] + "..."
	}
	return s
}
