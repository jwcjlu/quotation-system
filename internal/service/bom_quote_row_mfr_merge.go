package service

import (
	"strings"

	"caichip/internal/biz"
)

// mergeQuoteRowsWithSessionLineReads 将 ListBomQuoteItemsForSessionLineRead 读到的评审状态/规范厂牌合并进缓存 quotes_json 解析出的行（按型号+厂牌原文+封装+平台对齐）。
func mergeQuoteRowsWithSessionLineReads(rows []biz.AgentQuoteRow, dbRows []biz.BomQuoteItemReadRow, platformID string) []biz.AgentQuoteRow {
	if len(rows) == 0 || len(dbRows) == 0 {
		return rows
	}
	pid := biz.NormalizePlatformID(platformID)
	out := append([]biz.AgentQuoteRow(nil), rows...)
	used := make(map[uint64]struct{})
	for i := range out {
		r := &out[i]
		for j := range dbRows {
			d := &dbRows[j]
			if biz.NormalizePlatformID(d.PlatformID) != pid {
				continue
			}
			if _, ok := used[d.ItemID]; ok {
				continue
			}
			if !agentQuoteRowMatchesReadRow(r, d) {
				continue
			}
			used[d.ItemID] = struct{}{}
			r.ManufacturerReviewStatus = d.ManufacturerReviewStatus
			if canon := strings.TrimSpace(d.ManufacturerCanonicalID); canon != "" {
				c := canon
				r.ManufacturerCanonicalID = &c
			}
			break
		}
	}
	return out
}

func agentQuoteRowMatchesReadRow(row *biz.AgentQuoteRow, d *biz.BomQuoteItemReadRow) bool {
	if row == nil || d == nil {
		return false
	}
	if biz.NormalizeMPNForBOMSearch(row.Model) != biz.NormalizeMPNForBOMSearch(d.Model) {
		return false
	}
	if biz.NormalizeMfrString(row.Manufacturer) != biz.NormalizeMfrString(d.Manufacturer) {
		return false
	}
	return biz.NormalizeMfrString(row.Package) == biz.NormalizeMfrString(d.Package)
}
