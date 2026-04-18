package data

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"unicode"
)

const hsLLMExtractSystemPrompt = `你是电子元器件 datasheet 结构化抽取助手。
只输出一个 JSON 对象，不允许输出 markdown、代码块、解释文字。
输出结构必须严格包含字段：
{
  "tech_category": "",
  "tech_category_ranked": [
    {"rank": 1, "tech_category": "", "confidence": 0.0}
  ],
  "component_name": "",
  "package_form": "",
  "key_specs": {
    "voltage": "",
    "current": "",
    "power": "",
    "frequency": "",
    "temperature": "",
    "other": []
  },
  "evidence": [{"field":"","quote":"","page":0}]
}
tech_category 与 tech_category_ranked 中每一项的 tech_category 只能取以下字面量之一（与下列中文完全一致，勿改写或加前后缀）：
半导体器件、集成电路、无源器件、电路板、其他。
若无法明确归入其中任一类，顶字段 tech_category 填空字符串 ""，且 tech_category_ranked 必须为 []。
tech_category_ranked：0~3 条；每条 tech_category 须为上述五类之一；confidence 为 0~1 的自评数值；
按 confidence 从高到低排序；rank 从 1 连续递增且与排序一致。禁止为凑满 3 条编造次要类别；仅一类有依据时只输出 1 条。
顶字段 tech_category 必须与归一后 tech_category_ranked[0].tech_category 一致（若 ranked 为空数组则顶字段也必须为 ""）。
若 tech_category_ranked 含 2 条及以上，对 rank≥2 的每一类须在 evidence 中有能支撑该类判断的条目（例如 field 为 tech_category 并引用手册原文）。
component_name 只能从下列字面量中择一输出（英文区分大小写时请输出表中小写英文；中文与表中完全一致）：
单片机、mcu、microcontroller、微控制器、处理器、cpu、processor、存储器、memory、ram、rom、flash、
二极管、diode、晶体管、transistor、电容、电容器、capacitor、电阻、电阻器、resistor、连接器、connector、
电感、电感器、inductor。
若无法与上表任一项对应（含自拟类别名），component_name 必须填空字符串 ""，禁止输出表外名称。
缺失其他字段填空字符串或空数组，不得臆造。`

// hsLLMAllowedTechCategories 为 datasheet 抽取 tech_category 的封闭取值集合（与提示词一致）。
var hsLLMAllowedTechCategories = map[string]struct{}{
	"半导体器件": {},
	"集成电路":  {},
	"无源器件":  {},
	"电路板":   {},
	"其他":    {},
}

func normalizeHsLLMTechCategory(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	if _, ok := hsLLMAllowedTechCategories[s]; ok {
		return s
	}
	return ""
}

// hsLLMComponentNameCanonList 为 datasheet 抽取 component_name 的封闭取值集合（与提示词一致）。
var hsLLMComponentNameCanonList = []string{
	"单片机", "mcu", "microcontroller", "微控制器",
	"处理器", "cpu", "processor",
	"存储器", "memory", "ram", "rom", "flash",
	"二极管", "diode",
	"晶体管", "transistor",
	"电容", "电容器", "capacitor",
	"电阻", "电阻器", "resistor",
	"连接器", "connector",
	"电感", "电感器", "inductor",
}

// hsLLMComponentNameLookup：精确匹配或（输入全为 ASCII 时）不区分大小写匹配到规范字面量。
var hsLLMComponentNameLookup map[string]string

func init() {
	hsLLMComponentNameLookup = make(map[string]string, len(hsLLMComponentNameCanonList)*2)
	for _, canon := range hsLLMComponentNameCanonList {
		hsLLMComponentNameLookup[canon] = canon
		allASCII := true
		for _, r := range canon {
			if r > unicode.MaxASCII {
				allASCII = false
				break
			}
		}
		if allASCII {
			hsLLMComponentNameLookup[strings.ToLower(canon)] = canon
		}
	}
}

func normalizeHsLLMComponentName(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	if canon, ok := hsLLMComponentNameLookup[s]; ok {
		return canon
	}
	allASCII := true
	for _, r := range s {
		if r > unicode.MaxASCII {
			allASCII = false
			break
		}
	}
	if allASCII {
		if canon, ok := hsLLMComponentNameLookup[strings.ToLower(s)]; ok {
			return canon
		}
	}
	return ""
}

type hsLLMChatter interface {
	Chat(ctx context.Context, system, user string) (string, error)
}

type HsLLMExtractClient struct {
	chat hsLLMChatter
}

func NewHsLLMExtractClient(chat hsLLMChatter) *HsLLMExtractClient {
	return &HsLLMExtractClient{chat: chat}
}

type HsLLMExtractKeySpecs struct {
	Voltage     string   `json:"voltage"`
	Current     string   `json:"current"`
	Power       string   `json:"power"`
	Frequency   string   `json:"frequency"`
	Temperature string   `json:"temperature"`
	Other       []string `json:"other"`
}

type HsLLMExtractEvidence struct {
	Field string `json:"field"`
	Quote string `json:"quote"`
	Page  int    `json:"page"`
}

type HsLLMExtractTechCategoryRank struct {
	Rank         int     `json:"rank"`
	TechCategory string  `json:"tech_category"`
	Confidence   float64 `json:"confidence"`
}

type HsLLMExtractResult struct {
	TechCategory       string                         `json:"tech_category"`
	TechCategoryRanked []HsLLMExtractTechCategoryRank `json:"tech_category_ranked"`
	ComponentName      string                         `json:"component_name"`
	PackageForm        string                         `json:"package_form"`
	KeySpecs           HsLLMExtractKeySpecs           `json:"key_specs"`
	Evidence           []HsLLMExtractEvidence         `json:"evidence"`
}

func (c *HsLLMExtractClient) Extract(ctx context.Context, datasheetText string) (*HsLLMExtractResult, error) {
	if c == nil || c.chat == nil {
		return nil, fmt.Errorf("hs llm extract client not configured")
	}
	raw, err := c.chat.Chat(ctx, hsLLMExtractSystemPrompt, strings.TrimSpace(datasheetText))
	if err != nil {
		return nil, err
	}
	return c.ParseStrictJSON(raw)
}

func (c *HsLLMExtractClient) ParseStrictJSON(raw string) (*HsLLMExtractResult, error) {
	_ = c
	body := strings.TrimSpace(raw)
	if body == "" {
		return nil, fmt.Errorf("llm extract response is empty")
	}
	if !json.Valid([]byte(body)) {
		return nil, fmt.Errorf("llm extract response must be strict json")
	}
	if err := validateHsLLMExtractRequiredKeys(body); err != nil {
		return nil, err
	}
	dec := json.NewDecoder(strings.NewReader(body))
	dec.DisallowUnknownFields()
	var out HsLLMExtractResult
	if err := dec.Decode(&out); err != nil {
		return nil, fmt.Errorf("llm extract response invalid: %w", err)
	}
	if err := dec.Decode(new(struct{})); err != io.EOF {
		return nil, fmt.Errorf("llm extract response must contain exactly one json object")
	}
	if err := validateHsLLMExtractResult(&out); err != nil {
		return nil, err
	}
	return &out, nil
}

func validateHsLLMExtractRequiredKeys(body string) error {
	var root map[string]json.RawMessage
	if err := json.Unmarshal([]byte(body), &root); err != nil {
		return fmt.Errorf("llm extract response must be json object: %w", err)
	}
	requiredTop := []string{"tech_category", "tech_category_ranked", "component_name", "package_form", "key_specs", "evidence"}
	for _, key := range requiredTop {
		if _, ok := root[key]; !ok {
			return fmt.Errorf("llm extract response missing required key: %s", key)
		}
	}
	var rankedProbe []json.RawMessage
	if err := json.Unmarshal(root["tech_category_ranked"], &rankedProbe); err != nil {
		return fmt.Errorf("llm extract response invalid tech_category_ranked: must be json array: %w", err)
	}
	var keySpecs map[string]json.RawMessage
	if err := json.Unmarshal(root["key_specs"], &keySpecs); err != nil {
		return fmt.Errorf("llm extract response invalid key_specs: %w", err)
	}
	requiredKeySpecs := []string{"voltage", "current", "power", "frequency", "temperature", "other"}
	for _, key := range requiredKeySpecs {
		if _, ok := keySpecs[key]; !ok {
			return fmt.Errorf("llm extract response missing required key_specs key: %s", key)
		}
	}
	return nil
}

func validateHsLLMExtractResult(out *HsLLMExtractResult) error {
	if out == nil {
		return fmt.Errorf("llm extract response is nil")
	}
	topNorm := normalizeHsLLMTechCategory(out.TechCategory)
	out.TechCategoryRanked = normalizeHsLLMExtractTechCategoryRanked(out.TechCategoryRanked, topNorm)
	if len(out.TechCategoryRanked) == 0 {
		out.TechCategory = ""
	} else {
		out.TechCategory = out.TechCategoryRanked[0].TechCategory
	}
	out.ComponentName = normalizeHsLLMComponentName(out.ComponentName)
	out.PackageForm = strings.TrimSpace(out.PackageForm)
	out.KeySpecs.Voltage = strings.TrimSpace(out.KeySpecs.Voltage)
	out.KeySpecs.Current = strings.TrimSpace(out.KeySpecs.Current)
	out.KeySpecs.Power = strings.TrimSpace(out.KeySpecs.Power)
	out.KeySpecs.Frequency = strings.TrimSpace(out.KeySpecs.Frequency)
	out.KeySpecs.Temperature = strings.TrimSpace(out.KeySpecs.Temperature)

	// spec: fields can be empty; only validate evidence item shape when provided.
	for i := range out.Evidence {
		out.Evidence[i].Field = strings.TrimSpace(out.Evidence[i].Field)
		out.Evidence[i].Quote = strings.TrimSpace(out.Evidence[i].Quote)
		if out.Evidence[i].Field == "" {
			return fmt.Errorf("llm extract response invalid: evidence[%d].field is empty", i)
		}
		if out.Evidence[i].Quote == "" {
			return fmt.Errorf("llm extract response invalid: evidence[%d].quote is empty", i)
		}
	}
	return nil
}
