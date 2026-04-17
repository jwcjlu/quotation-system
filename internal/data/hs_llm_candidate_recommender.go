package data

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"caichip/internal/biz"
)

type HsLLMCandidateRecommender struct {
	client *HsLLMRecommendClient
}

func NewHsLLMCandidateRecommender(client *HsLLMRecommendClient) *HsLLMCandidateRecommender {
	return &HsLLMCandidateRecommender{client: client}
}

func (r *HsLLMCandidateRecommender) Recommend(
	ctx context.Context,
	input biz.HsPrefilterInput,
	candidates []biz.HsItemCandidate,
	limit int,
) ([]biz.HsItemCandidate, error) {
	if r == nil || r.client == nil {
		return nil, errors.New("hs llm candidate recommender not configured")
	}
	if len(candidates) == 0 {
		return nil, nil
	}
	if limit <= 0 || limit > len(candidates) {
		limit = len(candidates)
	}
	candidates = candidates[:limit]

	featureJSON, err := json.Marshal(input)
	if err != nil {
		return nil, err
	}
	candidateJSON, codeList := buildLLMRecommendCandidates(candidates)

	got, err := r.client.Recommend(ctx, string(featureJSON), candidateJSON, codeList)
	if err != nil {
		return nil, err
	}
	index := make(map[string]biz.HsItemCandidate, len(candidates))
	for i := range candidates {
		index[strings.TrimSpace(candidates[i].CodeTS)] = candidates[i]
	}
	out := make([]biz.HsItemCandidate, 0, len(got.Top3))
	for i := range got.Top3 {
		code := strings.TrimSpace(got.Top3[i].CodeTS)
		row, ok := index[code]
		if !ok {
			continue
		}
		row.Score = got.Top3[i].Score
		out = append(out, row)
	}
	return out, nil
}

func buildLLMRecommendCandidates(candidates []biz.HsItemCandidate) (string, []string) {
	type llmCandidate struct {
		CodeTS string  `json:"code_ts"`
		GName  string  `json:"g_name"`
		Score  float64 `json:"score"`
	}
	payload := make([]llmCandidate, 0, len(candidates))
	codeList := make([]string, 0, len(candidates))
	for i := range candidates {
		code := strings.TrimSpace(candidates[i].CodeTS)
		if code == "" {
			continue
		}
		payload = append(payload, llmCandidate{
			CodeTS: code,
			GName:  strings.TrimSpace(candidates[i].GName),
			Score:  candidates[i].Score,
		})
		codeList = append(codeList, code)
	}
	body, _ := json.Marshal(payload)
	return string(body), codeList
}

var _ biz.HsModelCandidateRecommender = (*HsLLMCandidateRecommender)(nil)
