package data

import (
	"context"
	"encoding/json"

	"caichip/internal/biz"
)

type quoteItemBatchRow struct {
	QuoteID       uint64 `gorm:"column:quote_id"`
	Model         string `gorm:"column:model"`
	Manufacturer  string `gorm:"column:manufacturer"`
	Package       string `gorm:"column:package"`
	Desc          string `gorm:"column:desc"`
	Stock         string `gorm:"column:stock"`
	MOQ           string `gorm:"column:moq"`
	PriceTiers    string `gorm:"column:price_tiers"`
	HKPrice       string `gorm:"column:hk_price"`
	MainlandPrice string `gorm:"column:mainland_price"`
	LeadTime      string `gorm:"column:lead_time"`
	QueryModel    string `gorm:"column:query_model"`
	DatasheetURL  string `gorm:"column:datasheet_url"`
}

func (r *BOMSearchTaskRepo) loadQuoteRowsJSONByCacheIDs(ctx context.Context, cacheIDs []uint64) (map[uint64][]byte, error) {
	out := make(map[uint64][]byte, len(cacheIDs))
	if !r.DBOk() || len(cacheIDs) == 0 {
		return out, nil
	}

	var items []quoteItemBatchRow
	if err := r.db.WithContext(ctx).
		Model(&BomQuoteItem{}).
		Select("quote_id, model, manufacturer, package, `desc`, stock, moq, price_tiers, hk_price, mainland_price, lead_time, query_model, datasheet_url").
		Where("quote_id IN ?", cacheIDs).
		Order("quote_id ASC, id ASC").
		Find(&items).Error; err != nil {
		return nil, err
	}

	grouped := make(map[uint64][]biz.AgentQuoteRow, len(cacheIDs))
	for i := range items {
		it := items[i]
		grouped[it.QuoteID] = append(grouped[it.QuoteID], biz.AgentQuoteRow{
			Model:         it.Model,
			Manufacturer:  it.Manufacturer,
			Package:       it.Package,
			Desc:          it.Desc,
			Stock:         it.Stock,
			MOQ:           it.MOQ,
			PriceTiers:    it.PriceTiers,
			HKPrice:       it.HKPrice,
			MainlandPrice: it.MainlandPrice,
			LeadTime:      it.LeadTime,
			QueryModel:    it.QueryModel,
			DatasheetURL:  it.DatasheetURL,
		})
	}

	for _, cacheID := range cacheIDs {
		rows := grouped[cacheID]
		if len(rows) == 0 {
			out[cacheID] = []byte("[]")
			continue
		}
		raw, err := json.Marshal(rows)
		if err != nil {
			return nil, err
		}
		out[cacheID] = raw
	}
	return out, nil
}
