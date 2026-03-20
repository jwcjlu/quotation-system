package parser

import (
	"bytes"
	"context"
	"testing"

	"github.com/xuri/excelize/v2"
)

func TestParse_Auto(t *testing.T) {
	data := makeTestExcel([]string{"型号", "厂牌", "封装", "数量"}, [][]interface{}{
		{"MP1658GTF-Z", "MPS", "LGA-16", 100},
		{"SN74HC595PWR", "TI", "TSSOP-16", 50},
	})
	ctx := context.Background()

	items, err := Parse(ctx, data, ParseModeAuto, nil)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}
	if items[0].Model != "MP1658GTF-Z" || items[0].Manufacturer != "MPS" || items[0].Quantity != 100 {
		t.Errorf("item 0: got model=%s mfr=%s qty=%d", items[0].Model, items[0].Manufacturer, items[0].Quantity)
	}
	if items[1].Model != "SN74HC595PWR" || items[1].Quantity != 50 {
		t.Errorf("item 1: got model=%s qty=%d", items[1].Model, items[1].Quantity)
	}
}

func TestParse_Custom(t *testing.T) {
	data := makeTestExcel([]string{"Part", "Mfr", "Pkg", "Qty"}, [][]interface{}{
		{"ABC123", "Vendor", "SOT-23", 20},
	})
	ctx := context.Background()

	mapping := ColumnMapping{
		"model":        "A",
		"manufacturer": "B",
		"package":      "C",
		"quantity":     "D",
	}
	items, err := Parse(ctx, data, ParseModeCustom, mapping)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
	if items[0].Model != "ABC123" || items[0].Manufacturer != "Vendor" || items[0].Package != "SOT-23" || items[0].Quantity != 20 {
		t.Errorf("got %+v", items[0])
	}
}

func TestParse_EmptyFile(t *testing.T) {
	ctx := context.Background()
	_, err := Parse(ctx, nil, ParseModeAuto, nil)
	if err == nil {
		t.Fatal("expected error for empty file")
	}
}

func TestParse_InvalidExcel(t *testing.T) {
	ctx := context.Background()
	_, err := Parse(ctx, []byte("not excel"), ParseModeAuto, nil)
	if err == nil {
		t.Fatal("expected error for invalid excel")
	}
}

func makeTestExcel(headers []string, rows [][]interface{}) []byte {
	f := excelize.NewFile()
	sheet := "Sheet1"
	for i, h := range headers {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		f.SetCellValue(sheet, cell, h)
	}
	for r, row := range rows {
		for c, v := range row {
			cell, _ := excelize.CoordinatesToCellName(c+1, r+2)
			f.SetCellValue(sheet, cell, v)
		}
	}
	var buf bytes.Buffer
	_ = f.Write(&buf)
	return buf.Bytes()
}
