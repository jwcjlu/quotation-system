package biz

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"
)

// MaxBomLLMSheetRows LLM 模式下单表最大行数（含表头），防止超出模型上下文。
const MaxBomLLMSheetRows = 8000

// BomLLMSystemPrompt 返回 BOM 全表解析用的系统提示（与 BuildBomLLMUserPrompt 配对）。
func BomLLMSystemPrompt() string { return bomLLMSystemPrompt }

const bomLLMSystemPrompt = `你是电子元器件 BOM/物料表解析助手。用户会提供 Excel 首个工作表的**全部行**（TSV）：第一行是表头，从第二行起为数据。
你的任务：读懂表头与数据，输出**每一条有效物料行**的结构化结果。

硬性要求：
- 只输出**一个** JSON 对象，禁止 markdown 代码块，禁止任何解释或前后缀文字。
- 顶层结构严格为：{"items":[...]} 。
- items 为数组；每个元素必须是对象，且**必须**包含以下键（值可为空字符串或数字，但键名不可缺）：
  - "line_no"：整数，对应 Excel 行号（从 1 起计，表头为第 1 行，第一条数据为第 2 行）。
  - "model"：字符串，该行的物料型号/料号/MPN（**必填**，从表中读出；无则填 "" 且该行将被丢弃逻辑由下游处理）。
  - "manufacturer"：厂牌/品牌，无则 ""。
  - "package"：封装，无则 ""。
  - "params"：参数、规格、备注等合并为一个字符串，无则 ""。
  - "quantity"：**数字**（JSON number），无数量列或为空时填 1。
  - "raw_text"：可选整行原文摘要，无则 ""。
- 跳过完全空白的行；不要臆造表中不存在的列内容。
- items 顺序应与 Excel 数据行顺序一致（自上而下）。`

var fenceJSON = regexp.MustCompile("(?s)```(?:json)?\\s*([\\s\\S]*?)```")

const (
	maxBomLLMCols      = 64
	maxBomLLMCellRunes = 2000
	// MaxBomLLMPromptBytes 用户消息（TSV）字节上限，避免单次请求过大；超出请拆分表格。
	MaxBomLLMPromptBytes = 2_000_000
)

// BuildBomLLMUserPrompt 将整张首表压成 TSV（含表头与所有数据行）。行数超过 MaxBomLLMSheetRows 或字节超过 MaxBomLLMPromptBytes 时由调用方先校验。
func BuildBomLLMUserPrompt(rows [][]string) string {
	var b strings.Builder
	for ri := 0; ri < len(rows); ri++ {
		row := rows[ri]
		nc := len(row)
		if nc > maxBomLLMCols {
			nc = maxBomLLMCols
		}
		for ci := 0; ci < nc; ci++ {
			if ci > 0 {
				b.WriteByte('\t')
			}
			cell := strings.ReplaceAll(strings.TrimSpace(row[ci]), "\t", " ")
			cell = strings.ReplaceAll(cell, "\n", " ")
			cell = strings.ReplaceAll(cell, "\r", "")
			if utf8.RuneCountInString(cell) > maxBomLLMCellRunes {
				cell = string([]rune(cell)[:maxBomLLMCellRunes]) + "…"
			}
			b.WriteString(cell)
		}
		b.WriteByte('\n')
	}
	return b.String()
}

type llmBomItem struct {
	LineNo       int             `json:"line_no"`
	Model        string          `json:"model"`
	Manufacturer string          `json:"manufacturer"`
	Package      string          `json:"package"`
	Params       string          `json:"params"`
	RawText      string          `json:"raw_text"`
	Quantity     json.RawMessage `json:"quantity"`
}

// ParseBomImportLinesFromLLMJSON 从模型回复解析 items → BomImportLine（与 Excel 规则对齐：默认数量 1、params 进 ExtraJSON）。
func ParseBomImportLinesFromLLMJSON(raw string) ([]BomImportLine, []BomImportError) {
	s := strings.TrimSpace(raw)
	if m := fenceJSON.FindStringSubmatch(s); len(m) > 1 {
		s = strings.TrimSpace(m[1])
	}

	var wrap struct {
		Items []llmBomItem `json:"items"`
	}
	if err := json.Unmarshal([]byte(s), &wrap); err != nil {
		return nil, []BomImportError{{Row: 0, Field: "llm", Reason: fmt.Sprintf("invalid json: %v", err)}}
	}
	if wrap.Items == nil {
		return nil, []BomImportError{{Row: 0, Field: "llm", Reason: "missing items array"}}
	}

	out := make([]BomImportLine, 0, len(wrap.Items))
	for i, it := range wrap.Items {
		mpn := strings.TrimSpace(it.Model)
		if mpn == "" {
			continue
		}

		rowHint := it.LineNo
		if rowHint <= 0 {
			rowHint = i + 2
		}

		qty, qerr := parseLLMQuantity(it.Quantity)
		if qerr != nil {
			return nil, []BomImportError{{Row: rowHint, Field: "qty", Reason: qerr.Error()}}
		}
		if qty == nil {
			one := 1.0
			qty = &one
		}

		lineNo := it.LineNo
		if lineNo <= 0 {
			lineNo = i + 2
		}
		line := BomImportLine{
			LineNo:    lineNo,
			Mpn:       mpn,
			Mfr:       strings.TrimSpace(it.Manufacturer),
			Package:   strings.TrimSpace(it.Package),
			Qty:       qty,
			RawText:   strings.TrimSpace(it.RawText),
			ExtraJSON: nil,
		}
		if p := strings.TrimSpace(it.Params); p != "" {
			b, _ := json.Marshal(map[string]string{"params": p})
			line.ExtraJSON = b
		}
		out = append(out, line)
	}

	if len(out) == 0 {
		return nil, []BomImportError{{Row: 0, Field: "llm", Reason: "no valid rows (every item missing model)"}}
	}
	return out, nil
}

func parseLLMQuantity(raw json.RawMessage) (*float64, error) {
	s := strings.TrimSpace(string(raw))
	if s == "" || s == "null" {
		return nil, nil
	}
	var f float64
	if err := json.Unmarshal(raw, &f); err == nil {
		return &f, nil
	}
	var sVal string
	if err := json.Unmarshal(raw, &sVal); err == nil {
		sVal = strings.TrimSpace(sVal)
		if sVal == "" {
			return nil, nil
		}
		v, err := parseQtyText(sVal)
		if err != nil {
			return nil, fmt.Errorf("quantity not numeric")
		}
		return &v, nil
	}
	return nil, fmt.Errorf("quantity not numeric")
}
