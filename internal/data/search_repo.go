package data

import (
	"context"
	"sync"

	"caichip/internal/biz"
)

// searchRepo 搜索与配单结果存储
type searchRepo struct {
	bomRepo  biz.BOMRepo
	searchUC *biz.SearchUseCase

	mu          sync.RWMutex
	quotesCache map[string][]*biz.ItemQuotes
	matchCache  map[string][]*biz.MatchItem
}

// NewSearchRepo 创建搜索仓储
func NewSearchRepo(bomRepo biz.BOMRepo, searchUC *biz.SearchUseCase) biz.SearchRepo {
	return &searchRepo{
		bomRepo:     bomRepo,
		searchUC:    searchUC,
		quotesCache: make(map[string][]*biz.ItemQuotes),
		matchCache:  make(map[string][]*biz.MatchItem),
	}
}

func (r *searchRepo) GetQuotesByBOM(ctx context.Context, bomID string) ([]*biz.ItemQuotes, error) {
	r.mu.RLock()
	cached := r.quotesCache[bomID]
	r.mu.RUnlock()

	if cached != nil {
		return cached, nil
	}

	results, err := r.searchUC.SearchQuotes(ctx, bomID, nil)
	if err != nil {
		return nil, err
	}

	r.mu.Lock()
	r.quotesCache[bomID] = results
	r.mu.Unlock()

	return results, nil
}

func (r *searchRepo) SaveQuotes(ctx context.Context, bomID string, quotes []*biz.ItemQuotes) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.quotesCache[bomID] = quotes
	return nil
}

func (r *searchRepo) SaveMatchResult(ctx context.Context, bomID string, items []*biz.MatchItem) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.matchCache[bomID] = items
	return nil
}

func (r *searchRepo) GetMatchResult(ctx context.Context, bomID string) ([]*biz.MatchItem, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	items := r.matchCache[bomID]
	return items, nil
}
