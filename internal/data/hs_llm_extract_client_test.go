package data

import (
	"caichip/internal/conf"
	"caichip/pkg/pdftext"
	"context"
	"fmt"
	"testing"
)

func TestLLMExtractClient_ParseStrictJSON(t *testing.T) {
	client := NewHsLLMExtractClient(nil)
	t.Run("empty response", func(t *testing.T) {
		if _, err := client.ParseStrictJSON("   "); err == nil {
			t.Fatalf("expected empty response to fail")
		}
	})

	t.Run("non json response", func(t *testing.T) {
		if _, err := client.ParseStrictJSON("not-json"); err == nil {
			t.Fatalf("expected non-json to fail")
		}
	})

	t.Run("multiple json objects", func(t *testing.T) {
		raw := `{"tech_category":"a","tech_category_ranked":[],"component_name":"b","package_form":"c","key_specs":{"voltage":"","current":"","power":"","frequency":"","temperature":"","other":[]},"evidence":[{"field":"f","quote":"q","page":1}]} {"x":1}`
		if _, err := client.ParseStrictJSON(raw); err == nil {
			t.Fatalf("expected multiple json objects to fail")
		}
	})

	t.Run("unknown field", func(t *testing.T) {
		raw := `{"tech_category":"a","tech_category_ranked":[],"component_name":"b","package_form":"c","key_specs":{"voltage":"","current":"","power":"","frequency":"","temperature":"","other":[]},"evidence":[{"field":"f","quote":"q","page":1}],"unknown":1}`
		if _, err := client.ParseStrictJSON(raw); err == nil {
			t.Fatalf("expected unknown field to fail")
		}
	})

	t.Run("missing required top-level key", func(t *testing.T) {
		raw := `{"tech_category":"","tech_category_ranked":[],"component_name":"","package_form":"","key_specs":{"voltage":"","current":"","power":"","frequency":"","temperature":"","other":[]}}`
		if _, err := client.ParseStrictJSON(raw); err == nil {
			t.Fatalf("expected missing evidence key to fail")
		}
	})

	t.Run("missing tech_category_ranked key", func(t *testing.T) {
		raw := `{"tech_category":"集成电路","component_name":"mcu","package_form":"x","key_specs":{"voltage":"","current":"","power":"","frequency":"","temperature":"","other":[]},"evidence":[{"field":"component_name","quote":"mcu","page":1}]}`
		if _, err := client.ParseStrictJSON(raw); err == nil {
			t.Fatalf("expected missing tech_category_ranked to fail")
		}
	})

	t.Run("tech_category_ranked must be json array", func(t *testing.T) {
		raw := `{"tech_category":"","tech_category_ranked":{},"component_name":"","package_form":"","key_specs":{"voltage":"","current":"","power":"","frequency":"","temperature":"","other":[]},"evidence":[]}`
		if _, err := client.ParseStrictJSON(raw); err == nil {
			t.Fatalf("expected non-array tech_category_ranked to fail")
		}
	})

	t.Run("missing required key_specs key", func(t *testing.T) {
		raw := `{"tech_category":"","tech_category_ranked":[],"component_name":"","package_form":"","key_specs":{"voltage":"","current":"","power":"","frequency":"","temperature":""},"evidence":[]}`
		if _, err := client.ParseStrictJSON(raw); err == nil {
			t.Fatalf("expected missing key_specs.other key to fail")
		}
	})

	t.Run("invalid evidence struct", func(t *testing.T) {
		raw := `{"tech_category":"a","tech_category_ranked":[],"component_name":"b","package_form":"c","key_specs":{"voltage":"","current":"","power":"","frequency":"","temperature":"","other":[]},"evidence":[{"field":"","quote":"q","page":1}]}`
		if _, err := client.ParseStrictJSON(raw); err == nil {
			t.Fatalf("expected invalid evidence to fail")
		}
	})

	t.Run("missing fields but valid", func(t *testing.T) {
		// spec: allow empty strings and empty evidence.
		raw := `{"tech_category":"","tech_category_ranked":[],"component_name":"","package_form":"","key_specs":{"voltage":"","current":"","power":"","frequency":"","temperature":"","other":[]},"evidence":[]}`
		got, err := client.ParseStrictJSON(raw)
		if err != nil {
			t.Fatalf("expected empty fields allowed, got err=%v", err)
		}
		if got == nil {
			t.Fatalf("expected non-nil result")
		}
	})

	t.Run("evidence page zero allowed", func(t *testing.T) {
		raw := `{"tech_category":"","tech_category_ranked":[],"component_name":"","package_form":"","key_specs":{"voltage":"","current":"","power":"","frequency":"","temperature":"","other":[]},"evidence":[{"field":"component_name","quote":"MOSFET","page":0}]}`
		if _, err := client.ParseStrictJSON(raw); err != nil {
			t.Fatalf("expected evidence page=0 allowed, got err=%v", err)
		}
	})

	t.Run("valid path", func(t *testing.T) {
		raw := `{"tech_category":"半导体器件","tech_category_ranked":[],"component_name":"transistor","package_form":"SOT-23","key_specs":{"voltage":"30V","current":"5A","power":"","frequency":"","temperature":"125C","other":["Rds(on)"]},"evidence":[{"field":"component_name","quote":"N-Channel MOSFET","page":1}]}`
		got, err := client.ParseStrictJSON(raw)
		if err != nil {
			t.Fatalf("expected valid json success, got err=%v", err)
		}
		if got.TechCategory != "半导体器件" || got.ComponentName != "transistor" {
			t.Fatalf("unexpected parsed content")
		}
		if len(got.TechCategoryRanked) != 1 || got.TechCategoryRanked[0].TechCategory != "半导体器件" {
			t.Fatalf("expected ranked backfill from tech_category, got %#v", got.TechCategoryRanked)
		}
	})

	t.Run("tech_category_ranked sorts dedupes caps three", func(t *testing.T) {
		raw := `{"tech_category":"集成电路","tech_category_ranked":[{"rank":2,"tech_category":"无源器件","confidence":0.9},{"rank":1,"tech_category":"集成电路","confidence":0.5},{"rank":3,"tech_category":"集成电路","confidence":0.99},{"rank":4,"tech_category":"其他","confidence":0.1},{"rank":5,"tech_category":"电路板","confidence":0.2}],"component_name":"mcu","package_form":"x","key_specs":{"voltage":"","current":"","power":"","frequency":"","temperature":"","other":[]},"evidence":[{"field":"component_name","quote":"mcu","page":1},{"field":"tech_category","quote":"passive","page":1}]}`
		got, err := client.ParseStrictJSON(raw)
		if err != nil {
			t.Fatalf("expected success, got err=%v", err)
		}
		if len(got.TechCategoryRanked) != 3 {
			t.Fatalf("expected 3 ranked after cap, got %d", len(got.TechCategoryRanked))
		}
		if got.TechCategoryRanked[0].TechCategory != "集成电路" || got.TechCategoryRanked[0].Confidence != 0.99 {
			t.Fatalf("unexpected rank1: %#v", got.TechCategoryRanked[0])
		}
		if got.TechCategoryRanked[1].TechCategory != "无源器件" {
			t.Fatalf("unexpected rank2: %#v", got.TechCategoryRanked[1])
		}
		if got.TechCategoryRanked[2].TechCategory != "电路板" {
			t.Fatalf("unexpected rank3: %#v", got.TechCategoryRanked[2])
		}
		if got.TechCategory != "集成电路" {
			t.Fatalf("expected top tech_category from ranked[0], got %q", got.TechCategory)
		}
		for i := range got.TechCategoryRanked {
			if got.TechCategoryRanked[i].Rank != i+1 {
				t.Fatalf("expected rank %d got %d", i+1, got.TechCategoryRanked[i].Rank)
			}
		}
	})

	t.Run("component name ascii normalized to canon lowercase", func(t *testing.T) {
		raw := `{"tech_category":"集成电路","tech_category_ranked":[],"component_name":"CPU","package_form":"LQFP","key_specs":{"voltage":"","current":"","power":"","frequency":"","temperature":"","other":[]},"evidence":[{"field":"component_name","quote":"CPU","page":1}]}`
		got, err := client.ParseStrictJSON(raw)
		if err != nil {
			t.Fatalf("expected valid json success, got err=%v", err)
		}
		if got.ComponentName != "cpu" {
			t.Fatalf("expected component_name cpu, got %q", got.ComponentName)
		}
	})

	t.Run("tech category not in allowed set cleared", func(t *testing.T) {
		raw := `{"tech_category":"功率器件","tech_category_ranked":[],"component_name":"MOSFET","package_form":"SOT-23","key_specs":{"voltage":"","current":"","power":"","frequency":"","temperature":"","other":[]},"evidence":[{"field":"component_name","quote":"MOSFET","page":1}]}`
		got, err := client.ParseStrictJSON(raw)
		if err != nil {
			t.Fatalf("expected valid json success, got err=%v", err)
		}
		if got.TechCategory != "" {
			t.Fatalf("expected tech_category cleared, got %q", got.TechCategory)
		}
		if got.ComponentName != "" {
			t.Fatalf("expected component_name cleared, got %q", got.ComponentName)
		}
	})

	t.Run("tech category allowed values preserved", func(t *testing.T) {
		for _, want := range []string{"半导体器件", "集成电路", "无源器件", "电路板", "其他"} {
			raw := `{"tech_category":"` + want + `","tech_category_ranked":[],"component_name":"mcu","package_form":"y","key_specs":{"voltage":"","current":"","power":"","frequency":"","temperature":"","other":[]},"evidence":[{"field":"component_name","quote":"mcu","page":1}]}`
			got, err := client.ParseStrictJSON(raw)
			if err != nil {
				t.Fatalf("want=%q: %v", want, err)
			}
			if got.TechCategory != want {
				t.Fatalf("want=%q got=%q", want, got.TechCategory)
			}
			if got.ComponentName != "mcu" {
				t.Fatalf("want component mcu got=%q", got.ComponentName)
			}
		}
	})
}

/*
api_key: "sk-cebf97382d7a4b0f85c60edf2fbcfa9e"
base_url: "https://api.deepseek.com/v1"   # 可选，代理/Azure 网关
model: "deepseek-chat"
*/
func TestLLMExtractClient_Extract(t *testing.T) {
	aiChat := NewOpenAIChat(&conf.Bootstrap{
		Openai: &conf.OpenAI{
			ApiKey:  "sk-cebf97382d7a4b0f85c60edf2fbcfa9e",
			BaseUrl: "https://api.deepseek.com/v1",
			Model:   "deepseek-chat"}})
	client := NewHsLLMExtractClient(aiChat)

	filePath := "D://960b8eab58f119ac5f30d3a0a28c754f.pdf"
	// 只取 PDF 正文前 10000 字符，避免把样式/对象元数据喂给 LLM。
	const maxLen = 10000
	pdfText, err := pdftext.ReadBodyHeadFromFile(filePath, maxLen)
	if err != nil {
		t.Fatalf("failed to extract PDF body text: %v", err)
	}
	t.Logf("Read PDF body text: %s, size=%d chars", filePath, len([]rune(pdfText)))
	result, err := client.Extract(context.WithoutCancel(context.Background()), pdfText)
	if err != nil {
		panic(err)
	}
	fmt.Println(result)
}
