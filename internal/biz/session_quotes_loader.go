package biz

import (
	"context"
	"encoding/json"
	"strings"
	"time"
)

// SessionQuoteRawRow 供 LoadItemQuotesForSession 消费的 DB 投影（实现方通常为 *data.BOMSearchTaskRepo）。
type SessionQuoteRawRow struct {
	MpnNorm    string
	PlatformID string
	QuotesJSON []byte
}

// SessionQuotesSource 从 DB 加载 succeeded_quotes 对应的 quotes_json。
type SessionQuotesSource interface {
	LoadSucceededQuoteRowsForSession(ctx context.Context, sessionID string, bizDate time.Time) ([]SessionQuoteRawRow, error)
}

// LoadItemQuotesForSession 聚合成 []*ItemQuotes（Model 为 BOM 行展示型号，与 MatchUseCase 索引一致）。
func LoadItemQuotesForSession(ctx context.Context, src SessionQuotesSource, bom *BOM, bizDate time.Time) ([]*ItemQuotes, error) {
	if src == nil || bom == nil || strings.TrimSpace(bom.ID) == "" {
		return nil, nil
	}
	rows, err := src.LoadSucceededQuoteRowsForSession(ctx, bom.ID, bizDate)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, nil
	}
	byNorm := make(map[string][]*Quote)
	for _, row := range rows {
		qs := ParseQuotesJSON(row.QuotesJSON, row.PlatformID)
		if len(qs) == 0 {
			continue
		}
		k := NormalizeMPNForTask(row.MpnNorm)
		byNorm[k] = append(byNorm[k], qs...)
	}
	seen := make(map[string]struct{})
	out := make([]*ItemQuotes, 0, len(byNorm))
	for _, item := range bom.Items {
		if item == nil {
			continue
		}
		norm := NormalizeMPNForTask(item.Model)
		qs := byNorm[norm]
		if len(qs) == 0 {
			continue
		}
		if _, ok := seen[item.Model]; ok {
			continue
		}
		seen[item.Model] = struct{}{}
		out = append(out, &ItemQuotes{
			Model:    item.Model,
			Quantity: item.Quantity,
			Quotes:   qs,
		})
	}
	return out, nil
}

// ParseQuotesJSON 将 quotes_json 解析为 Quote 列表（数组、单对象或 {"quotes":[]}）；缺 platform 时填 platformID。
func ParseQuotesJSON(raw []byte, platformID string) []*Quote {
	s := strings.TrimSpace(string(raw))
	if s == "" {
		return nil
	}
	raw = []byte(s)
	platformID = strings.TrimSpace(platformID)

	var arr []Quote
	if err := json.Unmarshal(raw, &arr); err == nil && len(arr) > 0 {
		out := make([]*Quote, len(arr))
		for i := range arr {
			out[i] = &arr[i]
		}
		fillQuotePlatform(out, platformID)
		return out
	}
	var one Quote
	if err := json.Unmarshal(raw, &one); err == nil {
		if strings.TrimSpace(one.Platform) == "" {
			one.Platform = platformID
		}
		return []*Quote{&one}
	}
	var wrap struct {
		Quotes []Quote `json:"quotes"`
	}
	if err := json.Unmarshal(raw, &wrap); err == nil && len(wrap.Quotes) > 0 {
		out := make([]*Quote, len(wrap.Quotes))
		for i := range wrap.Quotes {
			out[i] = &wrap.Quotes[i]
		}
		fillQuotePlatform(out, platformID)
		return out
	}
	return nil
}

func fillQuotePlatform(qs []*Quote, platformID string) {
	for _, q := range qs {
		if q == nil {
			continue
		}
		if strings.TrimSpace(q.Platform) == "" {
			q.Platform = platformID
		}
	}
}
