package data

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

const hsLLMExtractSystemPrompt = `你是电子元器件 datasheet 结构化抽取助手。
只输出一个 JSON 对象，不允许输出 markdown、代码块、解释文字。
输出结构必须严格包含字段：
{
  "tech_category": "",
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
缺失字段填空字符串或空数组，不得臆造。`

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

type HsLLMExtractResult struct {
	TechCategory  string                 `json:"tech_category"`
	ComponentName string                 `json:"component_name"`
	PackageForm   string                 `json:"package_form"`
	KeySpecs      HsLLMExtractKeySpecs   `json:"key_specs"`
	Evidence      []HsLLMExtractEvidence `json:"evidence"`
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
	requiredTop := []string{"tech_category", "component_name", "package_form", "key_specs", "evidence"}
	for _, key := range requiredTop {
		if _, ok := root[key]; !ok {
			return fmt.Errorf("llm extract response missing required key: %s", key)
		}
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
	out.TechCategory = strings.TrimSpace(out.TechCategory)
	out.ComponentName = strings.TrimSpace(out.ComponentName)
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
