package data

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"caichip/internal/biz"

	"gorm.io/gorm"
)

func collectTechCategoriesForItemQuery(input biz.HsPrefilterInput) []string {
	var out []string
	seen := map[string]struct{}{}
	for _, tr := range input.TechCategoryRanked {
		c := strings.TrimSpace(tr.TechCategory)
		if c == "" {
			continue
		}
		if _, ok := seen[c]; ok {
			continue
		}
		seen[c] = struct{}{}
		out = append(out, c)
		if len(out) >= 3 {
			break
		}
	}
	if len(out) > 0 {
		return out
	}
	if c := strings.TrimSpace(input.TechCategory); c != "" {
		return []string{c}
	}
	return nil
}

func (r *HsItemQueryRepo) QueryCandidatesByRules(ctx context.Context, input biz.HsPrefilterInput, limit int) ([]biz.HsItemCandidate, error) {
	if !r.DBOk() {
		return nil, gorm.ErrInvalidDB
	}
	cats := collectTechCategoriesForItemQuery(input)
	if len(cats) == 0 {
		return nil, fmt.Errorf("hs item query: no tech categories")
	}
	if limit <= 0 {
		limit = biz.HsPrefilterUnboundedCap
	}
	byCode := make(map[string]biz.HsItemCandidate, 512)
	for _, cat := range cats {
		sub := input
		sub.TechCategoryRanked = nil
		sub.TechCategory = cat
		part, err := r.queryCandidatesForSingleTechCategory(ctx, sub, limit)
		if err != nil {
			return nil, err
		}
		for i := range part {
			c := part[i]
			code := strings.TrimSpace(c.CodeTS)
			if code == "" {
				continue
			}
			if old, ok := byCode[code]; !ok || c.Score > old.Score {
				byCode[code] = c
			}
		}
	}
	out := make([]biz.HsItemCandidate, 0, len(byCode))
	for _, c := range byCode {
		out = append(out, c)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Score == out[j].Score {
			return out[i].CodeTS < out[j].CodeTS
		}
		return out[i].Score > out[j].Score
	})
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (r *HsItemQueryRepo) queryCandidatesForSingleTechCategory(ctx context.Context, input biz.HsPrefilterInput, limit int) ([]biz.HsItemCandidate, error) {
	component := strings.TrimSpace(input.ComponentName)
	componentTerms := expandComponentTerms(component)
	pkg := strings.TrimSpace(input.PackageForm)
	prefixes := hs6PrefixesByTechCategory(input.TechCategory)

	fetchLimit := limit * 8
	if fetchLimit < 500 {
		fetchLimit = 500
	}
	if fetchLimit > 20000 {
		fetchLimit = 20000
	}

	q := r.d.DB.WithContext(ctx).
		Table(hsItemTableName).
		Select("code_ts, g_name, unit_1, unit_2, control_mark, source_core_hs6, raw_json").
		Limit(fetchLimit)

	if len(componentTerms) > 0 {
		q = q.Where(componentWhereClause(len(componentTerms)), componentWhereArgs(componentTerms)...)
	}
	if len(prefixes) > 0 {
		q = q.Where(prefixWhereClause(len(prefixes)), prefixWhereArgs(prefixes)...)
	}
	q = q.Order("updated_at DESC")

	var rows []hsItemRow
	if err := q.Find(&rows).Error; err != nil {
		return nil, err
	}
	candidates := make([]biz.HsItemCandidate, 0, len(rows))
	for i := range rows {
		candidate, ok := scoreHsItemCandidate(rows[i], input, pkg, prefixes, componentTerms)
		if !ok {
			continue
		}
		candidates = append(candidates, candidate)
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].Score == candidates[j].Score {
			return candidates[i].CodeTS < candidates[j].CodeTS
		}
		return candidates[i].Score > candidates[j].Score
	})
	return candidates, nil
}
