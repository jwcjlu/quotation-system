package data

import (
	"caichip/pkg/pdftext"
	"context"
	"fmt"
	"os"
	"strings"

	"caichip/internal/biz"
)

const hsExtractInputMaxBytes = 24 * 1024

// HsLLMFeatureExtractor 将 datasheet 资产转换为预筛输入。
type HsLLMFeatureExtractor struct {
	client *HsLLMExtractClient
}

func NewHsLLMFeatureExtractor(client *HsLLMExtractClient) *HsLLMFeatureExtractor {
	return &HsLLMFeatureExtractor{client: client}
}

func (e *HsLLMFeatureExtractor) Extract(
	ctx context.Context,
	model, manufacturer string,
	asset *biz.HsDatasheetAssetRecord,
) (biz.HsPrefilterInput, error) {
	if e == nil || e.client == nil {
		return biz.HsPrefilterInput{}, fmt.Errorf("hs llm feature extractor not configured")
	}
	prompt := buildExtractPrompt(model, manufacturer, asset)
	out, err := e.client.Extract(ctx, prompt)
	if err != nil {
		return biz.HsPrefilterInput{}, err
	}
	return mapExtractResultToPrefilterInput(model, out), nil
}

func buildExtractPrompt(model, manufacturer string, asset *biz.HsDatasheetAssetRecord) string {
	parts := []string{
		"MODEL: " + strings.TrimSpace(model),
		"MANUFACTURER: " + strings.TrimSpace(manufacturer),
	}
	if asset != nil {

		if p := strings.TrimSpace(asset.LocalPath); p != "" {
			data, err := pdftext.ReadBodyHeadFromFile(p, 10000)
			if err != nil {

			}
			parts = append(parts, "DATASHEET_DATA: "+data)
		}
	}
	return strings.Join(parts, "\n")
}

func readLocalDatasheetSnippet(path string) string {
	data, err := os.ReadFile(path)
	if err != nil || len(data) == 0 {
		return ""
	}
	if len(data) > hsExtractInputMaxBytes {
		data = data[:hsExtractInputMaxBytes]
	}
	text := strings.TrimSpace(string(data))
	return sanitizeTextBlock(text)
}

func sanitizeTextBlock(s string) string {
	if s == "" {
		return ""
	}
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if r == '\n' || r == '\r' || r == '\t' || (r >= 32 && r <= 126) || (r >= 0x4e00 && r <= 0x9fff) {
			b.WriteRune(r)
			continue
		}
		b.WriteRune(' ')
	}
	return strings.TrimSpace(b.String())
}

func mapExtractResultToPrefilterInput(model string, out *HsLLMExtractResult) biz.HsPrefilterInput {
	if out == nil {
		return biz.HsPrefilterInput{ComponentName: strings.TrimSpace(model)}
	}
	// component_name 经抽取端白名单校验；无法归入允许集合时保持空串，不回填 model。
	component := strings.TrimSpace(out.ComponentName)
	keySpecs := map[string]string{
		"voltage":     strings.TrimSpace(out.KeySpecs.Voltage),
		"current":     strings.TrimSpace(out.KeySpecs.Current),
		"power":       strings.TrimSpace(out.KeySpecs.Power),
		"frequency":   strings.TrimSpace(out.KeySpecs.Frequency),
		"temperature": strings.TrimSpace(out.KeySpecs.Temperature),
	}
	for i, other := range out.KeySpecs.Other {
		v := strings.TrimSpace(other)
		if v == "" {
			continue
		}
		keySpecs[fmt.Sprintf("other_%d", i+1)] = v
	}
	ranked := make([]biz.HsTechCategoryRank, 0, len(out.TechCategoryRanked))
	for i := range out.TechCategoryRanked {
		ranked = append(ranked, biz.HsTechCategoryRank{
			Rank:         out.TechCategoryRanked[i].Rank,
			TechCategory: strings.TrimSpace(out.TechCategoryRanked[i].TechCategory),
			Confidence:   out.TechCategoryRanked[i].Confidence,
		})
	}
	return biz.HsPrefilterInput{
		TechCategory:       strings.TrimSpace(out.TechCategory),
		TechCategoryRanked: ranked,
		ComponentName:      component,
		PackageForm:        strings.TrimSpace(out.PackageForm),
		KeySpecs:           keySpecs,
	}
}

var _ biz.HsModelFeatureExtractor = (*HsLLMFeatureExtractor)(nil)
