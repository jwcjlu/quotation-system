package data

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// 默认 Frankfurter API（ECB 系列汇率，文档见 https://www.frankfurter.app/docs/ ）
const frankfurterAPIBase = "https://api.frankfurter.app"

// fetchFrankfurterRate 使用默认官方 base；单测可用 fetchFrankfurterRateAt 指向 httptest。
func fetchFrankfurterRate(ctx context.Context, client *http.Client, from, to string, date time.Time) (rate float64, err error) {
	return fetchFrankfurterRateAt(ctx, client, frankfurterAPIBase, from, to, date)
}

// frankfurterResponse 与 Frankfurter JSON 对齐。
type frankfurterResponse struct {
	Amount float64            `json:"amount"`
	Base   string             `json:"base"`
	Date   string             `json:"date"`
	Rates  map[string]float64 `json:"rates"`
}

// fetchFrankfurterRateAt 请求 1 from = rate × to；date 为业务日 UTC 日历日；baseURL 不含尾斜杠。
func fetchFrankfurterRateAt(ctx context.Context, client *http.Client, baseURL, from, to string, date time.Time) (rate float64, err error) {
	if client == nil {
		return 0, fmt.Errorf("frankfurter: http client nil")
	}
	baseURL = strings.TrimSuffix(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		return 0, fmt.Errorf("frankfurter: empty base url")
	}
	from = strings.ToUpper(strings.TrimSpace(from))
	to = strings.ToUpper(strings.TrimSpace(to))
	if from == "" || to == "" {
		return 0, fmt.Errorf("frankfurter: empty currency")
	}
	if from == to {
		return 1, nil
	}
	y, m, d := date.In(time.UTC).Date()
	dateStr := fmt.Sprintf("%04d-%02d-%02d", y, int(m), d)
	u, err := url.Parse(baseURL + "/" + dateStr)
	if err != nil {
		return 0, err
	}
	q := u.Query()
	q.Set("from", from)
	q.Set("to", to)
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return 0, err
	}
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return 0, err
	}
	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("frankfurter: HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var env frankfurterResponse
	if err := json.Unmarshal(body, &env); err != nil {
		return 0, fmt.Errorf("frankfurter: json: %w", err)
	}
	if len(env.Rates) == 0 {
		return 0, fmt.Errorf("frankfurter: empty rates")
	}
	r, ok := env.Rates[to]
	if !ok {
		return 0, fmt.Errorf("frankfurter: missing rate for %s", to)
	}
	if r <= 0 || env.Amount <= 0 {
		return 0, fmt.Errorf("frankfurter: non-positive rate")
	}
	// amount 通常为 1；语义 1 base = r × to
	return r / env.Amount, nil
}
