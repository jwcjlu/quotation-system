package data

import (
	"context"
	"sort"
	"strings"

	"caichip/internal/biz"

	"gorm.io/gorm"
)

const hsItemTableName = "t_hs_item"

// HsItemQueryRepo 实现 biz.HsItemQueryRepo。
type HsItemQueryRepo struct {
	d *Data
}

func NewHsItemQueryRepo(d *Data) *HsItemQueryRepo {
	return &HsItemQueryRepo{d: d}
}

func (r *HsItemQueryRepo) DBOk() bool {
	return r != nil && r.d != nil && r.d.DB != nil
}

type hsItemRow struct {
	CodeTS        string `gorm:"column:code_ts"`
	GName         string `gorm:"column:g_name"`
	Unit1         string `gorm:"column:unit_1"`
	Unit2         string `gorm:"column:unit_2"`
	ControlMark   string `gorm:"column:control_mark"`
	SourceCoreHS6 string `gorm:"column:source_core_hs6"`
	RawJSON       []byte `gorm:"column:raw_json"`
}

func (r *HsItemQueryRepo) QueryCandidatesByRules(ctx context.Context, input biz.HsPrefilterInput, limit int) ([]biz.HsItemCandidate, error) {
	if !r.DBOk() {
		return nil, gorm.ErrInvalidDB
	}
	if limit <= 0 {
		limit = biz.DefaultHsPrefilterTopN
	}
	component := strings.TrimSpace(input.ComponentName)
	componentTerms := expandComponentTerms(component)
	pkg := strings.TrimSpace(input.PackageForm)
	prefixes := hs6PrefixesByTechCategory(input.TechCategory)

	fetchLimit := limit * 8
	if fetchLimit < 120 {
		fetchLimit = 120
	}
	if fetchLimit > 400 {
		fetchLimit = 400
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
	if len(candidates) > limit {
		candidates = candidates[:limit]
	}
	return candidates, nil
}

func scoreHsItemCandidate(row hsItemRow, input biz.HsPrefilterInput, normalizedPackage string, prefixes []string, componentTerms []string) (biz.HsItemCandidate, bool) {
	nameLower := strings.ToLower(row.GName)
	rawLower := strings.ToLower(string(row.RawJSON))

	component := strings.TrimSpace(input.ComponentName)
	tech := strings.TrimSpace(input.TechCategory)

	detail := biz.HsPrefilterScoreDetail{
		KeySpecsMatched: make([]string, 0, len(input.KeySpecs)),
		KeySpecsMissed:  make([]string, 0, len(input.KeySpecs)),
	}
	score := 0.0

	if component != "" && componentMatchedByTerms(nameLower, componentTerms) {
		detail.ComponentNameMatched = true
		score += 0.55
	}
	if tech != "" && matchCodePrefix(row.CodeTS, prefixes) {
		detail.TechCategoryMatched = true
		score += 0.25
	}
	if normalizedPackage != "" && strings.Contains(nameLower, strings.ToLower(normalizedPackage)) {
		detail.PackageFormMatched = true
		score += 0.10
	}
	for k, v := range input.KeySpecs {
		key := strings.TrimSpace(k)
		val := strings.TrimSpace(v)
		if key == "" || val == "" {
			continue
		}
		token := strings.ToLower(val)
		if strings.Contains(nameLower, token) || strings.Contains(rawLower, token) {
			detail.KeySpecsMatched = append(detail.KeySpecsMatched, key)
			score += 0.05
		} else {
			detail.KeySpecsMissed = append(detail.KeySpecsMissed, key)
		}
	}

	// 若提供了 component_name 或 tech_category，至少命中一个强约束才保留候选。
	if (component != "" || tech != "") && !detail.ComponentNameMatched && !detail.TechCategoryMatched {
		return biz.HsItemCandidate{}, false
	}

	return biz.HsItemCandidate{
		CodeTS:        row.CodeTS,
		GName:         row.GName,
		Unit1:         row.Unit1,
		Unit2:         row.Unit2,
		ControlMark:   row.ControlMark,
		SourceCoreHS6: row.SourceCoreHS6,
		RawJSON:       append([]byte(nil), row.RawJSON...),
		Score:         score,
		ScoreDetail:   detail,
	}, true
}

func hs6PrefixesByTechCategory(category string) []string {
	switch strings.TrimSpace(category) {
	case "半导体器件":
		return []string{"8541"}
	case "集成电路":
		return []string{"8542"}
	case "无源器件":
		return []string{"8532", "8533"}
	case "电路板":
		return []string{"8534"}
	case "其他":
		return []string{"8504", "8535", "8536"}
	default:
		return nil
	}
}

func prefixWhereClause(n int) string {
	if n <= 0 {
		return ""
	}
	parts := make([]string, 0, n)
	for i := 0; i < n; i++ {
		parts = append(parts, "code_ts LIKE ?")
	}
	return "(" + strings.Join(parts, " OR ") + ")"
}

func componentWhereClause(n int) string {
	if n <= 0 {
		return ""
	}
	parts := make([]string, 0, n)
	for i := 0; i < n; i++ {
		parts = append(parts, "g_name LIKE ?")
	}
	return "(" + strings.Join(parts, " OR ") + ")"
}

func componentWhereArgs(terms []string) []any {
	args := make([]any, 0, len(terms))
	for i := range terms {
		args = append(args, "%"+terms[i]+"%")
	}
	return args
}

func prefixWhereArgs(prefixes []string) []any {
	args := make([]any, 0, len(prefixes))
	for i := range prefixes {
		args = append(args, prefixes[i]+"%")
	}
	return args
}

func expandComponentTerms(component string) []string {
	component = strings.TrimSpace(component)
	if component == "" {
		return nil
	}
	seen := map[string]struct{}{}
	terms := make([]string, 0, 6)
	push := func(v string) {
		v = strings.TrimSpace(v)
		if v == "" {
			return
		}
		key := strings.ToLower(v)
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		terms = append(terms, v)
	}
	push(component)
	for _, s := range componentSynonyms(component) {
		push(s)
	}
	return terms
}

func componentSynonyms(component string) []string {
	switch strings.ToLower(strings.TrimSpace(component)) {
	case "单片机", "mcu", "microcontroller", "微控制器":
		return []string{"mcu", "microcontroller", "controller", "微控制器", "单片机"}
	case "处理器", "cpu", "processor":
		return []string{"cpu", "processor", "处理器"}
	case "存储器", "memory", "ram", "rom", "flash":
		return []string{"memory", "ram", "rom", "flash", "存储器"}
	case "二极管", "diode":
		return []string{"diode", "二极管"}
	case "晶体管", "transistor":
		return []string{"transistor", "晶体管"}
	case "电容", "电容器", "capacitor":
		return []string{"capacitor", "电容", "电容器"}
	case "电阻", "电阻器", "resistor":
		return []string{"resistor", "电阻", "电阻器"}
	case "连接器", "connector":
		return []string{"connector", "连接器"}
	case "电感", "电感器", "inductor":
		return []string{"inductor", "电感", "电感器"}
	default:
		return nil
	}
}

func componentMatchedByTerms(nameLower string, terms []string) bool {
	for i := range terms {
		if strings.Contains(nameLower, strings.ToLower(terms[i])) {
			return true
		}
	}
	return false
}

func matchCodePrefix(code string, prefixes []string) bool {
	for i := range prefixes {
		if strings.HasPrefix(code, prefixes[i]) {
			return true
		}
	}
	return false
}

var _ biz.HsItemQueryRepo = (*HsItemQueryRepo)(nil)
