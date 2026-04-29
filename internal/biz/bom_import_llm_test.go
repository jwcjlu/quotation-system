package biz

import "testing"

func TestParseBomImportLinesFromLLMJSON(t *testing.T) {
	raw := `{"items":[
		{"line_no":2,"model":"STM32F103","manufacturer":"ST","package":"LQFP64","quantity":10,"params":"工业级","raw_text":""},
		{"line_no":3,"model":"  ","manufacturer":"","package":"","quantity":1,"params":"","raw_text":""},
		{"line_no":4,"model":"LM358","manufacturer":"","package":"DIP8","quantity":null,"params":"","raw_text":""}
	]}`
	lines, errs := ParseBomImportLinesFromLLMJSON(raw)
	if len(errs) > 0 {
		t.Fatal(errs)
	}
	if len(lines) != 2 {
		t.Fatalf("want 2 lines (skip empty model), got %d", len(lines))
	}
	if lines[0].Mpn != "STM32F103" || lines[0].LineNo != 2 || *lines[0].Qty != 10 {
		t.Fatalf("line0: %+v", lines[0])
	}
	if lines[1].Mpn != "LM358" || *lines[1].Qty != 1.0 {
		t.Fatalf("line1: %+v", lines[1])
	}
}

func TestParseBomImportLinesFromLLMJSONFenced(t *testing.T) {
	raw := "```json\n{\"items\":[{\"line_no\":2,\"model\":\"X\",\"manufacturer\":\"\",\"package\":\"\",\"quantity\":1,\"params\":\"\",\"raw_text\":\"\"}]}\n```"
	lines, errs := ParseBomImportLinesFromLLMJSON(raw)
	if len(errs) != 0 {
		t.Fatal(errs)
	}
	if len(lines) != 1 || lines[0].Mpn != "X" {
		t.Fatal(lines)
	}
}

func TestParseBomImportLinesFromLLMJSONRequiresItems(t *testing.T) {
	_, errs := ParseBomImportLinesFromLLMJSON(`{}`)
	if len(errs) == 0 {
		t.Fatal("expected error")
	}
}

func TestParseBomImportLinesFromLLMJSONQuantityString(t *testing.T) {
	raw := `{"items":[{"line_no":2,"model":"A","manufacturer":"","package":"","quantity":"12","params":"","raw_text":""}]}`
	lines, errs := ParseBomImportLinesFromLLMJSON(raw)
	if len(errs) != 0 {
		t.Fatal(errs)
	}
	if *lines[0].Qty != 12 {
		t.Fatal(*lines[0].Qty)
	}
}

func TestParseBomImportLinesFromLLMJSONQuantityRangeString(t *testing.T) {
	raw := `{"items":[{"line_no":2,"model":"A","manufacturer":"","package":"","quantity":"10000-12000","params":"","raw_text":""}]}`
	lines, errs := ParseBomImportLinesFromLLMJSON(raw)
	if len(errs) != 0 {
		t.Fatal(errs)
	}
	if *lines[0].Qty != 10000 {
		t.Fatal(*lines[0].Qty)
	}
}
