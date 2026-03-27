package biz

import (
	"encoding/json"
	"testing"
)

func TestParseTaskStdoutQuotes_Array(t *testing.T) {
	in := `[{"seq":1,"model":"ABC","manufacturer":"M","package":"P","desc":"D","stock":"100","moq":"1","price_tiers":"N/A","hk_price":"N/A","mainland_price":"N/A","lead_time":"7-9 工作日","query_model":"ABC"}]`
	b, ok := ParseTaskStdoutQuotes(in)
	if !ok || b == nil {
		t.Fatalf("expected ok, got ok=%v b=%v", ok, b)
	}
	var rows []AgentQuoteRow
	if err := json.Unmarshal(b, &rows); err != nil || len(rows) != 1 || rows[0].Model != "ABC" {
		t.Fatalf("unmarshal: %v rows=%v", err, rows)
	}
}

func TestParseTaskStdoutQuotes_EmptyArray(t *testing.T) {
	b, ok := ParseTaskStdoutQuotes(`[]`)
	if !ok || string(b) != `[]` {
		t.Fatalf("expected ok and [], got ok=%v b=%s", ok, string(b))
	}
}

func TestParseTaskStdoutQuotes_ResultsObject(t *testing.T) {
	in := `{"error":"","results":[{"seq":1,"model":"X","manufacturer":"M","package":"","desc":"","stock":"","moq":"","price_tiers":"","hk_price":"","mainland_price":"","lead_time":""}]}`
	b, ok := ParseTaskStdoutQuotes(in)
	if !ok {
		t.Fatal("expected ok")
	}
	var rows []AgentQuoteRow
	if err := json.Unmarshal(b, &rows); err != nil || len(rows) != 1 || rows[0].Model != "X" {
		t.Fatal(err)
	}
}

func TestParseTaskStdoutQuotes_ErrorField(t *testing.T) {
	in := `{"error":"oops","results":[]}`
	_, ok := ParseTaskStdoutQuotes(in)
	if ok {
		t.Fatal("expected not ok")
	}
}

func TestParseTaskStdoutQuotes_RejectMissingModel(t *testing.T) {
	in := `[{"seq":1,"manufacturer":"M"}]`
	_, ok := ParseTaskStdoutQuotes(in)
	if ok {
		t.Fatal("expected not ok without model")
	}
}

func TestParseTaskStdoutQuotes_Empty(t *testing.T) {
	if _, ok := ParseTaskStdoutQuotes(""); ok {
		t.Fatal("expected not ok")
	}
	if _, ok := ParseTaskStdoutQuotes("  "); ok {
		t.Fatal("expected not ok")
	}
}
