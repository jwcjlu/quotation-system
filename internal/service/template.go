package service

import (
	"bytes"

	"github.com/xuri/excelize/v2"
)

// generateBOMTemplate 生成 BOM 模板 Excel
func generateBOMTemplate() ([]byte, error) {
	f := excelize.NewFile()
	sheet := "Sheet1"

	headers := []string{"型号", "厂牌", "封装", "数量", "参数"}
	for i, h := range headers {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		f.SetCellValue(sheet, cell, h)
	}

	f.SetCellValue(sheet, "A2", "SN74HC595PWR")
	f.SetCellValue(sheet, "B2", "TI")
	f.SetCellValue(sheet, "C2", "TSSOP-16")
	f.SetCellValue(sheet, "D2", 10)
	f.SetCellValue(sheet, "E2", "")

	var buf bytes.Buffer
	if err := f.Write(&buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
