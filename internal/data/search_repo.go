package data

import (
	"context"
	"sync"

	"caichip/internal/biz"
)

// searchRepo 搜索与配单结果存储（经典多平台实时搜价已下线，报价仅来自 SaveQuotes 写入的缓存）
type searchRepo struct {
	bomRepo biz.BOMRepo

	mu          sync.RWMutex
	quotesCache map[string][]*biz.ItemQuotes
	matchCache  map[string][]*biz.MatchItem
}

// NewSearchRepo 创建搜索仓储
func NewSearchRepo(bomRepo biz.BOMRepo) biz.SearchRepo {
	return &searchRepo{
		bomRepo:     bomRepo,
		quotesCache: make(map[string][]*biz.ItemQuotes),
		matchCache:  make(map[string][]*biz.MatchItem),
	}
}

func (r *searchRepo) GetQuotesByBOM(ctx context.Context, bomID string) ([]*biz.ItemQuotes, error) {
	_ = ctx
	r.mu.RLock()
	defer r.mu.RUnlock()
	cached := r.quotesCache[bomID]
	if cached == nil {
		return nil, nil
	}
	out := make([]*biz.ItemQuotes, len(cached))
	copy(out, cached)
	return out, nil
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
