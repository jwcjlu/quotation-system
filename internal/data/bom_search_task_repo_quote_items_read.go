package data

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"caichip/internal/biz"
)

func bomQuoteItemReadRow(it *BomQuoteItem, platformID string) biz.BomQuoteItemReadRow {
	canon := ""
	if it.ManufacturerCanonicalID != nil {
		canon = strings.TrimSpace(*it.ManufacturerCanonicalID)
	}
	sid := ""
	if it.SessionID != nil {
		sid = strings.TrimSpace(*it.SessionID)
	}
	var lid int64
	if it.LineID != nil {
		lid = *it.LineID
	}
	return biz.BomQuoteItemReadRow{
		PlatformID:               strings.TrimSpace(platformID),
		QuoteID:                  it.QuoteID,
		ItemID:                   it.ID,
		Model:                    it.Model,
		Manufacturer:             it.Manufacturer,
		ManufacturerCanonicalID:  canon,
		ManufacturerReviewStatus: strings.TrimSpace(it.ManufacturerReviewStatus),
		Package:                  it.Package,
		Stock:                    it.Stock,
		Desc:                     it.Desc,
		MOQ:                      it.MOQ,
		LeadTime:                 it.LeadTime,
		PriceTiers:               it.PriceTiers,
		HKPrice:                  it.HKPrice,
		MainlandPrice:            it.MainlandPrice,
		QueryModel:               it.QueryModel,
		DatasheetURL:             it.DatasheetURL,
		SourceType:               it.SourceType,
		SessionID:                sid,
		LineID:                   lid,
	}
}

// ListBomQuoteItemsForSessionLineRead 读取该行在业务日、勾选平台下经缓存关联的报价子表行，并合并 session_id+line_id 直挂行。
func (r *BOMSearchTaskRepo) ListBomQuoteItemsForSessionLineRead(ctx context.Context, sessionID string, lineID int64, bizDate time.Time, mergeMpn string, platformIDs []string) ([]biz.BomQuoteItemReadRow, error) {
	if !r.DBOk() {
		return nil, nil
	}
	sessionID = strings.TrimSpace(sessionID)
	dateStr := bizDate.Format("2006-01-02")
	bd, err := time.ParseInLocation("2006-01-02", dateStr, time.Local)
	if err != nil {
		return nil, err
	}
	mn := normalizeMPNForSearchTask(mergeMpn)
	seen := make(map[uint64]struct{})
	outMap := make(map[uint64]biz.BomQuoteItemReadRow)

	var uniqPlats []string
platLoop:
	for _, p := range platformIDs {
		pid := strings.TrimSpace(p)
		if pid == "" {
			continue
		}
		for _, u := range uniqPlats {
			if u == pid {
				continue platLoop
			}
		}
		uniqPlats = append(uniqPlats, pid)
	}
	if len(uniqPlats) > 0 {
		var caches []BomQuoteCache
		if err = r.db.WithContext(ctx).
			Model(&BomQuoteCache{}).
			Select("id", "platform_id").
			Where("mpn_norm = ? AND biz_date = ? AND platform_id IN ?", mn, bd, uniqPlats).
			Find(&caches).Error; err != nil {
			return nil, err
		}
		platByQuote := make(map[uint64]string, len(caches))
		var quoteIDs []uint64
		for i := range caches {
			c := caches[i]
			if c.ID == 0 {
				continue
			}
			platByQuote[c.ID] = strings.TrimSpace(c.PlatformID)
			quoteIDs = append(quoteIDs, c.ID)
		}
		if len(quoteIDs) > 0 {
			var items []BomQuoteItem
			if err = r.db.WithContext(ctx).
				Where("quote_id IN ?", quoteIDs).
				Order("quote_id ASC, id ASC").
				Find(&items).Error; err != nil {
				return nil, err
			}
			for i := range items {
				it := &items[i]
				if _, ok := seen[it.ID]; ok {
					continue
				}
				seen[it.ID] = struct{}{}
				outMap[it.ID] = bomQuoteItemReadRow(it, platByQuote[it.QuoteID])
			}
		}
	}

	if sessionID != "" && lineID > 0 {
		var items2 []BomQuoteItem
		if err = r.db.WithContext(ctx).
			Where("session_id = ? AND line_id = ?", sessionID, lineID).
			Order("quote_id ASC, id ASC").
			Find(&items2).Error; err != nil {
			return nil, err
		}
		quoteIDSeen := make(map[uint64]struct{})
		var quoteIDs []uint64
		for i := range items2 {
			qid := items2[i].QuoteID
			if qid == 0 {
				continue
			}
			if _, ok := quoteIDSeen[qid]; ok {
				continue
			}
			quoteIDSeen[qid] = struct{}{}
			quoteIDs = append(quoteIDs, qid)
		}
		platByQuote := make(map[uint64]string, len(quoteIDs))
		if len(quoteIDs) > 0 {
			var caches2 []BomQuoteCache
			if err = r.db.WithContext(ctx).
				Model(&BomQuoteCache{}).
				Select("id", "platform_id").
				Where("id IN ?  and stock > 0", quoteIDs).
				Find(&caches2).Error; err != nil {
				return nil, err
			}
			for i := range caches2 {
				platByQuote[caches2[i].ID] = strings.TrimSpace(caches2[i].PlatformID)
			}
		}
		for i := range items2 {
			it := &items2[i]
			if _, ok := seen[it.ID]; ok {
				continue
			}
			seen[it.ID] = struct{}{}
			plat := platByQuote[it.QuoteID]
			outMap[it.ID] = bomQuoteItemReadRow(it, plat)
		}
	}

	out := make([]biz.BomQuoteItemReadRow, 0, len(outMap))
	for _, row := range outMap {
		out = append(out, row)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].PlatformID != out[j].PlatformID {
			return out[i].PlatformID < out[j].PlatformID
		}
		return out[i].ItemID < out[j].ItemID
	})
	return out, nil
}

// CountQuoteMfrReviewPendingForSession 见 biz.BOMSearchTaskRepo。
func (r *BOMSearchTaskRepo) CountQuoteMfrReviewPendingForSession(ctx context.Context, sessionID string) (int64, error) {
	if !r.DBOk() {
		return 0, nil
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return 0, nil
	}
	join := fmt.Sprintf(
		"INNER JOIN %s AS l ON l.id = %s.line_id AND l.session_id = ?",
		TableBomSessionLine,
		TableBomQuoteItem,
	)
	var n int64
	err := r.db.WithContext(ctx).
		Model(&BomQuoteItem{}).
		Joins(join, sessionID).
		Where(TableBomQuoteItem+".session_id = ? AND "+TableBomQuoteItem+".manufacturer_review_status = ?", sessionID, biz.MfrReviewPending).
		Where("l.mfr IS NOT NULL AND TRIM(l.mfr) <> ?", "").
		Count(&n).Error
	return n, err
}
