package data

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
)

const hsLLMRecommendSystemPrompt = `你是电子元器件 HS 编码推荐助手。
输入包含结构化特征与候选集，请仅从候选集中选择并输出严格 JSON：
{
  "best_code_ts": "",
  "best_score": 0.0,
  "top3": [
    {"rank":1,"code_ts":"","g_name":"","score":0.0,"reason":""}
  ],
  "decision_note": ""
}
禁止输出 markdown 或解释文字。best_code_ts 必须来自输入候选集，且必须等于 top3 中 rank=1 的 code_ts。
top3 数组长度须为 1～min(3, 候选税号个数)：候选不足 3 个时只输出实际条数；rank 从 1 起连续递增且不重复。`

type HsLLMRecommendClient struct {
	chat hsLLMChatter
}

func NewHsLLMRecommendClient(chat hsLLMChatter) *HsLLMRecommendClient {
	return &HsLLMRecommendClient{chat: chat}
}

type HsLLMRecommendItem struct {
	Rank   int     `json:"rank"`
	CodeTS string  `json:"code_ts"`
	GName  string  `json:"g_name"`
	Score  float64 `json:"score"`
	Reason string  `json:"reason"`
}

type HsLLMRecommendResult struct {
	BestCodeTS   string               `json:"best_code_ts"`
	BestScore    float64              `json:"best_score"`
	Top3         []HsLLMRecommendItem `json:"top3"`
	DecisionNote string               `json:"decision_note"`
}

func (c *HsLLMRecommendClient) Recommend(
	ctx context.Context,
	featureJSON string,
	candidateJSON string,
	candidateCodeTS []string,
) (*HsLLMRecommendResult, error) {
	if c == nil || c.chat == nil {
		return nil, fmt.Errorf("hs llm recommend client not configured")
	}
	user := strings.TrimSpace(featureJSON) + "\n\n候选集：\n" + strings.TrimSpace(candidateJSON)
	raw, err := c.chat.Chat(ctx, hsLLMRecommendSystemPrompt, user)
	if err != nil {
		return nil, err
	}
	return c.ParseStrictJSON(raw, candidateCodeTS)
}

func (c *HsLLMRecommendClient) ParseStrictJSON(raw string, candidateCodeTS []string) (*HsLLMRecommendResult, error) {
	_ = c
	body := strings.TrimSpace(raw)
	if body == "" {
		return nil, fmt.Errorf("llm recommend response is empty")
	}
	if !json.Valid([]byte(body)) {
		return nil, fmt.Errorf("llm recommend response must be strict json")
	}
	if err := validateHsLLMRecommendRequiredKeys(body); err != nil {
		return nil, err
	}
	dec := json.NewDecoder(strings.NewReader(body))
	dec.DisallowUnknownFields()
	var out HsLLMRecommendResult
	if err := dec.Decode(&out); err != nil {
		return nil, fmt.Errorf("llm recommend response invalid: %w", err)
	}
	if err := dec.Decode(new(struct{})); err != io.EOF {
		return nil, fmt.Errorf("llm recommend response must contain exactly one json object")
	}

	allowed := make(map[string]struct{}, len(candidateCodeTS))
	for _, code := range candidateCodeTS {
		code = strings.TrimSpace(code)
		if code == "" {
			continue
		}
		allowed[code] = struct{}{}
	}
	bestCode := strings.TrimSpace(out.BestCodeTS)
	if bestCode == "" {
		return nil, fmt.Errorf("llm recommend response missing best_code_ts")
	}
	if _, ok := allowed[bestCode]; !ok {
		return nil, fmt.Errorf("llm recommend response invalid: best_code_ts not in candidates")
	}

	n := len(out.Top3)
	maxTop := 3
	if len(allowed) < maxTop {
		maxTop = len(allowed)
	}
	if n < 1 {
		return nil, fmt.Errorf("llm recommend response invalid: top3 must contain at least 1 item")
	}
	if n > maxTop {
		return nil, fmt.Errorf("llm recommend response invalid: top3 must contain at most %d items (got %d)", maxTop, n)
	}

	seenRank := make(map[int]struct{}, n)
	rankOneCode := ""
	for i := range out.Top3 {
		rank := out.Top3[i].Rank
		if rank < 1 || rank > n {
			return nil, fmt.Errorf("llm recommend response invalid: top3[%d].rank must be in [1,%d]", i, n)
		}
		if _, exists := seenRank[rank]; exists {
			return nil, fmt.Errorf("llm recommend response invalid: top3 rank duplicated")
		}
		seenRank[rank] = struct{}{}

		code := strings.TrimSpace(out.Top3[i].CodeTS)
		if code == "" {
			return nil, fmt.Errorf("llm recommend response invalid: top3[%d].code_ts is empty", i)
		}
		if _, ok := allowed[code]; !ok {
			return nil, fmt.Errorf("llm recommend response invalid: top3[%d].code_ts not in candidates", i)
		}
		out.Top3[i].CodeTS = code
		if rank == 1 {
			rankOneCode = code
		}
	}
	for r := 1; r <= n; r++ {
		if _, ok := seenRank[r]; !ok {
			return nil, fmt.Errorf("llm recommend response invalid: top3 must include rank %d exactly once", r)
		}
	}
	// Rule: best_code_ts must be exactly the code_ts of rank=1.
	if rankOneCode == "" || bestCode != rankOneCode {
		return nil, fmt.Errorf("llm recommend response invalid: best_code_ts must equal top3 rank=1 code_ts")
	}
	sort.SliceStable(out.Top3, func(i, j int) bool { return out.Top3[i].Rank < out.Top3[j].Rank })
	out.BestCodeTS = bestCode
	return &out, nil
}

func validateHsLLMRecommendRequiredKeys(body string) error {
	var root map[string]json.RawMessage
	if err := json.Unmarshal([]byte(body), &root); err != nil {
		return fmt.Errorf("llm recommend response must be json object: %w", err)
	}
	requiredTop := []string{"best_code_ts", "best_score", "top3", "decision_note"}
	for _, key := range requiredTop {
		if _, ok := root[key]; !ok {
			return fmt.Errorf("llm recommend response missing required key: %s", key)
		}
	}
	var top3Items []map[string]json.RawMessage
	if err := json.Unmarshal(root["top3"], &top3Items); err != nil {
		return fmt.Errorf("llm recommend response invalid top3: %w", err)
	}
	requiredItemKeys := []string{"rank", "code_ts", "g_name", "score", "reason"}
	for i := range top3Items {
		for _, key := range requiredItemKeys {
			if _, ok := top3Items[i][key]; !ok {
				return fmt.Errorf("llm recommend response missing required top3[%d] key: %s", i, key)
			}
		}
	}
	return nil
}
