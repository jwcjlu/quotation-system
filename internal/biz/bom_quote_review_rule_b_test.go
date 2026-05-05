package biz

import (
	"testing"
	"time"
)

func f64p(v float64) *float64 { return &v }

func row(inE bool, status string, price *float64, itemID uint64, ts time.Time) QuoteReviewRowInput {
	return QuoteReviewRowInput{InE: inE, Status: status, ComparePrice: price, ItemID: itemID, UpdatedAt: ts}
}

// V-1：S 共 3 条，价格序 10/20/30，均为 pending → 不满足；全转 accepted 后满足。
func TestQuoteReviewRuleB_V1_threePendingThenAccepted(t *testing.T) {
	cfg := DefaultQuoteReviewConfig()
	t0 := time.Unix(100, 0)
	rows := []QuoteReviewRowInput{
		row(true, MfrReviewPending, f64p(10), 1, t0),
		row(true, MfrReviewPending, f64p(20), 2, t0),
		row(true, MfrReviewPending, f64p(30), 3, t0),
	}
	o := ComputeQuoteReviewLineOutcome(cfg, rows)
	if o.RuleBOk {
		t.Fatalf("expected RuleBOk=false, got true")
	}
	if o.CandidatePoolM != 3 {
		t.Fatalf("CandidatePoolM want 3 got %d", o.CandidatePoolM)
	}
	rows[0].Status = MfrReviewAccepted
	rows[1].Status = MfrReviewAccepted
	rows[2].Status = MfrReviewAccepted
	o = ComputeQuoteReviewLineOutcome(cfg, rows)
	if !o.RuleBOk {
		t.Fatalf("expected RuleBOk=true")
	}
}

// V-2：S 共 5 条，最便宜 3 条中 1 条 pending → 不满足。
func TestQuoteReviewRuleB_V2_topKHasPending(t *testing.T) {
	cfg := DefaultQuoteReviewConfig()
	t0 := time.Unix(1, 0)
	rows := []QuoteReviewRowInput{
		row(true, MfrReviewAccepted, f64p(1), 10, t0),
		row(true, MfrReviewPending, f64p(2), 11, t0),
		row(true, MfrReviewAccepted, f64p(3), 12, t0),
		row(true, MfrReviewAccepted, f64p(4), 13, t0),
		row(true, MfrReviewAccepted, f64p(5), 14, t0),
	}
	o := ComputeQuoteReviewLineOutcome(cfg, rows)
	if o.RuleBOk {
		t.Fatalf("expected RuleBOk=false")
	}
}

// V-3：最便宜一条 rejected → 不参与 S；TopK 在剩余集合上重算。
func TestQuoteReviewRuleB_V3_cheapestRejectedRestRecalc(t *testing.T) {
	cfg := DefaultQuoteReviewConfig()
	t0 := time.Unix(1, 0)
	rows := []QuoteReviewRowInput{
		row(true, MfrReviewRejected, f64p(1), 1, t0),
		row(true, MfrReviewAccepted, f64p(10), 2, t0),
		row(true, MfrReviewAccepted, f64p(20), 3, t0),
	}
	o := ComputeQuoteReviewLineOutcome(cfg, rows)
	if !o.RuleBOk {
		t.Fatalf("expected RuleBOk=true after excluding rejected from S")
	}
	if len(o.TopKItemIDs) != 2 {
		t.Fatalf("TopK len want 2 got %d", len(o.TopKItemIDs))
	}
}

// V-4：同价并列 → TopK 由稳定次序键唯一确定。
func TestQuoteReviewRuleB_V4_tieBreakStable(t *testing.T) {
	cfg := DefaultQuoteReviewConfig()
	t1 := time.Unix(10, 0)
	t2 := time.Unix(20, 0)
	rows := []QuoteReviewRowInput{
		row(true, MfrReviewAccepted, f64p(5), 300, t2),
		row(true, MfrReviewAccepted, f64p(5), 100, t1),
		row(true, MfrReviewAccepted, f64p(5), 200, t1),
	}
	o := ComputeQuoteReviewLineOutcome(cfg, rows)
	if !o.RuleBOk {
		t.Fatalf("expected RuleBOk=true")
	}
	// sorted: t1 before t2, then item id 100,200,300 for first 3 -> all 3 accepted
	want := []uint64{100, 200, 300}
	if len(o.TopKItemIDs) != len(want) {
		t.Fatalf("TopK len %d", len(o.TopKItemIDs))
	}
	for i := range want {
		if o.TopKItemIDs[i] != want[i] {
			t.Fatalf("TopK[%d] want %d got %d", i, want[i], o.TopKItemIDs[i])
		}
	}
}

// V-5：m=0 → 不满足结案。
func TestQuoteReviewRuleB_V5_emptyS(t *testing.T) {
	cfg := DefaultQuoteReviewConfig()
	if ComputeQuoteReviewLineOutcome(cfg, nil).RuleBOk {
		t.Fatal("empty rows")
	}
	rows := []QuoteReviewRowInput{row(true, MfrReviewRejected, f64p(1), 1, time.Time{})}
	if ComputeQuoteReviewLineOutcome(cfg, rows).RuleBOk {
		t.Fatal("only rejected")
	}
	// M1：无 compare 不入 S
	rows2 := []QuoteReviewRowInput{row(true, MfrReviewPending, nil, 1, time.Time{})}
	if ComputeQuoteReviewLineOutcome(cfg, rows2).RuleBOk {
		t.Fatal("m1 missing price -> empty S")
	}
}

// V-6：最便宜 K 条均 accepted，下方大量 pending → 满足（B-aux-2）。
func TestQuoteReviewRuleB_V6_topKAcceptedRestPending(t *testing.T) {
	cfg := DefaultQuoteReviewConfig()
	t0 := time.Unix(1, 0)
	rows := []QuoteReviewRowInput{
		row(true, MfrReviewAccepted, f64p(1), 1, t0),
		row(true, MfrReviewAccepted, f64p(2), 2, t0),
		row(true, MfrReviewAccepted, f64p(3), 3, t0),
		row(true, MfrReviewPending, f64p(4), 4, t0),
		row(true, MfrReviewPending, f64p(5), 5, t0),
	}
	o := ComputeQuoteReviewLineOutcome(cfg, rows)
	if !o.RuleBOk {
		t.Fatalf("expected RuleBOk=true")
	}
}

func TestQuoteReviewRuleB_M2_blocksWhenMissingPrice(t *testing.T) {
	cfg := QuoteReviewConfig{MissingPrice: QuoteReviewMissingPriceM2, BAux: QuoteReviewBAux2, TopN: 5}
	rows := []QuoteReviewRowInput{
		row(true, MfrReviewPending, f64p(10), 1, time.Time{}),
		row(true, MfrReviewPending, nil, 2, time.Time{}),
	}
	o := ComputeQuoteReviewLineOutcome(cfg, rows)
	if o.RuleBOk {
		t.Fatal("m2 should block")
	}
}

func TestQuoteReviewRuleB_BAux1_requiresThreeAcceptedWhenMGe3(t *testing.T) {
	cfg := QuoteReviewConfig{MissingPrice: QuoteReviewMissingPriceM1, BAux: QuoteReviewBAux1, TopN: 5}
	t0 := time.Unix(1, 0)
	rows := []QuoteReviewRowInput{
		row(true, MfrReviewAccepted, f64p(1), 1, t0),
		row(true, MfrReviewAccepted, f64p(2), 2, t0),
		row(true, MfrReviewAccepted, f64p(3), 3, t0),
		row(true, MfrReviewPending, f64p(100), 4, t0),
		row(true, MfrReviewPending, f64p(200), 5, t0),
	}
	// Top3 all accepted but only 3 accepted in E — need >=3 accepted in E: we have exactly 3, OK
	o := ComputeQuoteReviewLineOutcome(cfg, rows)
	if !o.RuleBOk {
		t.Fatalf("expected ok with 3 accepted in E")
	}
	rows[2].Status = MfrReviewPending
	o = ComputeQuoteReviewLineOutcome(cfg, rows)
	if o.RuleBOk {
		t.Fatalf("expected false: top3 not all accepted")
	}
}
