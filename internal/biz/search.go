package biz

import (
	"context"
	"fmt"
	"sync"

	"caichip/pkg/platform"
)

// PlatformSearcher 平台搜索接口（适配 pkg/platform）
type PlatformSearcher interface {
	Name() string
	Search(model string, quantity int) ([]*platform.Quote, error)
}

// BatchSearcher 支持批量搜索的接口，一次调用多型号，平台内部可多线程并行
type BatchSearcher interface {
	PlatformSearcher
	SearchBatch(reqs []platform.SearchRequest) (map[string][]*platform.Quote, error)
}

// SearchUseCase 搜索业务用例
type SearchUseCase struct {
	bomRepo   BOMRepo
	searchers []PlatformSearcher
}

// NewSearchUseCase 创建搜索用例
func NewSearchUseCase(bomRepo BOMRepo, searchers []PlatformSearcher) *SearchUseCase {
	return &SearchUseCase{
		bomRepo:   bomRepo,
		searchers: searchers,
	}
}

// SearchQuotes 多平台搜索报价
func (uc *SearchUseCase) SearchQuotes(ctx context.Context, bomID string, platforms []string) ([]*ItemQuotes, error) {
	bom, err := uc.bomRepo.GetBOM(ctx, bomID)
	if err != nil {
		return nil, err
	}
	if bom == nil {
		return nil, ErrBOMNotFound
	}

	platformSet := make(map[string]bool)
	if len(platforms) > 0 {
		for _, p := range platforms {
			platformSet[p] = true
		}
	}

	var items []*BOMItem
	for _, item := range bom.Items {
		if item.Model != "" {
			items = append(items, item)
		}
	}
	if len(items) == 0 {
		return nil, nil
	}

	// 筛选平台
	var batchSearchers []BatchSearcher
	var normalSearchers []PlatformSearcher
	for _, s := range uc.searchers {
		if len(platformSet) > 0 && !platformSet[s.Name()] {
			continue
		}
		if bs, ok := s.(BatchSearcher); ok {
			batchSearchers = append(batchSearchers, bs)
		} else {
			normalSearchers = append(normalSearchers, s)
		}
	}

	// Phase 1: 批量平台一次调用
	batchResults := make(map[string]map[string][]*platform.Quote)
	reqs := make([]platform.SearchRequest, len(items))
	for i, it := range items {
		reqs[i] = platform.SearchRequest{Model: it.Model, Quantity: it.Quantity}
	}
	for _, bs := range batchSearchers {
		m, err := bs.SearchBatch(reqs)
		if err != nil {
			fmt.Printf("SearchBatch err: %v", err)
			continue
		}
		batchResults[bs.Name()] = m
	}

	// Phase 2: 按 item 汇总各平台报价
	results := make([]*ItemQuotes, 0, len(items))
	for _, item := range items {
		var allQuotes []*Quote

		for _, m := range batchResults {
			for _, q := range m[item.Model] {
				allQuotes = append(allQuotes, platformQuoteToBiz(q))
			}
		}

		var wg sync.WaitGroup
		var mu sync.Mutex
		for _, s := range normalSearchers {
			wg.Add(1)
			go func(searcher PlatformSearcher) {
				defer wg.Done()
				quotes, err := searcher.Search(item.Model, item.Quantity)
				if err != nil {
					return
				}
				mu.Lock()
				for _, q := range quotes {
					allQuotes = append(allQuotes, platformQuoteToBiz(q))
				}
				mu.Unlock()
			}(s)
		}
		wg.Wait()

		results = append(results, &ItemQuotes{
			Model:    item.Model,
			Quantity: item.Quantity,
			Quotes:   allQuotes,
		})
	}

	return results, nil
}

func platformQuoteToBiz(q *platform.Quote) *Quote {
	return &Quote{
		Platform:      q.Platform,
		MatchedModel:  q.MatchedModel,
		Manufacturer:  q.Manufacturer,
		Package:       q.Package,
		Description:   q.Description,
		Stock:         q.Stock,
		LeadTime:      q.LeadTime,
		MOQ:           q.MOQ,
		Increment:     q.Increment,
		PriceTiers:    q.PriceTiers,
		HKPrice:       q.HKPrice,
		MainlandPrice: q.MainlandPrice,
		UnitPrice:     q.UnitPrice,
		Subtotal:      q.Subtotal,
	}
}
