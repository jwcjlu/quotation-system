package parser

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/xuri/excelize/v2"
)

// 各平台预设列名（表头匹配用）
var (
	// 立创商城 BOM 模板列名
	szlcscHeaders = map[string][]string{
		"model":        {"型号", "料号", "Part Number"},
		"manufacturer": {"品牌", "厂牌", "制造商", "Manufacturer"},
		"package":      {"封装", "Package"},
		"quantity":     {"数量", "需求数量", "Quantity"},
		"params":       {"参数", "规格", "备注"},
	}
	// 云汉芯城 BOM 模板列名
	ickeyHeaders = map[string][]string{
		"model":        {"型号", "料号", "Part Number"},
		"manufacturer": {"品牌", "厂牌", "制造商", "Manufacturer"},
		"package":      {"封装", "Package"},
		"quantity":     {"数量", "需求数量", "Quantity"},
		"params":       {"参数", "规格", "备注"},
	}
)

// ExcelParser BOM Excel 解析器
type ExcelParser struct{}

// Parse 解析 Excel 文件
func (p *ExcelParser) Parse(ctx context.Context, data []byte, mode ParseMode, mapping ColumnMapping) ([]*ParsedItem, error) {
	f, err := excelize.OpenReader(strings.NewReader(string(data)))
	if err != nil {
		return nil, fmt.Errorf("open excel: %w", err)
	}
	defer f.Close()

	sheets := f.GetSheetList()
	if len(sheets) == 0 {
		return nil, fmt.Errorf("no sheets found")
	}

	rows, err := f.GetRows(sheets[0])
	if err != nil {
		return nil, fmt.Errorf("read rows: %w", err)
	}
	if len(rows) < 2 {
		return nil, fmt.Errorf("insufficient rows (need at least header + 1 data row)")
	}

	headerRow := rows[0]
	dataRows := rows[1:]

	var colMap map[string]int
	switch mode {
	case ParseModeSZLCSC:
		colMap = p.detectColumns(headerRow, szlcscHeaders)
	case ParseModeIckey:
		colMap = p.detectColumns(headerRow, ickeyHeaders)
	case ParseModeAuto:
		colMap = p.detectColumns(headerRow, szlcscHeaders)
		if colMap == nil {
			colMap = p.detectColumns(headerRow, ickeyHeaders)
		}
		if colMap == nil {
			colMap = p.autoDetectColumns(headerRow)
		}
	case ParseModeCustom:
		colMap = p.parseCustomMapping(headerRow, mapping)
		if colMap == nil {
			// 自定义模式未提供列映射时，回退到自动识别
			colMap = p.autoDetectColumns(headerRow)
		}
	default:
		colMap = p.detectColumns(headerRow, szlcscHeaders)
		if colMap == nil {
			colMap = p.autoDetectColumns(headerRow)
		}
	}

	if colMap == nil {
		if mode == ParseModeCustom {
			return nil, fmt.Errorf("cannot detect columns: 自定义模式需提供 column_mapping，或表格表头无法自动识别")
		}
		return nil, fmt.Errorf("cannot detect columns for mode %s", mode)
	}

	items := make([]*ParsedItem, 0, len(dataRows))
	for i, row := range dataRows {
		item := p.parseRow(row, colMap, i+1)
		if item.Model != "" || item.Raw != "" {
			items = append(items, item)
		}
	}

	return items, nil
}

func (p *ExcelParser) detectColumns(headerRow []string, headers map[string][]string) map[string]int {
	colMap := make(map[string]int)
	for field, names := range headers {
		for _, name := range names {
			for j, h := range headerRow {
				if strings.TrimSpace(strings.ToLower(h)) == strings.TrimSpace(strings.ToLower(name)) {
					colMap[field] = j
					break
				}
			}
			if _, ok := colMap[field]; ok {
				break
			}
		}
	}
	if len(colMap) < 2 {
		return nil
	}
	return colMap
}

func (p *ExcelParser) autoDetectColumns(headerRow []string) map[string]int {
	colMap := make(map[string]int)
	allHeaders := map[string][]string{
		"model":        {"型号", "料号", "part number", "model", "partno", "item", "part", "description"},
		"manufacturer": {"品牌", "厂牌", "制造商", "manufacturer", "mfr", "brand"},
		"package":      {"封装", "package", "pkg"},
		"quantity":     {"数量", "需求数量", "quantity", "qty", "需求", "amount", "ammount", "pcs"},
		"params":       {"参数", "规格", "备注", "params", "spec"},
	}
	for field, names := range allHeaders {
		for _, name := range names {
			for j, h := range headerRow {
				hh := strings.TrimSpace(strings.ToLower(h))
				if hh == name || strings.Contains(hh, name) {
					colMap[field] = j
					break
				}
			}
			if _, ok := colMap[field]; ok {
				break
			}
		}
	}
	if len(colMap) < 1 {
		return nil
	}
	return colMap
}

func (p *ExcelParser) parseCustomMapping(headerRow []string, mapping ColumnMapping) map[string]int {
	if mapping == nil || len(mapping) == 0 {
		return nil
	}
	colMap := make(map[string]int)
	for field, col := range mapping {
		idx := p.columnToIndex(col, headerRow)
		if idx >= 0 {
			colMap[field] = idx
		}
	}
	if len(colMap) < 1 {
		return nil
	}
	return colMap
}

func (p *ExcelParser) columnToIndex(col string, headerRow []string) int {
	col = strings.TrimSpace(strings.ToUpper(col))
	/*if len(col) == 1 {
		return int(col[0] - 'A')
	}
	if len(col) == 2 {
		return (int(col[0]-'A')+1)*26 + int(col[1]-'A')
	}*/
	for i, h := range headerRow {
		if strings.TrimSpace(strings.ToLower(h)) == strings.TrimSpace(strings.ToLower(col)) {
			return i
		}
	}
	return -1
}

func (p *ExcelParser) parseRow(row []string, colMap map[string]int, index int) *ParsedItem {
	item := &ParsedItem{Index: index}
	get := func(field string) string {
		if idx, ok := colMap[field]; ok && idx < len(row) {
			return strings.TrimSpace(row[idx])
		}
		return ""
	}

	item.Model = get("model")
	item.Manufacturer = get("manufacturer")
	item.Package = get("package")
	item.Params = get("params")

	// 若 model 列有值但 manufacturer/package 为空，且 model 含空格，尝试从 Item 拆分：名称 型号 厂牌 封装 参数
	if item.Model != "" && item.Manufacturer == "" && item.Package == "" && strings.Contains(item.Model, " ") {
		_, model, mfr, pkg, params := parseCombinedItem(item.Model)
		if model != "" {
			item.Model = model
			item.Manufacturer = mfr
			item.Package = pkg
			if item.Params == "" {
				item.Params = params
			}
		}
	}

	if q := get("quantity"); q != "" {
		// 去除千位分隔符（逗号、空格等）再解析
		q = strings.ReplaceAll(q, ",", "")
		q = strings.ReplaceAll(q, " ", "")
		if n, err := strconv.Atoi(regexp.MustCompile(`\d+`).FindString(q)); err == nil {
			item.Quantity = n
		}
	}
	if item.Quantity <= 0 && item.Model != "" {
		item.Quantity = 1
	}

	item.Raw = strings.Join(row, " ")
	return item
}

// parseCombinedItem 从 "名称 型号 厂牌 封装 参数" 格式拆分，空格分隔
func parseCombinedItem(s string) (name, model, manufacturer, pkg, params string) {
	tokens := strings.Fields(s)
	if len(tokens) < 2 {
		return "", s, "", "", ""
	}

	// 找到型号：通常为字母数字组合，可能含 . - 等
	modelIdx := -1
	modelRe := regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9.\-]*[A-Za-z0-9]$|^[A-Za-z0-9][A-Za-z0-9.\-]+$`)
	for i, t := range tokens {
		if modelRe.MatchString(t) && len(t) >= 2 {
			modelIdx = i
			break
		}
	}
	if modelIdx < 0 {
		// 未匹配到，取第二个 token 作为型号（名称通常在前）
		modelIdx = 1
	}

	name = strings.Join(tokens[:modelIdx], " ")
	model = tokens[modelIdx]
	rest := tokens[modelIdx+1:]

	if len(rest) == 0 {
		return name, model, "", "", ""
	}

	// 从后往前识别：参数（尺寸如 8.1x5.3x3.2、括号内容）、封装（SMT/THT 或 SOT-23 等）
	pkgIdx := -1
	paramsIdx := len(rest)
	dimRe := regexp.MustCompile(`^[\d.]+x[\d.]+(x[\d.]+)?$`)
	smtThtRe := regexp.MustCompile(`^(SMT|THT)$`)
	pkgRe := regexp.MustCompile(`^(SOT|TO-|DO-|TSSOP|LGA|QFN|DFN|BGA|SOIC|DIP|SOD|TNT|DFM)|^[A-Z]{2,}[\d\-]+`)

	// 先找 SMT/THT（常见封装）
	for i, t := range rest {
		if smtThtRe.MatchString(t) {
			pkgIdx = i
			break
		}
	}
	// 再找参数（尺寸、括号）
	for i := len(rest) - 1; i >= 0; i-- {
		t := rest[i]
		if dimRe.MatchString(t) || (strings.HasPrefix(t, "(") || strings.HasSuffix(t, ")")) {
			paramsIdx = i
			break
		}
	}
	// 若无 SMT/THT，找其他封装（SOT-23、TO-269 等）
	if pkgIdx < 0 {
		for i := len(rest) - 1; i >= 0; i-- {
			t := rest[i]
			if pkgRe.MatchString(t) && i < paramsIdx {
				pkgIdx = i
				break
			}
		}
	}

	if pkgIdx >= 0 {
		pkg = rest[pkgIdx]
		manufacturer = strings.Join(rest[:pkgIdx], " ")
		if pkgIdx+1 < len(rest) {
			params = strings.Join(rest[pkgIdx+1:], " ")
		}
	} else if paramsIdx < len(rest) {
		params = strings.Join(rest[paramsIdx:], " ")
		manufacturer = strings.Join(rest[:paramsIdx], " ")
	} else {
		manufacturer = strings.Join(rest, " ")
	}

	return name, model, manufacturer, pkg, params
}
