package biz

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/xuri/excelize/v2"
)

func TestBomImport_HeaderAliasesMpnQty(t *testing.T) {
	f := excelize.NewFile()
	sheet := f.GetSheetName(0)
	_ = f.SetSheetRow(sheet, "A1", &[]any{"型号", "数量"})
	_ = f.SetSheetRow(sheet, "A2", &[]any{"LM358", 10})
	_ = f.SetSheetRow(sheet, "A3", &[]any{"TL431", ""})

	var buf bytes.Buffer
	if err := f.Write(&buf); err != nil {
		t.Fatal(err)
	}
	_ = f.Close()

	lines, errs := ParseBomImportRows(&buf, false)
	if len(errs) != 0 {
		t.Fatalf("errs: %v", errs)
	}
	if len(lines) != 2 {
		t.Fatalf("lines len %d", len(lines))
	}
	if lines[0].Mpn != "LM358" || lines[0].Qty == nil || *lines[0].Qty != 10 {
		t.Fatalf("line0 %+v", lines[0])
	}
	if lines[1].Mpn != "TL431" || *lines[1].Qty != 1 {
		t.Fatalf("line1 default qty %+v", lines[1])
	}
}

func TestBomImport_EmptyMpnRow5(t *testing.T) {
	f := excelize.NewFile()
	sheet := f.GetSheetName(0)
	_ = f.SetSheetRow(sheet, "A1", &[]any{"型号", "数量"})
	_ = f.SetSheetRow(sheet, "A2", &[]any{"OK1", 1})
	_ = f.SetSheetRow(sheet, "A3", &[]any{"OK2", 1})
	_ = f.SetSheetRow(sheet, "A4", &[]any{"OK3", 1})
	_ = f.SetSheetRow(sheet, "A5", &[]any{"", 1})

	var buf bytes.Buffer
	if err := f.Write(&buf); err != nil {
		t.Fatal(err)
	}
	_ = f.Close()

	_, errs := ParseBomImportRows(&buf, false)
	if len(errs) != 1 {
		t.Fatalf("expected 1 error, got %v", errs)
	}
	if errs[0].Row != 5 || errs[0].Field != "mpn" {
		t.Fatalf("got %+v", errs[0])
	}
}

func TestBomImport_PartialSkipsBadRow(t *testing.T) {
	f := excelize.NewFile()
	sheet := f.GetSheetName(0)
	_ = f.SetSheetRow(sheet, "A1", &[]any{"型号", "数量"})
	_ = f.SetSheetRow(sheet, "A2", &[]any{"GOOD", 1})
	_ = f.SetSheetRow(sheet, "A3", &[]any{"", 1})

	var buf bytes.Buffer
	if err := f.Write(&buf); err != nil {
		t.Fatal(err)
	}
	_ = f.Close()

	lines, errs := ParseBomImportRows(&buf, true)
	if len(errs) != 1 || errs[0].Field != "mpn" {
		t.Fatalf("errs %v", errs)
	}
	if len(lines) != 1 || lines[0].Mpn != "GOOD" {
		t.Fatalf("lines %+v", lines)
	}
}

func TestBomImport_HeaderAliasPartNumber(t *testing.T) {
	f := excelize.NewFile()
	sheet := f.GetSheetName(0)
	_ = f.SetSheetRow(sheet, "A1", &[]any{"Part Number", "Qty"})
	_ = f.SetSheetRow(sheet, "A2", &[]any{"LM358", 5})

	var buf bytes.Buffer
	if err := f.Write(&buf); err != nil {
		t.Fatal(err)
	}
	_ = f.Close()

	lines, errs := ParseBomImportRows(&buf, false)
	if len(errs) != 0 {
		t.Fatalf("errs: %v", errs)
	}
	if len(lines) != 1 || lines[0].Mpn != "LM358" {
		t.Fatalf("lines %+v", lines)
	}
}

func TestBomImport_ColumnMappingCustomHeader(t *testing.T) {
	f := excelize.NewFile()
	sheet := f.GetSheetName(0)
	_ = f.SetSheetRow(sheet, "A1", &[]any{"品名", "需求量", "品牌"})
	_ = f.SetSheetRow(sheet, "A2", &[]any{"TL431", 3, "TI"})

	var buf bytes.Buffer
	if err := f.Write(&buf); err != nil {
		t.Fatal(err)
	}
	_ = f.Close()

	m := map[string]string{
		"model":        "品名",
		"quantity":     "需求量",
		"manufacturer": "品牌",
	}
	lines, errs := ParseBomImportRowsWithColumnMapping(&buf, false, m)
	if len(errs) != 0 {
		t.Fatalf("errs: %v", errs)
	}
	if len(lines) != 1 || lines[0].Mpn != "TL431" || lines[0].Mfr != "TI" {
		t.Fatalf("lines %+v", lines)
	}
	if lines[0].Qty == nil || *lines[0].Qty != 3 {
		t.Fatalf("qty %+v", lines[0].Qty)
	}
}

// 可选：仓库根目录放置 Tolen1226_req_221222_1.xls 等老版 .xls 时，验证不再出现 row0/file 格式错误。
func TestBomImport_LegacyXLS_NoFileFormatError(t *testing.T) {
	path := filepath.Join("..", "..", "Tolen1226_req_221222_1.xls")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Skip("optional sample .xls at repo root:", err)
	}
	_, errs := ParseBomImportRows(bytes.NewReader(data), true)
	for _, e := range errs {
		if e.Row == 0 && e.Field == "file" {
			t.Fatalf("file-level error (expected legacy .xls to open): %v", e)
		}
	}
}

func TestBomImport_InvalidQty(t *testing.T) {
	f := excelize.NewFile()
	sheet := f.GetSheetName(0)
	_ = f.SetSheetRow(sheet, "A1", &[]any{"型号", "数量"})
	_ = f.SetSheetRow(sheet, "A2", &[]any{"X", "abc"})

	var buf bytes.Buffer
	if err := f.Write(&buf); err != nil {
		t.Fatal(err)
	}
	_ = f.Close()

	_, errs := ParseBomImportRows(&buf, false)
	if len(errs) != 1 || errs[0].Field != "qty" {
		t.Fatalf("got %+v", errs)
	}
}

func TestBomImport_QtyRangeUsesLeftValue(t *testing.T) {
	f := excelize.NewFile()
	sheet := f.GetSheetName(0)
	_ = f.SetSheetRow(sheet, "A1", &[]any{"型号", "数量"})
	_ = f.SetSheetRow(sheet, "A2", &[]any{"X", "10000-12000"})

	var buf bytes.Buffer
	if err := f.Write(&buf); err != nil {
		t.Fatal(err)
	}
	_ = f.Close()

	lines, errs := ParseBomImportRows(&buf, false)
	if len(errs) != 0 {
		t.Fatalf("errs: %v", errs)
	}
	if len(lines) != 1 || lines[0].Qty == nil || *lines[0].Qty != 10000 {
		t.Fatalf("lines %+v", lines)
	}
}
