package biz

import (
	"sort"
	"time"
)

// QuoteReviewMissingPriceMode 对应 SRS REQ-DEF-002。
type QuoteReviewMissingPriceMode byte

const (
	// QuoteReviewMissingPriceM1 缺失 compare 的报价行不得进入 S。
	QuoteReviewMissingPriceM1 QuoteReviewMissingPriceMode = iota
	// QuoteReviewMissingPriceM2 存在 E 内 pending/accepted 且缺价则规则 B 恒假。
	QuoteReviewMissingPriceM2
)

// QuoteReviewBAuxMode 对应 SRS REQ-RULE-B-004。
type QuoteReviewBAuxMode byte

const (
	// QuoteReviewBAux2 不设 accepted 条数下限（推荐默认）。
	QuoteReviewBAux2 QuoteReviewBAuxMode = iota
	// QuoteReviewBAux1 若 m≥3 则 E 内 accepted 总数 ≥3；m<3 时退化为 S 全 accepted（与 REQ-EDGE-002 一致）。
	QuoteReviewBAux1
)

// QuoteReviewConfig 报价评审规则 B / TopN 的可调参数（P0-1 可由配置注入；默认对齐 SRS 推荐）。
type QuoteReviewConfig struct {
	MissingPrice QuoteReviewMissingPriceMode
	BAux         QuoteReviewBAuxMode
	TopN         int
}

// DefaultQuoteReviewConfig 返回 SRS 推荐默认：M1 + B-aux-2 + TopN=5。
func DefaultQuoteReviewConfig() QuoteReviewConfig {
	return QuoteReviewConfig{
		MissingPrice: QuoteReviewMissingPriceM1,
		BAux:         QuoteReviewBAux2,
		TopN:         5,
	}
}

// QuoteReviewRowInput 单行下一条报价的归一化输入（不依赖 DB）。
type QuoteReviewRowInput struct {
	InE          bool
	Status       string
	ComparePrice *float64
	UpdatedAt    time.Time
	ItemID       uint64
}

// QuoteReviewLineOutcome 单需求行计算结果（规则 B + TopK/TopN 标识）。
type QuoteReviewLineOutcome struct {
	RuleBOk        bool
	CandidatePoolM int
	TopKItemIDs    []uint64
	TopNItemIDs    []uint64
}

func rowInCandidatePoolForTopN(r QuoteReviewRowInput, _ QuoteReviewMissingPriceMode) bool {
	// TopN 需可比价全序（REQ-QUEUE-001）；无 compare 不参与排序池。
	if !r.InE || r.ComparePrice == nil {
		return false
	}
	return true
}

func rowInS(r QuoteReviewRowInput, mode QuoteReviewMissingPriceMode) bool {
	if !r.InE {
		return false
	}
	if r.Status != MfrReviewPending && r.Status != MfrReviewAccepted {
		return false
	}
	if mode == QuoteReviewMissingPriceM1 && r.ComparePrice == nil {
		return false
	}
	return true
}

func blockedM2(rows []QuoteReviewRowInput) bool {
	for i := range rows {
		r := rows[i]
		if !r.InE {
			continue
		}
		if r.Status != MfrReviewPending && r.Status != MfrReviewAccepted {
			continue
		}
		if r.ComparePrice == nil {
			return true
		}
	}
	return false
}

func buildSRows(rows []QuoteReviewRowInput, mode QuoteReviewMissingPriceMode) []QuoteReviewRowInput {
	out := make([]QuoteReviewRowInput, 0)
	for i := range rows {
		if rowInS(rows[i], mode) {
			out = append(out, rows[i])
		}
	}
	return out
}

func sortSRows(s []QuoteReviewRowInput) {
	sort.Slice(s, func(i, j int) bool {
		a, b := s[i], s[j]
		if a.ComparePrice == nil && b.ComparePrice == nil {
			if a.UpdatedAt.Equal(b.UpdatedAt) {
				return a.ItemID < b.ItemID
			}
			return a.UpdatedAt.Before(b.UpdatedAt)
		}
		if a.ComparePrice == nil {
			return true
		}
		if b.ComparePrice == nil {
			return false
		}
		if *a.ComparePrice != *b.ComparePrice {
			return *a.ComparePrice < *b.ComparePrice
		}
		if a.UpdatedAt.Equal(b.UpdatedAt) {
			return a.ItemID < b.ItemID
		}
		return a.UpdatedAt.Before(b.UpdatedAt)
	})
}

func countAcceptedInE(rows []QuoteReviewRowInput) int {
	n := 0
	for i := range rows {
		r := rows[i]
		if r.InE && r.Status == MfrReviewAccepted {
			n++
		}
	}
	return n
}

// ComputeQuoteReviewLineOutcome 计算单条需求行上的规则 B 与 TopK/TopN 集合（SRS §4、§5；验收 V-1～V-6）。
// rows 必须为「一条 t_bom_session_line」下跨平台合并后的报价输入（调用方用 ListBomQuoteItemsForSessionLineRead 全平台结果）；不在此按 platform 分桶。
func ComputeQuoteReviewLineOutcome(cfg QuoteReviewConfig, rows []QuoteReviewRowInput) QuoteReviewLineOutcome {
	topN := cfg.TopN
	if topN < 1 {
		topN = 5
	}

	if cfg.MissingPrice == QuoteReviewMissingPriceM2 && blockedM2(rows) {
		sRows := buildSRows(rows, cfg.MissingPrice)
		return QuoteReviewLineOutcome{
			RuleBOk:        false,
			CandidatePoolM: len(sRows),
			TopKItemIDs:    nil,
			TopNItemIDs:    topNPoolIDs(rows, cfg.MissingPrice, topN),
		}
	}

	sRows := buildSRows(rows, cfg.MissingPrice)
	m := len(sRows)
	if m == 0 {
		return QuoteReviewLineOutcome{
			RuleBOk:        false,
			CandidatePoolM: 0,
			TopKItemIDs:    nil,
			TopNItemIDs:    topNPoolIDs(rows, cfg.MissingPrice, topN),
		}
	}

	sortSRows(sRows)

	k := m
	if k > 3 {
		k = 3
	}
	topK := make([]uint64, 0, k)
	for i := 0; i < k; i++ {
		topK = append(topK, sRows[i].ItemID)
	}

	ruleB := true
	for i := 0; i < k; i++ {
		if sRows[i].Status != MfrReviewAccepted {
			ruleB = false
			break
		}
	}

	if ruleB && cfg.BAux == QuoteReviewBAux1 {
		if m >= 3 {
			if countAcceptedInE(rows) < 3 {
				ruleB = false
			}
		}
	}

	return QuoteReviewLineOutcome{
		RuleBOk:        ruleB,
		CandidatePoolM: m,
		TopKItemIDs:    topK,
		TopNItemIDs:    topNPoolIDs(rows, cfg.MissingPrice, topN),
	}
}

func topNPoolIDs(rows []QuoteReviewRowInput, mode QuoteReviewMissingPriceMode, topN int) []uint64 {
	pool := make([]QuoteReviewRowInput, 0)
	for i := range rows {
		if rowInCandidatePoolForTopN(rows[i], mode) {
			pool = append(pool, rows[i])
		}
	}
	sortSRows(pool)
	if len(pool) > topN {
		pool = pool[:topN]
	}
	out := make([]uint64, 0, len(pool))
	for i := range pool {
		out = append(out, pool[i].ItemID)
	}
	return out
}
