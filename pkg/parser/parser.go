package parser

import (
	"context"
	"fmt"
)

// ParseMode 解析模式
type ParseMode string

const (
	ParseModeSZLCSC ParseMode = "szlcsc" // 立创商城
	ParseModeIckey  ParseMode = "ickey"  // 云汉芯城
	ParseModeAuto   ParseMode = "auto"   // 自动识别
	ParseModeCustom ParseMode = "custom" // 自定义列映射
)

// ParsedItem 解析后的物料项
type ParsedItem struct {
	Index        int
	Raw          string
	Model        string
	Manufacturer string
	Package      string
	Quantity     int
	Params       string
}

// ColumnMapping 自定义列映射，key: model/manufacturer/package/quantity/params, value: 列名(A,B,C...)或列索引
type ColumnMapping map[string]string

// Parser BOM Excel 解析器接口
type Parser interface {
	Parse(ctx context.Context, data []byte, mode ParseMode, mapping ColumnMapping) ([]*ParsedItem, error)
}

// Parse 解析 BOM Excel
func Parse(ctx context.Context, data []byte, mode ParseMode, mapping ColumnMapping) ([]*ParsedItem, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("empty file")
	}
	p := &ExcelParser{}
	return p.Parse(ctx, data, mode, mapping)
}
