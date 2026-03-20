package data

import (
	"context"
	"sync"

	"caichip/internal/biz"
)

// bomRepo 内存存储 BOM
type bomRepo struct {
	mu   sync.RWMutex
	boms map[string]*biz.BOM
}

// NewBOMRepo 创建 BOM 仓储
func NewBOMRepo() biz.BOMRepo {
	return &bomRepo{
		boms: make(map[string]*biz.BOM),
	}
}

func (r *bomRepo) SaveBOM(ctx context.Context, bom *biz.BOM) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.boms[bom.ID] = bom
	return nil
}

func (r *bomRepo) GetBOM(ctx context.Context, bomID string) (*biz.BOM, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	bom, ok := r.boms[bomID]
	if !ok {
		return nil, nil
	}
	return bom, nil
}
