package data

import "testing"

func TestLLMRecommendClient_ParseStrictJSON(t *testing.T) {
	client := NewHsLLMRecommendClient(nil)
	candidates := []string{"1111111111", "2222222222", "3333333333"}

	t.Run("empty response", func(t *testing.T) {
		if _, err := client.ParseStrictJSON(" ", candidates); err == nil {
			t.Fatalf("expected empty response to fail")
		}
	})

	t.Run("multiple json objects", func(t *testing.T) {
		raw := `{"best_code_ts":"1111111111","best_score":0.9,"top3":[{"rank":1,"code_ts":"1111111111","g_name":"a","score":0.9,"reason":"r1"},{"rank":2,"code_ts":"2222222222","g_name":"b","score":0.8,"reason":"r2"},{"rank":3,"code_ts":"3333333333","g_name":"c","score":0.7,"reason":"r3"}],"decision_note":"ok"} {"x":1}`
		if _, err := client.ParseStrictJSON(raw, candidates); err == nil {
			t.Fatalf("expected multiple json objects to fail")
		}
	})

	t.Run("unknown field", func(t *testing.T) {
		raw := `{"best_code_ts":"1111111111","best_score":0.9,"top3":[{"rank":1,"code_ts":"1111111111","g_name":"a","score":0.9,"reason":"r1"},{"rank":2,"code_ts":"2222222222","g_name":"b","score":0.8,"reason":"r2"},{"rank":3,"code_ts":"3333333333","g_name":"c","score":0.7,"reason":"r3"}],"decision_note":"ok","extra":"x"}`
		if _, err := client.ParseStrictJSON(raw, candidates); err == nil {
			t.Fatalf("expected unknown field to fail")
		}
	})

	t.Run("missing required top-level key", func(t *testing.T) {
		raw := `{"best_code_ts":"1111111111","best_score":0.9,"top3":[{"rank":1,"code_ts":"1111111111","g_name":"a","score":0.9,"reason":"r1"},{"rank":2,"code_ts":"2222222222","g_name":"b","score":0.8,"reason":"r2"},{"rank":3,"code_ts":"3333333333","g_name":"c","score":0.7,"reason":"r3"}]}`
		if _, err := client.ParseStrictJSON(raw, candidates); err == nil {
			t.Fatalf("expected missing decision_note key to fail")
		}
	})

	t.Run("missing required top3 item key", func(t *testing.T) {
		raw := `{"best_code_ts":"1111111111","best_score":0.9,"top3":[{"rank":1,"code_ts":"1111111111","g_name":"a","score":0.9},{"rank":2,"code_ts":"2222222222","g_name":"b","score":0.8,"reason":"r2"},{"rank":3,"code_ts":"3333333333","g_name":"c","score":0.7,"reason":"r3"}],"decision_note":"ok"}`
		if _, err := client.ParseStrictJSON(raw, candidates); err == nil {
			t.Fatalf("expected missing top3 reason key to fail")
		}
	})

	t.Run("best_code_ts outside candidates", func(t *testing.T) {
		raw := `{"best_code_ts":"9999999999","best_score":0.9,"top3":[{"rank":1,"code_ts":"9999999999","g_name":"a","score":0.9,"reason":"r1"},{"rank":2,"code_ts":"2222222222","g_name":"b","score":0.8,"reason":"r2"},{"rank":3,"code_ts":"3333333333","g_name":"c","score":0.7,"reason":"r3"}],"decision_note":"ok"}`
		if _, err := client.ParseStrictJSON(raw, candidates); err == nil {
			t.Fatalf("expected best_code_ts outside candidates to fail")
		}
	})

	t.Run("top3 length exceeds candidate cap", func(t *testing.T) {
		two := []string{"1111111111", "2222222222"}
		raw := `{"best_code_ts":"1111111111","best_score":0.9,"top3":[{"rank":1,"code_ts":"1111111111","g_name":"a","score":0.9,"reason":"r1"},{"rank":2,"code_ts":"2222222222","g_name":"b","score":0.8,"reason":"r2"},{"rank":3,"code_ts":"3333333333","g_name":"c","score":0.7,"reason":"r3"}],"decision_note":"ok"}`
		if _, err := client.ParseStrictJSON(raw, two); err == nil {
			t.Fatalf("expected top3 longer than candidate count to fail")
		}
	})

	t.Run("valid top2 when three candidates", func(t *testing.T) {
		raw := `{"best_code_ts":"1111111111","best_score":0.9,"top3":[{"rank":1,"code_ts":"1111111111","g_name":"a","score":0.9,"reason":"r1"},{"rank":2,"code_ts":"2222222222","g_name":"b","score":0.8,"reason":"r2"}],"decision_note":"ok"}`
		got, err := client.ParseStrictJSON(raw, candidates)
		if err != nil {
			t.Fatalf("expected ok, err=%v", err)
		}
		if len(got.Top3) != 2 || got.Top3[0].Rank != 1 || got.Top3[1].Rank != 2 {
			t.Fatalf("unexpected top3: %+v", got.Top3)
		}
	})

	t.Run("valid single top3 when one candidate", func(t *testing.T) {
		one := []string{"1111111111"}
		raw := `{"best_code_ts":"1111111111","best_score":0.9,"top3":[{"rank":1,"code_ts":"1111111111","g_name":"a","score":0.9,"reason":"only"}],"decision_note":"ok"}`
		got, err := client.ParseStrictJSON(raw, one)
		if err != nil {
			t.Fatalf("expected ok, err=%v", err)
		}
		if len(got.Top3) != 1 || got.BestCodeTS != "1111111111" {
			t.Fatalf("unexpected: %+v", got)
		}
	})

	t.Run("top3 rank duplicated", func(t *testing.T) {
		raw := `{"best_code_ts":"1111111111","best_score":0.9,"top3":[{"rank":1,"code_ts":"1111111111","g_name":"a","score":0.9,"reason":"r1"},{"rank":1,"code_ts":"2222222222","g_name":"b","score":0.8,"reason":"r2"},{"rank":3,"code_ts":"3333333333","g_name":"c","score":0.7,"reason":"r3"}],"decision_note":"ok"}`
		if _, err := client.ParseStrictJSON(raw, candidates); err == nil {
			t.Fatalf("expected duplicated rank to fail")
		}
	})

	t.Run("best_code_ts not equal rank1 code", func(t *testing.T) {
		raw := `{"best_code_ts":"2222222222","best_score":0.9,"top3":[{"rank":1,"code_ts":"1111111111","g_name":"a","score":0.9,"reason":"r1"},{"rank":2,"code_ts":"2222222222","g_name":"b","score":0.8,"reason":"r2"},{"rank":3,"code_ts":"3333333333","g_name":"c","score":0.7,"reason":"r3"}],"decision_note":"ok"}`
		if _, err := client.ParseStrictJSON(raw, candidates); err == nil {
			t.Fatalf("expected best_code_ts/rank1 mismatch to fail")
		}
	})

	t.Run("valid path", func(t *testing.T) {
		raw := `{"best_code_ts":"1111111111","best_score":0.9,"top3":[{"rank":1,"code_ts":"1111111111","g_name":"a","score":0.9,"reason":"r1"},{"rank":2,"code_ts":"2222222222","g_name":"b","score":0.8,"reason":"r2"},{"rank":3,"code_ts":"3333333333","g_name":"c","score":0.7,"reason":"r3"}],"decision_note":"ok"}`
		got, err := client.ParseStrictJSON(raw, candidates)
		if err != nil {
			t.Fatalf("expected valid path success, got err=%v", err)
		}
		if got.BestCodeTS != "1111111111" {
			t.Fatalf("unexpected best_code_ts: %s", got.BestCodeTS)
		}
	})

	t.Run("valid path with empty optional strings", func(t *testing.T) {
		raw := `{"best_code_ts":"1111111111","best_score":0.0,"top3":[{"rank":1,"code_ts":"1111111111","g_name":"","score":0.0,"reason":""},{"rank":2,"code_ts":"2222222222","g_name":"","score":0.0,"reason":""},{"rank":3,"code_ts":"3333333333","g_name":"","score":0.0,"reason":""}],"decision_note":""}`
		if _, err := client.ParseStrictJSON(raw, candidates); err != nil {
			t.Fatalf("expected complete keys with empty values to pass, got err=%v", err)
		}
	})
}
