package data

import "sort"

type hsLLMRankedWork struct {
	cat  string
	conf float64
	ord  int
}

// normalizeHsLLMExtractTechCategoryRanked 按 hs-model-to-code-ts 设计 8.1：合法类目过滤、confidence 钳位到 [0,1]、
// 按 confidence 降序（同分保持输入顺序）、按 tech_category 去重保留首次、最多 3 条、rank 从 1 连续编号。
// 若归一后为空且 fallbackTop 为合法非空类目，则退化为单条 rank=1、confidence=1.0（向后兼容）。
func normalizeHsLLMExtractTechCategoryRanked(in []HsLLMExtractTechCategoryRank, fallbackTop string) []HsLLMExtractTechCategoryRank {
	work := make([]hsLLMRankedWork, 0, len(in))
	for i := range in {
		cat := normalizeHsLLMTechCategory(in[i].TechCategory)
		if cat == "" {
			continue
		}
		conf := in[i].Confidence
		if conf < 0 {
			conf = 0
		}
		if conf > 1 {
			conf = 1
		}
		work = append(work, hsLLMRankedWork{cat: cat, conf: conf, ord: i})
	}
	sort.SliceStable(work, func(i, j int) bool {
		if work[i].conf != work[j].conf {
			return work[i].conf > work[j].conf
		}
		return work[i].ord < work[j].ord
	})
	seen := make(map[string]struct{}, len(work))
	out := make([]HsLLMExtractTechCategoryRank, 0, 3)
	for _, w := range work {
		if _, ok := seen[w.cat]; ok {
			continue
		}
		seen[w.cat] = struct{}{}
		out = append(out, HsLLMExtractTechCategoryRank{
			Rank:         len(out) + 1,
			TechCategory: w.cat,
			Confidence:   w.conf,
		})
		if len(out) >= 3 {
			break
		}
	}
	if len(out) == 0 && fallbackTop != "" {
		return []HsLLMExtractTechCategoryRank{
			{Rank: 1, TechCategory: fallbackTop, Confidence: 1},
		}
	}
	return out
}
