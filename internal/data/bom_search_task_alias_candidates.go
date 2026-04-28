package data

import (
	"context"
	"strings"
	"time"

	"caichip/internal/biz"
)

// ListManufacturerAliasQuoteRows 只读取厂牌别名审核需要的报价轻字段，避免加载整份报价 JSON。
func (r *BOMSearchTaskRepo) ListManufacturerAliasQuoteRows(ctx context.Context, bizDate time.Time, pairs []biz.MpnPlatformPair) (map[string][]biz.AgentQuoteRow, error) {
	out := make(map[string][]biz.AgentQuoteRow)
	if !r.DBOk() || len(pairs) == 0 {
		return out, nil
	}
	bd, err := time.ParseInLocation("2006-01-02", bizDate.Format("2006-01-02"), time.Local)
	if err != nil {
		return nil, err
	}
	uniq := uniqueQuoteCachePairs(pairs)
	if len(uniq) == 0 {
		return out, nil
	}

	q := r.db.WithContext(ctx).Table(TableBomQuoteCache).Where("biz_date = ?", bd)
	{
		var parts []string
		var args []interface{}
		for _, p := range uniq {
			parts = append(parts, "(mpn_norm = ? AND platform_id = ?)")
			args = append(args, p.MpnNorm, p.PlatformID)
		}
		q = q.Where(strings.Join(parts, " OR "), args...)
	}
	type cacheRow struct {
		ID         uint64 `gorm:"column:id"`
		MpnNorm    string `gorm:"column:mpn_norm"`
		PlatformID string `gorm:"column:platform_id"`
		Outcome    string `gorm:"column:outcome"`
	}
	var caches []cacheRow
	if err := q.Select("id, mpn_norm, platform_id, outcome").Find(&caches).Error; err != nil {
		return nil, err
	}

	cacheIDToKey := make(map[uint64]string, len(caches))
	quoteIDs := make([]uint64, 0, len(caches))
	for _, row := range caches {
		if row.ID == 0 || quoteCacheOutcomeEmpty(row.Outcome) {
			continue
		}
		key := row.MpnNorm + "\x00" + row.PlatformID
		cacheIDToKey[row.ID] = key
		quoteIDs = append(quoteIDs, row.ID)
		if _, ok := out[key]; !ok {
			out[key] = nil
		}
	}
	if len(quoteIDs) == 0 {
		return out, nil
	}

	var items []quoteItemRow
	if err := r.db.WithContext(ctx).
		Model(&BomQuoteItem{}).
		Select("quote_id, model, manufacturer, manufacturer_canonical_id, package").
		Where("quote_id IN ?", quoteIDs).
		Where("manufacturer_canonical_id IS NULL").
		Order("quote_id ASC, id ASC").
		Find(&items).Error; err != nil {
		return nil, err
	}
	for _, item := range items {
		key, ok := cacheIDToKey[item.QuoteID]
		if !ok {
			continue
		}
		out[key] = append(out[key], biz.AgentQuoteRow{
			Seq:                     len(out[key]) + 1,
			Model:                   item.Model,
			Manufacturer:            item.Manufacturer,
			ManufacturerCanonicalID: item.ManufacturerCanonicalID,
			Package:                 item.Package,
		})
	}
	return out, nil
}

func uniqueQuoteCachePairs(pairs []biz.MpnPlatformPair) []biz.MpnPlatformPair {
	seen := make(map[string]struct{}, len(pairs))
	out := make([]biz.MpnPlatformPair, 0, len(pairs))
	for _, p := range pairs {
		mn := normalizeMPNForSearchTask(p.MpnNorm)
		pid := strings.TrimSpace(p.PlatformID)
		if mn == "" || pid == "" {
			continue
		}
		key := mn + "\x00" + pid
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, biz.MpnPlatformPair{MpnNorm: mn, PlatformID: pid})
	}
	return out
}

func quoteCacheOutcomeEmpty(outcome string) bool {
	switch strings.ToLower(strings.TrimSpace(outcome)) {
	case "no_mpn_match", "no_result":
		return true
	default:
		return false
	}
}
