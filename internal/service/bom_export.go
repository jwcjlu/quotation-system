package service

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"strconv"
	"strings"

	"caichip/internal/biz"

	"github.com/xuri/excelize/v2"
)

func exportSessionLinesToXLSX(lines []*biz.BOMSessionLine, sessionID string) ([]byte, string, error) {
	f := excelize.NewFile()
	const sheet = "BOM"
	_ = f.SetSheetName("Sheet1", sheet)
	headers := []string{"行号", "型号(MPN)", "厂牌", "封装", "数量", "原始行", "备注(JSON)"}
	for i, h := range headers {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		_ = f.SetCellValue(sheet, cell, h)
	}
	for r, ln := range lines {
		row := r + 2
		_ = f.SetCellValue(sheet, cellName(1, row), ln.LineNo)
		_ = f.SetCellValue(sheet, cellName(2, row), ln.MPN)
		_ = f.SetCellValue(sheet, cellName(3, row), ln.MFR)
		_ = f.SetCellValue(sheet, cellName(4, row), ln.Package)
		if ln.Qty != nil {
			_ = f.SetCellValue(sheet, cellName(5, row), *ln.Qty)
		}
		_ = f.SetCellValue(sheet, cellName(6, row), ln.RawText)
		if len(ln.ExtraJSON) > 0 {
			_ = f.SetCellValue(sheet, cellName(7, row), string(ln.ExtraJSON))
		}
	}
	buf, err := f.WriteToBuffer()
	if err != nil {
		return nil, "", err
	}
	short := sessionID
	if len(short) > 8 {
		short = short[:8]
	}
	return buf.Bytes(), fmt.Sprintf("bom_session_%s_export.xlsx", short), nil
}

func exportSessionLinesToCSV(lines []*biz.BOMSessionLine, sessionID string) ([]byte, string, error) {
	var b bytes.Buffer
	w := csv.NewWriter(&b)
	_ = w.Write([]string{"line_no", "mpn", "mfr", "package", "qty", "raw_text", "extra_json"})
	for _, ln := range lines {
		qty := ""
		if ln.Qty != nil {
			qty = strconv.FormatFloat(*ln.Qty, 'f', -1, 64)
		}
		ex := ""
		if len(ln.ExtraJSON) > 0 {
			ex = string(ln.ExtraJSON)
		}
		_ = w.Write([]string{
			strconv.Itoa(ln.LineNo),
			ln.MPN,
			ln.MFR,
			ln.Package,
			qty,
			strings.ReplaceAll(ln.RawText, "\n", " "),
			ex,
		})
	}
	w.Flush()
	if err := w.Error(); err != nil {
		return nil, "", err
	}
	short := sessionID
	if len(short) > 8 {
		short = short[:8]
	}
	return b.Bytes(), fmt.Sprintf("bom_session_%s_export.csv", short), nil
}

func cellName(col, row int) string {
	c, _ := excelize.CoordinatesToCellName(col, row)
	return c
}
