package biz

import "testing"

func TestDecideBatchResolvableLines(t *testing.T) {
	decisions := DecideBatchResolvableLines([]HsBatchResolveLineInput{
		{LineNo: 1, Model: "A", MatchStatus: "exact", HsCodeStatus: "hs_not_mapped"},
		{LineNo: 2, Model: "B", MatchStatus: "pending", HsCodeStatus: "hs_not_mapped"},
		{LineNo: 3, Model: "C", MatchStatus: "exact", HsCodeStatus: "hs_found"},
		{LineNo: 4, Model: " ", MatchStatus: "exact", HsCodeStatus: "hs_not_mapped"},
	})
	if len(decisions) != 4 {
		t.Fatalf("expect 4 decisions, got %d", len(decisions))
	}
	if !decisions[0].Accept {
		t.Fatalf("line 1 should be accepted")
	}
	if decisions[1].Accept || decisions[1].Reason == "" {
		t.Fatalf("line 2 should be skipped with reason")
	}
	if decisions[2].Accept || decisions[2].Reason == "" {
		t.Fatalf("line 3 should be skipped with reason")
	}
	if decisions[3].Accept || decisions[3].Reason == "" {
		t.Fatalf("line 4 should be skipped with reason")
	}
}
