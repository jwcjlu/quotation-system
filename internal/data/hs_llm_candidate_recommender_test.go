package data

import (
	"context"
	"testing"

	"caichip/internal/biz"
)

type stubRecommendChat struct {
	raw string
	err error
}

func (s stubRecommendChat) Chat(_ context.Context, _, _ string) (string, error) {
	return s.raw, s.err
}

func TestHsLLMCandidateRecommender_Recommend(t *testing.T) {
	t.Parallel()
	client := NewHsLLMRecommendClient(stubRecommendChat{
		raw: `{"best_code_ts":"1111111111","best_score":0.9,"top3":[{"rank":1,"code_ts":"1111111111","g_name":"a","score":0.9,"reason":"r1"},{"rank":2,"code_ts":"2222222222","g_name":"b","score":0.8,"reason":"r2"},{"rank":3,"code_ts":"3333333333","g_name":"c","score":0.7,"reason":"r3"}],"decision_note":"ok"}`,
	})
	recommender := NewHsLLMCandidateRecommender(client)
	got, err := recommender.Recommend(context.Background(), biz.HsPrefilterInput{
		TechCategory:  "IC",
		ComponentName: "STM32F103",
	}, []biz.HsItemCandidate{
		{CodeTS: "1111111111", GName: "a", Score: 0.2},
		{CodeTS: "2222222222", GName: "b", Score: 0.1},
		{CodeTS: "3333333333", GName: "c", Score: 0.05},
	}, 3)
	if err != nil {
		t.Fatalf("recommend failed: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("expected 3 candidates, got %d", len(got))
	}
	if got[0].CodeTS != "1111111111" || got[0].Score != 0.9 {
		t.Fatalf("unexpected top1: %+v", got[0])
	}
	if got[0].Reason != "r1" {
		t.Fatalf("expected LLM reason on candidate, got %q", got[0].Reason)
	}
}
