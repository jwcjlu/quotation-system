package xlsread

import (
	"bytes"
	"errors"
	"fmt"

	"github.com/extrame/xls"
)

// OLE CFB 文件头（老版 .xls 为 OLE 复合文档，而非 OOXML ZIP）。
var headerOLECFB = []byte{0xD0, 0xCF, 0x11, 0xE0, 0xA1, 0xB1, 0x1A, 0xE1}

// IsOLECompound reports whether data begins with the legacy OLE compound file signature (typical for BIFF .xls).
func IsOLECompound(data []byte) bool {
	return len(data) >= len(headerOLECFB) && bytes.Equal(data[:len(headerOLECFB)], headerOLECFB)
}

// FirstSheetRows reads the first worksheet as row-major cell strings (same shape as excelize GetRows).
func FirstSheetRows(data []byte) ([][]string, error) {
	if len(data) == 0 {
		return nil, errors.New("empty file")
	}
	wb, err := xls.OpenReader(bytes.NewReader(data), "utf-8")
	if err != nil {
		return nil, err
	}
	if wb == nil {
		return nil, errors.New("open returned nil workbook")
	}
	if wb.NumSheets() == 0 {
		return nil, errors.New("no sheets")
	}
	sheet := wb.GetSheet(0)
	if sheet == nil {
		return nil, errors.New("first sheet is nil")
	}
	return worksheetToRows(sheet), nil
}

func worksheetToRows(sheet *xls.WorkSheet) [][]string {
	maxRow := int(sheet.MaxRow)
	out := make([][]string, 0, maxRow+1)
	for i := 0; i <= maxRow; i++ {
		r := sheet.Row(i)
		if r == nil {
			out = append(out, []string{})
			continue
		}
		last := r.LastCol()
		if last < 0 {
			out = append(out, []string{})
			continue
		}
		width := last + 1
		row := make([]string, width)
		for j := 0; j < width; j++ {
			row[j] = r.Col(j)
		}
		out = append(out, row)
	}
	return out
}

// FormatError wraps low-level reader errors for API messages.
func FormatError(err error) string {
	if err == nil {
		return ""
	}
	return fmt.Sprintf("xls: %v", err)
}
