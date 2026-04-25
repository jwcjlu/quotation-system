package biz

import "testing"

func TestLineGapActiveKeyOnlyForOpen(t *testing.T) {
	g := BOMLineGap{
		SessionID: "sid",
		LineID:    10,
		GapType:   LineGapNoData,
		Status:    LineGapOpen,
	}
	if got := g.ActiveKey(); got != "sid:10:NO_DATA" {
		t.Fatalf("active key=%q", got)
	}
	g.Status = LineGapResolved
	if got := g.ActiveKey(); got != "" {
		t.Fatalf("resolved active key=%q, want empty", got)
	}
}

func TestLineGapCanTransition(t *testing.T) {
	tests := []struct {
		from string
		to   string
		ok   bool
	}{
		{LineGapOpen, LineGapManualQuoteAdded, true},
		{LineGapOpen, LineGapSubstituteSelected, true},
		{LineGapManualQuoteAdded, LineGapResolved, true},
		{LineGapSubstituteSelected, LineGapResolved, true},
		{LineGapResolved, LineGapOpen, false},
	}
	for _, tt := range tests {
		if got := CanTransitionLineGap(tt.from, tt.to); got != tt.ok {
			t.Fatalf("%s -> %s got %v", tt.from, tt.to, got)
		}
	}
}
