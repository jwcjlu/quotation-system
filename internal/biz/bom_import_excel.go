package biz

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"

	"caichip/pkg/xlsread"

	"github.com/xuri/excelize/v2"
)

// headerAliases 设计 spec §6.1：表头别名不区分大小写，去首尾空格后匹配。
var headerAliases = map[string][]string{
	"line_no": {"行号", "序号", "no", "#", "line"},
	"mpn": {
		"型号", "mpn", "料号", "part", "型号*",
		"part number", "part no", "partno", "pn",
		"model", "物料编码", "物料代码", "物料号", "规格型号", "品名",
		"产品型号", "元件型号", "商品编码", "产品编码", "内部料号",
	},
	"mfr":      {"厂牌", "制造商", "品牌", "mfr", "manufacturer"},
	"package":  {"封装", "package"},
	"qty":      {"数量", "qty", "用量", "quantity"},
	"params":   {"参数", "规格", "description", "备注", "参数说明"},
	"raw_text": {"原始文本", "原文"},
}

// BomImportLine Excel 解析后的一行（待写入 bom_session_line）。
type BomImportLine struct {
	LineNo                  int
	Mpn                     string
	Mfr                     string
	ManufacturerCanonicalID *string
	Package                 string
	Qty                     *float64
	ExtraJSON               []byte
	RawText                 string
}

// BomImportError 校验错误（可映射为 API errors 数组）。
type BomImportError struct {
	Row    int
	Field  string
	Reason string
}

func (e BomImportError) Error() string {
	return fmt.Sprintf("row %d field %s: %s", e.Row, e.Field, e.Reason)
}

// columnMappingProtoKeyToLogical 与 UploadBOMRequest.column_mapping 的 key 对齐（不区分大小写）。
var columnMappingProtoKeyToLogical = map[string]string{
	"model":        "mpn",
	"manufacturer": "mfr",
	"mfr":          "mfr",
	"package":      "package",
	"quantity":     "qty",
	"qty":          "qty",
	"params":       "params",
	"raw":          "raw_text",
	"raw_text":     "raw_text",
}

// ParseBomImportRows 解析首个工作表：首行为表头。partial=false 时任一错误行则整表失败（仍返回全部 errors 便于展示）。
func ParseBomImportRows(r io.Reader, partial bool) ([]BomImportLine, []BomImportError) {
	return ParseBomImportRowsWithColumnMapping(r, partial, nil)
}

// ParseBomImportRowsWithColumnMapping 与 ParseBomImportRows 相同；当 columnMapping 非空时，按首行表头与映射值（Excel 列标题原文）精确匹配列，不再仅用内置别名。
func ParseBomImportRowsWithColumnMapping(r io.Reader, partial bool, columnMapping map[string]string) ([]BomImportLine, []BomImportError) {
	rows, fileErrs := readBomImportFirstSheetRows(r)
	if len(fileErrs) > 0 {
		return nil, fileErrs
	}
	return ParseBomImportRowsFromMatrix(rows, partial, columnMapping)
}

// ParseBomImportRowsFromMatrix 在已读入的首个工作表矩阵上解析（首行为表头）。用于 LLM 推断列映射后与本地解析复用同一套规则。
func ParseBomImportRowsFromMatrix(rows [][]string, partial bool, columnMapping map[string]string) ([]BomImportLine, []BomImportError) {
	if len(rows) == 0 {
		return nil, []BomImportError{{Row: 1, Field: "header", Reason: "empty sheet"}}
	}

	var colMap map[string]int
	if len(columnMapping) > 0 {
		colMap = mapHeaderFromProtoColumnMapping(rows[0], columnMapping)
	} else {
		colMap = mapHeaderRow(rows[0])
	}
	if _, ok := colMap["mpn"]; !ok {
		reason := "missing mpn column: row 1 must include a recognized part/model header (e.g. 型号, mpn, 料号, part number, model, 规格型号)"
		if len(columnMapping) > 0 {
			if h := strings.TrimSpace(columnMappingGetCI(columnMapping, "model")); h != "" {
				reason = fmt.Sprintf("column_mapping: mapped model header %q not found in row 1 (check spelling and BOM characters)", h)
			} else {
				reason = "column_mapping: field model must map to the Excel header text in row 1"
			}
		}
		return nil, []BomImportError{{Row: 1, Field: "header", Reason: reason}}
	}

	var out []BomImportLine
	var errs []BomImportError
	seq := 0
	for i := 1; i < len(rows); i++ {
		excelRow := i + 1
		row := rows[i]
		if rowIsEmpty(row) {
			continue
		}
		line, rowErrs := parseDataRow(excelRow, row, colMap, &seq)
		errs = append(errs, rowErrs...)
		if len(rowErrs) > 0 {
			if !partial {
				return nil, errs
			}
			continue
		}
		out = append(out, line)
	}

	if !partial && len(errs) > 0 {
		return nil, errs
	}
	return out, errs
}

func mapHeaderRow(header []string) map[string]int {
	colMap := make(map[string]int)
	for i, h := range header {
		key := normalizeHeaderCell(h)
		if key == "" {
			continue
		}
		for logical, aliases := range headerAliases {
			for _, a := range aliases {
				if key == normalizeHeaderCell(a) {
					if _, taken := colMap[logical]; !taken {
						colMap[logical] = i
					}
					break
				}
			}
		}
	}
	return colMap
}

// ReadBomImportFirstSheetFromReader 读取首个工作表全部行（首行表头）。UploadBOM 在 llm 模式下先读表再推断列映射。
func ReadBomImportFirstSheetFromReader(r io.Reader) ([][]string, []BomImportError) {
	return readBomImportFirstSheetRows(r)
}

func readBomImportFirstSheetRows(r io.Reader) ([][]string, []BomImportError) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, []BomImportError{{Row: 0, Field: "file", Reason: err.Error()}}
	}
	if len(data) == 0 {
		return nil, []BomImportError{{Row: 0, Field: "file", Reason: "empty file"}}
	}

	if xlsread.IsOLECompound(data) {
		rows, xerr := xlsread.FirstSheetRows(data)
		if xerr != nil {
			return nil, []BomImportError{{Row: 0, Field: "file", Reason: xlsread.FormatError(xerr)}}
		}
		if len(rows) == 0 {
			return nil, []BomImportError{{Row: 1, Field: "header", Reason: "empty sheet"}}
		}
		return rows, nil
	}

	f, err := excelize.OpenReader(bytes.NewReader(data))
	if err != nil {
		return nil, []BomImportError{{Row: 0, Field: "file", Reason: err.Error()}}
	}
	defer func() { _ = f.Close() }()
	sheets := f.GetSheetList()
	if len(sheets) == 0 {
		return nil, []BomImportError{{Row: 0, Field: "file", Reason: "no sheets"}}
	}
	rows, err := f.GetRows(sheets[0])
	if err != nil {
		return nil, []BomImportError{{Row: 0, Field: "file", Reason: err.Error()}}
	}
	if len(rows) == 0 {
		return nil, []BomImportError{{Row: 1, Field: "header", Reason: "empty sheet"}}
	}
	return rows, nil
}

func columnMappingGetCI(m map[string]string, key string) string {
	want := strings.ToLower(strings.TrimSpace(key))
	for k, v := range m {
		if strings.ToLower(strings.TrimSpace(k)) == want {
			return v
		}
	}
	return ""
}

func mapHeaderFromProtoColumnMapping(header []string, columnMapping map[string]string) map[string]int {
	colMap := make(map[string]int)
	for protoKey, excelHdr := range columnMapping {
		logical, ok := columnMappingProtoKeyToLogical[strings.ToLower(strings.TrimSpace(protoKey))]
		if !ok || strings.TrimSpace(excelHdr) == "" {
			continue
		}
		want := normalizeHeaderCell(excelHdr)
		if want == "" {
			continue
		}
		for i, cell := range header {
			if normalizeHeaderCell(cell) == want {
				if _, taken := colMap[logical]; !taken {
					colMap[logical] = i
				}
				break
			}
		}
	}
	return colMap
}

func normalizeHeaderCell(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "\ufeff")
	s = strings.TrimSpace(s)
	return strings.ToLower(s)
}

func rowIsEmpty(row []string) bool {
	for _, c := range row {
		if strings.TrimSpace(c) != "" {
			return false
		}
	}
	return true
}

func cellAt(row []string, colIdx int) string {
	if colIdx < 0 || colIdx >= len(row) {
		return ""
	}
	return strings.TrimSpace(row[colIdx])
}

func parseDataRow(excelRow int, row []string, colMap map[string]int, seq *int) (BomImportLine, []BomImportError) {
	var errs []BomImportError
	mpn := cellAt(row, colMap["mpn"])
	if mpn == "" {
		errs = append(errs, BomImportError{Row: excelRow, Field: "mpn", Reason: "empty"})
		return BomImportLine{}, errs
	}

	line := BomImportLine{Mpn: mpn}
	if idx, ok := colMap["line_no"]; ok {
		if s := cellAt(row, idx); s != "" {
			n, err := strconv.Atoi(s)
			if err != nil {
				errs = append(errs, BomImportError{Row: excelRow, Field: "line_no", Reason: "not_integer"})
			} else {
				line.LineNo = n
			}
		}
	}
	if line.LineNo == 0 {
		*seq++
		line.LineNo = *seq
	}

	if idx, ok := colMap["mfr"]; ok {
		line.Mfr = cellAt(row, idx)
	}
	if idx, ok := colMap["package"]; ok {
		line.Package = cellAt(row, idx)
	}

	if idx, ok := colMap["qty"]; ok {
		qs := cellAt(row, idx)
		if qs == "" {
			one := 1.0
			line.Qty = &one
		} else {
			q, err := parseQtyText(qs)
			if err != nil {
				errs = append(errs, BomImportError{Row: excelRow, Field: "qty", Reason: "not_numeric"})
			} else {
				line.Qty = &q
			}
		}
	} else {
		one := 1.0
		line.Qty = &one
	}

	extra := map[string]string{}
	if idx, ok := colMap["params"]; ok {
		if v := cellAt(row, idx); v != "" {
			extra["params"] = v
		}
	}
	if len(extra) > 0 {
		b, _ := json.Marshal(extra)
		line.ExtraJSON = b
	}

	if idx, ok := colMap["raw_text"]; ok {
		line.RawText = cellAt(row, idx)
	}

	if len(errs) > 0 {
		return BomImportLine{}, errs
	}
	return line, nil
}
